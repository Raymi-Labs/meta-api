package grouphealth

import (
	"net/http"
	"strings"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
)

// Request folds retries within one concrete group, not across fallback groups.
// It is owned by the relay request goroutine and flushed exactly once on return.
type Request struct {
	model        string
	clientStream bool
	groups       map[string]Sample
}

func NewRequest(modelName string, clientStream bool) *Request {
	return &Request{model: modelName, clientStream: clientStream, groups: make(map[string]Sample)}
}

func (r *Request) Failure(group string) {
	r.observeGroup(group, OutcomeServiceFailure)
}

// OutcomeForError keeps uncertain failures (including downstream disconnects)
// in the availability denominator. A disconnect does not prove user intent.
func OutcomeForError(err *types.NewAPIError) Outcome {
	if err != nil && err.GetErrorType() == types.ErrorTypeNewAPIError && err.UpstreamStatusCode == 0 {
		if (err.GetErrorCode() == types.ErrorCodeInvalidRequest && err.StatusCode == http.StatusBadRequest) ||
			(err.GetErrorCode() == types.ErrorCodeAccessDenied && err.StatusCode == http.StatusForbidden) {
			return OutcomeUserError
		}
	}
	switch types.ClassifyFailure(err) {
	case types.FailureUserRequest:
		return OutcomeUserError
	case types.FailureClientCanceled:
		return OutcomeClientCanceled
	case types.FailureService:
		return OutcomeServiceFailure
	default:
		return OutcomeUnknownFailure
	}
}

// ExcludeUserError is reserved for trusted local policy rejection sites. An
// upstream status or provider-supplied error code cannot call this path.
func (r *Request) ExcludeUserError(group string) {
	r.observeGroup(group, OutcomeUserError)
}

func (r *Request) ObserveFailure(group string, err *types.NewAPIError) {
	r.observeGroup(group, OutcomeForError(err))
}

// ObserveUnrecordedFailure covers failures before an upstream attempt without
// rewriting the result already attributed to this group inside the retry loop.
func (r *Request) ObserveUnrecordedFailure(group string, err *types.NewAPIError) {
	if r == nil || err == nil {
		return
	}
	if _, exists := r.groups[group]; !exists {
		r.ObserveFailure(group, err)
	}
}

func (r *Request) observeGroup(group string, outcome Outcome) {
	if r == nil || group == "" || group == "auto" {
		return
	}
	r.store(Sample{Group: group, Model: r.model, Outcome: outcome})
}

// delivered describes the observed response writes, not the current request
// context: a client may disconnect after a response has already completed.
func (r *Request) ObserveResult(info *relaycommon.RelayInfo, err *types.NewAPIError, delivered bool, now time.Time) {
	if err != nil {
		r.observe(info, OutcomeForError(err), now)
		return
	}
	outcome := OutcomeUnknownFailure
	if delivered {
		outcome = OutcomeSuccess
	}
	r.observe(info, outcome, now)
}

func (r *Request) Observe(info *relaycommon.RelayInfo, success bool, now time.Time) {
	outcome := OutcomeUnknownFailure
	if success {
		outcome = OutcomeSuccess
	}
	r.observe(info, outcome, now)
}

func (r *Request) observe(info *relaycommon.RelayInfo, outcome Outcome, now time.Time) {
	if r == nil || info == nil || info.UsingGroup == "" || info.UsingGroup == "auto" {
		return
	}
	if outcome == OutcomeSuccess && !info.StreamStatus.IsSuccessful() {
		// Stream errors also include downstream write failures, so an incomplete
		// protocol alone does not establish service-side responsibility.
		outcome = OutcomeUnknownFailure
		if info.StreamStatus.EndReason == relaycommon.StreamEndReasonTimeout {
			outcome = OutcomeServiceFailure
		}
	}
	if outcome == OutcomeUnknownFailure && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone {
		outcome = OutcomeClientCanceled
	}
	success := outcome == OutcomeSuccess
	sample := Sample{Group: info.UsingGroup, Model: r.model, Outcome: outcome}
	if success && info.GroupHealthCacheUsage != nil {
		usage := *info.GroupHealthCacheUsage
		sample.CacheUsage = &usage
	}
	if success && info.GroupHealthTokenUsage != nil {
		usage := *info.GroupHealthTokenUsage
		sample.TokenUsage = &usage
	}
	isImage := info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits
	if success && !info.StartTime.IsZero() {
		if r.clientStream && !isImage && !info.GroupHealthFirstOutputTime.IsZero() &&
			!info.GroupHealthFirstOutputTime.Before(info.StartTime) && !info.GroupHealthFirstOutputTime.After(now) {
			value := info.GroupHealthFirstOutputTime.Sub(info.StartTime).Milliseconds()
			sample.TTFTMs = &value
			if sample.TokenUsage != nil && sample.TokenUsage.OutputTokens > 0 {
				// Usage is captured after the adapter has finished the response,
				// before settlement. Do not count billing/database latency as
				// generation time. Missing evidence remains unmeasured.
				completedAt := info.GroupHealthUsageObservedAt
				generationMs := completedAt.Sub(info.GroupHealthFirstOutputTime).Milliseconds()
				if !completedAt.After(now) && generationMs > 0 {
					sample.GenerationMs = &generationMs
				}
			}
		}
		if isImage && !info.IsStream && !now.Before(info.StartTime) {
			value := now.Sub(info.StartTime).Milliseconds()
			sample.CompletionMs = &value
		}
	}
	r.store(sample)
}

func (r *Request) store(sample Sample) {
	previous, exists := r.groups[sample.Group]
	if exists && (previous.Outcome == OutcomeSuccess ||
		(previous.Outcome == OutcomeServiceFailure && sample.Outcome != OutcomeSuccess) ||
		(sample.Outcome == OutcomeUserError && previous.Outcome != OutcomeUserError)) {
		// A later cancellation cannot undo delivered success; an excluded request
		// error or uncertain interruption cannot erase an established service
		// failure from an earlier same-group attempt. A successful retry can.
		return
	}
	r.groups[sample.Group] = sample
}

func (r *Request) Samples(now time.Time) []Sample {
	if r == nil {
		return nil
	}
	samples := make([]Sample, 0, len(r.groups))
	for _, sample := range r.groups {
		sample.At = now
		samples = append(samples, sample)
	}
	return samples
}

func (r *Request) Finish() {
	for _, sample := range r.Samples(time.Now()) {
		Record(sample)
	}
}

// SupportsRequest excludes asynchronous task submission/polling and realtime
// sessions. Their HTTP acceptance is not evidence of completed generation.
func SupportsRequest(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	switch path {
	case "/v1/chat/completions", "/v1/completions", "/v1/messages", "/v1/responses", "/v1/responses/compact",
		"/v1/images/generations", "/v1/images/edits", "/v1/edits", "/pg/chat/completions",
		"/v1/embeddings", "/v1/moderations", "/v1/rerank", "/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations":
		return true
	}
	return (strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/v1/models/")) &&
		(strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent") || strings.HasSuffix(path, ":embedContent") || strings.HasSuffix(path, ":batchEmbedContents"))
}

// ResetAttempt prevents the previous failed upstream's stream state and first
// output timestamp from contaminating a successful retry. Request start remains
// unchanged, so successful latency includes selection, queueing and retries.
func ResetAttempt(info *relaycommon.RelayInfo) {
	info.GroupHealthFirstOutputTime = time.Time{}
	info.StreamStatus = nil
	info.GroupHealthCacheUsage = nil
	info.GroupHealthTokenUsage = nil
	info.GroupHealthUsageObservedAt = time.Time{}
}
