package helper

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const DownstreamWriteErrorCode types.ErrorCode = "downstream_write_failed"

func IsDownstreamWriteError(err *types.NewAPIError) bool {
	return err != nil && err.GetErrorType() == types.ErrorTypeNewAPIError &&
		err.UpstreamStatusCode == 0 && err.GetErrorCode() == DownstreamWriteErrorCode
}

const (
	streamWriteTimeout  = 30 * time.Second
	streamMinClientRate = 8 << 10
)

// StreamWriteBudget includes queued bytes from earlier writes: the network
// buffer may still contain those bytes when the next Write starts. An idle
// stream starts a fresh budget instead of accumulating unused time.
type StreamWriteBudget struct {
	drainBy time.Time
}

func (b *StreamWriteBudget) Deadline(now time.Time, n int) time.Time {
	if b.drainBy.Before(now) {
		b.drainBy = now
	}
	if n > 0 {
		b.drainBy = b.drainBy.Add(time.Duration(n) * time.Second / streamMinClientRate)
	}
	return b.drainBy.Add(streamWriteTimeout)
}

type streamResponseWriter struct {
	gin.ResponseWriter
	mu     sync.Mutex
	budget StreamWriteBudget
}

func (w *streamResponseWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(w.budget.Deadline(time.Now(), len(data)))
	n, err := w.ResponseWriter.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (w *streamResponseWriter) WriteString(data string) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(w.budget.Deadline(time.Now(), len(data)))
	n, err := w.ResponseWriter.WriteString(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (w *streamResponseWriter) Flush() { _ = w.FlushError() }

func (w *streamResponseWriter) FlushError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return flushStreamWriter(w.ResponseWriter)
}

func (w *streamResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush only once, through the outer writer. Unwrapping to look for a second
// flush would bypass buffering middleware or duplicate its side effects.
func flushStreamWriter(writer gin.ResponseWriter) error {
	if flusher, ok := writer.(interface{ FlushError() error }); ok {
		return flusher.FlushError()
	}
	writer.Flush()
	return nil
}
