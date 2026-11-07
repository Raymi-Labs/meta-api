// Package routeinsight measures every upstream attempt per channel and records
// what a health/cost-aware router would have chosen ("shadow decisions"). It
// is observation only: nothing here changes channel selection or billing.
package routeinsight

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/google/uuid"
)

const (
	AttemptBucketSeconds = int64(60)
	maxHotAttemptBuckets = 16384
)

type Outcome string

const (
	OutcomeSuccess          Outcome = "success"
	OutcomeServiceFailure   Outcome = "service_failure"
	OutcomeRateLimited      Outcome = "rate_limited"
	OutcomeCapacityRejected Outcome = "capacity_rejected"
	OutcomeUnknownFailure   Outcome = "unknown_failure"
	OutcomeUserError        Outcome = "user_error"
	OutcomeClientCanceled   Outcome = "client_canceled"
	// OutcomeLocalDelivery is a proven local write failure toward the client;
	// it is not evidence about the upstream channel.
	OutcomeLocalDelivery Outcome = "local_delivery"
)

// IsUpstreamFailure reports outcomes with evidence against the upstream route,
// including unknown outcomes (nothing delivered completely without a
// classified cause). User errors, client cancellations, local delivery
// failures and pre-execution capacity refusals never penalize a channel.
func (o Outcome) IsUpstreamFailure() bool {
	return o == OutcomeServiceFailure || o == OutcomeRateLimited || o == OutcomeUnknownFailure
}

type AttemptInput struct {
	Info              *relaycommon.RelayInfo
	Err               *types.NewAPIError
	Delivered         bool
	DownstreamWrite   bool
	CapacityRejection bool
}

// ClassifyAttempt mirrors the group health rules for one attempt, adding the
// rate-limit and pre-execution capacity refusal distinctions routing needs.
func ClassifyAttempt(in AttemptInput) Outcome {
	switch {
	case in.DownstreamWrite:
		return OutcomeLocalDelivery
	case in.CapacityRejection:
		return OutcomeCapacityRejected
	case in.Err != nil:
		if in.Err.StatusCode == http.StatusTooManyRequests || in.Err.UpstreamStatusCode == http.StatusTooManyRequests {
			if outcome := grouphealth.OutcomeForError(in.Err); outcome == grouphealth.OutcomeServiceFailure {
				return OutcomeRateLimited
			}
		}
		return Outcome(grouphealth.OutcomeForError(in.Err))
	}
	if !in.Delivered {
		return OutcomeUnknownFailure
	}
	if in.Info != nil && !in.Info.StreamStatus.IsSuccessful() {
		if in.Info.StreamStatus.EndReason == relaycommon.StreamEndReasonTimeout {
			return OutcomeServiceFailure
		}
		if in.Info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone {
			return OutcomeClientCanceled
		}
		return OutcomeUnknownFailure
	}
	return OutcomeSuccess
}

type Attempt struct {
	ChannelID  int
	Group      string
	Model      string
	Outcome    Outcome
	TTFTMs     *int64
	DurationMs *int64
	At         time.Time
}

type attemptKey struct {
	channelID    int
	group, model string
	ts           int64
}

type attemptRow struct {
	row      model.RouteAttemptMetric
	ttft     Histogram
	duration Histogram
}

type attemptCollector struct {
	mu        sync.Mutex
	flushMu   sync.Mutex
	buckets   map[attemptKey]*attemptRow
	persisted map[attemptKey]int64
	dropped   atomic.Int64
	save      func(context.Context, []model.RouteAttemptMetric) error
}

func newAttemptCollector(save func(context.Context, []model.RouteAttemptMetric) error) *attemptCollector {
	return &attemptCollector{buckets: map[attemptKey]*attemptRow{}, persisted: map[attemptKey]int64{}, save: save}
}

func (c *attemptCollector) record(a Attempt) {
	if a.ChannelID <= 0 || a.Model == "" || a.Group == "" || a.Group == "auto" || len(a.Group) > 64 || len(a.Model) > 255 {
		return
	}
	if a.At.IsZero() {
		a.At = time.Now()
	}
	key := attemptKey{channelID: a.ChannelID, group: a.Group, model: a.Model, ts: a.At.Unix() / AttemptBucketSeconds * AttemptBucketSeconds}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.buckets[key]
	if entry == nil {
		if len(c.buckets) >= maxHotAttemptBuckets {
			if count := c.dropped.Add(1); count == 1 || count%1000 == 0 {
				common.SysError(fmt.Sprintf("route insight attempt collector full: dropped=%d", count))
			}
			return
		}
		entry = &attemptRow{
			row:      model.RouteAttemptMetric{ID: uuid.NewString(), BucketTs: key.ts, ChannelID: a.ChannelID, GroupName: a.Group, ModelName: a.Model},
			ttft:     newHistogram(),
			duration: newHistogram(),
		}
		c.buckets[key] = entry
	}
	row := &entry.row
	row.AttemptCount++
	switch a.Outcome {
	case OutcomeSuccess:
		row.SuccessCount++
		if a.TTFTMs != nil {
			entry.ttft.Observe(*a.TTFTMs)
		}
		if a.DurationMs != nil {
			entry.duration.Observe(*a.DurationMs)
		}
	case OutcomeServiceFailure:
		row.ServiceFailureCount++
	case OutcomeRateLimited:
		row.RateLimitedCount++
	case OutcomeCapacityRejected:
		row.CapacityRejectedCount++
	case OutcomeUserError:
		row.UserErrorCount++
	case OutcomeClientCanceled:
		row.ClientCanceledCount++
	case OutcomeLocalDelivery:
		row.LocalDeliveryCount++
	default:
		row.UnknownFailureCount++
	}
	row.UpdatedAt = a.At.Unix()
}

func (c *attemptCollector) flush(ctx context.Context, now time.Time) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	c.mu.Lock()
	rows := make([]model.RouteAttemptMetric, 0, len(c.buckets))
	for key, entry := range c.buckets {
		if c.persisted[key] != entry.row.AttemptCount {
			row := entry.row
			row.TTFTHistogram = entry.ttft.encode()
			row.DurationHistogram = entry.duration.encode()
			rows = append(rows, row)
		}
	}
	c.mu.Unlock()
	if err := c.save(ctx, rows); err != nil {
		return err
	}
	current := now.Unix() / AttemptBucketSeconds * AttemptBucketSeconds
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, saved := range rows {
		c.persisted[attemptKey{channelID: saved.ChannelID, group: saved.GroupName, model: saved.ModelName, ts: saved.BucketTs}] = saved.AttemptCount
	}
	for key, entry := range c.buckets {
		if key.ts < current && c.persisted[key] == entry.row.AttemptCount {
			delete(c.buckets, key)
			delete(c.persisted, key)
		}
	}
	return nil
}
