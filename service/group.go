package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}

// CheckUserChannelGroupConflict rejects a name registered both as a user
// group and as a channel group, so the two lists stay distinguishable. The
// default group is exempt: new users get it, and a token without a group uses
// its user group name as the channel group.
func CheckUserChannelGroupConflict(userGroups map[string]string, channelGroups map[string]float64) error {
	return setting.CheckUserChannelGroupConflict(userGroups, channelGroups)
}

// ResolveTokenGroup preserves legacy inheritance and only gives registered,
// identity-only groups an authorized default channel. Explicit choices never change.
func ResolveTokenGroup(userGroup, tokenGroup string) string {
	if tokenGroup != "" {
		return tokenGroup
	}
	if userGroup == "auto" || ratio_setting.ContainsGroupRatio(userGroup) {
		return userGroup
	}
	if setting.IsRegisteredUserGroup(userGroup) && ratio_setting.ContainsGroupRatio(setting.DefaultUserGroup) && GroupInUserUsableGroups(userGroup, setting.DefaultUserGroup) {
		return setting.DefaultUserGroup
	}
	return userGroup
}
