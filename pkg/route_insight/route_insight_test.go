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

func ms(v int64) *int64           { return &v }
func ratioPtr(v float64) *float64 { return &v }

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

func TestEvaluateRequiresEvidenceAndUsesLowerBound(t *testing.T) {
	window := func(success, failure, unknown int64) Window {
		w := newWindow()
		w.Success, w.ServiceFailure, w.UnknownFailure = success, failure, unknown
		w.Attempts = success + failure + unknown
		return w
	}
	small := Evaluate(window(9, 0, 0), window(9, 0, 0))
	require.Equal(t, StatusUnknown, small.Status, "9/9 is below the minimum sample size")

	perfect := Evaluate(window(20, 0, 0), window(20, 0, 0))
	require.Equal(t, StatusHealthy, perfect.Status)
	require.Less(t, perfect.Score, 0.9, "20/20 must not read as near-certain reliability")

	unknownOnly := Evaluate(window(0, 0, 50), window(0, 0, 50))
	require.Equal(t, StatusUnknown, unknownOnly.Status, "unknown outcomes are not upstream evidence")

	low := Evaluate(window(10, 0, 0), window(14, 6, 0))
	require.Equal(t, StatusUnhealthy, low.Status)
	require.Equal(t, "low_success_lower_bound", low.Reason)

	burst := Evaluate(window(2, 4, 0), window(200, 4, 0))
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

func TestDecideHealthBeforeCost(t *testing.T) {
	health := map[int]RouteHealth{
		1: {Status: StatusUnhealthy, Score: 0.5},
		2: {Status: StatusHealthy, Score: 0.95},
		3: {Status: StatusHealthy, Score: 0.94},
		4: {Status: StatusHealthy, Score: 0.70 + 0.2},
	}
	lookup := func(id int) RouteHealth {
		if h, ok := health[id]; ok {
			return h
		}
		return RouteHealth{Status: StatusUnknown}
	}

	cheapButBroken := []Candidate{
		{ChannelID: 1, Group: "a", GroupIndex: 0, Priority: 10, PurchaseRatio: ratioPtr(0.1)},
		{ChannelID: 2, Group: "b", GroupIndex: 1, Priority: 0, PurchaseRatio: ratioPtr(0.9)},
	}
	decision := Decide(cheapButBroken, lookup)
	require.Equal(t, 2, decision.ChannelID, "low price must not offset failing health")
	require.Equal(t, "b", decision.Group)

	equivalent := []Candidate{
		{ChannelID: 2, Group: "a", PurchaseRatio: ratioPtr(0.9)},
		{ChannelID: 3, Group: "a", PurchaseRatio: ratioPtr(0.6)},
		{ChannelID: 4, Group: "a", PurchaseRatio: ratioPtr(0.1)},
	}
	decision = Decide(equivalent, lookup)
	require.Equal(t, 3, decision.ChannelID, "cheapest among equivalent healthy; 4 is healthy but clearly worse")
	require.Equal(t, "cheapest_healthy", decision.Reason)

	missingCost := []Candidate{{ChannelID: 2, Group: "a"}, {ChannelID: 3, Group: "a", PurchaseRatio: ratioPtr(0.1)}}
	decision = Decide(missingCost, lookup)
	require.Equal(t, 2, decision.ChannelID, "unknown cost is never treated as cheapest")
	require.Equal(t, "best_health_cost_unknown", decision.Reason)

	unknown := []Candidate{{ChannelID: 8, GroupIndex: 1, Priority: 5}, {ChannelID: 9, GroupIndex: 0, Priority: 0}}
	decision = Decide(unknown, lookup)
	require.Equal(t, 9, decision.ChannelID, "without data keep production group order")
	require.Equal(t, "no_healthy_data", decision.Reason)

	decision = Decide([]Candidate{{ChannelID: 1}}, lookup)
	require.Equal(t, "all_unhealthy", decision.Reason)
	require.Equal(t, "no_candidates", Decide(nil, lookup).Reason)
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
	c.observe(ShadowObservation{Model: "m", Decision: Decision{ChannelID: 1, Group: "a", Status: StatusHealthy, Reason: "best_health"},
		ActualChannel: 1, ActualGroup: "a", ActualOutcome: OutcomeSuccess, At: now})
	c.observe(ShadowObservation{Model: "m", Decision: Decision{ChannelID: 2, Group: "b", Status: StatusHealthy, Reason: "cheapest_healthy"},
		ActualChannel: 1, ActualGroup: "a", ActualStatus: StatusUnhealthy, ActualOutcome: OutcomeServiceFailure,
		ActualPurchase: ratioPtr(1), ShadowPurchase: ratioPtr(0.5), At: now})
	c.finishRequest("m", true, 2, now)
	c.finishRequest("m", false, 0, now)
	require.NoError(t, c.flush(ctx, now))

	reports, err := ShadowReports(ctx, now.Add(time.Second), 24, "")
	require.NoError(t, err)
	require.Len(t, reports, 1)
	total := reports[0].Total
	require.Equal(t, int64(2), total.DecisionCount)
	require.Equal(t, int64(1), total.AgreeSuccessCount)
	require.Equal(t, int64(1), total.DisagreeFailureCount)
	require.Equal(t, int64(1), total.CrossGroupCount)
	require.Equal(t, int64(1), total.AvoidUnhealthyCount)
	require.Equal(t, int64(1), total.CheaperCount)
	require.Equal(t, int64(1), total.RequestCount, "requests without attempts are not counted")
	require.Equal(t, int64(1), total.RetriedCount)
	require.InDelta(t, 2.0, *reports[0].AvgAttempts, 1e-9)
	require.Len(t, c.recentSamples("m"), 1, "only disagreements are sampled")
}
