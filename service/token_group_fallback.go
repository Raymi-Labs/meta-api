package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// TokenRoutingGroups keeps a concrete preferred group first. Backup groups use
// the existing ordered, user-permitted auto catalog; auto itself grants no access.
func TokenRoutingGroups(userGroup, preferredGroup string, fallback bool) []string {
	if preferredGroup == "" {
		preferredGroup = userGroup
	}
	if preferredGroup == "auto" {
		return GetUserAutoGroup(userGroup)
	}
	groups := []string{preferredGroup}
	if !fallback {
		return groups
	}
	seen := map[string]bool{preferredGroup: true, "": true, "auto": true}
	for _, group := range GetUserAutoGroup(userGroup) {
		if !seen[group] {
			groups = append(groups, group)
			seen[group] = true
		}
	}
	return groups
}

func UsesPreferredGroupFallback(c *gin.Context, group string) bool {
	return group != "auto" && common.GetContextKeyBool(c, constant.ContextKeyTokenAutoGroupFallback)
}

func RequestRoutingGroups(c *gin.Context, preferredGroup string) []string {
	return TokenRoutingGroups(common.GetContextKeyString(c, constant.ContextKeyUserGroup), preferredGroup,
		common.GetContextKeyBool(c, constant.ContextKeyTokenAutoGroupFallback))
}

// Candidate discovery must not advance the request's group state: capacity
// polling needs to revisit a group if its temporary exclusions are removed.
func preferredGroupCandidates(param *RetryParam) ([]*model.Channel, string, error) {
	groups := RequestRoutingGroups(param.Ctx, param.TokenGroup)
	start := common.GetContextKeyInt(param.Ctx, constant.ContextKeyAutoGroupIndex)
	if start < 0 {
		start = 0
	}
	for index := start; index < len(groups); index++ {
		priorityRetry := 0
		if index == start {
			priorityRetry = param.GetRetry() - common.GetContextKeyInt(param.Ctx, constant.ContextKeyAutoGroupRetryIndex)
			if priorityRetry < 0 {
				priorityRetry = 0
			}
		}
		channels, err := model.GetSatisfiedChannelCandidates(groups[index], param.ModelName, priorityRetry,
			param.AllowedChannels, param.ExcludeChannels, buildChannelSelectionFilters(param)...)
		if err != nil || len(channels) > 0 {
			return channels, groups[index], err
		}
	}
	return nil, param.TokenGroup, nil
}

func commitPreferredGroupSelection(param *RetryParam, group string) {
	for index, candidate := range RequestRoutingGroups(param.Ctx, param.TokenGroup) {
		if candidate != group {
			continue
		}
		if index != common.GetContextKeyInt(param.Ctx, constant.ContextKeyAutoGroupIndex) {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, param.GetRetry())
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, index)
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, group)
		return
	}
}
