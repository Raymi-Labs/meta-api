package service

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const requestCancellationObservationKey = "request_cancellation_observation"

// CancellationDiagnostics describes observed transport progress, not why the
// remote peer disconnected. Nil means this transport was not instrumented.
type CancellationDiagnostics struct {
	DurationMs              int64  `json:"duration_ms"`
	Phase                   string `json:"phase"`
	UpstreamResponseStarted *bool  `json:"upstream_response_started"`
}

type requestCancellationObservation struct {
	mu      sync.Mutex
	start   time.Time
	phase   string
	started *bool
	frozen  *CancellationDiagnostics
}

func (o *requestCancellationObservation) freeze() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.frozen == nil {
		o.frozen = &CancellationDiagnostics{DurationMs: time.Since(o.start).Milliseconds(), Phase: o.phase, UpstreamResponseStarted: o.started}
	}
}

// BeginRequestCancellationObservation installs one bounded observation per
// request. Finish must run before final logging and releases the callback.
func BeginRequestCancellationObservation(c *gin.Context, start time.Time) func() {
	o := &requestCancellationObservation{start: start, phase: "unknown"}
	c.Set(requestCancellationObservationKey, o)
	done := make(chan struct{})
	stop := context.AfterFunc(c.Request.Context(), func() { o.freeze(); close(done) })
	var finishOnce sync.Once
	return func() {
		finishOnce.Do(func() {
			if !stop() {
				<-done
			} else if c.Request.Context().Err() != nil {
				o.freeze()
			}
		})
	}
}

func cancellationObservation(c *gin.Context) *requestCancellationObservation {
	if c == nil {
		return nil
	}
	value, _ := c.Get(requestCancellationObservationKey)
	o, _ := value.(*requestCancellationObservation)
	return o
}

func (o *requestCancellationObservation) progress(ctx context.Context, phase string, started bool) {
	if o == nil {
		return
	}
	// Freeze before recording late transport activity after downstream cancellation.
	if ctx.Err() != nil {
		o.freeze()
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.frozen == nil {
		o.phase, o.started = phase, &started
	}
}

// BeginCancellationHTTPAttempt resets progress for the current attempt, so a
// previous failed channel's response cannot masquerade as the final response.
func BeginCancellationHTTPAttempt(c *gin.Context) {
	cancellationObservation(c).progress(c.Request.Context(), "waiting_headers", false)
}

func ObserveCancellationHTTPResponse(c *gin.Context, body io.ReadCloser) io.ReadCloser {
	o := cancellationObservation(c)
	if o == nil {
		return body
	}
	o.progress(c.Request.Context(), "waiting_body", false)
	if body == nil {
		return nil
	}
	return &cancellationObservedBody{ReadCloser: body, observation: o, ctx: c.Request.Context()}
}

type cancellationObservedBody struct {
	io.ReadCloser
	observation *requestCancellationObservation
	ctx         context.Context
	once        sync.Once
}

func (b *cancellationObservedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.once.Do(func() { b.observation.progress(b.ctx, "receiving_body", true) })
	}
	return n, err
}

func appendCancellationDiagnostics(c *gin.Context, err *types.NewAPIError, admin map[string]interface{}, elapsed time.Duration) {
	if types.ClassifyFailure(err) != types.FailureClientCanceled {
		return
	}
	snapshot := CancellationDiagnostics{DurationMs: elapsed.Milliseconds(), Phase: "unknown"}
	if o := cancellationObservation(c); o != nil {
		o.freeze()
		o.mu.Lock()
		snapshot = *o.frozen
		o.mu.Unlock()
	}
	admin["cancellation"] = snapshot
}
