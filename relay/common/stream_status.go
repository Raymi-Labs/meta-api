package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type StreamEndReason string

type ResponseOutcome string

const (
	ResponseOutcomeCompleted  ResponseOutcome = "completed"
	ResponseOutcomeFailed     ResponseOutcome = "failed"
	ResponseOutcomeIncomplete ResponseOutcome = "incomplete"
	ResponseOutcomeCancelled  ResponseOutcome = "cancelled"
)

const (
	StreamEndReasonNone        StreamEndReason = ""
	StreamEndReasonDone        StreamEndReason = "done"
	StreamEndReasonTimeout     StreamEndReason = "timeout"
	StreamEndReasonClientGone  StreamEndReason = "client_gone"
	StreamEndReasonScannerErr  StreamEndReason = "scanner_error"
	StreamEndReasonHandlerStop StreamEndReason = "handler_stop"
	StreamEndReasonEOF         StreamEndReason = "eof"
	StreamEndReasonPanic       StreamEndReason = "panic"
	StreamEndReasonPingFail    StreamEndReason = "ping_fail"
)

const maxStreamErrorEntries = 20

type StreamErrorEntry struct {
	Message   string
	Timestamp time.Time
}

type StreamStatus struct {
	EndReason StreamEndReason
	EndError  error
	endOnce   sync.Once

	mu                  sync.Mutex
	Errors              []StreamErrorEntry
	ErrorCount          int
	response            ResponseOutcome
	expectsTerminal     bool
	deliveredCompletion bool
}

// MarkDeliveredCompletion is only called after a valid protocol completion
// has been written and flushed successfully. A later cancellation cannot undo
// that delivery; receiving a completion object alone does not qualify.
func (s *StreamStatus) MarkDeliveredCompletion() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrorCount == 0 && s.EndError == nil && s.response != ResponseOutcomeFailed {
		s.response = ResponseOutcomeCompleted
		s.deliveredCompletion = true
	}
}

func (s *StreamStatus) HasDeliveredCompletion() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deliveredCompletion && s.response == ResponseOutcomeCompleted && s.ErrorCount == 0 && s.EndError == nil
}

func NewStreamStatus() *StreamStatus {
	return &StreamStatus{}
}

func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	if s == nil {
		return
	}
	s.endOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.EndReason = reason
		s.EndError = err
	})
}

func (s *StreamStatus) RequireTerminal() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expectsTerminal = true
}

// Keep the first terminal result; an explicit failure always takes precedence.
func (s *StreamStatus) MarkOutcome(outcome ResponseOutcome) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.response == "" || outcome == ResponseOutcomeFailed {
		s.response = outcome
	}
}

// IsSuccessful separates protocol success from HTTP 200 / transport EOF.
func (s *StreamStatus) IsSuccessful() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrorCount > 0 || s.EndError != nil {
		return false
	}
	if s.response != "" {
		return s.response == ResponseOutcomeCompleted && s.EndReason != StreamEndReasonClientGone
	}
	if s.expectsTerminal {
		return false
	}
	return s.EndReason == StreamEndReasonDone || s.EndReason == StreamEndReasonEOF || s.EndReason == StreamEndReasonHandlerStop
}

func (s *StreamStatus) RecordError(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrorEntries {
		s.Errors = append(s.Errors, StreamErrorEntry{
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
}

func (s *StreamStatus) HasErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

func (s *StreamStatus) TotalErrorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount
}

func (s *StreamStatus) IsNormalEnd() bool {
	if s == nil {
		return true
	}
	return s.EndReason == StreamEndReasonDone ||
		s.EndReason == StreamEndReasonEOF ||
		s.EndReason == StreamEndReasonHandlerStop
}

func (s *StreamStatus) Summary() string {
	if s == nil {
		return "StreamStatus<nil>"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "reason=%s", s.EndReason)
	if s.EndError != nil {
		fmt.Fprintf(b, " end_error=%q", s.EndError.Error())
	}
	s.mu.Lock()
	if s.ErrorCount > 0 {
		fmt.Fprintf(b, " soft_errors=%d", s.ErrorCount)
	}
	s.mu.Unlock()
	return b.String()
}
