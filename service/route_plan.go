package service

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// ContextRoutePlan returns the plan snapshot locked when the token was
// authenticated, or nil for a manual-group token.
func ContextRoutePlan(c *gin.Context) *types.RoutePlanSnapshot {
	if c == nil {
		return nil
	}
	value, ok := common.GetContextKey(c, constant.ContextKeyRoutePlan)
	if !ok {
		return nil
	}
	plan, _ := value.(*types.RoutePlanSnapshot)
	return plan
}

// ResolveTokenRoutePlan re-checks the plan on every request: a disabled plan
// or revoked grant fails closed instead of reverting to manual group pricing.
func ResolveTokenRoutePlan(token *model.Token, userGroup string) (*types.RoutePlanSnapshot, error) {
	if token == nil || token.RoutePlanId <= 0 {
		return nil, nil
	}
	plan, err := model.GetRoutePlanCached(token.RoutePlanId)
	if err != nil {
		if errors.Is(err, model.ErrRoutePlanNotFound) {
			return nil, errors.New("令牌绑定的路由方案不存在")
		}
		return nil, err
	}
	if plan.Status != model.RoutePlanStatusEnabled {
		return nil, errors.New("令牌绑定的路由方案已停用")
	}
	if !plan.AuthorizedFor(token.UserId, userGroup) {
		return nil, errors.New("无权使用该路由方案")
	}
	return plan.Snapshot()
}

// ValidateTokenRoutePlanForSave applies the plan's pool to the token. Plan
// tokens never carry a manual routing_config or group fallback; the plan's
// first group is stored as the token group only for display and legacy reads.
func ValidateTokenRoutePlanForSave(userID int, userGroup string, token *model.Token) error {
	if token == nil || token.RoutePlanId <= 0 {
		if token != nil {
			token.RoutePlanId = 0
		}
		return nil
	}
	if token.SourceType == model.TokenSourceTypeSellableToken {
		return errors.New("可售令牌不支持路由方案")
	}
	plan, err := model.GetRoutePlanByID(token.RoutePlanId)
	if err != nil {
		return err
	}
	if !plan.AuthorizedFor(userID, userGroup) {
		return fmt.Errorf("无权使用路由方案 %s 或方案已停用", plan.Name)
	}
	snapshot, err := plan.Snapshot()
	if err != nil {
		return err
	}
	token.Group = snapshot.Groups[0]
	token.RoutingConfig = ""
	token.AutoGroupFallback = false
	token.CrossGroupRetry = false
	return nil
}

// RoutePlanRatio is the contract multiplier billed for this request. The
// group ratio, user-group special ratio and time rules do not apply.
func RoutePlanRatio(plan *types.RoutePlanSnapshot, modelName string) float64 {
	return plan.RatioFor(modelName, ratio_setting.FormatMatchingModelName(modelName))
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

// planRoutingActive reports whether this request ranks plan candidates by
// health and cost. With the switch off, plan tokens use the ordered groups.
func planRoutingActive(c *gin.Context) bool {
	return ContextRoutePlan(c) != nil && operation_setting.GetRouteRoutingSetting().PlanRoutingEnabled && routeinsight.Enabled()
}

// planRouteCandidates ranks every permitted plan channel (all groups, all
// priorities, minus retry exclusions) and returns the equivalent top tier.
// ok=false means the memory cache is unavailable; callers then fall back to
// the ordered-group path rather than scanning the database per request.
func planRouteCandidates(param *RetryParam) ([]*model.Channel, string, bool) {
	plan := ContextRoutePlan(param.Ctx)
	filters := buildChannelSelectionFilters(param)
	byID := map[int]*model.Channel{}
	candidates := make([]routeinsight.Candidate, 0)
	for index, group := range plan.Groups {
		channels, ok := model.GetAllSatisfiedChannels(group, param.ModelName, param.AllowedChannels, param.ExcludeChannels, filters...)
		if !ok {
			return nil, "", false
		}
		for _, channel := range channels {
			if _, seen := byID[channel.Id]; seen {
				continue
			}
			byID[channel.Id] = channel
			candidates = append(candidates, RouteShadowCandidate(channel, group, index))
		}
	}
	if len(candidates) == 0 {
		return nil, plan.Groups[0], true
	}
	setting := operation_setting.GetRouteRoutingSetting()
	tier, decision := routeinsight.Tier(candidates, func(id int) routeinsight.RouteHealth {
		return routeinsight.Health(id, param.ModelName)
	}, routeProbeDraw(param), setting.ProbePercent)
	result := make([]*model.Channel, 0, len(tier))
	for _, candidate := range tier {
		result = append(result, byID[candidate.ChannelID])
	}
	return result, decision.Group, true
}
