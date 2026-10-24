package openai

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const deliveredResponseEvent = `data: {"type":"response.completed","response":{"id":"resp_delivered","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":23,"output_tokens":7,"total_tokens":30}}}` + "\n\n"

type completionBlockingBody struct {
	reader *strings.Reader
	closed chan struct{}
	once   sync.Once
	cancel context.CancelFunc
}

func newCompletionBlockingBody(body string) *completionBlockingBody {
	return &completionBlockingBody{reader: strings.NewReader(body), closed: make(chan struct{})}
}
func (b *completionBlockingBody) Read(data []byte) (int, error) {
	if b.reader.Len() > 0 {
		return b.reader.Read(data)
	}
	<-b.closed
	return 0, errors.New("body closed during cleanup")
}
func (b *completionBlockingBody) Close() error {
	b.once.Do(func() {
		if b.cancel != nil {
			b.cancel()
		}
		close(b.closed)
	})
	return nil
}

type completionDeliveryWriter struct {
	gin.ResponseWriter
	scenario string
	terminal bool
	cancel   context.CancelFunc
	flushes  int
}

func (w *completionDeliveryWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), `"type":"response.completed"`) {
		w.terminal = true
		if w.scenario == "write" {
			return 0, io.ErrClosedPipe
		}
		if w.scenario == "short" {
			return len(data) - 1, nil
		}
	}
	return w.ResponseWriter.Write(data)
}
func (w *completionDeliveryWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}
func (w *completionDeliveryWriter) Flush() { _ = w.FlushError() }
func (w *completionDeliveryWriter) FlushError() error {
	w.flushes++
	if w.terminal && w.scenario == "flush" {
		return errors.New("flush failed")
	}
	w.ResponseWriter.Flush()
	if w.terminal && w.cancel != nil {
		w.cancel()
	}
	return nil
}

func TestResponsesDeliveredCompletionStopsWithoutUpstreamEOF(t *testing.T) {
	for _, scenario := range []string{"success", "cancel-after-confirmed-delivery", "cancel-during-flush", "write", "short", "flush"} {
		t.Run(scenario, func(t *testing.T) {
			c, recorder := newResponsesStreamTestContext()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			writer := &completionDeliveryWriter{ResponseWriter: c.Writer, scenario: scenario}
			if scenario == "cancel-during-flush" {
				writer.cancel = cancel
			}
			c.Writer = writer
			body := newCompletionBlockingBody(deliveredResponseEvent)
			if scenario == "cancel-after-confirmed-delivery" {
				body.cancel = cancel
			}
			defer body.Close()
			info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test"}}
			start := time.Now()
			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: body})
			require.Less(t, time.Since(start), time.Second)
			select {
			case <-body.closed:
			default:
				t.Fatal("completion did not close upstream")
			}
			if scenario == "success" || scenario == "cancel-after-confirmed-delivery" {
				require.Nil(t, apiErr)
				require.True(t, info.StreamStatus.HasDeliveredCompletion())
				require.True(t, info.StreamStatus.IsSuccessful())
				require.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
				require.Equal(t, 23, usage.PromptTokens)
				require.Equal(t, 7, usage.CompletionTokens)
				require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed"))
				require.Equal(t, 1, writer.flushes, "must not flush twice through wrappers")
			} else {
				require.NotNil(t, apiErr)
				require.True(t, types.IsSkipRetryError(apiErr))
				require.False(t, info.StreamStatus.HasDeliveredCompletion())
				require.False(t, info.StreamStatus.IsSuccessful())
			}
		})
	}
}

func TestResponsesInvalidCompletionNeverConfirmsDelivery(t *testing.T) {
	for _, completion := range []string{
		`{"type":"response.completed"}`,
		`{"type":"response.completed","response":{"status":"in_progress"}}`,
		`{"type":"response.completed","response":{"status":"failed","error":{"message":"failed"}}}`,
	} {
		c, _ := newResponsesStreamTestContext()
		info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test"}}
		body := `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\ndata: " + completion + "\n\n"
		_, apiErr := OaiResponsesStreamHandler(c, info, newResponsesStreamHTTPResponse(body))
		require.NotNil(t, apiErr)
		require.False(t, info.StreamStatus.HasDeliveredCompletion())
		require.False(t, info.StreamStatus.IsSuccessful())
	}
}

func TestResponsesRealHTTPCompletionAndDisconnect(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "completed-client-close", false: "partial-client-close"}[complete], func(t *testing.T) {
			upstreamClosed := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(upstreamClosed)
				w.Header().Set("Content-Type", "text/event-stream")
				data := `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"
				if complete {
					data = deliveredResponseEvent
				}
				_, _ = io.WriteString(w, data)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer upstream.Close()
			type result struct{ delivered, success, failed bool }
			finished := make(chan result, 1)
			r := gin.New()
			r.POST("/v1/responses", func(c *gin.Context) {
				req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstream.URL, nil)
				if err != nil {
					finished <- result{failed: true}
					return
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					finished <- result{failed: true}
					return
				}
				info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test"}}
				_, apiErr := OaiResponsesStreamHandler(c, info, resp)
				finished <- result{info.StreamStatus.HasDeliveredCompletion(), info.StreamStatus.IsSuccessful(), apiErr != nil}
			})
			gateway := httptest.NewServer(r)
			defer gateway.Close()
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Post(gateway.URL+"/v1/responses", "application/json", strings.NewReader("{}"))
			require.NoError(t, err)
			reader := bufio.NewReader(resp.Body)
			for {
				line, readErr := reader.ReadString('\n')
				require.NoError(t, readErr)
				if line == "\n" {
					break
				}
			}
			require.NoError(t, resp.Body.Close())
			select {
			case got := <-finished:
				require.Equal(t, complete, got.delivered)
				require.Equal(t, complete, got.success)
				require.Equal(t, !complete, got.failed)
			case <-time.After(time.Second):
				t.Fatal("handler kept waiting after terminal event or disconnect")
			}
			select {
			case <-upstreamClosed:
			case <-time.After(time.Second):
				t.Fatal("upstream request survived handler cleanup")
			}
		})
	}
}

type flushDisconnectWriter struct {
	gin.ResponseWriter
	started chan struct{}
	done    <-chan struct{}
	flushes int
}

// Deliberately expose only Flush, like Gin and buffering middleware. The
// request cancellation is the only observable evidence of its hidden failure.
func (w *flushDisconnectWriter) Flush() {
	w.flushes++
	if w.flushes == 1 {
		close(w.started)
	}
	<-w.done
	w.ResponseWriter.Flush()
}

func TestResponsesRealHTTPHiddenFlushDisconnect(t *testing.T) {
	flushStarted := make(chan struct{})
	type result struct {
		delivered bool
		failed    bool
		flushes   int
	}
	finished := make(chan result, 1)
	r := gin.New()
	r.POST("/v1/responses", func(c *gin.Context) {
		writer := &flushDisconnectWriter{ResponseWriter: c.Writer, started: flushStarted, done: c.Request.Context().Done()}
		c.Writer = writer
		info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test"}}
		_, apiErr := OaiResponsesStreamHandler(c, info, newResponsesStreamHTTPResponse(deliveredResponseEvent))
		finished <- result{info.StreamStatus.HasDeliveredCompletion(), apiErr != nil, writer.flushes}
	})
	gateway := httptest.NewServer(r)
	defer gateway.Close()
	conn, err := net.DialTimeout("tcp", gateway.Listener.Addr().String(), time.Second)
	require.NoError(t, err)
	defer conn.Close()
	_, err = io.WriteString(conn, "POST /v1/responses HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\n\r\n")
	require.NoError(t, err)
	select {
	case <-flushStarted:
	case <-time.After(time.Second):
		t.Fatal("completed event never reached the buffered flush")
	}
	require.NoError(t, conn.(*net.TCPConn).SetLinger(0))
	require.NoError(t, conn.Close())
	select {
	case got := <-finished:
		require.False(t, got.delivered)
		require.True(t, got.failed)
		require.Equal(t, 1, got.flushes, "fallback must not bypass middleware or flush twice")
	case <-time.After(time.Second):
		t.Fatal("real downstream disconnect did not end the request")
	}
}
