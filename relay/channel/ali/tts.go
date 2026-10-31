package ali

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const ttsWriteTimeout = 10 * time.Second

type ttsFrame struct {
	kind int
	data []byte
	err  error
}

func readTTSFrames(ctx context.Context, conn *websocket.Conn, frames chan<- ttsFrame, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		kind, data, err := conn.ReadMessage()
		select {
		case frames <- ttsFrame{kind, data, err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func writeTTSFrame(conn *websocket.Conn, kind int, data []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(ttsWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(kind, data)
}

func writeTTSEvent(conn *websocket.Conn, event ttsEvent) error {
	data, err := common.Marshal(event)
	if err != nil {
		return err
	}
	return writeTTSFrame(conn, websocket.TextMessage, data)
}

type ttsTaskState struct {
	id                                   string
	started, finishing, cancelled, audio bool
	characters                           int
	meter                                ttsMeter
	seen                                 map[string]bool
}

func (s *ttsTaskState) acceptClient(e *ttsEvent, origin, upstream string) error {
	if e.Header.TaskID == "" || len(e.Header.TaskID) > 64 || e.Header.Streaming != "duplex" {
		return errors.New("task_id and streaming=duplex are required")
	}
	switch e.Header.Action {
	case "run-task":
		if s.id != "" {
			return errors.New("wait for task-finished before starting another task")
		}
		if s.seen[e.Header.TaskID] || len(s.seen) >= 100 {
			return errors.New("use a new task_id; at most 100 tasks per connection")
		}
		if e.Payload.Model != origin {
			return errors.New("run-task model must match the authorized query model")
		}
		if e.Payload.TaskGroup != "audio" || e.Payload.Task != "tts" || e.Payload.Function != "SpeechSynthesizer" || len(e.Payload.Input) != 0 {
			return errors.New("only audio/tts/SpeechSynthesizer with empty initial input is supported")
		}
		if voice, _ := e.Payload.Parameters["voice"].(string); voice == "" {
			return errors.New("voice is required")
		}
		e.Payload.Model = upstream
		s.id, s.started, s.finishing, s.cancelled, s.audio, s.characters = e.Header.TaskID, false, false, false, false, 0
		s.meter = ttsMeter{tokenModel: upstream == "qwen-audio-3.1-tts-flash"}
		s.seen[s.id] = true
	case "continue-task", "finish-task":
		if e.Header.TaskID != s.id || !s.started {
			return errors.New("wait for task-started and use the active task_id")
		}
		directive, _ := e.Payload.Input["directive"].(string)
		if s.finishing && !(e.Header.Action == "finish-task" && directive == "cancel") {
			return errors.New("task input is already finished")
		}
		if e.Header.Action == "continue-task" {
			text, ok := e.Payload.Input["text"].(string)
			n := utf8.RuneCountInString(text)
			if !ok || n == 0 || n > 20000 || s.characters+n > 200000 || len(e.Payload.Input) != 1 {
				return errors.New("continue-task requires text only, 1..20000 characters per message and 200000 per task")
			}
			s.characters += n
		} else {
			if len(e.Payload.Input) != 0 && (len(e.Payload.Input) != 1 || directive != "cancel") {
				return errors.New("finish-task input must be empty or directive=cancel")
			}
			s.finishing, s.cancelled = true, directive == "cancel"
		}
		// Follow-up frames cannot change model, task or synthesis parameters.
		e.Payload.Model, e.Payload.TaskGroup, e.Payload.Task, e.Payload.Function = "", "", "", ""
		e.Payload.Parameters = nil
	default:
		return errors.New("unsupported TTS action")
	}
	e.Header.Event, e.Header.ErrorCode, e.Header.Message = "", "", ""
	e.Payload.Usage = nil
	return nil
}

// RelayTTS owns all writes, state transitions and billing callbacks in one
// goroutine. Reader goroutines only deliver frames and are joined before return.
// request=nil selects the duplex endpoint; otherwise it bridges OpenAI speech.
func RelayTTS(c *gin.Context, info *relaycommon.RelayInfo, request *dto.AudioRequest, reserve func(*dto.Usage) error) (usage *dto.Usage, apiErr *types.NewAPIError) {
	total := dto.Usage{}
	state := ttsTaskState{seen: make(map[string]bool)}
	defer func() { u := addTTSUsage(total, state.meter.usage()); usage = &u }()
	fail := func(err error) *types.NewAPIError {
		var apiError *types.NewAPIError
		if errors.As(err, &apiError) {
			types.ErrOptionWithSkipRetry()(apiError)
			return apiError
		}
		return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	invalid := func(err error) error {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if !SupportsTTS(info.UpstreamModelName) {
		return nil, fail(invalid(errors.New("unsupported Ali TTS model; Qwen3 Realtime is a different protocol")))
	}
	var initial ttsEvent
	format := ""
	if request != nil {
		var err error
		initial, format, err = speechTask(request, info.UpstreamModelName)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	target, err := channel.DoWssRequest(&Adaptor{}, c, info, nil)
	if err != nil {
		return nil, fail(err)
	}
	info.IsStream = true
	ctx, stop := context.WithCancel(context.Background())
	var readers sync.WaitGroup
	upstream := make(chan ttsFrame, 1)
	downstream := make(chan ttsFrame, 1)
	target.SetReadLimit(4 << 20)
	readers.Add(1)
	go readTTSFrames(ctx, target, upstream, &readers)
	client := info.ClientWs
	if client != nil {
		client.SetReadLimit(128 << 10)
		readers.Add(1)
		go readTTSFrames(ctx, client, downstream, &readers)
	}
	defer func() {
		stop()
		_ = target.Close()
		if client != nil {
			_ = client.SetReadDeadline(time.Now())
		}
		readers.Wait()
	}()
	reserveCurrent := func() error {
		// Reserve ahead of submitted text, not only after upstream consumption.
		// This is a conservative reservation, never the final bill. Instructions
		// and prosody can change actual usage; upstream snapshots remain authoritative.
		pending := state.meter.usage()
		pending.PromptTokens = max(pending.PromptTokens, state.characters*2)
		pending.PromptTokensDetails.TextTokens = pending.PromptTokens
		if state.meter.tokenModel {
			pending.CompletionTokens = max(pending.CompletionTokens, state.characters*32)
			pending.CompletionTokenDetails.AudioTokens = pending.CompletionTokens
		}
		pending.TotalTokens = pending.PromptTokens + pending.CompletionTokens
		u := addTTSUsage(total, pending)
		if reserve != nil {
			return reserve(&u)
		}
		return nil
	}
	send := func(e ttsEvent) error {
		if setting.ShouldCheckPromptSensitive() {
			text, _ := e.Payload.Input["text"].(string)
			instruction, _ := e.Payload.Parameters["instruction"].(string)
			if blocked, _ := service.CheckSensitiveText(text + "\n" + instruction); blocked {
				return invalid(errors.New("TTS text rejected by the configured sensitive-word policy"))
			}
		}
		if err := state.acceptClient(&e, info.OriginModelName, info.UpstreamModelName); err != nil {
			return invalid(err)
		}
		if err := reserveCurrent(); err != nil {
			return err
		}
		return writeTTSEvent(target, e)
	}
	if request != nil {
		initial.Payload.Model = info.OriginModelName
		if err := send(initial); err != nil {
			return nil, fail(err)
		}
	}
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	idle := time.NewTimer(60 * time.Second)
	defer idle.Stop()
	var drain <-chan time.Time
	var drainTimer *time.Timer
	defer func() {
		if drainTimer != nil {
			drainTimer.Stop()
		}
	}()
	requestDone := c.Request.Context().Done()
	var terminalErr error
	connected := true
	beginDrain := func(cause error) {
		if drain != nil {
			return
		}
		terminalErr = cause
		requestDone = nil
		if state.id != "" {
			e := taskCommand("finish-task", state.id)
			e.Payload.Input["directive"] = "cancel"
			_ = writeTTSEvent(target, e)
			state.cancelled, state.finishing = true, true
		}
		drainTimer = time.NewTimer(2 * time.Second)
		drain = drainTimer.C
	}
	writeOutput := func(kind int, data []byte) error {
		if !connected {
			return nil
		}
		if client != nil {
			return writeTTSFrame(client, kind, data)
		}
		if kind != websocket.BinaryMessage {
			return nil
		}
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(ttsWriteTimeout))
		if request.StreamFormat == "sse" {
			c.Header("Content-Type", "text/event-stream")
			payload, _ := common.Marshal(map[string]any{"type": "speech.audio.delta", "audio": base64.StdEncoding.EncodeToString(data)})
			_, err = fmt.Fprintf(c.Writer, "data: %s\n\n", payload)
		} else {
			c.Header("Content-Type", audioContentType(format))
			_, err = c.Writer.Write(data)
		}
		if err != nil {
			return err
		}
		return http.NewResponseController(c.Writer).Flush()
	}
	for {
		select {
		case <-requestDone:
			connected = false
			if state.id == "" {
				return nil, fail(c.Request.Context().Err())
			}
			beginDrain(c.Request.Context().Err())
		case <-deadline.C:
			beginDrain(errors.New("TTS connection exceeded 10 minutes"))
		case <-idle.C:
			if state.id == "" {
				if len(state.seen) == 0 {
					return nil, fail(errors.New("no TTS task was submitted"))
				}
				return nil, nil
			}
			beginDrain(errors.New("TTS upstream idle timeout"))
		case <-drain:
			return nil, fail(terminalErr)
		case frame := <-downstream:
			if drain != nil {
				continue
			}
			if frame.err != nil {
				connected = false
				if state.id == "" {
					if len(state.seen) == 0 {
						return nil, fail(errors.New("no TTS task was submitted"))
					}
					return nil, nil
				}
				beginDrain(types.NewClientCanceledError())
				continue
			}
			var event ttsEvent
			if frame.kind != websocket.TextMessage || common.Unmarshal(frame.data, &event) != nil {
				beginDrain(invalid(errors.New("TTS client messages must be JSON text frames")))
				continue
			}
			if err := send(event); err != nil {
				beginDrain(err)
			}
		case frame := <-upstream:
			if frame.err != nil {
				if terminalErr != nil {
					return nil, fail(terminalErr)
				}
				if state.id == "" && len(state.seen) > 0 {
					return nil, nil
				}
				return nil, fail(errors.New("TTS upstream closed before task-finished"))
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(60 * time.Second)
			if frame.kind == websocket.BinaryMessage {
				if state.id == "" || !state.started {
					return nil, fail(errors.New("unexpected TTS audio frame"))
				}
				if state.cancelled {
					continue
				}
				state.audio = true
				info.SetFirstResponseTime()
				if err := writeOutput(frame.kind, frame.data); err != nil {
					connected = false
					beginDrain(err)
				}
				continue
			}
			var event ttsEvent
			if frame.kind != websocket.TextMessage || common.Unmarshal(frame.data, &event) != nil || state.id == "" || event.Header.TaskID != state.id {
				return nil, fail(errors.New("invalid TTS upstream event or task_id"))
			}
			if err := state.meter.observe(event.Payload.Usage); err != nil {
				return nil, fail(err)
			}
			if event.Payload.Usage != nil && drain == nil {
				if err := reserveCurrent(); err != nil {
					beginDrain(err)
				}
			}
			switch event.Header.Event {
			case "task-started":
				if state.started {
					return nil, fail(errors.New("duplicate task-started"))
				}
				state.started = true
				if request != nil && drain == nil {
					e := taskCommand("continue-task", state.id)
					e.Payload.Input["text"] = request.Input
					if err := send(e); err != nil {
						beginDrain(err)
						continue
					}
					if err := send(taskCommand("finish-task", state.id)); err != nil {
						beginDrain(err)
					}
				}
			case "result-generated":
				if !state.started {
					return nil, fail(errors.New("result before task-started"))
				}
			case "task-failed":
				_ = writeOutput(frame.kind, frame.data)
				return nil, fail(fmt.Errorf("Ali TTS task failed (%s): %s", event.Header.ErrorCode, event.Header.Message))
			case "task-finished":
				if !state.finishing {
					return nil, fail(errors.New("unexpected task-finished before finish-task"))
				}
				if !state.meter.known {
					return nil, fail(errors.New("Ali TTS terminal usage missing"))
				}
				if !state.cancelled && (!state.started || !state.audio) {
					return nil, fail(errors.New("Ali TTS finished without audio"))
				}
				total = addTTSUsage(total, state.meter.usage())
				state.meter = ttsMeter{}
				state.id = ""
				if drain != nil {
					return nil, fail(terminalErr)
				}
				if request != nil {
					if request.StreamFormat == "sse" {
						data, _ := common.Marshal(map[string]any{"type": "speech.audio.done", "usage": total})
						if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
							return nil, fail(err)
						}
						if err := http.NewResponseController(c.Writer).Flush(); err != nil {
							return nil, fail(err)
						}
					}
					info.StreamStatus.MarkDeliveredCompletion()
					return nil, nil
				}
			default:
				return nil, fail(errors.New("unknown TTS upstream event"))
			}
			if err := writeOutput(frame.kind, frame.data); err != nil {
				connected = false
				if state.id == "" {
					return nil, fail(err)
				}
				beginDrain(err)
			}
		}
	}
}
