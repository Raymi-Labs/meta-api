package routeinsight

import (
	"context"
	"sort"
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
	CostKnown      bool         `json:"cost_known"`
}

type scored struct {
	Candidate
	health RouteHealth
}

// Decide is pure: it ranks candidates on health first, then on configured
// purchase cost among equivalent healthy routes. A sales group ratio is never
// treated as cost, and missing cost is never treated as zero.
func Decide(candidates []Candidate, health func(channelID int) RouteHealth) Decision {
	decision := Decision{CandidateCount: len(candidates), Reason: "no_candidates"}
	if len(candidates) == 0 {
		return decision
	}
	all := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		h := health(candidate.ChannelID)
		switch h.Status {
		case StatusHealthy:
			decision.Healthy++
		case StatusUnhealthy:
			decision.Unhealthy++
		default:
			decision.Unknown++
		}
		all = append(all, scored{Candidate: candidate, health: h})
	}
	// Production order: earlier group first, then higher priority.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].GroupIndex != all[j].GroupIndex {
			return all[i].GroupIndex < all[j].GroupIndex
		}
		return all[i].Priority > all[j].Priority
	})
	pick := func(s scored, reason string) Decision {
		decision.ChannelID, decision.Group, decision.Reason, decision.Status = s.ChannelID, s.Group, reason, s.health.Status
		decision.CostKnown = s.PurchaseRatio != nil
		return decision
	}
	pool := filterStatus(all, StatusHealthy)
	if len(pool) == 0 {
		if unknown := filterStatus(all, StatusUnknown); len(unknown) > 0 {
			// No evidence: keep the production order rather than explore.
			return pick(unknown[0], "no_healthy_data")
		}
		best := all[0]
		for _, s := range all[1:] {
			if s.health.Score > best.health.Score {
				best = s
			}
		}
		return pick(best, "all_unhealthy")
	}
	bestScore := pool[0].health.Score
	for _, s := range pool[1:] {
		if s.health.Score > bestScore {
			bestScore = s.health.Score
		}
	}
	equivalent := make([]scored, 0, len(pool))
	costKnown := true
	for _, s := range pool {
		if s.health.Score >= bestScore-ScoreTolerance {
			equivalent = append(equivalent, s)
			if s.PurchaseRatio == nil {
				costKnown = false
			}
		}
	}
	if costKnown && len(equivalent) > 1 {
		best := equivalent[0]
		for _, s := range equivalent[1:] {
			if *s.PurchaseRatio < *best.PurchaseRatio {
				best = s
			}
		}
		return pick(best, "cheapest_healthy")
	}
	best := equivalent[0]
	for _, s := range equivalent[1:] {
		if s.health.Score > best.health.Score {
			best = s
		}
	}
	reason := "best_health"
	if !costKnown {
		reason = "best_health_cost_unknown"
	}
	return pick(best, reason)
}

func filterStatus(all []scored, status HealthStatus) []scored {
	result := make([]scored, 0, len(all))
	for _, s := range all {
		if s.health.Status == status {
			result = append(result, s)
		}
	}
	return result
}

// ShadowObservation joins one decision with what production actually did.
type ShadowObservation struct {
	Model          string
	Decision       Decision
	ActualChannel  int
	ActualGroup    string
	ActualStatus   HealthStatus
	ActualOutcome  Outcome
	ActualPurchase *float64
	ShadowPurchase *float64
	At             time.Time
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
	if o.Decision.ChannelID == 0 || o.Decision.Reason == "no_healthy_data" {
		row.NoDataCount++
	}
	if o.Decision.Reason == "best_health_cost_unknown" {
		row.CostUnknownCount++
	}
	success := o.ActualOutcome == OutcomeSuccess
	failure := o.ActualOutcome.IsUpstreamFailure()
	if o.Decision.ChannelID == 0 || o.Decision.ChannelID == o.ActualChannel {
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
	if o.ActualStatus == StatusUnhealthy && o.Decision.Status == StatusHealthy {
		row.AvoidUnhealthyCount++
	}
	if o.ShadowPurchase != nil && o.ActualPurchase != nil && *o.ShadowPurchase < *o.ActualPurchase {
		row.CheaperCount++
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
