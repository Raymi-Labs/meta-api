package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}
	if channel, err := model.GetChannelById(channelError.ChannelId, true); err == nil {
		if claimed, claimErr := model.ClaimCRSAutoDisabledChannel(channel, reason); claimErr != nil {
			common.SysError(fmt.Sprintf("claim CRS auto-disabled channel failed: channel_id=%d err=%v", channelError.ChannelId, claimErr))
		} else if claimed {
			common.SysLog(fmt.Sprintf("通道「%s」（#%d）存在新的自动禁用原因，已取消 CRS 自动恢复所有权", channelError.ChannelName, channelError.ChannelId))
			return
		}
	}

	if err := model.ClearChannelPreDisable(channelError.ChannelId, channelError.UsingKey); err != nil {
		common.SysError(fmt.Sprintf("clear channel pending disable failed: channel_id=%d err=%v", channelError.ChannelId, err))
	}
	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	if err := model.ClearChannelPreDisable(channelId, usingKey); err != nil {
		common.SysError(fmt.Sprintf("clear channel pending disable failed on enable: channel_id=%d err=%v", channelId, err))
	}
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

// RelayErrorSource distinguishes provider failures from local validation before applying health rules.
func RelayErrorSource(err *types.NewAPIError) string {
	if err == nil {
		return ""
	}
	if _, ok := relaycommon.AsParamOverrideReturnError(err); ok {
		return "local"
	}
	switch {
	case types.IsChannelError(err):
		return "channel"
	case err.GetErrorCode() == types.ErrorCodeDoRequestFailed:
		return "transport"
	case err.UpstreamStatusCode > 0 || err.GetErrorType() != types.ErrorTypeNewAPIError:
		return "upstream"
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeReadResponseBodyFailed, types.ErrorCodeBadResponseStatusCode, types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody, types.ErrorCodeEmptyResponse, types.ErrorCodeAwsInvokeError:
		return "upstream"
	}
	var inner *types.NewAPIError
	if errors.As(err.Unwrap(), &inner) && inner != err {
		return RelayErrorSource(inner)
	}
	return "local"
}

func ShouldDisableChannel(channelType int, err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	switch RelayErrorSource(err) {
	case "channel":
		return true
	case "local":
		return false
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldUseUserScopedCircuitBreakerByStatusCode(err.StatusCode) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}
	//if err.StatusCode == http.StatusUnauthorized {
	//	return true
	//}
	if err.StatusCode == http.StatusForbidden {
		switch channelType {
		case constant.ChannelTypeGemini:
			return true
		}
	}
	oaiErr := err.ToOpenAIError()
	switch oaiErr.Code {
	case "invalid_api_key":
		return true
	case "account_deactivated":
		return true
	case "billing_not_active":
		return true
	case "pre_consume_token_quota_failed":
		return true
	case "Arrearage":
		return true
	}
	switch oaiErr.Type {
	case "insufficient_quota":
		return true
	case "insufficient_user_quota":
		return true
	// https://docs.anthropic.com/claude/reference/errors
	case "authentication_error":
		return true
	case "permission_error":
		return true
	case "forbidden":
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}

// IsQuotaRelatedError 判断是否为“额度相关”的上游错误。
func IsQuotaRelatedError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}

	switch err.GetErrorCode() {
	case types.ErrorCodeInsufficientUserQuota, types.ErrorCodePreConsumeTokenQuotaFailed:
		return true
	}

	oaiErr := err.ToOpenAIError()
	if code := strings.ToLower(fmt.Sprintf("%v", oaiErr.Code)); code != "" {
		switch code {
		case "insufficient_quota", "insufficient_user_quota", "billing_not_active", "pre_consume_token_quota_failed":
			return true
		}
	}

	switch strings.ToLower(oaiErr.Type) {
	case "insufficient_quota", "insufficient_user_quota":
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	if strings.Contains(lowerMessage, "insufficient quota") ||
		strings.Contains(lowerMessage, "quota is not enough") ||
		strings.Contains(lowerMessage, "billing_not_active") ||
		strings.Contains(lowerMessage, "额度不足") ||
		strings.Contains(lowerMessage, "余额不足") {
		return true
	}

	return false
}

func IsChannelModelMismatchError(err *types.NewAPIError) bool {
	if err == nil || RelayErrorSource(err) == "local" {
		return false
	}

	lowerMessage := strings.ToLower(err.Error())
	if strings.Contains(lowerMessage, "stream must be set to true") {
		return true
	}
	if strings.Contains(lowerMessage, "model is not supported when using codex with a chatgpt account") {
		return true
	}
	if strings.Contains(lowerMessage, "endpoint not supported") {
		return true
	}
	if strings.Contains(lowerMessage, "only /v1/responses") && strings.Contains(lowerMessage, "codex channel") {
		return true
	}
	if strings.Contains(lowerMessage, "not supported") && strings.Contains(lowerMessage, "codex") {
		return true
	}

	return false
}

func IsUpstreamModelTemporaryUnavailableError(err *types.NewAPIError) bool {
	if err == nil || RelayErrorSource(err) == "local" {
		return false
	}

	lowerMessage := normalizeUpstreamErrorMessage(err)
	if strings.Contains(lowerMessage, "no available") &&
		strings.Contains(lowerMessage, "support the requested model") {
		return true
	}

	return strings.Contains(lowerMessage, "at capacity") ||
		(strings.Contains(lowerMessage, "temporarily unavailable") &&
			(strings.Contains(lowerMessage, "model") || strings.Contains(lowerMessage, "service"))) ||
		(strings.Contains(lowerMessage, "try a different model") &&
			(strings.Contains(lowerMessage, "selected model") || strings.Contains(lowerMessage, "capacity")))
}

func IsUpstreamAccountPoolUnavailableError(err *types.NewAPIError) bool {
	if err == nil || types.IsSkipRetryError(err) {
		return false
	}
	lowerMessage := normalizeUpstreamErrorMessage(err)
	return strings.Contains(lowerMessage, "no available") &&
		strings.Contains(lowerMessage, "accounts") &&
		strings.Contains(lowerMessage, "group")
}

func IsRequestedModelUnavailableError(err *types.NewAPIError) bool {
	return IsUpstreamModelTemporaryUnavailableError(err)
}

func IsRetryableSpecialBadRequestError(err *types.NewAPIError) bool {
	if err == nil || err.StatusCode != http.StatusBadRequest {
		return false
	}

	if IsRequestedModelUnavailableError(err) {
		return true
	}

	lowerMessage := normalizeUpstreamErrorMessage(err)
	return strings.Contains(lowerMessage, "an error occurred while processing your request") &&
		strings.Contains(lowerMessage, "you can retry your request") &&
		strings.Contains(lowerMessage, "help.openai.com") &&
		strings.Contains(lowerMessage, "please include the request id")
}

func normalizeUpstreamErrorMessage(err *types.NewAPIError) string {
	if err == nil {
		return ""
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = strings.TrimSpace(err.ToOpenAIError().Message)
	}
	if message == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(message), " "))
}
