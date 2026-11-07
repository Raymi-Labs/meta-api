package service

import (
	"math/rand"

	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func healthRoutingActive(c *gin.Context) bool {
	return routeinsight.Enabled() && operation_setting.GetRouteRoutingSetting().HealthRoutingEnabled
}

// routeProbeDraw is drawn once per attempt (keyed by the exclusion count) so
// every filter evaluation inside one selection sees the same decision, while
// a retry after a failed probe draws again.
func routeProbeDraw(param *RetryParam) float64 {
	key := len(param.ExcludeChannels) + 1
	if param.routeProbeKey != key {
		param.routeProbe = rand.Float64()
		param.routeProbeKey = key
	}
	return param.routeProbe
}

// routeHealthAvoidSet lists channels the health router skips for this
// attempt. The token's group order, priorities and weights are kept; health
// only moves unhealthy channels behind the rest:
//   - inside a group, an unhealthy channel is skipped while that group still
//     has a non-unhealthy candidate for the model;
//   - a group with no non-unhealthy candidate is skipped entirely when one of
//     the token's later groups has one, so the request goes straight to the
//     next group instead of failing first (billed at the group actually used,
//     as with ordinary group fallback);
//   - if no group has a non-unhealthy candidate, nothing is skipped.
//
// Unknown routes are never skipped, and a won probe draw skips nothing, so an
// unhealthy route still receives a bounded trickle of traffic to recover.
// A channel shared by several groups is skipped only if every group holding
// it can do without it.
func routeHealthAvoidSet(param *RetryParam, base []model.ChannelFilter) map[int]bool {
	groups := RequestModelRoutingGroups(param.Ctx, param.TokenGroup, param.ModelName)
	unhealthy := map[int]bool{}
	groupsOf := map[int][]int{}
	healthyCount := make([]int, len(groups))
	for index, group := range groups {
		channels, ok := model.GetAllSatisfiedChannels(group, param.ModelName, param.AllowedChannels, param.ExcludeChannels, base...)
		if !ok {
			return nil
		}
		for _, channel := range channels {
			groupsOf[channel.Id] = append(groupsOf[channel.Id], index)
			if routeinsight.Health(channel.Id, param.ModelName).Status == routeinsight.StatusUnhealthy {
				unhealthy[channel.Id] = true
			} else {
				healthyCount[index]++
			}
		}
	}
	if len(unhealthy) == 0 {
		return nil
	}
	if routeProbeDraw(param) < float64(operation_setting.GetRouteRoutingSetting().ProbePercent)/100 {
		// One probe attempt keeps every unhealthy route reachable.
		return nil
	}
	// laterHealthy[i]: some group after i still has a non-unhealthy candidate.
	laterHealthy := make([]bool, len(groups))
	for index := len(groups) - 2; index >= 0; index-- {
		laterHealthy[index] = laterHealthy[index+1] || healthyCount[index+1] > 0
	}
	avoid := map[int]bool{}
	for id := range unhealthy {
		skippable := true
		for _, index := range groupsOf[id] {
			if healthyCount[index] == 0 && !laterHealthy[index] {
				skippable = false
				break
			}
		}
		if skippable {
			avoid[id] = true
		}
	}
	return avoid
}

// RouteHealthAvoids is used by session affinity: a sticky channel that is now
// unhealthy is released to normal selection, which applies the full rules.
func RouteHealthAvoids(c *gin.Context, modelName string, channelID int) bool {
	return healthRoutingActive(c) && routeinsight.Health(channelID, modelName).Status == routeinsight.StatusUnhealthy
}
