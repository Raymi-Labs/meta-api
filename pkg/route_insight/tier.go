package routeinsight

import "math"

// Tier returns the set of candidates the scheduler may treat as equivalent to
// the Decide() choice: same group, same health status, health within
// ScoreTolerance and, when the choice was cost driven, the same lowest cost.
// Load balancing and capacity admission then pick inside this set.
//
// probe (in [0,1)) is a pre-drawn random number compared with probePercent:
// when it wins and a non-healthy candidate exists, that candidate is offered
// alone so unknown or unhealthy routes keep receiving a bounded trickle of
// evidence (half-open recovery) instead of being starved forever.
func Tier(candidates []Candidate, health func(channelID int) RouteHealth, probe float64, probePercent int) ([]Candidate, Decision) {
	decision := Decide(candidates, health)
	if decision.ChannelID == 0 {
		return nil, decision
	}
	if decision.Status == StatusHealthy && probePercent > 0 && probe < float64(probePercent)/100 {
		var fallback *Candidate
		for index := range candidates {
			status := health(candidates[index].ChannelID).Status
			if status == StatusUnknown {
				decision.ChannelID, decision.Group, decision.Status, decision.Reason = candidates[index].ChannelID, candidates[index].Group, status, "probe"
				return []Candidate{candidates[index]}, decision
			}
			if status == StatusUnhealthy && fallback == nil {
				fallback = &candidates[index]
			}
		}
		if fallback != nil {
			decision.ChannelID, decision.Group, decision.Status, decision.Reason = fallback.ChannelID, fallback.Group, StatusUnhealthy, "probe"
			return []Candidate{*fallback}, decision
		}
	}
	var best Candidate
	for _, candidate := range candidates {
		if candidate.ChannelID == decision.ChannelID {
			best = candidate
			break
		}
	}
	bestHealth := health(best.ChannelID)
	tier := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Group != best.Group {
			continue
		}
		h := health(candidate.ChannelID)
		if h.Status != bestHealth.Status {
			continue
		}
		switch decision.Status {
		case StatusHealthy:
			if h.Score < bestHealth.Score-ScoreTolerance {
				continue
			}
			if decision.Reason == "cheapest_healthy" {
				if candidate.PurchaseRatio == nil || best.PurchaseRatio == nil || math.Abs(*candidate.PurchaseRatio-*best.PurchaseRatio) > 1e-9 {
					continue
				}
			}
		case StatusUnknown:
			// Without evidence keep the production tier: same group position
			// and the highest priority available there.
			if candidate.GroupIndex != best.GroupIndex || candidate.Priority != best.Priority {
				continue
			}
		default:
			if candidate.ChannelID != best.ChannelID {
				continue
			}
		}
		tier = append(tier, candidate)
	}
	return tier, decision
}
