package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func visibleHealthGroups(userGroup string, requested string) []string {
	active := ratio_setting.GetGroupRatioCopy()
	groups := make([]string, 0)
	for group := range service.GetUserUsableGroups(userGroup) {
		if group == "" || group == "auto" || (requested != "" && requested != group) {
			continue
		}
		if _, exists := active[group]; exists {
			groups = append(groups, group)
		}
	}
	sort.Strings(groups)
	return groups
}

func currentHealthUser(c *gin.Context) *model.User {
	c.Header("Cache-Control", "private, no-store")
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "请先登录"})
		return nil
	}
	// Read current membership from DB rather than trusting query parameters or
	// a possibly stale session/cache when deciding visibility.
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.SysError("failed to resolve group health permissions: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "分组健康数据暂不可用"})
		return nil
	}
	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "用户已被禁用"})
		return nil
	}
	return user
}

func healthWindowHours(c *gin.Context) (int, bool) {
	hours, err := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if err != nil || (hours != 24 && hours != 72) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "查询参数无效"})
		return 0, false
	}
	return hours, true
}

func GetGroupHealth(c *gin.Context) {
	user := currentHealthUser(c)
	if user == nil {
		return
	}
	hours, valid := healthWindowHours(c)
	if !valid {
		return
	}
	modelName := strings.TrimSpace(c.Query("model"))
	requested := strings.TrimSpace(c.Query("group"))
	if len(modelName) > 255 || len(requested) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "查询参数无效"})
		return
	}
	groups := visibleHealthGroups(user.Group, requested)
	if modelName != "" && !grouphealth.IsModelAuthorized(modelName, user.Role) {
		groups = nil
	}
	result, err := grouphealth.Query(c.Request.Context(), groups, modelName, user.Role, time.Now(), hours)
	if err != nil {
		common.SysError("failed to query group health: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "分组健康数据暂不可用"})
		return
	}
	if modelName != "" && len(groups) > 0 {
		needs := []string{}
		for _, g := range result.Groups {
			if needsPerformanceReference(g.Metrics) {
				needs = append(needs, g.Group)
			}
		}
		emptyUsageGroups := []string{}
		for _, g := range result.Groups {
			if g.ObservedCount == 0 && g.RequestCount == 0 && g.TotalTokens == nil {
				emptyUsageGroups = append(emptyUsageGroups, g.Group)
			}
		}
		var usageRef *grouphealth.ModelUsageReference
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			usageRef = service.OpenRouterModelUsage(c.Request.Context(), emptyUsageGroups, modelName)
		}()
		refs := service.OpenRouterPerformance(c.Request.Context(), needs, []string{modelName})
		wg.Wait()
		result.ModelUsageReference = usageRef
		for i := range result.Groups {
			result.Groups[i].PerformanceReference = refs[modelName][result.Groups[i].Group]
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func GetModelHealthSummaries(c *gin.Context) {
	user := currentHealthUser(c)
	if user == nil {
		return
	}
	hours, valid := healthWindowHours(c)
	if !valid {
		return
	}
	names := c.QueryArray("model")
	if len(names) == 0 || len(names) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "查询参数无效"})
		return
	}
	models := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || len(name) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "查询参数无效"})
			return
		}
		if !seen[name] {
			seen[name] = true
			models = append(models, name)
		}
	}
	result, err := grouphealth.QueryModels(c.Request.Context(), visibleHealthGroups(user.Group, ""), models, user.Role, hours, time.Now())
	if err != nil {
		common.SysError("failed to query model health summaries: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "分组健康数据暂不可用"})
		return
	}
	needs := []string{}
	for _, m := range result.Models {
		if needsPerformanceReference(m.Metrics) {
			needs = append(needs, m.Model)
		}
	}
	groups := visibleHealthGroups(user.Group, "")
	refs := service.OpenRouterPerformance(c.Request.Context(), groups, needs)
	for i := range result.Models {
		for _, group := range groups {
			if ref := refs[result.Models[i].Model][group]; ref != nil {
				result.Models[i].PerformanceReference = ref
				break
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func needsPerformanceReference(m grouphealth.Metrics) bool {
	return (m.AvgTTFTMs == nil && (m.LastValid == nil || m.LastValid.TTFT == nil)) || m.AvgOutputTPS == nil
}
