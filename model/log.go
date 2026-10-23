package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:1;index:idx_user_id_id,priority:2"`
	UserId           int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type;index:idx_channel_type_time,priority:3"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type;index:idx_channel_type_time,priority:2"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	IsStream         bool   `json:"is_stream"`
	ChannelId        int    `json:"channel" gorm:"index;index:idx_channel_type_time,priority:1"`
	ChannelName      string `json:"channel_name" gorm:"->"`
	TokenId          int    `json:"token_id" gorm:"default:0;index"`
	Group            string `json:"group" gorm:"index"`
	Ip               string `json:"ip" gorm:"index;default:''"`
	RequestId        string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	Other            string `json:"other"`
}

type PublicTokenLogItem struct {
	Id               int    `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	ModelName        string `json:"model_name"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
	UseTime          int    `json:"use_time"`
	IsStream         bool   `json:"is_stream"`
	Content          string `json:"content"`
}

type publicTokenLogQueryRow struct {
	PublicTokenLogItem
	Other string `gorm:"column:other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

var userVisibleAdminIdentityPattern = regexp.MustCompile(`管理员\(ID:\d+\)`)

func redactUserVisibleManageContent(content string) string {
	return userVisibleAdminIdentityPattern.ReplaceAllString(content, "管理员")
}

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		logs[i].Content = redactUserVisibleManageContent(logs[i].Content)
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			delete(otherMap, "reject_reason")
			delete(otherMap, "po")
			delete(otherMap, "pulse_funding")
		}
		if logs[i].Type == LogTypeError {
			// Error payloads used to include channel and upstream diagnostics at
			// the top level. Use an allowlist for both legacy and new user views.
			public := make(map[string]interface{})
			for _, key := range []string{"request_failure", "failure_category", "failure_reason", "failure_hint", "request_path", "error_code", "status_code", "http_status", "latency_ms", "retry_after_seconds"} {
				if value, ok := otherMap[key]; ok {
					public[key] = value
				}
			}
			if public["request_failure"] != true {
				logs[i].Content = "请求失败，请提供请求 ID 联系管理员排查。"
			} else if reason, ok := public["failure_reason"].(string); ok {
				logs[i].Content = reason
			}
			logs[i].ChannelId = 0
			otherMap = public
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = startIdx + i + 1
	}
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func GetPublicTokenLogsByTokenIDs(tokenIDs []int, startTimestamp int64, endTimestamp int64, offset int, limit int) (items []PublicTokenLogItem, total int64, err error) {
	if len(tokenIDs) == 0 {
		return items, 0, nil
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	countQuery := LOG_DB.Model(&Log{}).
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	rows := make([]publicTokenLogQueryRow, 0, limit)
	dataQuery := LOG_DB.Model(&Log{}).
		Select("id, created_at, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, content, other").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)

	if startTimestamp != 0 {
		countQuery = countQuery.Where("created_at >= ?", startTimestamp)
		dataQuery = dataQuery.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		countQuery = countQuery.Where("created_at <= ?", endTimestamp)
		dataQuery = dataQuery.Where("created_at <= ?", endTimestamp)
	}

	if err = countQuery.Count(&total).Error; err != nil {
		common.SysError("failed to count public token logs: " + err.Error())
		return items, 0, errors.New("查询调用明细失败")
	}

	if err = dataQuery.Order("id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		common.SysError("failed to query public token logs: " + err.Error())
		return items, 0, errors.New("查询调用明细失败")
	}

	items = make([]PublicTokenLogItem, 0, len(rows))
	for i := range rows {
		row := rows[i]
		row.Content = strings.TrimSpace(row.Content)
		row.CacheReadTokens, row.CacheWriteTokens = getPromptCacheSummaryFromOther(row.Other)
		items = append(items, row.PublicTokenLogItem)
	}

	return items, total, nil
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		log.Other = common.MapToJsonStr(map[string]interface{}{
			"admin_info": adminInfo,
		})
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordTopupLog records a top-up log (Type=LogTypeTopup) with administrator-only
// audit fields (server IP, caller IP, payment method, callback payment method,
// system version) stored under Other.admin_info. Non-admin views strip this
// field via formatUserLogs. Extras merge into admin_info and override the
// defaults when the same key is provided (e.g. admin_id/admin_username for
// manual top-ups).
func RecordTopupLog(userId int, content string, callerIp string,
	paymentMethod string, callbackPaymentMethod string,
	extras map[string]interface{}) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	for k, v := range extras {
		adminInfo[k] = v
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other: common.MapToJsonStr(map[string]interface{}{
			"admin_info": adminInfo,
		}),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId: requestId,
		Other:     otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	snapshot := unknownPulseFunding(params.Quota)
	if raw, exists := c.Get(PulseFundingContextKey); exists {
		if verified, ok := raw.(PulseFundingSnapshot); ok && verified.Status == "verified" && verified.UserID == userId && verified.RequestID == requestId && verified.PaidQuota > 0 && verified.PaidQuota <= int64(params.Quota) && verified.PaidQuota+verified.UnknownQuota == int64(params.Quota) {
			snapshot = verified
		}
	}
	otherStr := common.MapToJsonStr(withPulseFunding(params.Other, snapshot))
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId: requestId,
		Other:     otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	// RequestId links asynchronous task billing events to the original
	// consume log. It is intentionally a correlation key, not request content.
	RequestId string
	Other     map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		RequestId: params.RequestId,
		Other:     common.MapToJsonStr(withPulseFunding(params.Other, unknownPulseFunding(params.Quota))),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, scopes ...LogScope) (logs []*Log, total int64, err error) {
	filters := AdminLogQueryFilters{
		LogType:        logType,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      modelName,
		Username:       username,
		TokenName:      tokenName,
		Channel:        channel,
		Group:          group,
		RequestID:      requestId,
	}
	tx, err := applyAdminLogFilters(applyLogScope(LOG_DB, scopes...), filters, true)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string) (logs []*Log, total int64, err error) {
	tx, err := applyUserLogFilters(LOG_DB, userId, AdminLogQueryFilters{
		LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp,
		ModelName: modelName, TokenName: tokenName, Group: group, RequestID: requestId,
	})
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type Stat struct {
	Quota           int     `json:"quota"`
	Rpm             int     `json:"rpm"`
	Tpm             int     `json:"tpm"`
	CacheHitRate    float64 `json:"cache_hit_rate"`
	CacheGlobalRate float64 `json:"cache_global_rate"`
}

type logCacheRateRow struct {
	PromptTokens int    `gorm:"column:prompt_tokens"`
	Other        string `gorm:"column:other"`
}

func getDisplayedInputTokens(promptTokens, cacheReadTokens, cacheWriteTokens int, other tokenUsageOtherInfo) int {
	if promptTokens < 0 {
		promptTokens = 0
	}
	if promptTokensExcludeCache(promptTokens, cacheReadTokens, cacheWriteTokens, other) {
		return promptTokens + cacheReadTokens + cacheWriteTokens
	}
	return promptTokens
}

// cacheRateAccumulator 以逐行累加的方式统计缓存率，使调用方不必把结果集
// 全部读入内存。缓存率是比值，只依赖这几个累计量，与行的到达顺序无关。
type cacheRateAccumulator struct {
	globalReadTokens  int
	globalInputTokens int
	hitReadTokens     int
	hitInputTokens    int
}

func (a *cacheRateAccumulator) add(row logCacheRateRow) {
	other := tokenUsageOtherInfo{}
	if row.Other != "" {
		_ = common.UnmarshalJsonStr(row.Other, &other)
	}

	cacheReadTokens := other.CacheTokens
	if cacheReadTokens < 0 {
		cacheReadTokens = 0
	}
	cacheWriteTokens := sumCacheCreationTokens(other)
	if cacheWriteTokens < 0 {
		cacheWriteTokens = 0
	}

	displayedInputTokens := getDisplayedInputTokens(
		row.PromptTokens,
		cacheReadTokens,
		cacheWriteTokens,
		other,
	)

	a.globalReadTokens += cacheReadTokens
	a.globalInputTokens += displayedInputTokens
	if cacheReadTokens > 0 {
		a.hitReadTokens += cacheReadTokens
		a.hitInputTokens += displayedInputTokens
	}
}

func (a *cacheRateAccumulator) rates() (cacheHitRate float64, cacheGlobalRate float64) {
	if a.globalInputTokens > 0 {
		cacheGlobalRate = float64(a.globalReadTokens) / float64(a.globalInputTokens)
	}
	if a.hitInputTokens > 0 {
		cacheHitRate = float64(a.hitReadTokens) / float64(a.hitInputTokens)
	}
	return cacheHitRate, cacheGlobalRate
}

func calculateCacheRates(rows []logCacheRateRow) (cacheHitRate float64, cacheGlobalRate float64) {
	accumulator := cacheRateAccumulator{}
	for _, row := range rows {
		accumulator.add(row)
	}
	return accumulator.rates()
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, requestID string, fuzzyUsername bool, scopes ...LogScope) (stat Stat, err error) {
	tx := LOG_DB.Table("logs").Select("sum(quota) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")
	cacheRateQuery := LOG_DB.Table("logs").Select("prompt_tokens, other")

	if tx, err = applyLogUserFilter(tx, username, fuzzyUsername); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyLogUserFilter(rpmTpmQuery, username, fuzzyUsername); err != nil {
		return stat, err
	}
	if cacheRateQuery, err = applyLogUserFilter(cacheRateQuery, username, fuzzyUsername); err != nil {
		return stat, err
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
		cacheRateQuery = cacheRateQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
		cacheRateQuery = cacheRateQuery.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
		cacheRateQuery = cacheRateQuery.Where("created_at <= ?", endTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "model_name", modelName); err != nil {
		return stat, err
	}
	if cacheRateQuery, err = applyExplicitLogTextFilter(cacheRateQuery, "model_name", modelName); err != nil {
		return stat, err
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
		cacheRateQuery = cacheRateQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
		cacheRateQuery = cacheRateQuery.Where(logGroupCol+" = ?", group)
	}
	if requestID != "" {
		tx = tx.Where("request_id = ?", requestID)
		rpmTpmQuery = rpmTpmQuery.Where("request_id = ?", requestID)
		cacheRateQuery = cacheRateQuery.Where("request_id = ?", requestID)
	}

	tx = applyLogScope(tx, scopes...)
	rpmTpmQuery = applyLogScope(rpmTpmQuery, scopes...)
	cacheRateQuery = applyLogScope(cacheRateQuery, scopes...)
	// RPM/TPM remain a recent-minute metric, intersected with the selected range.
	if startTimestamp != 0 {
		rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		rpmTpmQuery = rpmTpmQuery.Where("created_at <= ?", endTimestamp)
	}
	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)
	cacheRateQuery = cacheRateQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	// Scan resets its destination in newer GORM versions. Keep the quota
	// aggregate separate from the recent-minute throughput aggregate.
	var throughput Stat
	if err := rpmTpmQuery.Scan(&throughput).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	stat.Rpm, stat.Tpm = throughput.Rpm, throughput.Tpm
	stat.CacheHitRate, stat.CacheGlobalRate, err = scanCacheRates(cacheRateQuery)
	if err != nil {
		return stat, err
	}

	return stat, nil
}

// cacheRateScanLimit 限制缓存率统计读取的日志行数。
//
// 缓存率必须逐行解析 other 中的 JSON 才能算出，无法下推为跨数据库通用的聚合，
// 所以行数直接决定这次查询的代价。不设上限时，一个宽时间范围的统计请求会把
// 该范围内的全部日志行连同 other 列拉进内存，在日志量大的部署上足以拖垮
// 日志库。取最近的若干行计算，得到的是同一口径下的近似值；上限可通过
// LOG_CACHE_RATE_SCAN_LIMIT 调整，设为 0 表示不限制。
func cacheRateScanLimit() int {
	return common.GetEnvOrDefault("LOG_CACHE_RATE_SCAN_LIMIT", 50000)
}

// scanCacheRates 流式读取缓存率统计所需的行，逐行累加而不缓冲整个结果集。
func scanCacheRates(query *gorm.DB) (cacheHitRate float64, cacheGlobalRate float64, err error) {
	if limit := cacheRateScanLimit(); limit > 0 {
		// 按最新的行取样，使结果反映当前的缓存表现。
		query = query.Order("created_at DESC").Limit(limit)
	}

	rows, err := query.Rows()
	if err != nil {
		common.SysError("failed to query log cache stat: " + err.Error())
		return 0, 0, errors.New("查询统计数据失败")
	}
	defer rows.Close()

	accumulator := cacheRateAccumulator{}
	for rows.Next() {
		var promptTokens sql.NullInt64
		var other sql.NullString
		// 历史行的 other 可能为 NULL，此处不能直接扫描进 string。
		if err := rows.Scan(&promptTokens, &other); err != nil {
			common.SysError("failed to scan log cache stat: " + err.Error())
			return 0, 0, errors.New("查询统计数据失败")
		}
		accumulator.add(logCacheRateRow{
			PromptTokens: int(promptTokens.Int64),
			Other:        other.String,
		})
	}
	if err := rows.Err(); err != nil {
		common.SysError("failed to iterate log cache stat: " + err.Error())
		return 0, 0, errors.New("查询统计数据失败")
	}

	cacheHitRate, cacheGlobalRate = accumulator.rates()
	return cacheHitRate, cacheGlobalRate, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

type ModelStat struct {
	ModelName        string `json:"model" gorm:"column:model_name"`
	Count            int    `json:"count"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Quota            int    `json:"quota"`
}

type TokenUsageTokens struct {
	PromptTokens     int `json:"prompt_tokens" gorm:"column:prompt_tokens"`
	CompletionTokens int `json:"completion_tokens" gorm:"column:completion_tokens"`
}

type PublicTokenDistribution struct {
	InputTokens            int  `json:"input_tokens"`
	CompletionTokens       int  `json:"completion_tokens"`
	CacheReadTokens        int  `json:"cache_read_tokens"`
	CacheCreationTokens    int  `json:"cache_creation_tokens"`
	TotalTokens            int  `json:"total_tokens"`
	CacheCreationSupported bool `json:"cache_creation_supported"`
}

type tokenUsageLogRow struct {
	TokenID          int    `gorm:"column:token_id"`
	PromptTokens     int    `gorm:"column:prompt_tokens"`
	CompletionTokens int    `gorm:"column:completion_tokens"`
	Other            string `gorm:"column:other"`
}

type tokenStatRow struct {
	TokenID int `gorm:"column:token_id"`
	Stat
}

type tokenUsageRow struct {
	TokenID int `gorm:"column:token_id"`
	TokenUsageTokens
}

type tokenCountRow struct {
	TokenID int   `gorm:"column:token_id"`
	Count   int64 `gorm:"column:count"`
}

type tokenModelStatRow struct {
	TokenID int `gorm:"column:token_id"`
	ModelStat
}

type tokenUsageOtherInfo struct {
	CacheTokens           int      `json:"cache_tokens"`
	CacheCreationTokens   int      `json:"cache_creation_tokens"`
	CacheCreationTokens5m int      `json:"cache_creation_tokens_5m"`
	CacheCreationTokens1h int      `json:"cache_creation_tokens_1h"`
	Claude                bool     `json:"claude"`
	UsageSemantic         string   `json:"usage_semantic"`
	PromptTokensSemantic  string   `json:"prompt_tokens_semantic"`
	BillingMode           string   `json:"billing_mode"`
	RequestConversion     []string `json:"request_conversion"`
	TieredParams          *struct {
		Len *float64 `json:"len"`
	} `json:"tiered_params"`
}

// promptTokensExcludeCache interprets the stored prompt, not the client route.
// Legacy Messages logs marked every upstream as Anthropic, while OpenRouter
// additionally removed caches before storing prompt. A conversion chain alone
// cannot distinguish them. The saved tier input length can prove inclusivity.
func promptTokensExcludeCache(promptTokens, cacheReadTokens, cacheWriteTokens int, other tokenUsageOtherInfo) bool {
	switch other.PromptTokensSemantic {
	case "inclusive":
		return false
	case "non_cache":
		return true
	}
	legacyClaude := other.Claude || strings.EqualFold(other.UsageSemantic, "anthropic")
	chain := other.RequestConversion
	if legacyClaude && promptTokens >= 0 && (cacheReadTokens > 0 || cacheWriteTokens > 0) &&
		other.BillingMode == "tiered_expr" && len(chain) >= 2 && chain[0] == "Claude Messages" &&
		(chain[len(chain)-1] == "OpenAI Compatible" || chain[len(chain)-1] == "OpenAI Responses") &&
		other.TieredParams != nil && other.TieredParams.Len != nil && *other.TieredParams.Len == float64(promptTokens) {
		return false
	}
	return legacyClaude
}

func sumCacheCreationTokens(other tokenUsageOtherInfo) int {
	if other.CacheCreationTokens > 0 {
		return other.CacheCreationTokens
	}
	return other.CacheCreationTokens5m + other.CacheCreationTokens1h
}

func supportsCacheCreationUsage(rawOther string, other tokenUsageOtherInfo) bool {
	if other.Claude || strings.EqualFold(other.UsageSemantic, "anthropic") {
		return true
	}
	if rawOther == "" {
		return false
	}
	return strings.Contains(rawOther, "\"cache_creation_tokens\"") ||
		strings.Contains(rawOther, "\"cache_creation_tokens_5m\"") ||
		strings.Contains(rawOther, "\"cache_creation_tokens_1h\"")
}

func getPromptCacheSummaryFromOther(rawOther string) (cacheReadTokens int, cacheWriteTokens int) {
	if rawOther == "" {
		return 0, 0
	}

	other := tokenUsageOtherInfo{}
	if err := common.UnmarshalJsonStr(rawOther, &other); err != nil {
		return 0, 0
	}

	cacheReadTokens = other.CacheTokens
	if cacheReadTokens < 0 {
		cacheReadTokens = 0
	}

	cacheWriteTokens = sumCacheCreationTokens(other)
	if cacheWriteTokens < 0 {
		cacheWriteTokens = 0
	}

	return cacheReadTokens, cacheWriteTokens
}

func normalizeInputTokens(promptTokens, cacheReadTokens, cacheCreationTokens int, other tokenUsageOtherInfo) int {
	if promptTokensExcludeCache(promptTokens, cacheReadTokens, cacheCreationTokens, other) {
		if promptTokens < 0 {
			return 0
		}
		return promptTokens
	}

	inputTokens := promptTokens - cacheReadTokens - cacheCreationTokens
	if inputTokens < 0 {
		return 0
	}
	return inputTokens
}

func aggregatePublicTokenDistributionRows(rows []tokenUsageLogRow) PublicTokenDistribution {
	distribution := PublicTokenDistribution{}

	for _, row := range rows {
		other := tokenUsageOtherInfo{}
		if row.Other != "" {
			_ = common.UnmarshalJsonStr(row.Other, &other)
		}

		cacheReadTokens := other.CacheTokens
		if cacheReadTokens < 0 {
			cacheReadTokens = 0
		}
		cacheCreationTokens := sumCacheCreationTokens(other)
		if cacheCreationTokens < 0 {
			cacheCreationTokens = 0
		}
		inputTokens := normalizeInputTokens(row.PromptTokens, cacheReadTokens, cacheCreationTokens, other)

		distribution.InputTokens += inputTokens
		distribution.CompletionTokens += row.CompletionTokens
		distribution.CacheReadTokens += cacheReadTokens
		distribution.CacheCreationTokens += cacheCreationTokens
		if !distribution.CacheCreationSupported && supportsCacheCreationUsage(row.Other, other) {
			distribution.CacheCreationSupported = true
		}
	}

	distribution.TotalTokens = distribution.InputTokens +
		distribution.CompletionTokens +
		distribution.CacheReadTokens +
		distribution.CacheCreationTokens

	return distribution
}

func SumPublicTokenDistributionByTokenIDs(tokenIDs []int, startTimestamp int64, endTimestamp int64) (distribution PublicTokenDistribution, err error) {
	if len(tokenIDs) == 0 {
		return distribution, nil
	}

	rows := make([]tokenUsageLogRow, 0)
	tx := LOG_DB.Table("logs").
		Select("prompt_tokens, completion_tokens, other").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)

	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	if err := tx.Scan(&rows).Error; err != nil {
		common.SysError("failed to query public token distribution by token ids: " + err.Error())
		return distribution, errors.New("查询统计数据失败")
	}

	return aggregatePublicTokenDistributionRows(rows), nil
}

func SumPublicTokenDistributionByTokenIDsMap(tokenIDs []int, startTimestamp int64, endTimestamp int64) (map[int]PublicTokenDistribution, error) {
	distributionByID := make(map[int]PublicTokenDistribution, len(tokenIDs))
	if len(tokenIDs) == 0 {
		return distributionByID, nil
	}

	rows := make([]tokenUsageLogRow, 0)
	tx := LOG_DB.Table("logs").
		Select("token_id, prompt_tokens, completion_tokens, other").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	if err := tx.Scan(&rows).Error; err != nil {
		common.SysError("failed to query grouped public token distribution by token ids: " + err.Error())
		return distributionByID, errors.New("查询统计数据失败")
	}

	rowsByID := make(map[int][]tokenUsageLogRow, len(tokenIDs))
	for _, row := range rows {
		rowsByID[row.TokenID] = append(rowsByID[row.TokenID], row)
	}
	for tokenID, tokenRows := range rowsByID {
		distributionByID[tokenID] = aggregatePublicTokenDistributionRows(tokenRows)
	}
	return distributionByID, nil
}

func GetModelStatsByTokenName(tokenName string, startTimestamp int64, endTimestamp int64) ([]ModelStat, error) {
	var stats []ModelStat
	tx := LOG_DB.Table("logs").
		Select("model_name, count(*) as count, sum(prompt_tokens) as prompt_tokens, sum(completion_tokens) as completion_tokens, sum(quota) as quota").
		Where("type = ?", LogTypeConsume).
		Where("token_name = ?", tokenName)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	err := tx.Group("model_name").Order("quota DESC").Find(&stats).Error
	return stats, err
}

func SumUsedQuotaByTokenIDs(tokenIDs []int, startTimestamp int64, endTimestamp int64) (stat Stat, err error) {
	if len(tokenIDs) == 0 {
		return stat, nil
	}

	tx := LOG_DB.Table("logs").Select("COALESCE(sum(quota), 0) quota")
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) tpm")

	tx = tx.Where("type = ?", LogTypeConsume).Where("token_id IN ?", tokenIDs)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume).Where("token_id IN ?", tokenIDs)

	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat by token ids: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	var throughput Stat
	if err := rpmTpmQuery.Scan(&throughput).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat by token ids: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	stat.Rpm, stat.Tpm = throughput.Rpm, throughput.Tpm

	return stat, nil
}

func SumUsedQuotaByTokenIDsMap(tokenIDs []int, startTimestamp int64, endTimestamp int64) (map[int]Stat, error) {
	stats := make(map[int]Stat, len(tokenIDs))
	if len(tokenIDs) == 0 {
		return stats, nil
	}

	rows := make([]tokenStatRow, 0, len(tokenIDs))
	tx := LOG_DB.Table("logs").
		Select("token_id, COALESCE(sum(quota), 0) as quota").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if err := tx.Group("token_id").Scan(&rows).Error; err != nil {
		common.SysError("failed to query grouped log stat by token ids: " + err.Error())
		return stats, errors.New("查询统计数据失败")
	}
	for _, row := range rows {
		stats[row.TokenID] = row.Stat
	}
	return stats, nil
}

func SumUsedTokenDetailsByTokenIDs(tokenIDs []int, startTimestamp int64, endTimestamp int64) (tokens TokenUsageTokens, err error) {
	if len(tokenIDs) == 0 {
		return tokens, nil
	}

	tx := LOG_DB.Table("logs").
		Select("COALESCE(sum(prompt_tokens), 0) as prompt_tokens, COALESCE(sum(completion_tokens), 0) as completion_tokens").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)

	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	if err := tx.Scan(&tokens).Error; err != nil {
		common.SysError("failed to query token usage by token ids: " + err.Error())
		return tokens, errors.New("查询统计数据失败")
	}

	return tokens, nil
}

func SumUsedTokenDetailsByTokenIDsMap(tokenIDs []int, startTimestamp int64, endTimestamp int64) (map[int]TokenUsageTokens, error) {
	tokensByID := make(map[int]TokenUsageTokens, len(tokenIDs))
	if len(tokenIDs) == 0 {
		return tokensByID, nil
	}

	rows := make([]tokenUsageRow, 0, len(tokenIDs))
	tx := LOG_DB.Table("logs").
		Select("token_id, COALESCE(sum(prompt_tokens), 0) as prompt_tokens, COALESCE(sum(completion_tokens), 0) as completion_tokens").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if err := tx.Group("token_id").Scan(&rows).Error; err != nil {
		common.SysError("failed to query grouped token usage by token ids: " + err.Error())
		return tokensByID, errors.New("查询统计数据失败")
	}
	for _, row := range rows {
		tokensByID[row.TokenID] = row.TokenUsageTokens
	}
	return tokensByID, nil
}

func GetModelStatsByTokenIDs(tokenIDs []int, startTimestamp int64, endTimestamp int64) ([]ModelStat, error) {
	var stats []ModelStat
	if len(tokenIDs) == 0 {
		return stats, nil
	}

	tx := LOG_DB.Table("logs").
		Select("model_name, count(*) as count, COALESCE(sum(prompt_tokens), 0) as prompt_tokens, COALESCE(sum(completion_tokens), 0) as completion_tokens, COALESCE(sum(quota), 0) as quota").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	err := tx.Group("model_name").Order("quota DESC").Find(&stats).Error
	return stats, err
}

func GetModelStatsByTokenIDsMap(tokenIDs []int, startTimestamp int64, endTimestamp int64) (map[int][]ModelStat, error) {
	statsByID := make(map[int][]ModelStat, len(tokenIDs))
	if len(tokenIDs) == 0 {
		return statsByID, nil
	}

	rows := make([]tokenModelStatRow, 0)
	tx := LOG_DB.Table("logs").
		Select("token_id, model_name, count(*) as count, COALESCE(sum(prompt_tokens), 0) as prompt_tokens, COALESCE(sum(completion_tokens), 0) as completion_tokens, COALESCE(sum(quota), 0) as quota").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if err := tx.Group("token_id, model_name").Order("quota DESC").Scan(&rows).Error; err != nil {
		return statsByID, err
	}
	for _, row := range rows {
		statsByID[row.TokenID] = append(statsByID[row.TokenID], row.ModelStat)
	}
	return statsByID, nil
}

func CountLogsByTokenName(tokenName string, startTimestamp int64, endTimestamp int64) (int64, error) {
	var count int64
	tx := LOG_DB.Table("logs").
		Where("type = ?", LogTypeConsume).
		Where("token_name = ?", tokenName)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	err := tx.Count(&count).Error
	return count, err
}

func CountLogsByTokenIDs(tokenIDs []int, startTimestamp int64, endTimestamp int64) (int64, error) {
	var count int64
	if len(tokenIDs) == 0 {
		return 0, nil
	}

	tx := LOG_DB.Table("logs").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	err := tx.Count(&count).Error
	return count, err
}

func CountLogsByTokenIDsMap(tokenIDs []int, startTimestamp int64, endTimestamp int64) (map[int]int64, error) {
	counts := make(map[int]int64, len(tokenIDs))
	if len(tokenIDs) == 0 {
		return counts, nil
	}

	rows := make([]tokenCountRow, 0, len(tokenIDs))
	tx := LOG_DB.Table("logs").
		Select("token_id, count(*) as count").
		Where("type = ?", LogTypeConsume).
		Where("token_id IN ?", tokenIDs)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if err := tx.Group("token_id").Scan(&rows).Error; err != nil {
		return counts, err
	}
	for _, row := range rows {
		counts[row.TokenID] = row.Count
	}
	return counts, nil
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
