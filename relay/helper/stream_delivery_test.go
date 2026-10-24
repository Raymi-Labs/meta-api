package helper

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type deliveryProbeWriter struct {
	gin.ResponseWriter
	writeErr   error
	short      bool
	flushErr   error
	flushes    atomic.Int32
	writes     atomic.Int32
	writeReady chan struct{}
}

func (w *deliveryProbeWriter) Write(data []byte) (int, error) {
	w.writes.Add(1)
	if w.writeReady != nil {
		select {
		case w.writeReady <- struct{}{}:
		default:
		}
	}
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return len(data) - 1, nil
	}
	return w.ResponseWriter.Write(data)
}
func (w *deliveryProbeWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }
func (w *deliveryProbeWriter) FlushError() error {
	w.flushes.Add(1)
	if w.flushErr != nil {
		return w.flushErr
	}
	w.ResponseWriter.Flush()
	return nil
}
func (w *deliveryProbeWriter) Flush() { _ = w.FlushError() }

func TestResponseChunkDataPropagatesWriteShortAndSingleFlushErrors(t *testing.T) {
	for _, scenario := range []string{"write", "short", "flush", "success"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			writer := &deliveryProbeWriter{ResponseWriter: c.Writer}
			c.Writer = writer
			var expected error
			switch scenario {
			case "write":
				expected = io.ErrClosedPipe
				writer.writeErr = expected
			case "short":
				expected = io.ErrShortWrite
				writer.short = true
			case "flush":
				expected = errors.New("flush failed")
				writer.flushErr = expected
			}
			err := ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.completed"}, `{"type":"response.completed"}`)
			if expected == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, expected)
			}
			if scenario == "flush" || scenario == "success" {
				require.EqualValues(t, 1, writer.flushes.Load())
			} else {
				require.Zero(t, writer.flushes.Load())
			}
		})
	}
}

func TestStreamWriteBudgetAccountsForQueuedBytesAndIdle(t *testing.T) {
	now := time.Unix(100, 0)
	budget := StreamWriteBudget{}
	require.Equal(t, now.Add(30*time.Second), budget.Deadline(now, 0))
	require.Equal(t, now.Add(32*time.Second), budget.Deadline(now, 16<<10))
	require.Equal(t, now.Add(34*time.Second), budget.Deadline(now.Add(time.Second), 16<<10))
	require.Equal(t, now.Add(290*time.Second), budget.Deadline(now.Add(time.Second), 2<<20))
	require.Equal(t, now.Add(632*time.Second), budget.Deadline(now.Add(600*time.Second), 16<<10))
}

func TestStreamKeepAliveNestedOwnershipAndHandoff(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	probe := &deliveryProbeWriter{ResponseWriter: c.Writer, writeReady: make(chan struct{}, 8)}
	c.Writer = probe
	stop := StartStreamKeepAlive(c, 15*time.Millisecond, types.RelayFormatClaude)
	defer stop()
	writer := c.Writer.(*streamKeepAliveWriter)
	require.False(t, writer.Written(), "request headers must not be committed immediately")
	for range 2 {
		select {
		case <-probe.writeReady:
		case <-time.After(time.Second):
			t.Fatal("preparation heartbeat missing")
		}
		StartStreamKeepAlive(c, 15*time.Millisecond, types.RelayFormatClaude)()
	}
	require.NoError(t, ClaudeData(c, dto.ClaudeResponse{Type: "message_stop"}))
	select {
	case <-writer.done:
	default:
		t.Fatal("business write did not join heartbeat")
	}
	writes := probe.writes.Load()
	time.Sleep(35 * time.Millisecond)
	require.Equal(t, writes, probe.writes.Load(), "heartbeat survived business handoff")
	require.Contains(t, recorder.Body.String(), "event: ping\ndata: {\"type\":\"ping\"}")
	require.NotContains(t, recorder.Body.String(), ": PING")
	require.True(t, strings.HasSuffix(recorder.Body.String(), "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
}

func TestStreamKeepAliveCancellationAndImmediateResponse(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx, cancel := context.WithCancel(context.Background())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
		stop := StartStreamKeepAlive(c, time.Minute, types.RelayFormatClaude)
		if canceled {
			cancel()
		} else {
			_, err := c.Writer.WriteString("data: immediate\n\n")
			require.NoError(t, err)
		}
		done := make(chan struct{})
		go func() { stop(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("heartbeat cleanup blocked")
		}
		cancel()
	}
}

type headerCommitWriter struct {
	gin.ResponseWriter
	started chan struct{}
	release chan struct{}
}

func (w *headerCommitWriter) Write(data []byte) (int, error) {
	close(w.started)
	<-w.release
	// A real net/http first write reads this map while committing headers.
	_ = w.Header().Clone()
	return w.ResponseWriter.Write(data)
}

func TestStreamKeepAliveHeaderHandoffAndRetryRestart(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	for attempt := 0; attempt < 3; attempt++ {
		StopStreamKeepAlive(c)
		// Start again after a previous diagnostic header stopped the pinger.
		if previous, ok := c.Writer.(*streamKeepAliveWriter); ok {
			c.Writer = previous.ResponseWriter
		}
		base := c.Writer
		blocked := &headerCommitWriter{ResponseWriter: base, started: make(chan struct{}), release: make(chan struct{})}
		c.Writer = blocked
		stop := StartStreamKeepAlive(c, time.Millisecond, types.RelayFormatClaude)
		select {
		case <-blocked.started:
		case <-time.After(time.Second):
			t.Fatal("retry heartbeat did not restart")
		}
		headerDone := make(chan struct{})
		go func() { c.Header("X-Request-Adjusted", "normalized"); close(headerDone) }()
		select {
		case <-headerDone:
			t.Fatal("mutable response headers escaped before heartbeat joined")
		case <-time.After(10 * time.Millisecond):
		}
		close(blocked.release)
		select {
		case <-headerDone:
		case <-time.After(time.Second):
			t.Fatal("header handoff deadlocked")
		}
		stop()
		stop()
		require.Equal(t, "normalized", c.Writer.Header().Get("X-Request-Adjusted"))
		c.Writer = base
	}
}

type boundedBlockingWriter struct {
	gin.ResponseWriter
	deadline  time.Time
	requested time.Time
}

func (w *boundedBlockingWriter) SetWriteDeadline(deadline time.Time) error {
	w.requested = deadline
	w.deadline = time.Now().Add(20 * time.Millisecond)
	return nil
}
func (w *boundedBlockingWriter) Write(data []byte) (int, error) {
	if w.deadline.IsZero() {
		return 0, errors.New("missing write deadline")
	}
	<-time.After(time.Until(w.deadline))
	return 0, context.DeadlineExceeded
}

func TestStreamScannerSlowWriteClosesAndJoinsUpstream(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	blocked := &boundedBlockingWriter{ResponseWriter: c.Writer}
	c.Writer = blocked
	upstreamDone := make(chan struct{})
	go func() {
		defer close(upstreamDone)
		_, _ = io.WriteString(writer, "data: ready\n\n")
		_, _ = io.WriteString(writer, strings.Repeat("x", 128<<10))
	}()
	info := &relaycommon.RelayInfo{DisablePing: true}
	started := time.Now()
	StreamScannerHandler(c, &http.Response{Body: reader}, info, func(string) bool {
		_, err := c.Writer.Write(make([]byte, 2<<20))
		require.ErrorIs(t, err, context.DeadlineExceeded)
		info.StreamStatus.RecordError(err.Error())
		return false
	})
	require.Less(t, time.Since(started), time.Second)
	require.Greater(t, blocked.requested.Sub(started), 250*time.Second, "large writes need a byte budget, not a fixed timeout")
	select {
	case <-upstreamDone:
	case <-time.After(time.Second):
		t.Fatal("upstream reader survived handler return")
	}
	require.False(t, info.StreamStatus.IsSuccessful())
}
