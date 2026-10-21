package controller

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// groupHealthWriter observes the server's response writes for health sampling.
// It preserves relay behavior and does not claim that the client consumed them.
type groupHealthWriter struct {
	gin.ResponseWriter
	wrote  atomic.Bool
	failed atomic.Bool
}

var _ gin.ResponseWriter = (*groupHealthWriter)(nil)

func (w *groupHealthWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.observeWrite(n, len(data), err)
	return n, err
}

func (w *groupHealthWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	w.observeWrite(n, len(data), err)
	return n, err
}

func (w *groupHealthWriter) observeWrite(n, expected int, err error) {
	if err != nil || n != expected {
		w.failed.Store(true)
	}
	if n > 0 {
		w.wrote.Store(true)
	}
}

func (w *groupHealthWriter) Flush() {
	_ = w.FlushError()
}

// Keep error-aware writers observable without changing the Flusher contract
// used by existing relay helpers. A panic is recorded and rethrown unchanged.
func (w *groupHealthWriter) FlushError() (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.failed.Store(true)
			panic(recovered)
		}
		if err != nil {
			w.failed.Store(true)
		}
	}()
	if flusher, ok := w.ResponseWriter.(interface{ FlushError() error }); ok {
		return flusher.FlushError()
	}
	// Do not unwrap and bypass middleware flushes (which may buffer/compress).
	// Gin's plain Flush cannot expose errors discarded by its underlying writer.
	w.ResponseWriter.Flush()
	return nil
}

func (w *groupHealthWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *groupHealthWriter) beginAttempt() {
	// An earlier attempt's body is not evidence that this attempt wrote one.
	// A failed downstream write remains a failure for the whole response.
	w.wrote.Store(false)
}

func (w *groupHealthWriter) delivered() bool {
	return w != nil && w.wrote.Load() && !w.failed.Load() && w.Status() < http.StatusBadRequest
}
