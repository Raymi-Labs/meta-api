package claude

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeStreamIncompleteReturnsFailure(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		for _, suffix := range []string{"", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}`, "[DONE]"} {
			t.Run(string(format)+"/"+suffix, func(t *testing.T) {
				_, err, info, _, body := runUsageStream(t, format, usageStart, usageText, suffix)
				require.NotNil(t, err, "an interrupted stream must not settle as success")
				require.False(t, info.StreamStatus.IsSuccessful())
				require.True(t, types.IsSkipRetryError(err))
				require.NotContains(t, body, "[DONE]")
				require.NotContains(t, body, `"finish_reason":"stop"`)
			})
		}
	}
}

type stalledClaudeBody struct {
	closed chan struct{}
	once   sync.Once
}

func (b *stalledClaudeBody) Read(p []byte) (int, error) { <-b.closed; return 0, io.ErrClosedPipe }
func (b *stalledClaudeBody) Close() error               { b.once.Do(func() { close(b.closed) }); return nil }

type claudeTerminalWriter struct {
	gin.ResponseWriter
	mode     string
	terminal bool
	cancel   context.CancelFunc
}

func (w *claudeTerminalWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "message_stop") || strings.Contains(string(data), "[DONE]") {
		w.terminal = true
	}
	if w.terminal && w.mode == "write" {
		return 0, io.ErrClosedPipe
	}
	if w.terminal && w.mode == "short" {
		return 0, nil
	}
	return w.ResponseWriter.Write(data)
}
func (w *claudeTerminalWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }
func (w *claudeTerminalWriter) FlushError() error {
	if w.terminal && w.mode == "flush" {
		return io.ErrClosedPipe
	}
	w.ResponseWriter.Flush()
	if w.terminal && w.mode == "cancel" {
		w.cancel()
	}
	return nil
}

func TestClaudeStreamTerminalDeliveryFailures(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		for _, mode := range []string{"write", "short", "flush", "cancel"} {
			t.Run(string(format)+"/"+mode, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
				c.Writer = &claudeTerminalWriter{ResponseWriter: c.Writer, mode: mode, cancel: cancel}
				info := &relaycommon.RelayInfo{RelayFormat: format, IsStream: true, DisablePing: true, ShouldIncludeUsage: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
				body := &terminalGuardBody{Reader: strings.NewReader("data: " + strings.Join([]string{usageStart, usageText, usageStop}, "\n\ndata: ") + "\n\n")}
				u, err := ClaudeStreamHandler(c, &http.Response{StatusCode: 200, Body: body}, info)
				if mode == "cancel" {
					require.Nil(t, err)
					require.NotNil(t, u)
					require.False(t, info.StreamStatus.IsSuccessful())
					require.False(t, info.StreamStatus.HasDeliveredCompletion())
					require.True(t, body.closed)
					return
				}
				require.Nil(t, u)
				require.NotNil(t, err)
				require.True(t, types.IsSkipRetryError(err))
				require.False(t, info.StreamStatus.HasDeliveredCompletion())
				require.True(t, body.closed)
				require.True(t, helper.IsDownstreamWriteError(err))
			})
		}
	}
}

func TestClaudeStreamTimeoutAndReadFailure(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = old })
	for _, tc := range []struct {
		name   string
		body   io.ReadCloser
		status int
	}{
		{"timeout", &stalledClaudeBody{closed: make(chan struct{})}, 504},
		{"reset", &terminalGuardBody{Reader: strings.NewReader("data: " + usageStart + "\n\ndata: " + usageText + "\n\n"), readError: errors.New("read tcp private-upstream: connection reset by peer")}, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
			start := time.Now()
			u, e := ClaudeStreamHandler(c, &http.Response{StatusCode: 200, Body: tc.body}, info)
			require.Nil(t, u)
			require.NotNil(t, e)
			require.Equal(t, tc.status, e.StatusCode)
			require.Less(t, time.Since(start), 3*time.Second)
			require.False(t, info.StreamStatus.IsSuccessful())
			require.NotContains(t, e.Error(), "tcp")
		})
	}
	_, err, info, _, body := runUsageStream(t, types.RelayFormatClaude, usageStop)
	require.NotNil(t, err)
	require.False(t, info.StreamStatus.IsSuccessful())
	require.NotContains(t, body, "message_stop")
}

func TestClaudeStreamCompletedDoesNotReadTrailingError(t *testing.T) {
	_, err, info, _, body := runUsageStream(t, types.RelayFormatClaude, usageStart, usageStop, `{"type":"error","error":{"type":"api_error","message":"too late"}}`)
	require.Nil(t, err)
	require.True(t, info.StreamStatus.HasDeliveredCompletion())
	require.NotContains(t, body, "too late")
}

// A terminal event must end the request without reading the next upstream byte.
type terminalGuardBody struct {
	*strings.Reader
	readAfterTerminal bool
	closed            bool
	readError         error
}

func (b *terminalGuardBody) Read(p []byte) (int, error) {
	if b.Reader.Len() == 0 {
		b.readAfterTerminal = true
		if b.readError != nil {
			return 0, b.readError
		}
		return 0, io.ErrUnexpectedEOF
	}
	return b.Reader.Read(p)
}
func (b *terminalGuardBody) Close() error { b.closed = true; return nil }

func TestClaudeStreamStopsAtDeliveredTerminal(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: format, IsStream: true, DisablePing: true, ShouldIncludeUsage: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
			body := &terminalGuardBody{Reader: strings.NewReader("data: " + strings.Join([]string{usageStart, usageText, usageStop}, "\n\ndata: ") + "\n\n")}
			_, err := ClaudeStreamHandler(c, &http.Response{StatusCode: 200, Body: body}, info)
			require.Nil(t, err)
			require.False(t, body.readAfterTerminal, "must not wait for EOF after message_stop")
			require.True(t, body.closed)
			require.True(t, info.StreamStatus.HasDeliveredCompletion())
		})
	}
}

func TestClaudeStreamRealHTTPClosesAfterTerminal(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			exited := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(exited)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: "+strings.Join([]string{usageStart, usageText, usageStop}, "\n\ndata: ")+"\n\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done() // Deliberately never EOF: the relay must close us.
			}))
			defer server.Close()
			requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			request, e := http.NewRequestWithContext(requestCtx, http.MethodGet, server.URL, nil)
			require.NoError(t, e)
			response, e := server.Client().Do(request)
			require.NoError(t, e)
			defer response.Body.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(requestCtx)
			info := &relaycommon.RelayInfo{RelayFormat: format, IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
			_, apiErr := ClaudeStreamHandler(c, response, info)
			require.Nil(t, apiErr)
			require.NoError(t, requestCtx.Err())
			require.True(t, info.StreamStatus.HasDeliveredCompletion())
			select {
			case <-exited:
			case <-time.After(time.Second):
				t.Fatal("upstream worker did not exit")
			}
		})
	}
}
