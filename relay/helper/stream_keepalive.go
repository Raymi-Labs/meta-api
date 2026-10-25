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
	evidence    *keepAliveEvidence
}

// Only this wrapper's successful pings count as proof. The same evidence is
// handed to a replacement wrapper after its predecessor has been joined.
type keepAliveEvidence struct {
	unsafe bool
	ping   bool
	size   int
}

const requestKeepAliveWriterKey = "request_keepalive_writer"

func requestKeepAliveWriter(c *gin.Context) *streamKeepAliveWriter {
	value, _ := c.Get(requestKeepAliveWriterKey)
	writer, _ := value.(*streamKeepAliveWriter)
	return writer
}

func (w *streamKeepAliveWriter) ping() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ResponseWriter.Size() != w.evidence.size || w.ResponseWriter.Status() != http.StatusOK || w.format != types.RelayFormatClaude {
		w.evidence.unsafe = true
	}
	var err error
	if w.format == types.RelayFormatClaude {
		err = ClaudePingData(w.pingContext)
	} else {
		err = PingData(w.pingContext)
	}
	if err != nil {
		w.evidence.unsafe = true
	} else {
		w.evidence.ping = true
	}
	w.evidence.size = w.ResponseWriter.Size()
	return err
}

func (w *streamKeepAliveWriter) stop()                       { w.cancel(); <-w.done }
func (w *streamKeepAliveWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Header returns a mutable map. Join the pinger before handing it to the caller,
// since its first Write may still be committing those same headers. Pings use
// the original writer directly and therefore never wait on themselves.
func (w *streamKeepAliveWriter) Header() http.Header { w.stop(); return w.ResponseWriter.Header() }
func (w *streamKeepAliveWriter) Write(data []byte) (int, error) {
	w.stop()
	w.evidence.unsafe = true
	return w.ResponseWriter.Write(data)
}
func (w *streamKeepAliveWriter) WriteString(data string) (int, error) {
	w.stop()
	w.evidence.unsafe = true
	return w.ResponseWriter.WriteString(data)
}
func (w *streamKeepAliveWriter) WriteHeader(code int) {
	w.stop()
	w.evidence.unsafe = true
	w.ResponseWriter.WriteHeader(code)
}
func (w *streamKeepAliveWriter) WriteHeaderNow() {
	w.stop()
	w.evidence.unsafe = true
	w.ResponseWriter.WriteHeaderNow()
}
func (w *streamKeepAliveWriter) Flush() { _ = w.FlushError() }
func (w *streamKeepAliveWriter) FlushError() error {
	w.stop()
	w.evidence.unsafe = true
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
	} else if writer := requestKeepAliveWriter(c); writer != nil {
		writer.stop()
	}
}

// CanAdjustClaudeRequestBeforeOutput joins the pinger and proves that a response
// is uncommitted or contains only successfully written/flushed Claude pings.
// This is deliberately not a general retry predicate. Callers must still prove
// a specific upstream refusal and an eligible, bounded request adjustment.
func CanAdjustClaudeRequestBeforeOutput(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Context().Err() != nil || c.Writer == nil {
		return false
	}
	w, ok := c.Writer.(*streamKeepAliveWriter)
	if !ok {
		if previous := requestKeepAliveWriter(c); previous != nil {
			previous.stop()
			// An unknown replacement cannot inherit positive proof, and must
			// not erase a failed zero-byte ping whose writer is uncommitted.
			previous.evidence.unsafe = true
			return false
		}
		return !c.Writer.Written()
	}
	w.stop()
	return c.Request.Context().Err() == nil && !w.evidence.unsafe &&
		(!w.ResponseWriter.Written() || w.format == types.RelayFormatClaude && w.evidence.ping) &&
		w.ResponseWriter.Status() == http.StatusOK &&
		w.ResponseWriter.Size() == w.evidence.size
}

// StartStreamKeepAlive covers preparation and retry gaps. Nested HTTP attempts
// borrow an active request heartbeat, so returning from one attempt cannot stop
// it. The first heartbeat waits one interval, preserving ordinary JSON errors
// and headers for requests that finish promptly.
func StartStreamKeepAlive(c *gin.Context, interval time.Duration, format types.RelayFormat) func() {
	if c == nil || c.Writer == nil || c.Request == nil || c.Request.Context().Err() != nil {
		return func() {}
	}
	var evidence *keepAliveEvidence
	if previous, ok := c.Writer.(*streamKeepAliveWriter); ok {
		select {
		case <-previous.done:
			evidence = previous.evidence
			c.Writer = previous.ResponseWriter
		default:
			return func() {}
		}
	} else if previous := requestKeepAliveWriter(c); previous != nil {
		previous.stop()
		evidence = previous.evidence
		// Preserve negative evidence for the whole request, even if another
		// component replaced the writer. Unknown wrappers are never proof.
		evidence.unsafe = true
	}
	if evidence == nil {
		evidence = &keepAliveEvidence{unsafe: c.Writer.Written(), size: c.Writer.Size()}
	}
	SetEventStreamHeaders(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Minute)
	pingContext := c.Copy()
	pingContext.Writer = c.Writer
	w := &streamKeepAliveWriter{ResponseWriter: c.Writer, cancel: cancel, done: make(chan struct{}), pingContext: pingContext, format: format, evidence: evidence}
	c.Writer = w
	c.Set(requestKeepAliveWriterKey, w)
	go func() {
		defer close(w.done)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				w.evidence.unsafe = true
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
