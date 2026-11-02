package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

var upstreamAccountErrorMap = map[error]string{
	model.ErrUpstreamAccountRemoteMissingURL:      "远端额度观测已启用，但缺少远端地址",
	model.ErrUpstreamAccountRemoteMissingUID:      "远端额度观测已启用，但缺少远端用户 ID",
	model.ErrUpstreamAccountRemoteMissingToken:    "远端额度观测已启用，但缺少远端 access token",
	model.ErrUpstreamAccountRemoteMissingEmail:    "远端额度观测已启用，但缺少远端邮箱",
	model.ErrUpstreamAccountRemoteMissingPassword: "远端额度观测已启用，但缺少远端密码",
	model.ErrUpstreamAccountRemoteTokenEmpty:      "远端 access token 为空",
	model.ErrUpstreamAccountRemoteNotNewAPI:       "远端不是受支持的 new-api 实例",
	model.ErrUpstreamAccountRemoteURLInvalid:      "远端额度地址不可用",
	model.ErrUpstreamAccountRemoteRequestFail:     "远端请求失败",
	model.ErrUpstreamAccountTypeUnsupported:       "当前仅支持 new-api、sub2api 和 OpenRouter 上游账户",
	model.ErrUpstreamAccountNameEmpty:             "上游账户名称不能为空",
	model.ErrUpstreamAccountInvalid:               "无效的上游账户",
	model.ErrUpstreamAccountTokenEmpty:            "上游 access token 不能为空",
	model.ErrUpstreamAccountEmailEmpty:            "sub2api 邮箱不能为空",
	model.ErrUpstreamAccountPasswordEmpty:         "sub2api 密码不能为空",
}

func upstreamAccountApiError(c *gin.Context, err error) {
	for sentinel, message := range upstreamAccountErrorMap {
		if errors.Is(err, sentinel) {
			common.ApiErrorMsg(c, message)
			return
		}
	}
	common.ApiError(c, err)
}

func GetUpstreamAccounts(c *gin.Context) {
	accounts, err := model.GetUpstreamAccountOptions()
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, accounts)
}

func SaveUpstreamAccount(c *gin.Context) {
	account := model.UpstreamAccount{}
	if err := c.ShouldBindJSON(&account); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if rawID := strings.TrimSpace(c.Param("id")); rawID != "" {
		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			common.ApiErrorMsg(c, "无效的上游账户")
			return
		}
		account.Id = id
	}
	saved, err := model.SaveUpstreamAccount(account)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, saved)
}

func DeleteUpstreamAccount(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账户")
		return
	}
	if err := model.DeleteUpstreamAccount(id); err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": true})
}

func SyncUpstreamAccount(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账户")
		return
	}
	account, syncErr := model.SyncUpstreamAccount(id, true)
	if syncErr != nil {
		upstreamAccountApiError(c, syncErr)
		return
	}
	common.ApiSuccess(c, account)
}

func SyncAllUpstreamAccounts(c *gin.Context) {
	accounts, err := model.SyncAllUpstreamAccounts(true)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, accounts)
}

func GetUpstreamAccountTrend(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账户")
		return
	}
	startTimestamp, _ := strconv.ParseInt(strings.TrimSpace(c.Query("start_timestamp")), 10, 64)
	endTimestamp, _ := strconv.ParseInt(strings.TrimSpace(c.Query("end_timestamp")), 10, 64)
	customIntervalMinutes, _ := strconv.Atoi(strings.TrimSpace(c.Query("custom_interval_minutes")))
	trend, trendErr := model.GetUpstreamAccountTrend(
		id,
		startTimestamp,
		endTimestamp,
		c.DefaultQuery("granularity", "day"),
		customIntervalMinutes,
	)
	if trendErr != nil {
		upstreamAccountApiError(c, trendErr)
		return
	}
	common.ApiSuccess(c, trend)
}

func GetOpenRouterAccountFinance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账户")
		return
	}
	data, err := model.GetOpenRouterAccountFinance(c.Request.Context(), id)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}
