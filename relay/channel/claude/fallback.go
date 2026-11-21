package claude

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func addFallbackBeta(headers http.Header) {
	values := strings.Join(headers.Values("anthropic-beta"), ",")
	for _, beta := range strings.Split(values, ",") {
		if strings.HasPrefix(strings.TrimSpace(beta), "server-side-fallback-") {
			return
		}
	}
	const beta = "server-side-fallback-2026-06-01"
	if values != "" {
		values += ","
	}
	headers.Set("anthropic-beta", values+beta)
}

func fallbackAllowed(c *gin.Context, info *relaycommon.RelayInfo) bool {
	const target = relaycommon.ClaudeFallbackTarget
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenClaudeRefusalFallback) || info.ChannelMeta == nil ||
		info.ChannelType != constant.ChannelTypeAnthropic || info.OriginModelName != relaycommon.ClaudeFallbackPrimary ||
		info.UpstreamModelName != relaycommon.ClaudeFallbackPrimary ||
		(info.RelayFormat != types.RelayFormatClaude && info.RelayFormat != types.RelayFormatOpenAI) {
		return false
	}
	if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		allowed, _ := common.GetContextKeyType[map[string]bool](c, constant.ContextKeyTokenModelLimit)
		if !allowed[target] {
			return false
		}
	}
	return model.IsModelCallableByRole(target, service.RequestModelPermissionRole(c)) &&
		common.StringsContains(service.RequestModelRoutingGroups(c, info.TokenGroup, target), info.UsingGroup) &&
		model.IsChannelEnabledForGroupModel(info.UsingGroup, target, info.ChannelId)
}

func prepareOfficialFallback(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (io.Reader, error) {
	if !fallbackAllowed(c, info) {
		return body, nil
	}
	// The chosen channel and payer stay the same. Freeze the second model's
	// own price, including its no-discount policy and tiered expression.
	target := *info
	target.OriginModelName = relaycommon.ClaudeFallbackTarget
	meta := *info.ChannelMeta
	meta.UpstreamModelName, meta.IsModelMapped = target.OriginModelName, false
	target.ChannelMeta = &meta
	target.NoDiscountSnapshot, target.TieredBillingSnapshot, target.Billing = nil, nil, nil
	target.FinalPreConsumedQuota = 0
	if err := helper.ModelMappedHelper(c, &target, nil); err != nil || target.UpstreamModelName != target.OriginModelName {
		return body, nil
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	original := bytes.NewReader(data)
	if gjson.GetBytes(data, "model").String() != info.OriginModelName || gjson.GetBytes(data, "fallbacks").Exists() {
		return original, nil
	}
	if _, tiered := billing_setting.GetTieredExpr(target.OriginModelName); tiered {
		input, err := helper.ResolveIncomingBillingExprRequestInput(c, info)
		if err != nil {
			return original, nil
		}
		input.Body, err = sjson.SetBytes(input.Body, "model", target.OriginModelName)
		if err != nil {
			return original, nil
		}
		target.BillingRequestInput = &input
	}
	price, err := helper.ModelPriceHelper(c, &target, info.GetEstimatePromptTokens(), &types.TokenCountMeta{MaxTokens: info.PriceData.RequestedMaxTokens})
	if err != nil {
		return original, nil
	}
	updated, err := sjson.SetBytes(data, "fallbacks", []dto.ClaudeModelRef{{Model: target.OriginModelName}})
	if err != nil {
		return nil, err
	}
	if !price.FreeModel {
		extra := price.QuotaToPreConsume
		base := info.PriceData.QuotaToPreConsume
		if info.Billing == nil || extra < 0 || base < 0 || extra > math.MaxInt-base || info.Billing.Reserve(base+extra) != nil {
			return original, nil
		}
	}
	info.ClaudeFallback = &relaycommon.ClaudeFallback{Target: &target, ServingModel: info.OriginModelName}
	return bytes.NewReader(updated), nil
}

var claudeMessageIDPattern = regexp.MustCompile(`^msg_[A-Za-z0-9_-]{1,96}$`)

func observeOfficialFallback(info *relaycommon.RelayInfo, response *dto.ClaudeResponse) {
	id := response.Id
	if response.Message != nil && response.Type == "message_start" {
		id = response.Message.Id
	}
	if id != "" && claudeMessageIDPattern.MatchString(id) {
		info.ClaudeMessageID = id
	}
	stop, details := response.StopReason, response.StopDetails
	if response.Delta != nil && response.Delta.StopReason != nil {
		stop = *response.Delta.StopReason
		if response.Delta.StopDetails != nil {
			details = response.Delta.StopDetails
		}
	}
	if stop == "refusal" && details != nil {
		info.ClaudeRefusalCategory = knownRefusalCategory(details.Category)
	}
	state := info.ClaudeFallback
	if state == nil {
		return
	}
	if response.Type == "message_start" && response.Message != nil && response.Message.Model != "" {
		state.ServingModel = response.Message.Model
	}
	if response.Type == "message" && response.Model != "" {
		state.ServingModel = response.Model
	}
	observeBlock := func(block *dto.ClaudeMediaMessage) {
		if block == nil || block.Type != "fallback" || block.From == nil || block.To == nil ||
			block.From.Model != relaycommon.ClaudeFallbackPrimary || block.To.Model != relaycommon.ClaudeFallbackTarget {
			return
		}
		state.ServingModel = block.To.Model
		if block.Trigger != nil && block.Trigger.Type == "refusal" {
			state.PrimaryCategory = knownRefusalCategory(block.Trigger.Category)
		}
	}
	observeBlock(response.ContentBlock)
	for i := range response.Content {
		observeBlock(&response.Content[i])
	}
	if stop != "" {
		state.StopReason = stop
		if details != nil {
			state.FinalCategory = knownRefusalCategory(details.Category)
		}
	}
	if response.Usage != nil && len(response.Usage.Iterations) > 0 {
		attempts, ok := parseFallbackAttempts(response.Usage.Iterations)
		if ok && state.ServingModel == relaycommon.ClaudeFallbackTarget && attempts[len(attempts)-1].Model != state.ServingModel {
			ok = false
		}
		if ok {
			state.Attempts = attempts
			state.IncompleteUsage = false
			state.ServingModel = attempts[len(attempts)-1].Model
		} else if len(state.Attempts) == 0 {
			state.IncompleteUsage = true
		}
	}
	if state.ServingModel == relaycommon.ClaudeFallbackTarget {
		info.ObserveResponseModel(state.ServingModel)
	}
}

func knownRefusalCategory(category string) string {
	switch category {
	case "cyber", "bio", "frontier_llm", "reasoning_extraction", "general_harms":
		return category
	}
	return ""
}

func parseFallbackAttempts(data []byte) ([]relaycommon.ClaudeFallbackAttempt, bool) {
	rows := gjson.ParseBytes(data)
	if !rows.IsArray() {
		return nil, false
	}
	entries := rows.Array()
	if len(entries) < 1 || len(entries) > 2 {
		return nil, false
	}
	attempts := make([]relaycommon.ClaudeFallbackAttempt, 0, len(entries))
	for i, row := range entries {
		name, kind := row.Get("model").String(), row.Get("type").String()
		primary := i == 0 && name == relaycommon.ClaudeFallbackPrimary && kind == "message"
		fallback := i == len(entries)-1 && name == relaycommon.ClaudeFallbackTarget && kind == "fallback_message"
		if !primary && !fallback {
			return nil, false
		}
		var usage dto.ClaudeUsage
		if common.UnmarshalJsonStr(row.Raw, &usage) != nil || !usage.HasInputTokens() || !usage.HasOutputTokens() {
			return nil, false
		}
		parsed := ClaudeResponseInfo{Usage: &dto.Usage{}}
		mergeClaudeUsage(&parsed, &usage, true)
		if parsed.UsageError != nil {
			return nil, false
		}
		if usage.ServerToolUse != nil {
			parsed.Usage.WebSearchRequests = usage.ServerToolUse.WebSearchRequests
		}
		attempts = append(attempts, relaycommon.ClaudeFallbackAttempt{Model: name, Usage: *parsed.Usage})
	}
	return attempts, true
}
