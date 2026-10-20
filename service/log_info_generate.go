package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func appendRequestPath(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if other == nil {
		return
	}
	if ctx != nil && ctx.Request != nil && ctx.Request.URL != nil {
		if path := ctx.Request.URL.Path; path != "" {
			other["request_path"] = path
			return
		}
	}
	if relayInfo != nil && relayInfo.RequestURLPath != "" {
		path := relayInfo.RequestURLPath
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		other["request_path"] = path
	}
}

func appendResponsesRequestDiagnostics(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || relayInfo.Request == nil {
		return
	}

	diagnostics := map[string]interface{}{
		"previous_response_id_present": false,
	}
	switch req := relayInfo.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		appendPreviousResponseIDDiagnostic(diagnostics, req.PreviousResponseID)
		appendRawDiagnosticHash(diagnostics, "input_hash", req.Input)
		appendRawDiagnosticHash(diagnostics, "instructions_hash", req.Instructions)
		appendRawDiagnosticHash(diagnostics, "prompt_cache_key_hash", req.PromptCacheKey)
	case *dto.OpenAIResponsesCompactionRequest:
		appendPreviousResponseIDDiagnostic(diagnostics, req.PreviousResponseID)
		appendRawDiagnosticHash(diagnostics, "input_hash", req.Input)
		appendRawDiagnosticHash(diagnostics, "instructions_hash", req.Instructions)
	default:
		return
	}
	other["responses_request_diagnostics"] = diagnostics
}

func appendPreviousResponseIDDiagnostic(diagnostics map[string]interface{}, previousResponseID string) {
	previousResponseID = strings.TrimSpace(previousResponseID)
	if previousResponseID == "" {
		return
	}
	diagnostics["previous_response_id_present"] = true
	diagnostics["previous_response_id_hash"] = diagnosticHashString(previousResponseID)
}

func appendRawDiagnosticHash(diagnostics map[string]interface{}, key string, raw []byte) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return
	}
	diagnostics[key] = diagnosticHashBytes(raw)
}

func diagnosticHashString(value string) string {
	return diagnosticHashBytes([]byte(value))
}

func diagnosticHashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_ratio"] = modelRatio
	other["group_ratio"] = groupRatio
	other["completion_ratio"] = completionRatio
	other["cache_tokens"] = cacheTokens
	other["cache_ratio"] = cacheRatio
	other["model_price"] = modelPrice
	other["user_group_ratio"] = userGroupRatio
	other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
	if !relayInfo.FirstEffectiveOutputTime.IsZero() && relayInfo.FirstEffectiveOutputTime.After(relayInfo.StartTime) {
		other["first_effective_output_ms"] = float64(relayInfo.FirstEffectiveOutputTime.Sub(relayInfo.StartTime).Milliseconds())
	}
	if relayInfo.ReasoningEffort != "" {
		other["reasoning_effort"] = relayInfo.ReasoningEffort
	}
	if relayInfo.ChatToolProtocol != "" && relayInfo.ChatToolProtocol != dto.ChatToolProtocolNone {
		other["chat_tool_protocol"] = relayInfo.ChatToolProtocol
		other["chat_tools_count"] = relayInfo.ChatToolCount
	}
	if relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}
	if relayInfo.ResponseModel != nil {
		observation := *relayInfo.ResponseModel
		other["response_model"] = observation
	}

	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other["is_system_prompt_overwritten"] = true
	}
	if common.GetContextKeyBool(ctx, constant.ContextKeyClaudeIncrementalCache) {
		other["claude_incremental_cache"] = true
	}

	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	if attempts, exists := ctx.Get(requestFailureAttemptsKey); exists {
		adminInfo["attempts"] = attempts
	}
	if relayInfo.KimiUsageEvidence != nil {
		// An immutable metadata snapshot, stripped by formatUserLogs.
		snapshot := *relayInfo.KimiUsageEvidence
		if snapshot.Cache.Value != nil {
			count := *snapshot.Cache.Value
			snapshot.Cache.Value = &count
		}
		if snapshot.Reasoning.Value != nil {
			count := *snapshot.Reasoning.Value
			snapshot.Reasoning.Value = &count
		}
		adminInfo["kimi_usage_evidence"] = snapshot
	}
	isMultiKey := common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex)
	}

	isLocalCountTokens := common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens)
	if isLocalCountTokens {
		adminInfo["local_count_tokens"] = isLocalCountTokens
	}

	AppendChannelAffinityAdminInfo(ctx, adminInfo)
	AppendSlowTTFTAdminInfo(ctx, adminInfo)
	AppendRelayHTTPTimeout(ctx, adminInfo)
	AppendUpstreamRequestIDs(ctx, adminInfo)

	if len(relayInfo.ParamOverrideAudit) > 0 {
		other["po"] = append([]string(nil), relayInfo.ParamOverrideAudit...)
	}
	if relayInfo.StreamStatus != nil {
		status := "ok"
		if relayInfo.StreamStatus.HasErrors() || !relayInfo.StreamStatus.IsNormalEnd() {
			status = "error"
		}
		streamStatus := map[string]interface{}{
			"status":      status,
			"end_reason":  string(relayInfo.StreamStatus.EndReason),
			"error_count": relayInfo.StreamStatus.TotalErrorCount(),
		}
		if relayInfo.StreamStatus.EndError != nil {
			streamStatus["end_error"] = relayInfo.StreamStatus.EndError.Error()
		}
		other["stream_status"] = streamStatus
	}
	if relayInfo.ResponsesCompletedSummary != nil {
		other["responses_completed_summary"] = relayInfo.ResponsesCompletedSummary
	}
	if common.GetContextKeyBool(ctx, constant.ContextKeyResponsesAutoContinue) {
		other["responses_auto_continue"] = map[string]interface{}{
			"from_channel": common.GetContextKeyInt(ctx, constant.ContextKeyResponsesAutoContinueFromChannel),
			"to_channel":   common.GetContextKeyInt(ctx, constant.ContextKeyResponsesAutoContinueToChannel),
			"end_reason":   common.GetContextKeyString(ctx, constant.ContextKeyResponsesAutoContinueEndReason),
		}
	}

	other["admin_info"] = adminInfo
	appendResponsesRequestDiagnostics(relayInfo, other)
	appendRequestPath(ctx, relayInfo, other)
	appendRequestConversionChain(relayInfo, other)
	appendBillingInfo(relayInfo, other)
	appendTimeRatioInfo(relayInfo, other)
	return other
}

func appendTimeRatioInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	timeRatioInfo := relayInfo.PriceData.TimeRatioInfo
	if !timeRatioInfo.Matched() && timeRatioInfo.EffectiveRatio() == 1 {
		return
	}
	other["time_ratio"] = timeRatioInfo.EffectiveRatio()
	if timeRatioInfo.RuleID != "" {
		other["time_ratio_rule"] = timeRatioInfo.RuleID
	}
	if timeRatioInfo.Timezone != "" {
		other["time_ratio_timezone"] = timeRatioInfo.Timezone
	}
	if timeRatioInfo.MatchedAt != "" {
		other["time_ratio_matched_at"] = timeRatioInfo.MatchedAt
	}
}

func FormatTimeRatioContent(priceData types.PriceData) string {
	timeRatioInfo := priceData.TimeRatioInfo
	if !timeRatioInfo.Matched() && timeRatioInfo.EffectiveRatio() == 1 {
		return ""
	}
	if timeRatioInfo.RuleID != "" {
		return fmt.Sprintf("时间倍率 %.2f（%s）", timeRatioInfo.EffectiveRatio(), timeRatioInfo.RuleID)
	}
	return fmt.Sprintf("时间倍率 %.2f", timeRatioInfo.EffectiveRatio())
}

func appendBillingInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	// billing_source: "wallet" or "subscription"
	if relayInfo.BillingSource != "" {
		other["billing_source"] = relayInfo.BillingSource
	}
	if relayInfo.UserSetting.BillingPreference != "" {
		other["billing_preference"] = relayInfo.UserSetting.BillingPreference
	}
	if relayInfo.BillingSource == "subscription" {
		if relayInfo.SubscriptionId != 0 {
			other["subscription_id"] = relayInfo.SubscriptionId
		}
		if relayInfo.SubscriptionPreConsumed > 0 {
			other["subscription_pre_consumed"] = relayInfo.SubscriptionPreConsumed
		}
		// post_delta: settlement delta applied after actual usage is known (can be negative for refund)
		if relayInfo.SubscriptionPostDelta != 0 {
			other["subscription_post_delta"] = relayInfo.SubscriptionPostDelta
		}
		if relayInfo.SubscriptionPlanId != 0 {
			other["subscription_plan_id"] = relayInfo.SubscriptionPlanId
		}
		if relayInfo.SubscriptionPlanTitle != "" {
			other["subscription_plan_title"] = relayInfo.SubscriptionPlanTitle
		}
		// Compute "this request" subscription consumed + remaining
		consumed := relayInfo.SubscriptionPreConsumed + relayInfo.SubscriptionPostDelta
		usedFinal := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		if consumed < 0 {
			consumed = 0
		}
		if usedFinal < 0 {
			usedFinal = 0
		}
		if relayInfo.SubscriptionAmountTotal > 0 {
			remain := relayInfo.SubscriptionAmountTotal - usedFinal
			if remain < 0 {
				remain = 0
			}
			other["subscription_total"] = relayInfo.SubscriptionAmountTotal
			other["subscription_used"] = usedFinal
			other["subscription_remain"] = remain
		}
		if consumed > 0 {
			other["subscription_consumed"] = consumed
		}
		// Wallet quota is not deducted when billed from subscription.
		other["wallet_quota_deducted"] = 0
	}
}

func appendRequestConversionChain(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if len(relayInfo.RequestConversionChain) == 0 {
		return
	}
	chain := make([]string, 0, len(relayInfo.RequestConversionChain))
	for _, f := range relayInfo.RequestConversionChain {
		switch f {
		case types.RelayFormatOpenAI:
			chain = append(chain, "OpenAI Compatible")
		case types.RelayFormatClaude:
			chain = append(chain, "Claude Messages")
		case types.RelayFormatGemini:
			chain = append(chain, "Google Gemini")
		case types.RelayFormatOpenAIResponses:
			chain = append(chain, "OpenAI Responses")
		default:
			chain = append(chain, string(f))
		}
	}
	if len(chain) == 0 {
		return
	}
	other["request_conversion"] = chain
	if relayInfo.TextProtocolPlan != nil && relayInfo.TextProtocolPlan.RequiresConversion() {
		other["request_converter"] = string(relayInfo.TextProtocolPlan.Converter)
	}
}

func GenerateWssOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["ws"] = true
	info["audio_input"] = usage.InputTokenDetails.AudioTokens
	info["audio_output"] = usage.OutputTokenDetails.AudioTokens
	info["text_input"] = usage.InputTokenDetails.TextTokens
	info["text_output"] = usage.OutputTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateAudioOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["audio"] = true
	info["audio_input"] = usage.PromptTokensDetails.AudioTokens
	info["audio_output"] = usage.CompletionTokenDetails.AudioTokens
	info["text_input"] = usage.PromptTokensDetails.TextTokens
	info["text_output"] = usage.CompletionTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, userGroupRatio)
	info["claude"] = true
	info["cache_creation_tokens"] = cacheCreationTokens
	info["cache_creation_ratio"] = cacheCreationRatio
	if cacheCreationTokens5m != 0 {
		info["cache_creation_tokens_5m"] = cacheCreationTokens5m
		info["cache_creation_ratio_5m"] = cacheCreationRatio5m
	}
	if cacheCreationTokens1h != 0 {
		info["cache_creation_tokens_1h"] = cacheCreationTokens1h
		info["cache_creation_ratio_1h"] = cacheCreationRatio1h
	}
	return info
}

func GenerateMjOtherInfo(relayInfo *relaycommon.RelayInfo, priceData types.PriceData) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_price"] = priceData.ModelPrice
	other["group_ratio"] = priceData.GroupRatioInfo.GroupRatio
	if priceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
	}
	appendRequestPath(nil, relayInfo, other)
	appendTimeRatioInfo(relayInfo, other)
	return other
}

// InjectTieredBillingInfo overlays tiered billing fields onto an existing
// module-specific other map. Call this after GenerateTextOtherInfo /
// GenerateClaudeOtherInfo / etc. when the request used tiered_expr billing.
func InjectTieredBillingInfo(other map[string]interface{}, relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult, params *billingexpr.TokenParams) {
	if relayInfo == nil || other == nil {
		return
	}
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil {
		return
	}
	other["billing_mode"] = "tiered_expr"
	other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
	if params != nil {
		other["tiered_params"] = map[string]interface{}{
			"p":     params.P,
			"c":     params.C,
			"len":   params.Len,
			"cr":    params.CR,
			"cc":    params.CC,
			"cc1h":  params.CC1h,
			"img":   params.Img,
			"img_o": params.ImgO,
			"ai":    params.AI,
			"ao":    params.AO,
		}
	}
	if result != nil {
		if result.SettlementFallback {
			other["tiered_settle_failed"] = true
			other["tiered_settle_error"] = result.SettlementError
			other["tiered_fallback_quota"] = result.ActualQuotaAfterGroup
			return
		}
		other["matched_tier"] = result.MatchedTier
		other["tiered_quota_before_group"] = result.ActualQuotaBeforeGroup
		other["tiered_quota_after_group"] = result.ActualQuotaAfterGroup
		if snap.QuotaPerUnit > 0 {
			other["tiered_cost_usd"] = result.ActualQuotaBeforeGroup / snap.QuotaPerUnit
			other["tiered_settled_usd"] = float64(result.ActualQuotaAfterGroup) / snap.QuotaPerUnit
		}
	}
	if snap.TimeRatio > 0 {
		other["time_ratio"] = snap.TimeRatio
	}
	if snap.TimeRatioRuleID != "" {
		other["time_ratio_rule"] = snap.TimeRatioRuleID
	}
}
