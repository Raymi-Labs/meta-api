package helper

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type streamKeepAliveWriter struct {
	gin.ResponseWriter
	mu          sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	pingContext *gin.Context
	format      types.RelayFormat
}

func (w *streamKeepAliveWriter) ping() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.format == types.RelayFormatClaude {
		return ClaudePingData(w.pingContext)
	}
	return PingData(w.pingContext)
}

func (w *streamKeepAliveWriter) stop()                       { w.cancel(); <-w.done }
func (w *streamKeepAliveWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Header returns a mutable map. Join the pinger before handing it to the caller,
// since its first Write may still be committing those same headers. Pings use
// the original writer directly and therefore never wait on themselves.
func (w *streamKeepAliveWriter) Header() http.Header { w.stop(); return w.ResponseWriter.Header() }
func (w *streamKeepAliveWriter) Write(data []byte) (int, error) {
	w.stop()
	return w.ResponseWriter.Write(data)
}
func (w *streamKeepAliveWriter) WriteString(data string) (int, error) {
	w.stop()
	return w.ResponseWriter.WriteString(data)
}
func (w *streamKeepAliveWriter) WriteHeader(code int) { w.stop(); w.ResponseWriter.WriteHeader(code) }
func (w *streamKeepAliveWriter) WriteHeaderNow()      { w.stop(); w.ResponseWriter.WriteHeaderNow() }
func (w *streamKeepAliveWriter) Flush()               { _ = w.FlushError() }
func (w *streamKeepAliveWriter) FlushError() error {
	w.stop()
	return flushStreamWriter(w.ResponseWriter)
}
func (w *streamKeepAliveWriter) Status() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ResponseWriter.Status()
}
func (w *streamKeepAliveWriter) Size() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ResponseWriter.Size()
}
func (w *streamKeepAliveWriter) Written() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ResponseWriter.Written()
}

// StopStreamKeepAlive joins request-level heartbeats before any protocol writer
// changes headers or starts its own stream. It is safe to call more than once.
func StopStreamKeepAlive(c *gin.Context) {
	if c == nil {
		return
	}
	if writer, ok := c.Writer.(*streamKeepAliveWriter); ok {
		writer.stop()
	}
}

// StartStreamKeepAlive covers preparation and retry gaps. Nested HTTP attempts
// borrow an active request heartbeat, so returning from one attempt cannot stop
// it. The first heartbeat waits one interval, preserving ordinary JSON errors
// and headers for requests that finish promptly.
func StartStreamKeepAlive(c *gin.Context, interval time.Duration, format types.RelayFormat) func() {
	if c == nil || c.Writer == nil || c.Request == nil || c.Request.Context().Err() != nil {
		return func() {}
	}
	if previous, ok := c.Writer.(*streamKeepAliveWriter); ok {
		select {
		case <-previous.done:
			c.Writer = previous.ResponseWriter
		default:
			return func() {}
		}
	}
	SetEventStreamHeaders(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Minute)
	pingContext := c.Copy()
	pingContext.Writer = c.Writer
	w := &streamKeepAliveWriter{ResponseWriter: c.Writer, cancel: cancel, done: make(chan struct{}), pingContext: pingContext, format: format}
	c.Writer = w
	go func() {
		defer close(w.done)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.LogError(pingContext, "request keepalive panic")
			}
		}()
		ticker := time.NewTicker(NormalizeStreamPingInterval(interval))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				if err := w.ping(); err != nil {
					return
				}
			}
		}
	}()
	return w.stop
}
