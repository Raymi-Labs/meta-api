package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAllQuotaDates(c *gin.Context) {
	getDashboardUsage(c, false, false)
}

func GetQuotaDatesByUser(c *gin.Context) {
	getDashboardUsage(c, false, true)
}

func getDashboardUsage(c *gin.Context, self, byUser bool) {
	start, startErr := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, endErr := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startErr != nil || endErr != nil || start <= 0 || end <= start || end-start > 31*86400 || (self && end-start > 30*86400) {
		common.ApiError(c, errors.New("请选择有效且不超过允许天数的时间范围"))
		return
	}
	userID, username := 0, c.Query("username")
	if self {
		userID, username = c.GetInt("id"), ""
		if userID <= 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "用户未认证"})
			return
		}
	}
	usage, err := model.GetCachedDashboardUsage(c.Request.Context(), start, end, userID, username, byUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usage.Rows,
		"summary": usage.Summary,
	})
}

func GetModelChannelTagStats(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	username := c.Query("username")

	stats, err := model.GetDashboardModelChannelTagStats(startTimestamp, endTimestamp, username, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

func GetUserBalanceTrend(c *gin.Context) {
	days, _ := strconv.Atoi(c.Query("days"))
	if days > 0 {
		report, err := model.GetUserBalanceSnapshotReportByDays(days)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    report,
		})
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	report, err := model.GetUserBalanceSnapshotReport(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    report,
	})
}

func GetUserBalanceTrendUsers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	users, total, err := model.SearchUserBalanceTrendUsers(model.UserBalanceTrendUserSearchParams{
		Keyword:       c.Query("keyword"),
		IncludeStatus: c.Query("include_status"),
		StartIdx:      pageInfo.GetStartIdx(),
		PageSize:      pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
}

type UpdateUserBalanceTrendUserRequest struct {
	BalanceTrendDisabled bool `json:"balance_trend_disabled"`
}

func UpdateUserBalanceTrendUser(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiError(c, errors.New("无效的用户 ID"))
		return
	}
	var req UpdateUserBalanceTrendUserRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.UpdateUserBalanceTrendDisabled(userId, req.BalanceTrendDisabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, user)
}

func GetUserQuotaDates(c *gin.Context) {
	getDashboardUsage(c, true, false)
}
