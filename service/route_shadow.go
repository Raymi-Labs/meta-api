package service

import (
	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
)

// RouteShadowCandidates lists every channel the request is already permitted
// to use (token routing groups in order, allowed/excluded channels and the
// production availability filters, but not the health filter). It never
// widens access, and returns ok=false when the memory channel cache is
// unavailable.
func RouteShadowCandidates(param *RetryParam) ([]routeinsight.Candidate, bool) {
	if param == nil || param.Ctx == nil {
		return nil, false
	}
	if _, pinned := param.Ctx.Get("specific_channel_id"); pinned {
		return nil, false
	}
	groups := RequestModelRoutingGroups(param.Ctx, param.TokenGroup, param.ModelName)
	// Base filters only: the shadow applies health itself, so it must see the
	// unhealthy channels the live switch would hide.
	filters := buildBaseChannelSelectionFilters(param)
	seen := map[int]bool{}
	candidates := make([]routeinsight.Candidate, 0)
	for index, group := range groups {
		channels, ok := model.GetAllSatisfiedChannels(group, param.ModelName, param.AllowedChannels, param.ExcludeChannels, filters...)
		if !ok {
			return nil, false
		}
		for _, channel := range channels {
			// A channel in several groups is first reachable through the
			// earliest group; later duplicates would not change the decision.
			if seen[channel.Id] {
				continue
			}
			seen[channel.Id] = true
			candidates = append(candidates, RouteShadowCandidate(channel, group, index))
		}
	}
	return candidates, true
}

func RouteShadowCandidate(channel *model.Channel, group string, groupIndex int) routeinsight.Candidate {
	return routeinsight.Candidate{ChannelID: channel.Id, ChannelName: channel.Name, Group: group, GroupIndex: groupIndex,
		Priority: channel.GetPriority()}
}
