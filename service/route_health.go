package service

import (
	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func healthRoutingActive(c *gin.Context) bool {
	if !routeinsight.Enabled() || !operation_setting.GetRouteRoutingSetting().HealthRoutingEnabled {
		return false
	}
	// Plan tokens rank health themselves when plan routing is on.
	return !planRoutingActive(c)
}

// routeHealthAvoidSet lists channels the health router skips for this
// attempt. A channel is skipped only when it is unhealthy, did not win the
// probe draw, and every routing group containing it still has another
// non-unhealthy candidate, so health routing never empties a group that
// production could otherwise serve. Unknown routes are never skipped.
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
	probe := routeProbeDraw(param) < float64(operation_setting.GetRouteRoutingSetting().ProbePercent)/100
	avoid := map[int]bool{}
	for id := range unhealthy {
		if probe {
			// One probe attempt keeps every unhealthy route reachable.
			continue
		}
		replaceable := true
		for _, index := range groupsOf[id] {
			if healthyCount[index] == 0 {
				replaceable = false
				break
			}
		}
		if replaceable {
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
