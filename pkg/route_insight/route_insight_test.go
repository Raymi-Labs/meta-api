package routeinsight

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestMain keeps the background flush/snapshot loop from starting: tests
// drive collectors and refreshSnapshot directly, and a concurrent refresh
// would race with snapshot assertions.
func TestMain(m *testing.M) {
	startOnce.Do(func() {})
	os.Exit(m.Run())
}

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
	require.Equal(t, StatusUnhealthy, unknownOnly.Status, "answers that never complete count against the route")
	cutOff := Evaluate(window(30, 0, 2), window(76, 0, 24), 0.9)
	require.Equal(t, StatusUnhealthy, cutOff.Status, "24% cut-off streams with no classified error is unhealthy")
	require.Equal(t, "low_success_rate", cutOff.Reason)
	unknownBurst := Evaluate(window(2, 1, 3), window(200, 1, 3), 0.9)
	require.Equal(t, "recent_failure_burst", unknownBurst.Reason, "unknown outcomes join the 5-minute burst detector")
	notEvidence := newWindow()
	notEvidence.UserError, notEvidence.ClientCanceled, notEvidence.LocalDelivery, notEvidence.CapacityRejected = 40, 40, 40, 40
	require.Equal(t, StatusUnknown, Evaluate(notEvidence, notEvidence, 0.9).Status, "user, cancel, local and capacity outcomes are not evidence")

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
	require.True(t, OutcomeUnknownFailure.IsUpstreamFailure(), "an answer that never completes counts against the route")
	require.False(t, OutcomeClientCanceled.IsUpstreamFailure())
	require.False(t, OutcomeLocalDelivery.IsUpstreamFailure())
	require.False(t, OutcomeUserError.IsUpstreamFailure())
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

func TestApplySlowTTFTComparesWithSameModelPeers(t *testing.T) {
	ttftWindow := func(bin int, n int64) *Window {
		w := newWindow()
		w.TTFT[bin] = n
		return &w
	}
	// Bins: 5 -> <=5s, 7 -> <=12s, 8 -> <=20s, 6 -> <=8s.
	windows := map[routeKey]*Window{
		{channelID: 1, model: "m"}:               ttftWindow(7, 20), // ~12s
		{channelID: 2, model: "m"}:               ttftWindow(5, 30), // ~5s
		{channelID: 3, model: "m"}:               ttftWindow(5, 30), // ~5s
		{channelID: 4, model: "f"}:               ttftWindow(6, 20), // ~8s: 4x its peer but under the 10s floor
		{channelID: 8, model: "f"}:               ttftWindow(3, 30), // ~2s
		{channelID: 5, model: "slow-everywhere"}: ttftWindow(9, 30),
		{channelID: 6, model: "slow-everywhere"}: ttftWindow(9, 30),
		{channelID: 7, model: "m"}:               ttftWindow(8, 5), // too few samples
	}
	health := map[routeKey]RouteHealth{}
	for key := range windows {
		health[key] = RouteHealth{Status: StatusHealthy, Reason: "ok"}
	}
	health[routeKey{channelID: 3, model: "m"}] = RouteHealth{Status: StatusUnhealthy, Reason: "low_success_rate"}

	ApplySlowTTFT(health, windows, 2, 10000)
	slow := health[routeKey{channelID: 1, model: "m"}]
	require.Equal(t, StatusUnhealthy, slow.Status)
	require.Equal(t, "slow_ttft", slow.Reason)
	require.Equal(t, int64(12000), *slow.TTFTP75Ms)
	require.Equal(t, int64(5000), *slow.PeerTTFTP75Ms)
	require.Equal(t, StatusHealthy, health[routeKey{channelID: 2, model: "m"}].Status)
	require.Equal(t, StatusHealthy, health[routeKey{channelID: 4, model: "f"}].Status, "8s is under the 10s floor")
	require.Equal(t, "low_success_rate", health[routeKey{channelID: 3, model: "m"}].Reason, "an existing reason is kept")
	require.Equal(t, StatusHealthy, health[routeKey{channelID: 5, model: "slow-everywhere"}].Status, "a model slow on every channel is not penalized")
	require.Equal(t, StatusHealthy, health[routeKey{channelID: 7, model: "m"}].Status, "fewer than 10 first-token samples")
	require.Nil(t, health[routeKey{channelID: 7, model: "m"}].TTFTP75Ms)

	for key := range windows {
		health[key] = RouteHealth{Status: StatusHealthy}
	}
	ApplySlowTTFT(health, windows, 0, 10000)
	require.Equal(t, StatusHealthy, health[routeKey{channelID: 1, model: "m"}].Status, "ratio 0 disables the rule")
	ApplySlowTTFT(health, windows, 3, 10000)
	require.Equal(t, StatusHealthy, health[routeKey{channelID: 1, model: "m"}].Status, "12s is not 3x 5s")

	overflow := map[routeKey]*Window{{channelID: 1, model: "o"}: ttftWindow(12, 20), {channelID: 2, model: "o"}: ttftWindow(7, 20)}
	h2 := map[routeKey]RouteHealth{{channelID: 1, model: "o"}: {Status: StatusHealthy}, {channelID: 2, model: "o"}: {Status: StatusHealthy}}
	ApplySlowTTFT(h2, overflow, 2, 10000)
	require.Equal(t, "slow_ttft", h2[routeKey{channelID: 1, model: "o"}].Reason, ">120s overflow counts as slow")
}

func TestRefreshSnapshotMarksSlowFirstToken(t *testing.T) {
	useTestDB(t)
	ctx := context.Background()
	now := time.Now()
	c := newAttemptCollector(model.SaveRouteAttemptSnapshots)
	for i := 0; i < 20; i++ {
		c.record(Attempt{ChannelID: 1, Group: "g", Model: "m", Outcome: OutcomeSuccess, TTFTMs: ms(15000), At: now})
		c.record(Attempt{ChannelID: 2, Group: "g", Model: "m", Outcome: OutcomeSuccess, TTFTMs: ms(3000), At: now})
	}
	require.NoError(t, c.flush(ctx, now))
	require.NoError(t, refreshSnapshot(ctx, now.Add(time.Second)))
	snapshotAt.Store(time.Now().Unix())

	slow := Health(1, "m")
	require.Equal(t, StatusUnhealthy, slow.Status, "100% success but first token ~15s vs ~3s on the other channel")
	require.Equal(t, "slow_ttft", slow.Reason)
	require.Equal(t, StatusHealthy, Health(2, "m").Status)

	reports, err := ChannelReports(ctx, now.Add(time.Second), 60, "m")
	require.NoError(t, err)
	for _, report := range reports {
		if report.ChannelID == 1 {
			require.Equal(t, int64(20000), *report.CurrentTTFTP75Ms)
			require.Equal(t, int64(3000), *report.PeerTTFTP75Ms)
		}
	}
}

func TestUnhealthyRoutesFollowSnapshotFreshness(t *testing.T) {
	restore := SetHealthForTesting(
		HealthEntry{ChannelID: 2, Model: "b", Health: RouteHealth{Status: StatusUnhealthy, Reason: "slow_ttft"}},
		HealthEntry{ChannelID: 1, Model: "a", Health: RouteHealth{Status: StatusUnhealthy, Reason: "low_success_rate"}},
		HealthEntry{ChannelID: 3, Model: "a", Health: RouteHealth{Status: StatusHealthy}},
	)
	t.Cleanup(restore)
	routes, fresh := UnhealthyRoutes()
	require.True(t, fresh)
	require.Equal(t, []UnhealthyRoute{{ChannelID: 1, Model: "a", Reason: "low_success_rate"}, {ChannelID: 2, Model: "b", Reason: "slow_ttft"}}, routes)

	snapshotAt.Store(time.Now().Add(-time.Hour).Unix())
	routes, fresh = UnhealthyRoutes()
	require.False(t, fresh, "a stale snapshot is unknown, never an all-clear")
	require.Nil(t, routes)
}
