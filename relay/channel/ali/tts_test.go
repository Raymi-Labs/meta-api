package ali

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func ttsTestInfo(base string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{OriginModelName: "voice-model", RelayFormat: types.RelayFormatAliTTS, RelayMode: relayconstant.RelayModeAudioSpeech,
		StreamStatus: relaycommon.NewStreamStatus(), ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAli, ChannelBaseUrl: base, ApiKey: "fake-upstream", UpstreamModelName: "cosyvoice-v3-flash"}}
}

func readTTSTest(conn *websocket.Conn) (ttsEvent, error) {
	_, data, err := conn.ReadMessage()
	var e ttsEvent
	if err == nil {
		err = common.Unmarshal(data, &e)
	}
	return e, err
}

func ttsServerEvent(conn *websocket.Conn, id, event string, chars *int) error {
	e := ttsEvent{}
	e.Header.TaskID, e.Header.Event = id, event
	if chars != nil {
		e.Payload.Usage = &ttsUsage{Characters: chars}
	}
	return writeTTSEvent(conn, e)
}

func TestAliTTSHTTPStreamsBeforeCompletion(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"audio", "sse"} {
		t.Run(mode, func(t *testing.T) {
			allowFinish := make(chan struct{})
			upstreamErr := make(chan error, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api-ws/v1/inference" || r.Header.Get("Authorization") != "Bearer fake-upstream" {
					upstreamErr <- fmt.Errorf("bad upstream route/auth")
					return
				}
				u := websocket.Upgrader{}
				conn, err := u.Upgrade(w, r, nil)
				if err != nil {
					upstreamErr <- err
					return
				}
				defer conn.Close()
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				run, err := readTTSTest(conn)
				if err != nil {
					upstreamErr <- err
					return
				}
				if run.Payload.Model != "cosyvoice-v3-flash" {
					upstreamErr <- fmt.Errorf("model mapping lost")
					return
				}
				_ = ttsServerEvent(conn, run.Header.TaskID, "task-started", nil)
				text, err := readTTSTest(conn)
				if err != nil {
					upstreamErr <- err
					return
				}
				if text.Payload.Input["text"] != "你好" {
					upstreamErr <- fmt.Errorf("text lost")
					return
				}
				_, err = readTTSTest(conn)
				if err != nil {
					upstreamErr <- err
					return
				}
				_ = conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3, 4})
				select {
				case <-allowFinish:
				case <-time.After(5 * time.Second):
					upstreamErr <- fmt.Errorf("audio was buffered until completion")
					return
				}
				_ = ttsServerEvent(conn, run.Header.TaskID, "result-generated", common.GetPointer(4))
				_ = ttsServerEvent(conn, run.Header.TaskID, "task-finished", common.GetPointer(4))
				upstreamErr <- nil
			}))
			defer upstream.Close()
			result := make(chan *dto.Usage, 1)
			relayErr := make(chan *types.NewAPIError, 1)
			r := gin.New()
			r.POST("/speech", func(c *gin.Context) {
				info := ttsTestInfo(upstream.URL)
				u, e := RelayTTS(c, info, &dto.AudioRequest{Input: "你好", Voice: "saved-voice", StreamFormat: mode}, nil)
				result <- u
				relayErr <- e
			})
			gateway := httptest.NewServer(r)
			defer gateway.Close()
			client := &http.Client{Timeout: 8 * time.Second}
			resp, err := client.Post(gateway.URL+"/speech", "application/json", nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			if mode == "audio" {
				require.Equal(t, "audio/mpeg", resp.Header.Get("Content-Type"))
				b := make([]byte, 4)
				_, err = io.ReadFull(resp.Body, b)
				require.NoError(t, err)
				require.Equal(t, []byte{1, 2, 3, 4}, b)
			} else {
				line, err := bufio.NewReader(resp.Body).ReadString('\n')
				require.NoError(t, err)
				require.Contains(t, line, "speech.audio.delta")
				require.Contains(t, line, "AQIDBA==")
			}
			close(allowFinish)
			require.NoError(t, <-upstreamErr)
			require.Nil(t, <-relayErr)
			u := <-result
			require.Equal(t, 4, u.PromptTokens)
			require.Equal(t, 4, u.TotalTokens)
		})
	}
}

func TestAliTTSDuplexCancelReuseAndDisconnect(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)
	upstreamErr := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := websocket.Upgrader{}
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			upstreamErr <- err
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
		for n := 0; n < 2; n++ {
			run, err := readTTSTest(conn)
			if err != nil {
				upstreamErr <- err
				return
			}
			_ = ttsServerEvent(conn, run.Header.TaskID, "task-started", nil)
			_, err = readTTSTest(conn)
			if err != nil {
				upstreamErr <- err
				return
			}
			_ = conn.WriteMessage(websocket.BinaryMessage, []byte{255, 0, 128})
			_ = ttsServerEvent(conn, run.Header.TaskID, "result-generated", common.GetPointer(3))
			cancel, err := readTTSTest(conn)
			if err != nil {
				upstreamErr <- err
				return
			}
			if cancel.Header.Action != "finish-task" || cancel.Payload.Input["directive"] != "cancel" {
				upstreamErr <- fmt.Errorf("missing cancel")
				return
			}
			_ = ttsServerEvent(conn, run.Header.TaskID, "task-finished", common.GetPointer(5))
		}
		upstreamErr <- nil
	}))
	defer upstream.Close()
	result := make(chan *dto.Usage, 1)
	relayErr := make(chan *types.NewAPIError, 1)
	r := gin.New()
	r.GET("/stream", func(c *gin.Context) {
		u := websocket.Upgrader{}
		conn, err := u.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		info := ttsTestInfo(upstream.URL)
		info.ClientWs = conn
		usage, e := RelayTTS(c, info, nil, nil)
		result <- usage
		relayErr <- e
	})
	gateway := httptest.NewServer(r)
	defer gateway.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http")+"/stream", nil)
	require.NoError(t, err)
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	for n := 0; n < 2; n++ {
		run := taskCommand("run-task", fmt.Sprintf("task-%d", n))
		run.Payload.Model = "voice-model"
		run.Payload.TaskGroup = "audio"
		run.Payload.Task = "tts"
		run.Payload.Function = "SpeechSynthesizer"
		run.Payload.Parameters = map[string]any{"voice": "saved"}
		require.NoError(t, writeTTSEvent(conn, run))
		e, err := readTTSTest(conn)
		require.NoError(t, err)
		require.Equal(t, "task-started", e.Header.Event)
		text := taskCommand("continue-task", run.Header.TaskID)
		text.Payload.Input["text"] = "你好"
		require.NoError(t, writeTTSEvent(conn, text))
		kind, audio, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.BinaryMessage, kind)
		require.Equal(t, []byte{255, 0, 128}, audio)
		_, err = readTTSTest(conn)
		require.NoError(t, err)
		if n == 0 {
			cancel := taskCommand("finish-task", run.Header.TaskID)
			cancel.Payload.Input["directive"] = "cancel"
			require.NoError(t, writeTTSEvent(conn, cancel))
			e, err = readTTSTest(conn)
			require.NoError(t, err)
			require.Equal(t, "task-finished", e.Header.Event)
		} else {
			_ = conn.Close()
		}
	}
	require.NoError(t, <-upstreamErr)
	u := <-result
	require.Equal(t, 10, u.PromptTokens)
	require.NotNil(t, <-relayErr)
}

func TestAliTTSProtocolAndMeter(t *testing.T) {
	s := ttsTaskState{seen: map[string]bool{}}
	run := taskCommand("run-task", "task-1")
	run.Payload.Model = "forbidden"
	run.Payload.TaskGroup = "audio"
	run.Payload.Task = "tts"
	run.Payload.Function = "SpeechSynthesizer"
	run.Payload.Parameters = map[string]any{"voice": "saved"}
	require.ErrorContains(t, s.acceptClient(&run, "authorized", "cosyvoice-v3-flash"), "authorized")
	run.Payload.Model = "authorized"
	require.NoError(t, s.acceptClient(&run, "authorized", "cosyvoice-v3-flash"))
	require.Equal(t, "cosyvoice-v3-flash", run.Payload.Model)
	text := taskCommand("continue-task", "task-1")
	text.Payload.Input["text"] = "test"
	require.Error(t, s.acceptClient(&text, "authorized", "cosyvoice-v3-flash"))
	s.started = true
	text.Header.TaskID = "other"
	require.Error(t, s.acceptClient(&text, "authorized", "cosyvoice-v3-flash"))
	m := ttsMeter{tokenModel: true}
	require.NoError(t, m.observe(&ttsUsage{Input: common.GetPointer(4), Output: common.GetPointer(8), Characters: common.GetPointer(90)}))
	require.NoError(t, m.observe(&ttsUsage{Input: common.GetPointer(4), Output: common.GetPointer(8)}))
	u := m.usage()
	require.Equal(t, 12, u.TotalTokens)
	require.Equal(t, 8, u.CompletionTokenDetails.AudioTokens)
	require.Error(t, m.observe(&ttsUsage{Input: common.GetPointer(-1)}))
	_, _, err := speechTask(&dto.AudioRequest{Input: "hello", Voice: "voice", Metadata: []byte(`{"model":"other"}`)}, "cosyvoice-v3-flash")
	require.Error(t, err)
	_, err = ttsURL("https://example.com/compatible-mode/v1")
	require.Error(t, err)
	require.False(t, SupportsTTS("qwen3-tts-flash-realtime"))
}

func TestAliTTSCommandsKeepEmptyInputAndReserveBeforeText(t *testing.T) {
	data, err := common.Marshal(taskCommand("finish-task", "id"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"input":{}`)
	service.InitHttpClient()
	events := make(chan ttsEvent, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		run, err := readTTSTest(conn)
		if err != nil {
			return
		}
		_ = ttsServerEvent(conn, run.Header.TaskID, "task-started", nil)
		next, err := readTTSTest(conn)
		if err != nil {
			return
		}
		events <- next
		_ = ttsServerEvent(conn, run.Header.TaskID, "task-finished", common.GetPointer(0))
	}))
	defer upstream.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/speech", nil)
	info := ttsTestInfo(upstream.URL)
	usage, apiErr := RelayTTS(c, info, &dto.AudioRequest{Input: "hello", Voice: "saved"}, func(u *dto.Usage) error {
		if u.TotalTokens > 0 {
			return types.NewErrorWithStatusCode(fmt.Errorf("quota exhausted"), types.ErrorCodeInvalidRequest, http.StatusPaymentRequired)
		}
		return nil
	})
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusPaymentRequired, apiErr.StatusCode)
	require.Zero(t, usage.TotalTokens)
	select {
	case next := <-events:
		require.Equal(t, "finish-task", next.Header.Action, "text must not reach upstream without reservation")
		require.Equal(t, "cancel", next.Payload.Input["directive"])
	default:
		t.Fatal("cancel was not sent")
	}
}
