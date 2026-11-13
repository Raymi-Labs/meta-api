package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	scope, err := parseAdminLogScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, scope)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	scope, err := userLogPayerScope(c, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	logs, total, err := model.GetUserLogsFiltered(userId, model.AdminLogQueryFilters{
		Scope: scope, LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp,
		ModelName: modelName, TokenName: tokenName, Group: group, RequestID: requestId,
	}, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// SearchAllLogs 保留兼容入口，当前仅返回接口已废弃提示。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// SearchUserLogs 保留兼容入口，当前仅返回接口已废弃提示。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	scope, err := parseAdminLogScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestID := c.Query("request_id")
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestID, true, scope)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":             stat.Quota,
			"rpm":               stat.Rpm,
			"tpm":               stat.Tpm,
			"cache_hit_rate":    stat.CacheHitRate,
			"cache_global_rate": stat.CacheGlobalRate,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestID := c.Query("request_id")
	scope, err := userLogPayerScope(c, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestID, false, scope)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":             quotaNum.Quota,
			"rpm":               quotaNum.Rpm,
			"tpm":               quotaNum.Tpm,
			"cache_hit_rate":    quotaNum.CacheHitRate,
			"cache_global_rate": quotaNum.CacheGlobalRate,
			//"token": tokenNum,
		},
	})
	return
}

func GetTopUsers(c *gin.Context) {
	scope, err := parseAdminLogScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	username := c.Query("username")
	tokenName := c.Query("token_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestID := c.Query("request_id")
	viewMode := c.DefaultQuery("view_mode", "both")
	quotaOrder := c.DefaultQuery("quota_order", "desc")
	requestOrder := c.DefaultQuery("request_order", "desc")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	filters := model.AdminLogQueryFilters{
		Scope:          scope,
		LogType:        model.LogTypeConsume,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      modelName,
		Username:       username,
		TokenName:      tokenName,
		Channel:        channel,
		Group:          group,
		RequestID:      requestID,
	}
	byQuota, byRequests, err := model.GetTopUsers(filters, limit, quotaOrder, requestOrder)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"by_quota":      byQuota,
		"by_requests":   byRequests,
		"view_mode":     viewMode,
		"quota_order":   quotaOrder,
		"request_order": requestOrder,
		"limit":         limit,
	})
}

func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	// 每批删除的行数取自与自动清理任务相同的配置，避免手动清理以远小于
	// 后台任务的批次反复往返日志库。
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, service.LogRetentionBatchSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}
