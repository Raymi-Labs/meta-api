package routeinsight

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useTestDB(t *testing.T) {
	t.Helper()
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	require.NoError(t, db.AutoMigrate(&model.RouteAttemptMetric{}, &model.RouteShadowMetric{}))
}

func ms(v int64) *int64 { return &v }

func TestHistogramQuantileUsesConservativeBinUpperBound(t *testing.T) {
	h := newHistogram()
	require.Nil(t, h.Quantile(0.95), "no samples must stay unknown")
	for i := 0; i < 94; i++ {
		h.Observe(400)
	}
	for i := 0; i < 6; i++ {
		h.Observe(9000)
	}
	require.Equal(t, int64(500), *h.Quantile(0.5))
	require.Equal(t, int64(12000), *h.Quantile(0.95))
	h.Observe(999999)
	require.Equal(t, int64(-1), *h.Quantile(1), "overflow bin reports -1, not a fabricated bound")
	require.Equal(t, h, decodeHistogram(h.encode()))
	require.Equal(t, newHistogram(), decodeHistogram("[1,2]"), "different bin layout is ignored, not guessed")
}

func TestEvaluateRequiresEvidenceAndUsesSuccessThreshold(t *testing.T) {
	window := func(success, failure, unknown int64) Window {
		w := newWindow()
		w.Success, w.ServiceFailure, w.UnknownFailure = success, failure, unknown
		w.Attempts = success + failure + unknown
		return w
	}
	small := Evaluate(window(5, 4, 0), window(5, 4, 0), 0.9)
	require.Equal(t, StatusUnknown, small.Status, "9 samples are below the minimum sample size")

	perfect := Evaluate(window(20, 0, 0), window(20, 0, 0), 0.9)
	require.Equal(t, StatusHealthy, perfect.Status)
	require.Less(t, perfect.Score, 0.9, "the displayed score stays a conservative lower bound")

	unknownOnly := Evaluate(window(0, 0, 50), window(0, 0, 50), 0.9)
	require.Equal(t, StatusUnknown, unknownOnly.Status, "unknown outcomes are not upstream evidence")

	atThreshold := Evaluate(window(9, 1, 0), window(18, 2, 0), 0.9)
	require.Equal(t, StatusHealthy, atThreshold.Status, "exactly 90% is not below the threshold")
	below := Evaluate(window(8, 1, 0), window(17, 3, 0), 0.9)
	require.Equal(t, StatusUnhealthy, below.Status, "85% is below 90%")
	require.Equal(t, "low_success_rate", below.Reason)
	require.Equal(t, StatusHealthy, Evaluate(window(8, 1, 0), window(17, 3, 0), 0.8).Status, "the threshold is configurable")
	require.Equal(t, StatusUnhealthy, Evaluate(window(8, 1, 0), window(17, 3, 0), 0).Status, "0 falls back to the 90% default")

	burst := Evaluate(window(2, 4, 0), window(200, 4, 0), 0.9)
	require.Equal(t, StatusUnhealthy, burst.Status, "a 5-minute burst must not hide behind a good 15-minute history")
	require.Equal(t, "recent_failure_burst", burst.Reason)
}

func TestClassifyAttempt(t *testing.T) {
	rateLimited := types.NewErrorWithStatusCode(errors.New("slow down"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	require.Equal(t, OutcomeRateLimited, ClassifyAttempt(AttemptInput{Err: rateLimited}))
	upstream := types.NewErrorWithStatusCode(errors.New("bad gateway"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	require.Equal(t, OutcomeServiceFailure, ClassifyAttempt(AttemptInput{Err: upstream}))
	require.Equal(t, OutcomeLocalDelivery, ClassifyAttempt(AttemptInput{Err: upstream, DownstreamWrite: true}))
	require.Equal(t, OutcomeCapacityRejected, ClassifyAttempt(AttemptInput{Err: upstream, CapacityRejection: true}))
	require.Equal(t, OutcomeUnknownFailure, ClassifyAttempt(AttemptInput{Info: &relaycommon.RelayInfo{}}), "no delivery is not success")
	require.Equal(t, OutcomeSuccess, ClassifyAttempt(AttemptInput{Info: &relaycommon.RelayInfo{}, Delivered: true}))
	timeout := &relaycommon.RelayInfo{StreamStatus: &relaycommon.StreamStatus{EndReason: relaycommon.StreamEndReasonTimeout, ErrorCount: 1}}
	require.Equal(t, OutcomeServiceFailure, ClassifyAttempt(AttemptInput{Info: timeout, Delivered: true}))
	require.False(t, OutcomeUnknownFailure.IsUpstreamFailure())
	require.False(t, OutcomeCapacityRejected.IsUpstreamFailure(), "pre-execution refusal is tracked separately")
}

func TestDecideKeepsOrderAndSkipsUnhealthy(t *testing.T) {
	unhealthy, ok := RouteHealth{Status: StatusUnhealthy}, RouteHealth{Status: StatusHealthy}
	lookup := func(table map[int]RouteHealth) func(int) RouteHealth {
		return func(id int) RouteHealth {
			if h, found := table[id]; found {
				return h
			}
			return RouteHealth{Status: StatusUnknown}
		}
	}
	// Group a (index 0): 1 high priority, 2 and 3 low priority; group b: 4.
	candidates := []Candidate{
		{ChannelID: 1, Group: "a", GroupIndex: 0, Priority: 10},
		{ChannelID: 2, Group: "a", GroupIndex: 0, Priority: 5},
		{ChannelID: 3, Group: "a", GroupIndex: 0, Priority: 5},
		{ChannelID: 4, Group: "b", GroupIndex: 1, Priority: 10},
	}

	decision := Decide(candidates, lookup(map[int]RouteHealth{1: ok, 2: ok, 3: ok, 4: ok}))
	require.Equal(t, []int{1}, decision.Tier, "all healthy: production order and priority")
	require.Equal(t, "ordered", decision.Reason)

	decision = Decide(candidates, lookup(map[int]RouteHealth{1: unhealthy, 2: ok, 3: ok, 4: ok}))
	require.Equal(t, []int{2, 3}, decision.Tier, "skip the unhealthy top channel; weights choose within the next priority")
	require.Equal(t, "skip_unhealthy", decision.Reason)
	require.True(t, decision.includes(3))

	decision = Decide(candidates, lookup(map[int]RouteHealth{1: unhealthy, 2: unhealthy, 3: unhealthy, 4: ok}))
	require.Equal(t, []int{4}, decision.Tier, "a group with no healthy channel is skipped")
	require.Equal(t, "skip_group", decision.Reason)
	require.Equal(t, "b", decision.Group)

	decision = Decide(candidates, lookup(map[int]RouteHealth{1: unhealthy, 2: unhealthy, 3: unhealthy, 4: unhealthy}))
	require.Equal(t, []int{1}, decision.Tier, "everything unhealthy: keep the normal order")
	require.Equal(t, "all_unhealthy", decision.Reason)

	decision = Decide(candidates, lookup(map[int]RouteHealth{1: unhealthy}))
	require.Equal(t, []int{2, 3}, decision.Tier, "unknown channels are usable")

	decision = Decide(candidates, lookup(nil))
	require.Equal(t, []int{1}, decision.Tier)
	require.Equal(t, "no_data", decision.Reason)

	require.Equal(t, "no_candidates", Decide(nil, lookup(nil)).Reason)
}

func TestAttemptCollectorKeepsRetriedFailureAndMergesReplicas(t *testing.T) {
	useTestDB(t)
	ctx := context.Background()
	now := time.Now()
	replicaA := newAttemptCollector(model.SaveRouteAttemptSnapshots)
	replicaB := newAttemptCollector(model.SaveRouteAttemptSnapshots)
	// One logical request: channel 1 fails, retry on channel 2 succeeds.
	replicaA.record(Attempt{ChannelID: 1, Group: "g", Model: "m", Outcome: OutcomeServiceFailure, At: now})
	replicaA.record(Attempt{ChannelID: 2, Group: "g", Model: "m", Outcome: OutcomeSuccess, TTFTMs: ms(300), DurationMs: ms(900), At: now})
	for i := 0; i < 11; i++ {
		replicaB.record(Attempt{ChannelID: 1, Group: "g", Model: "m", Outcome: OutcomeServiceFailure, At: now})
	}
	replicaB.record(Attempt{ChannelID: 1, Group: "g", Model: "m", Outcome: OutcomeUserError, At: now})
	require.NoError(t, replicaA.flush(ctx, now))
	require.NoError(t, replicaA.flush(ctx, now), "re-flushing must not double count")
	require.NoError(t, replicaB.flush(ctx, now))

	require.NoError(t, refreshSnapshot(ctx, now.Add(time.Second)))
	snapshotAt.Store(time.Now().Unix())
	failed := Health(1, "m")
	require.Equal(t, int64(12), failed.Long.ServiceFailure, "both replicas and the retried failure are visible")
	require.Equal(t, int64(1), failed.Long.UserError)
	require.Equal(t, StatusUnhealthy, failed.Status)
	require.Equal(t, StatusUnknown, Health(2, "m").Status, "one success is not enough evidence")

	reports, err := ChannelReports(ctx, now.Add(time.Second), 60, "m")
	require.NoError(t, err)
	require.Len(t, reports, 2)
	for _, report := range reports {
		if report.ChannelID == 2 {
			require.Equal(t, int64(500), *report.TTFTP95Ms)
			require.Equal(t, int64(1000), *report.DurationP95Ms)
		}
	}

	snapshotAt.Store(time.Now().Add(-time.Hour).Unix())
	require.Equal(t, "stale_snapshot", Health(1, "m").Reason, "stale data is never trusted")
}

func TestShadowCollectorCountsAgreementAndRequests(t *testing.T) {
	useTestDB(t)
	ctx := context.Background()
	now := time.Now()
	c := newShadowCollector(model.SaveRouteShadowSnapshots)
	c.observe(ShadowObservation{Model: "m", Decision: Decision{ChannelID: 1, Group: "a", Status: StatusHealthy, Reason: "ordered", Tier: []int{1, 5}},
		ActualChannel: 5, ActualGroup: "a", ActualOutcome: OutcomeSuccess, At: now})
	c.observe(ShadowObservation{Model: "m", Decision: Decision{ChannelID: 2, Group: "b", Status: StatusHealthy, Reason: "skip_group", Tier: []int{2}},
		ActualChannel: 1, ActualGroup: "a", ActualStatus: StatusUnhealthy, ActualOutcome: OutcomeServiceFailure, At: now})
	c.finishRequest("m", true, 2, now)
	c.finishRequest("m", false, 0, now)
	require.NoError(t, c.flush(ctx, now))

	reports, err := ShadowReports(ctx, now.Add(time.Second), 24, "")
	require.NoError(t, err)
	require.Len(t, reports, 1)
	total := reports[0].Total
	require.Equal(t, int64(2), total.DecisionCount)
	require.Equal(t, int64(1), total.AgreeSuccessCount, "any channel in the tier counts as agreement")
	require.Equal(t, int64(1), total.DisagreeFailureCount)
	require.Equal(t, int64(1), total.CrossGroupCount)
	require.Equal(t, int64(1), total.AvoidUnhealthyCount)
	require.Equal(t, int64(1), total.RequestCount, "requests without attempts are not counted")
	require.Equal(t, int64(1), total.RetriedCount)
	require.InDelta(t, 2.0, *reports[0].AvgAttempts, 1e-9)
	require.Len(t, c.recentSamples("m"), 1, "only disagreements are sampled")
}
