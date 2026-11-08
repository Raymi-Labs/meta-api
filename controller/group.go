package controller

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func sortedGroupNames(groupRatios map[string]float64) []string {
	groupNames := make([]string, 0, len(groupRatios))
	for groupName := range groupRatios {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)
	return groupNames
}

func GetGroups(c *gin.Context) {
	groupNames := sortedGroupNames(ratio_setting.GetGroupRatioCopy())
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			usableGroups[groupName] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}

// validateUserGroupAssignment allows keeping a user's current (possibly
// unregistered, legacy) group; any new value must be a registered user group.
func validateUserGroupAssignment(current, next string) error {
	if next == "" {
		return errors.New("请选择用户分组")
	}
	if next == current || setting.IsRegisteredUserGroup(next) {
		return nil
	}
	return fmt.Errorf("用户分组「%s」尚未登记，请先在「分组相关设置 → 用户分组」中添加", next)
}

// validatePlanUpgradeGroup accepts no upgrade, the plan's current value, or a
// registered user group: upgrading changes the buyer's user group.
func validatePlanUpgradeGroup(current, next string) error {
	if next == "" || next == current || setting.IsRegisteredUserGroup(next) {
		return nil
	}
	return fmt.Errorf("升级分组必须是已登记的用户分组，「%s」尚未登记", next)
}

type userGroupOverviewItem struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
	// Registered is false for names still in use (users, plans or special
	// rules) but missing from the UserGroups list.
	Registered bool `json:"registered"`
	// ChannelGroup marks a name that is also a channel group; such a name
	// cannot be registered and its users should be moved to another group.
	ChannelGroup bool  `json:"channel_group"`
	UserCount    int64 `json:"user_count"`
	PlanCount    int64 `json:"plan_count"`
	RuleCount    int   `json:"rule_count"`
}

// GetUserGroupOverview lists registered user groups plus every user group
// name still referenced by users, subscription plans or special rules.
func GetUserGroupOverview(c *gin.Context) {
	userCounts, err := model.CountUsersByGroup()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	planCounts, err := model.CountSubscriptionPlansByUpgradeGroup()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	registered := setting.GetUserGroupsCopy()
	channelGroups := ratio_setting.GetGroupRatioCopy()
	ruleCounts := make(map[string]int)
	for name := range ratio_setting.GetGroupRatioSetting().GroupGroupRatio.ReadAll() {
		ruleCounts[name]++
	}
	for name := range ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll() {
		ruleCounts[name]++
	}

	names := make(map[string]bool)
	for name := range registered {
		names[name] = true
	}
	for name := range userCounts {
		names[name] = true
	}
	for name := range planCounts {
		names[name] = true
	}
	for name := range ruleCounts {
		names[name] = true
	}

	items := make([]userGroupOverviewItem, 0, len(names))
	for name := range names {
		desc, ok := registered[name]
		_, isChannelGroup := channelGroups[name]
		items = append(items, userGroupOverviewItem{
			Name:         name,
			Desc:         desc,
			Registered:   ok,
			ChannelGroup: isChannelGroup && name != setting.DefaultUserGroup,
			UserCount:    userCounts[name],
			PlanCount:    planCounts[name],
			RuleCount:    ruleCounts[name],
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Registered != items[j].Registered {
			return items[i].Registered
		}
		if items[i].UserCount != items[j].UserCount {
			return items[i].UserCount > items[j].UserCount
		}
		return items[i].Name < items[j].Name
	})
	common.ApiSuccess(c, items)
}

type staleTokenGroupItem struct {
	Name       string `json:"name"`
	TokenCount int64  `json:"token_count"`
}

// GetStaleTokenGroups lists channel groups that enabled tokens still use but
// that no longer exist in GroupRatio; requests from those tokens are rejected.
func GetStaleTokenGroups(c *gin.Context) {
	counts, err := model.CountEnabledTokensByGroup()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelGroups := ratio_setting.GetGroupRatioCopy()
	items := make([]staleTokenGroupItem, 0)
	for name, count := range counts {
		if _, ok := channelGroups[name]; ok {
			continue
		}
		items = append(items, staleTokenGroupItem{Name: name, TokenCount: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TokenCount != items[j].TokenCount {
			return items[i].TokenCount > items[j].TokenCount
		}
		return items[i].Name < items[j].Name
	})
	common.ApiSuccess(c, items)
}

type reassignUserGroupRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ReassignUnregisteredUserGroup moves all users of an unregistered user group
// to a registered one. Registered groups are changed per user instead.
func ReassignUnregisteredUserGroup(c *gin.Context) {
	var req reassignUserGroupRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	from := strings.TrimSpace(req.From)
	to := strings.TrimSpace(req.To)
	if from == "" || to == "" || from == to {
		common.ApiErrorMsg(c, "请选择要调整的用户分组和目标用户分组")
		return
	}
	if setting.IsRegisteredUserGroup(from) {
		common.ApiErrorMsg(c, fmt.Sprintf("「%s」是已登记的用户分组，请在用户管理中逐个调整", from))
		return
	}
	if !setting.IsRegisteredUserGroup(to) {
		common.ApiErrorMsg(c, fmt.Sprintf("目标用户分组「%s」尚未登记", to))
		return
	}
	ids, err := model.ReassignUserGroup(from, to)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(ids) > 0 {
		model.RecordLogWithAdminInfo(c.GetInt("id"), model.LogTypeManage,
			fmt.Sprintf("管理员将 %d 个用户的用户分组从「%s」改为「%s」", len(ids), from, to),
			map[string]interface{}{
				"admin_id":       c.GetInt("id"),
				"admin_username": c.GetString("username"),
			},
		)
	}
	common.ApiSuccess(c, gin.H{"updated": len(ids)})
}
