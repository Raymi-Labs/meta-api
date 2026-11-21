package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func usablePerfMetricGroups(group string) []string {
	usable := service.GetUserUsableGroups(group)
	groups := make([]string, 0, len(usable))
	for name := range usable {
		if name == "auto" || ratio_setting.ContainsGroupRatio(name) {
			groups = append(groups, name)
		}
	}
	return groups
}

func GetPerfMetricsSummary(c *gin.Context) {
	role, group := catalogViewer(c)
	getPerfMetricsSummary(c, usablePerfMetricGroups(group), &role)
}

func GetDashboardPerfMetricsSummary(c *gin.Context) {
	user := currentHealthUser(c)
	if user == nil {
		return
	}
	if user.Role < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权进行此操作，权限不足"})
		return
	}
	getPerfMetricsSummary(c, append(lo.Keys(ratio_setting.GetGroupRatioCopy()), "auto"), nil)
}

func getPerfMetricsSummary(c *gin.Context, groups []string, viewerRole *int) {
	c.Header("Cache-Control", "private, no-store")
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.QuerySummaryAll(hours, groups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if viewerRole != nil {
		result.Models = lo.Filter(result.Models, func(m perfmetrics.ModelSummary, _ int) bool {
			return model.IsModelVisibleToRole(m.ModelName, *viewerRole)
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	c.Header("Cache-Control", "private, no-store")
	role, group := catalogViewer(c)
	allowed := usablePerfMetricGroups(group)
	if !model.IsModelVisibleToRole(modelName, role) {
		allowed = []string{}
	}
	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		AllowedGroups: allowed,
		Model:         modelName,
		Group:         c.Query("group"),
		Hours:         hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
