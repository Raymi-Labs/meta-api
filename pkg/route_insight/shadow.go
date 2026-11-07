package routeinsight

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
)

const (
	ShadowBucketSeconds = int64(3600)
	maxRecentSamples    = 200
	maxHotShadowBuckets = 4096
)

// Candidate is one permitted channel for the request. GroupIndex is the
// position in the token's ordered routing groups; Priority is the channel
// priority the production scheduler uses inside a group.
type Candidate struct {
	ChannelID     int      `json:"channel_id"`
	ChannelName   string   `json:"channel_name"`
	Group         string   `json:"group"`
	GroupIndex    int      `json:"group_index"`
	Priority      int64    `json:"priority"`
	PurchaseRatio *float64 `json:"purchase_ratio,omitempty"`
	FaultDomain   string   `json:"fault_domain,omitempty"`
}

type Decision struct {
	ChannelID      int          `json:"channel_id"`
	Group          string       `json:"group"`
	Reason         string       `json:"reason"`
	Status         HealthStatus `json:"status"`
	CandidateCount int          `json:"candidates"`
	Healthy        int          `json:"healthy"`
	Unhealthy      int          `json:"unhealthy"`
	Unknown        int          `json:"unknown"`
	// Tier is every channel the router treats as an equally good pick (the
	// highest remaining priority in the chosen group); weights choose among
	// them, so production agrees when its channel is anywhere in the tier.
	Tier []int `json:"tier"`
}

// Decide simulates the smart routing switch, deterministically: keep the
// token's group order and channel priorities; inside a group, unhealthy
// channels go behind the rest; a group with no non-unhealthy candidate is
// skipped when a later group has one; if every group is unhealthy, the first
// group is used as usual. Unknown routes count as usable. Probing is not
// simulated, and neither purchase cost nor small score differences are used.
func Decide(candidates []Candidate, health func(channelID int) RouteHealth) Decision {
	decision := Decision{CandidateCount: len(candidates), Reason: "no_candidates"}
	if len(candidates) == 0 {
		return decision
	}
	status := make(map[int]HealthStatus, len(candidates))
	firstGroup, target := candidates[0].GroupIndex, -1
	for _, candidate := range candidates {
		h := health(candidate.ChannelID).Status
		status[candidate.ChannelID] = h
		switch h {
		case StatusHealthy:
			decision.Healthy++
		case StatusUnhealthy:
			decision.Unhealthy++
		default:
			decision.Unknown++
		}
		if candidate.GroupIndex < firstGroup {
			firstGroup = candidate.GroupIndex
		}
		if h != StatusUnhealthy && (target == -1 || candidate.GroupIndex < target) {
			target = candidate.GroupIndex
		}
	}
	reason := "ordered"
	pool := make([]Candidate, 0, len(candidates))
	if target == -1 {
		target, reason = firstGroup, "all_unhealthy"
		for _, candidate := range candidates {
			if candidate.GroupIndex == target {
				pool = append(pool, candidate)
			}
		}
	} else {
		for _, candidate := range candidates {
			if candidate.GroupIndex == target && status[candidate.ChannelID] != StatusUnhealthy {
				pool = append(pool, candidate)
			}
		}
	}
	top := pool[0].Priority
	for _, candidate := range pool[1:] {
		if candidate.Priority > top {
			top = candidate.Priority
		}
	}
	for _, candidate := range pool {
		if candidate.Priority == top {
			decision.Tier = append(decision.Tier, candidate.ChannelID)
		}
	}
	if reason == "ordered" {
		if target != firstGroup {
			reason = "skip_group"
		} else {
			for _, candidate := range candidates {
				if candidate.GroupIndex == target && status[candidate.ChannelID] == StatusUnhealthy && candidate.Priority >= top {
					reason = "skip_unhealthy"
					break
				}
			}
		}
		if decision.Healthy == 0 && decision.Unhealthy == 0 {
			reason = "no_data"
		}
	}
	for _, candidate := range pool {
		if candidate.ChannelID == decision.Tier[0] {
			decision.ChannelID, decision.Group, decision.Status = candidate.ChannelID, candidate.Group, status[candidate.ChannelID]
			break
		}
	}
	decision.Reason = reason
	return decision
}

func (d Decision) includes(channelID int) bool {
	for _, id := range d.Tier {
		if id == channelID {
			return true
		}
	}
	return false
}

// ShadowObservation joins one decision with what production actually did.
type ShadowObservation struct {
	Model         string
	Decision      Decision
	ActualChannel int
	ActualGroup   string
	ActualStatus  HealthStatus
	ActualOutcome Outcome
	At            time.Time
}

type RecentSample struct {
	At            int64        `json:"at"`
	Model         string       `json:"model"`
	ActualChannel int          `json:"actual_channel_id"`
	ActualGroup   string       `json:"actual_group"`
	ActualStatus  HealthStatus `json:"actual_status"`
	ActualOutcome Outcome      `json:"actual_outcome"`
	Shadow        Decision     `json:"shadow"`
}

type shadowCollector struct {
	mu        sync.Mutex
	flushMu   sync.Mutex
	buckets   map[shadowKey]*model.RouteShadowMetric
	persisted map[shadowKey]int64
	recent    []RecentSample
	save      func(context.Context, []model.RouteShadowMetric) error
}

type shadowKey struct {
	model string
	ts    int64
}

func newShadowCollector(save func(context.Context, []model.RouteShadowMetric) error) *shadowCollector {
	return &shadowCollector{buckets: map[shadowKey]*model.RouteShadowMetric{}, persisted: map[shadowKey]int64{}, save: save}
}

func (c *shadowCollector) row(name string, at time.Time) *model.RouteShadowMetric {
	if name == "" || len(name) > 255 {
		return nil
	}
	key := shadowKey{model: name, ts: at.Unix() / ShadowBucketSeconds * ShadowBucketSeconds}
	row := c.buckets[key]
	if row == nil {
		if len(c.buckets) >= maxHotShadowBuckets {
			return nil
		}
		row = &model.RouteShadowMetric{ID: uuid.NewString(), BucketTs: key.ts, ModelName: name}
		c.buckets[key] = row
	}
	row.UpdatedAt = at.Unix()
	return row
}

func (c *shadowCollector) observe(o ShadowObservation) {
	if o.At.IsZero() {
		o.At = time.Now()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	row := c.row(o.Model, o.At)
	if row == nil {
		return
	}
	row.DecisionCount++
	if o.Decision.ChannelID == 0 || o.Decision.Reason == "no_data" {
		row.NoDataCount++
	}
	success := o.ActualOutcome == OutcomeSuccess
	failure := o.ActualOutcome.IsUpstreamFailure()
	if o.Decision.ChannelID == 0 || o.Decision.includes(o.ActualChannel) {
		row.AgreeCount++
		if success {
			row.AgreeSuccessCount++
		} else if failure {
			row.AgreeFailureCount++
		}
		return
	}
	row.DisagreeCount++
	if success {
		row.DisagreeSuccessCount++
	} else if failure {
		row.DisagreeFailureCount++
	}
	if o.Decision.Group != o.ActualGroup {
		row.CrossGroupCount++
	}
	if o.ActualStatus == StatusUnhealthy {
		row.AvoidUnhealthyCount++
	}
	sample := RecentSample{At: o.At.Unix(), Model: o.Model, ActualChannel: o.ActualChannel, ActualGroup: o.ActualGroup,
		ActualStatus: o.ActualStatus, ActualOutcome: o.ActualOutcome, Shadow: o.Decision}
	c.recent = append(c.recent, sample)
	if len(c.recent) > maxRecentSamples {
		c.recent = c.recent[len(c.recent)-maxRecentSamples:]
	}
}

func (c *shadowCollector) finishRequest(name string, delivered bool, attempts int, at time.Time) {
	if attempts <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	row := c.row(name, at)
	if row == nil {
		return
	}
	row.RequestCount++
	row.TotalAttemptCount += int64(attempts)
	if attempts > 1 {
		row.RetriedCount++
	}
	if delivered {
		row.DeliveredCount++
	}
}

func shadowVersion(row *model.RouteShadowMetric) int64 {
	return row.DecisionCount + row.RequestCount
}

func (c *shadowCollector) flush(ctx context.Context, now time.Time) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	c.mu.Lock()
	rows := make([]model.RouteShadowMetric, 0, len(c.buckets))
	for key, row := range c.buckets {
		if c.persisted[key] != shadowVersion(row) {
			rows = append(rows, *row)
		}
	}
	c.mu.Unlock()
	if err := c.save(ctx, rows); err != nil {
		return err
	}
	current := now.Unix() / ShadowBucketSeconds * ShadowBucketSeconds
	c.mu.Lock()
	defer c.mu.Unlock()
	for index := range rows {
		c.persisted[shadowKey{model: rows[index].ModelName, ts: rows[index].BucketTs}] = shadowVersion(&rows[index])
	}
	for key, row := range c.buckets {
		if key.ts < current && c.persisted[key] == shadowVersion(row) {
			delete(c.buckets, key)
			delete(c.persisted, key)
		}
	}
	return nil
}

func (c *shadowCollector) recentSamples(name string) []RecentSample {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]RecentSample, 0, len(c.recent))
	for index := len(c.recent) - 1; index >= 0; index-- {
		if name == "" || c.recent[index].Model == name {
			result = append(result, c.recent[index])
		}
	}
	return result
}
