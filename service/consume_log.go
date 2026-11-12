package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func FinalizeConsumeLogAfterSettle(logContent string, other map[string]interface{}, actualQuota int, relayInfo *relaycommon.RelayInfo, settleErr error) (int, string, map[string]interface{}) {
	loggedQuota := actualQuota
	if settleErr == nil || relayInfo == nil || relayInfo.BillingSource != BillingSourceToken {
		return loggedQuota, logContent, other
	}

	loggedQuota = relayInfo.FinalPreConsumedQuota
	if loggedQuota < 0 {
		loggedQuota = 0
	}
	if loggedQuota == actualQuota {
		return loggedQuota, logContent, other
	}

	if other == nil {
		other = make(map[string]interface{})
	}
	other["actual_quota"] = actualQuota
	other["charged_quota"] = loggedQuota
	other["settle_failed"] = true
	other["settle_error"] = settleErr.Error()

	var note string
	if loggedQuota < actualQuota {
		note = fmt.Sprintf("实际消耗 %s，但令牌仅成功扣费 %s（超额部分未成功结算）", logger.FormatQuota(actualQuota), logger.FormatQuota(loggedQuota))
	} else {
		note = fmt.Sprintf("实际消耗 %s，但令牌仍按 %s 扣费（返还未成功）", logger.FormatQuota(actualQuota), logger.FormatQuota(loggedQuota))
	}

	if strings.TrimSpace(logContent) == "" {
		logContent = note
	} else {
		logContent += "，" + note
	}
	return loggedQuota, logContent, other
}

// RecordRelayUsageStats 记一次同步请求的用量统计。企业请求花的是企业预算，
// 只计个人的请求次数，不计入个人的已用额度；渠道统计照常。
func RecordRelayUsageStats(info *relaycommon.RelayInfo, quota int) {
	if info.EnterpriseId <= 0 {
		UpdateUsageStats(info.UserId, info.ChannelId, quota, true)
		return
	}
	model.UpdateUserRequestCount(info.UserId)
	RecordChannelPeriodCount(info.ChannelId)
	if quota != 0 {
		model.UpdateChannelUsedQuota(info.ChannelId, quota)
	}
}

func UpdateUsageStats(userId int, channelId int, quota int, countRequest bool) {
	if countRequest {
		model.UpdateUserUsedQuotaAndRequestCount(userId, quota)
		RecordChannelPeriodCount(channelId)
	} else if quota != 0 {
		model.UpdateUserUsedQuota(userId, quota)
	}
	if quota != 0 {
		model.UpdateChannelUsedQuota(channelId, quota)
	}
}
