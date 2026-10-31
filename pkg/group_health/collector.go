// Package grouphealth records user-facing group outcomes independently of the
// legacy first-packet performance metrics. Data starts at rollout; it is never
// backfilled from the legacy table.
package grouphealth

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/google/uuid"
)

const (
	BucketSeconds = int64(3600)
	FlushInterval = 5 * time.Second
	maxHotBuckets = 8192
)

// Outcome records only what the relay can establish. A disconnected client is
// an unknown failure, not evidence of an intentional stop, so it stays counted.
type Outcome string

const (
	OutcomeSuccess        Outcome = "success"
	OutcomeServiceFailure Outcome = "service_failure"
	OutcomeUnknownFailure Outcome = "unknown_failure"
	OutcomeUserError      Outcome = "user_error"
	OutcomeClientCanceled Outcome = "client_canceled"
)

type Sample struct {
	Group, Model string
	Probe        bool
	Outcome      Outcome
	TTFTMs       *int64
	CompletionMs *int64
	CacheUsage   *relaycommon.GroupHealthCacheUsage
	TokenUsage   *relaycommon.GroupHealthTokenUsage
	GenerationMs *int64
	At           time.Time
}

type bucketKey struct {
	group, model string
	ts           int64
}

type collector struct {
	mu            sync.Mutex
	flushMu       sync.Mutex
	buckets       map[bucketKey]model.GroupHealthMetric
	persisted     map[bucketKey]int64
	dropped       atomic.Int64
	lastDrop      atomic.Int64
	persistFailed atomic.Bool
	save          func(context.Context, []model.GroupHealthMetric) error
}

func newCollector(save func(context.Context, []model.GroupHealthMetric) error) *collector {
	return &collector{buckets: make(map[bucketKey]model.GroupHealthMetric), persisted: make(map[bucketKey]int64), save: save}
}

var defaultCollector = newCollector(model.SaveGroupHealthSnapshots)
var startOnce sync.Once

func Record(sample Sample) {
	startOnce.Do(func() { go run() })
	defaultCollector.record(sample)
}

func (c *collector) record(sample Sample) {
	if sample.Group == "" || sample.Group == "auto" || sample.Model == "" {
		return
	}
	// Do not allow one oversized configured name to poison every snapshot in
	// a MySQL/PostgreSQL batch. Keep the same bounds as the query endpoint.
	if len(sample.Group) > 64 || len(sample.Model) > 255 {
		c.markDropped("name exceeds storage limit")
		return
	}
	if sample.At.IsZero() {
		sample.At = time.Now()
	}
	key := bucketKey{group: sample.Group, model: sample.Model, ts: sample.At.Unix() / BucketSeconds * BucketSeconds}
	c.mu.Lock()
	defer c.mu.Unlock()
	row, exists := c.buckets[key]
	if !exists {
		if len(c.buckets) >= maxHotBuckets {
			c.markDropped("collector capacity exceeded")
			return
		}
		row = model.GroupHealthMetric{ID: uuid.NewString(), GroupName: sample.Group, ModelName: sample.Model, BucketTs: key.ts}
	}
	if sample.Probe {
		row.ProbeCount++
		if sample.Outcome == OutcomeSuccess {
			row.ProbeSuccessCount++
		}
	} else if sample.Outcome == OutcomeUserError {
		row.ExcludedUserErrorCount++
	} else {
		row.RequestCount++
		switch sample.Outcome {
		case OutcomeServiceFailure:
			row.ServiceFailureCount++
		case OutcomeClientCanceled:
			row.ClientCanceledCount++
		}
	}
	// Unknown or absent outcome values remain unknown failures. Only explicit
	// success can contribute successful latency or cache measurements.
	if !sample.Probe && sample.Outcome == OutcomeSuccess {
		row.SuccessCount++
		if usage := sample.TokenUsage; usage != nil && usage.InputTokens > 0 && usage.OutputTokens >= 0 {
			row.InputTokens += usage.InputTokens
			row.OutputTokens += usage.OutputTokens
			row.UsageSampleCount++
			if sample.GenerationMs != nil && *sample.GenerationMs > 0 && usage.OutputTokens > 0 {
				row.ThroughputOutputTokens += usage.OutputTokens
				row.GenerationSumMs += *sample.GenerationMs
				row.ThroughputSampleCount++
			}
		}
		if usage := sample.CacheUsage; usage != nil && usage.InputTokens > 0 && usage.ReadTokens >= 0 && usage.ReadTokens <= usage.InputTokens {
			row.CacheReadTokens += usage.ReadTokens
			row.CacheInputTokens += usage.InputTokens
			row.CacheSampleCount++
		}
		if sample.TTFTMs != nil && *sample.TTFTMs >= 0 {
			row.TTFTCount++
			row.TTFTSumMs += *sample.TTFTMs
		}
		if sample.CompletionMs != nil && *sample.CompletionMs >= 0 {
			row.CompletionCount++
			row.CompletionSumMs += *sample.CompletionMs
		}
	}
	row.UpdatedAt = sample.At.Unix()
	c.buckets[key] = row
}

func observedCount(row model.GroupHealthMetric) int64 {
	return row.RequestCount + row.ExcludedUserErrorCount
}

func snapshotCount(row model.GroupHealthMetric) int64 {
	return observedCount(row) + row.ProbeCount
}

func (c *collector) markDropped(reason string) {
	count := c.dropped.Add(1)
	c.lastDrop.Store(time.Now().Unix())
	if count == 1 || count%1000 == 0 {
		common.SysError(fmt.Sprintf("group health %s: dropped_samples=%d", reason, count))
	}
}

func (c *collector) flush(ctx context.Context, now time.Time) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	c.mu.Lock()
	rows := make([]model.GroupHealthMetric, 0, len(c.buckets))
	for key, row := range c.buckets {
		if c.persisted[key] != snapshotCount(row) {
			rows = append(rows, row)
		}
	}
	c.mu.Unlock()
	if err := c.save(ctx, rows); err != nil {
		c.persistFailed.Store(true)
		return err
	}
	c.persistFailed.Store(false)
	currentBucket := now.Unix() / BucketSeconds * BucketSeconds
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, saved := range rows {
		key := bucketKey{group: saved.GroupName, model: saved.ModelName, ts: saved.BucketTs}
		c.persisted[key] = snapshotCount(saved)
	}
	for key, current := range c.buckets {
		// Keep current-hour totals for stable idempotent overwrites. Retire only
		// fully saved old buckets; a late concurrent sample keeps the same ID.
		if key.ts < currentBucket && snapshotCount(current) == c.persisted[key] {
			delete(c.buckets, key)
			delete(c.persisted, key)
		}
	}
	return nil
}

// Flush supports graceful shutdown. An abrupt process exit may lose pending
// samples (normally the last five seconds, longer during a DB outage); the API
// only serves persisted snapshots and never
// merges hot counters with DB rows (which would double-count).
func Flush(ctx context.Context) error {
	return defaultCollector.flush(ctx, time.Now())
}

func CollectionIncomplete(now time.Time, windowHours ...int) bool {
	hours := 24
	if len(windowHours) > 0 && windowHours[0] == 72 {
		hours = 72
	}
	return defaultCollector.persistFailed.Load() || defaultCollector.lastDrop.Load() > now.Add(-time.Duration(hours)*time.Hour).Unix()
}

func run() {
	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()
	lastCleanup := time.Time{}
	for now := range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		if err := defaultCollector.flush(ctx, now); err != nil {
			common.SysError("failed to persist group health metrics: " + err.Error())
		}
		cancel()
		if now.Sub(lastCleanup) >= time.Hour {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			if err := model.DeleteExpiredGroupHealthMetrics(ctx, now); err != nil {
				common.SysError("failed to clean group health metrics: " + err.Error())
			} else {
				lastCleanup = now
			}
			cancel()
		}
	}
}
