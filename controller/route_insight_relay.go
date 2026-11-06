package controller

import (
	"time"

	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
)

// routeAttemptTracker owns route telemetry for one relay request. It only
// observes; it never alters the channel the production scheduler picked.
type routeAttemptTracker struct {
	model    string
	attempts int
	start    time.Time
	shadow   *routeinsight.Decision
	actual   *routeinsight.Candidate
}

func newRouteAttemptTracker(modelName string) *routeAttemptTracker {
	if !routeinsight.Enabled() {
		return nil
	}
	return &routeAttemptTracker{model: modelName}
}

// begin must run after selection and before the attempt mutates retry
// exclusions, so the shadow sees the same candidate set production saw.
func (t *routeAttemptTracker) begin(retryParam *service.RetryParam, channel *model.Channel, group string) {
	if t == nil || channel == nil {
		return
	}
	t.start = time.Now()
	t.shadow, t.actual = nil, nil
	actual := service.RouteShadowCandidate(channel, group, -1)
	t.actual = &actual
	candidates, ok := service.RouteShadowCandidates(retryParam)
	if !ok {
		return
	}
	// The production choice was eligible even if a filter changed since; keep
	// it in the comparison set so agreement is measurable.
	present := false
	for _, candidate := range candidates {
		if candidate.ChannelID == channel.Id {
			present = true
			break
		}
	}
	if !present {
		candidates = append(candidates, actual)
	}
	decision := routeinsight.Decide(candidates, func(id int) routeinsight.RouteHealth {
		return routeinsight.Health(id, t.model)
	})
	t.shadow = &decision
}

func (t *routeAttemptTracker) finishAttempt(info *relaycommon.RelayInfo, err *types.NewAPIError, delivered, downstreamWrite, capacity bool) {
	if t == nil || t.actual == nil || info == nil {
		return
	}
	t.attempts++
	now := time.Now()
	outcome := routeinsight.ClassifyAttempt(routeinsight.AttemptInput{Info: info, Err: err, Delivered: delivered,
		DownstreamWrite: downstreamWrite, CapacityRejection: capacity})
	attempt := routeinsight.Attempt{ChannelID: t.actual.ChannelID, Group: info.UsingGroup, Model: t.model, Outcome: outcome, At: now}
	if outcome == routeinsight.OutcomeSuccess {
		duration := now.Sub(t.start).Milliseconds()
		attempt.DurationMs = &duration
		first := info.GroupHealthFirstOutputTime
		if info.IsStream && !first.IsZero() && !first.Before(t.start) && !first.After(now) {
			ttft := first.Sub(t.start).Milliseconds()
			attempt.TTFTMs = &ttft
		}
	}
	routeinsight.RecordAttempt(attempt)
	if t.shadow == nil {
		return
	}
	observation := routeinsight.ShadowObservation{Model: t.model, Decision: *t.shadow, ActualChannel: t.actual.ChannelID,
		ActualGroup: info.UsingGroup, ActualStatus: routeinsight.Health(t.actual.ChannelID, t.model).Status,
		ActualOutcome: outcome, ActualPurchase: t.actual.PurchaseRatio, At: now}
	if t.shadow.ChannelID != 0 && t.shadow.ChannelID != t.actual.ChannelID {
		if shadowChannel, cacheErr := model.CacheGetChannel(t.shadow.ChannelID); cacheErr == nil && shadowChannel != nil {
			observation.ShadowPurchase = service.RouteShadowCandidate(shadowChannel, t.shadow.Group, 0).PurchaseRatio
		}
	}
	routeinsight.RecordShadow(observation)
}

func (t *routeAttemptTracker) finishRequest(delivered bool) {
	if t == nil {
		return
	}
	routeinsight.FinishRequest(t.model, delivered, t.attempts)
}
