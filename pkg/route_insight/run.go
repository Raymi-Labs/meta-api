package routeinsight

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	FlushInterval    = 5 * time.Second
	SnapshotInterval = 15 * time.Second
	ShortWindow      = 5 * time.Minute
	LongWindow       = 15 * time.Minute
	// A snapshot older than this is treated as no data, never as healthy.
	staleAfter = 2 * time.Minute
)

var (
	enabled         = common.GetEnvOrDefaultBool("ROUTE_INSIGHT_ENABLED", true)
	attempts        = newAttemptCollector(model.SaveRouteAttemptSnapshots)
	shadows         = newShadowCollector(model.SaveRouteShadowSnapshots)
	startOnce       sync.Once
	snapshotMu      sync.RWMutex
	snapshot        = map[routeKey]RouteHealth{}
	snapshotAt      atomic.Int64
	snapshotFailing atomic.Bool
)

type routeKey struct {
	channelID int
	model     string
}

// Enabled covers both attempt telemetry and shadow decisions; neither affects
// scheduling, so the default is on. Set ROUTE_INSIGHT_ENABLED=false to stop.
func Enabled() bool {
	return enabled
}

func start() {
	startOnce.Do(func() { go run() })
}

func RecordAttempt(a Attempt) {
	if !enabled {
		return
	}
	start()
	attempts.record(a)
}

func RecordShadow(o ShadowObservation) {
	if !enabled {
		return
	}
	start()
	shadows.observe(o)
}

func FinishRequest(modelName string, delivered bool, attemptCount int) {
	if !enabled {
		return
	}
	start()
	shadows.finishRequest(modelName, delivered, attemptCount, time.Now())
}

// Health reads the shared background snapshot (all replicas, via the DB).
// The request path never queries the database.
func Health(channelID int, modelName string) RouteHealth {
	if at := snapshotAt.Load(); at == 0 || time.Now().Unix()-at > int64(staleAfter/time.Second) {
		return RouteHealth{Status: StatusUnknown, Reason: "stale_snapshot"}
	}
	snapshotMu.RLock()
	defer snapshotMu.RUnlock()
	if h, ok := snapshot[routeKey{channelID: channelID, model: modelName}]; ok {
		return h
	}
	return RouteHealth{Status: StatusUnknown, Reason: "no_samples"}
}

// SnapshotState reports when the health snapshot was last rebuilt and whether
// the latest rebuild failed (stale data then stays in use as unknown-safe).
func SnapshotState() (int64, bool) {
	return snapshotAt.Load(), snapshotFailing.Load()
}

func RecentDisagreements(modelName string) []RecentSample {
	return shadows.recentSamples(modelName)
}

func Flush(ctx context.Context) error {
	now := time.Now()
	return errors.Join(attempts.flush(ctx, now), shadows.flush(ctx, now))
}

func refreshSnapshot(ctx context.Context, now time.Time) error {
	rows, err := model.QueryRouteAttemptMetrics(ctx, now.Add(-LongWindow).Unix(), now.Unix(), "")
	if err != nil {
		return err
	}
	shortStart := now.Add(-ShortWindow).Unix()
	shortWindows := map[routeKey]*Window{}
	longWindows := map[routeKey]*Window{}
	for _, row := range rows {
		key := routeKey{channelID: row.ChannelID, model: row.ModelName}
		long := longWindows[key]
		if long == nil {
			w := newWindow()
			long = &w
			longWindows[key] = long
			s := newWindow()
			shortWindows[key] = &s
		}
		ttft, duration := decodeHistogram(row.TTFTHistogram), decodeHistogram(row.DurationHistogram)
		long.addRow(row.AttemptCount, row.SuccessCount, row.ServiceFailureCount, row.RateLimitedCount, row.CapacityRejectedCount,
			row.UnknownFailureCount, row.UserErrorCount, row.ClientCanceledCount, row.LocalDeliveryCount, ttft, duration)
		if row.BucketTs >= shortStart {
			shortWindows[key].addRow(row.AttemptCount, row.SuccessCount, row.ServiceFailureCount, row.RateLimitedCount, row.CapacityRejectedCount,
				row.UnknownFailureCount, row.UserErrorCount, row.ClientCanceledCount, row.LocalDeliveryCount, ttft, duration)
		}
	}
	next := make(map[routeKey]RouteHealth, len(longWindows))
	for key, long := range longWindows {
		next[key] = Evaluate(*shortWindows[key], *long)
	}
	snapshotMu.Lock()
	snapshot = next
	snapshotMu.Unlock()
	snapshotAt.Store(now.Unix())
	return nil
}

func run() {
	flushTicker := time.NewTicker(FlushInterval)
	snapshotTicker := time.NewTicker(SnapshotInterval)
	defer flushTicker.Stop()
	defer snapshotTicker.Stop()
	lastCleanup := time.Time{}
	for {
		select {
		case now := <-flushTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			if err := errors.Join(attempts.flush(ctx, now), shadows.flush(ctx, now)); err != nil {
				common.SysError("failed to persist route insight metrics: " + err.Error())
			}
			cancel()
			if now.Sub(lastCleanup) >= time.Hour {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := model.DeleteExpiredRouteInsightMetrics(ctx, now); err != nil {
					common.SysError("failed to clean route insight metrics: " + err.Error())
				} else {
					lastCleanup = now
				}
				cancel()
			}
		case now := <-snapshotTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := refreshSnapshot(ctx, now); err != nil {
				snapshotFailing.Store(true)
				common.SysError("failed to refresh route insight snapshot: " + err.Error())
			} else {
				snapshotFailing.Store(false)
			}
			cancel()
		}
	}
}
