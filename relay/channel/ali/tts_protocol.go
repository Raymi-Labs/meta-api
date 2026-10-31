package ali

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/google/uuid"
)

// Only models using the DashScope duplex task protocol belong here. Qwen3-TTS
// Realtime uses a different event and accounting contract.
func SupportsTTS(model string) bool { return common.IsAliTTSModel(model) }

func ttsURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid Ali TTS base URL; use the workspace origin")
	}
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		u.Scheme = "ws"
	default:
		return "", errors.New("invalid Ali TTS URL scheme")
	}
	if u.Path != "" && u.Path != "/api-ws/v1/inference" {
		return "", errors.New("Ali TTS base URL must be an origin or /api-ws/v1/inference")
	}
	u.Path = "/api-ws/v1/inference"
	return u.String(), nil
}

type ttsUsage struct {
	Characters *int `json:"characters,omitempty"`
	Input      *int `json:"input_tokens,omitempty"`
	Output     *int `json:"output_tokens,omitempty"`
}

type ttsEvent struct {
	Header struct {
		Action    string `json:"action,omitempty"`
		Event     string `json:"event,omitempty"`
		TaskID    string `json:"task_id"`
		Streaming string `json:"streaming,omitempty"`
		ErrorCode string `json:"error_code,omitempty"`
		Message   string `json:"error_message,omitempty"`
	} `json:"header"`
	Payload struct {
		TaskGroup  string         `json:"task_group,omitempty"`
		Task       string         `json:"task,omitempty"`
		Function   string         `json:"function,omitempty"`
		Model      string         `json:"model,omitempty"`
		Parameters map[string]any `json:"parameters,omitempty"`
		Input      map[string]any `json:"input"`
		Usage      *ttsUsage      `json:"usage,omitempty"`
	} `json:"payload"`
}

func taskCommand(action, id string) ttsEvent {
	e := ttsEvent{}
	e.Header.Action, e.Header.TaskID, e.Header.Streaming = action, id, "duplex"
	e.Payload.Input = map[string]any{}
	return e
}

func speechTask(request *dto.AudioRequest, model string) (ttsEvent, string, error) {
	if !SupportsTTS(model) {
		return ttsEvent{}, "", errors.New("unsupported Ali TTS model")
	}
	if strings.TrimSpace(request.Input) == "" || utf8.RuneCountInString(request.Input) > 20000 {
		return ttsEvent{}, "", errors.New("input must contain 1 to 20000 characters")
	}
	if request.StreamFormat != "" && request.StreamFormat != "audio" && request.StreamFormat != "sse" {
		return ttsEvent{}, "", errors.New("stream_format must be audio or sse")
	}
	format := request.ResponseFormat
	if format == "" {
		format = "mp3"
	}
	if audioContentType(format) == "" {
		return ttsEvent{}, "", errors.New("Ali TTS response_format must be pcm, wav, mp3 or opus")
	}
	if request.Voice == "" || (request.Speed != 0 && (request.Speed < .5 || request.Speed > 2)) {
		return ttsEvent{}, "", errors.New("voice is required and speed must be between 0.5 and 2")
	}
	e := taskCommand("run-task", uuid.NewString())
	e.Payload.TaskGroup, e.Payload.Task, e.Payload.Function = "audio", "tts", "SpeechSynthesizer"
	e.Payload.Model = model
	e.Payload.Parameters = map[string]any{"text_type": "PlainText", "voice": request.Voice, "format": format, "sample_rate": 24000}
	if request.Speed != 0 {
		e.Payload.Parameters["rate"] = request.Speed
	}
	if request.Instructions != "" {
		e.Payload.Parameters["instruction"] = request.Instructions
	}
	// Extensions are parameters only; they cannot replace task/model/input.
	if len(request.Metadata) > 0 {
		var parameters map[string]any
		if err := common.Unmarshal(request.Metadata, &parameters); err != nil {
			return e, "", fmt.Errorf("invalid TTS metadata: %w", err)
		}
		for key, value := range parameters {
			switch key {
			case "sample_rate", "volume", "pitch", "bit_rate", "language_hints", "hot_fix", "seed", "enable_ssml", "word_timestamp_enabled":
				e.Payload.Parameters[key] = value
			default:
				return e, "", fmt.Errorf("unsupported TTS metadata parameter: %s", key)
			}
		}
	}
	return e, format, nil
}

func audioContentType(format string) string {
	return map[string]string{"mp3": "audio/mpeg", "pcm": "audio/pcm", "wav": "audio/wav", "opus": "audio/ogg"}[format]
}

// Each upstream task reports cumulative snapshots, including its final event.
// Never sum repeated sentence-end and task-finished snapshots.
type ttsMeter struct {
	tokenModel                bool
	input, output, characters int
	known                     bool
}

func (m *ttsMeter) observe(u *ttsUsage) error {
	if u == nil {
		return nil
	}
	for _, n := range []*int{u.Input, u.Output, u.Characters} {
		if n != nil && (*n < 0 || *n > 100000000) {
			return errors.New("invalid Ali TTS usage")
		}
	}
	if m.tokenModel {
		if u.Input != nil && u.Output != nil {
			m.input, m.output = max(m.input, *u.Input), max(m.output, *u.Output)
			m.known = true
		}
	} else if u.Characters != nil {
		m.characters = max(m.characters, *u.Characters)
		m.known = true
	}
	return nil
}

func (m *ttsMeter) usage() dto.Usage {
	u := dto.Usage{}
	if m.tokenModel {
		u.PromptTokens, u.CompletionTokens = m.input, m.output
		u.CompletionTokenDetails.AudioTokens = m.output
	} else {
		u.PromptTokens = m.characters
	}
	u.PromptTokensDetails.TextTokens = u.PromptTokens
	u.TotalTokens = u.PromptTokens + u.CompletionTokens
	return u
}

func addTTSUsage(a, b dto.Usage) dto.Usage {
	a.PromptTokens += b.PromptTokens
	a.CompletionTokens += b.CompletionTokens
	a.TotalTokens += b.TotalTokens
	a.PromptTokensDetails.TextTokens += b.PromptTokensDetails.TextTokens
	a.CompletionTokenDetails.AudioTokens += b.CompletionTokenDetails.AudioTokens
	return a
}
