package service

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type claudeAttemptCost struct {
	Model         string `json:"model"`
	Input         int    `json:"input_tokens"`
	Output        int    `json:"output_tokens"`
	CacheRead     int    `json:"cache_read_input_tokens"`
	CacheCreation int    `json:"cache_creation_input_tokens"`
	Quota         int    `json:"quota"`
	Refused       bool   `json:"refused,omitempty"`
}

func billedZeroOutputRefusal(category string) bool {
	switch category {
	case "bio", "frontier_llm", "reasoning_extraction":
		return true
	}
	return false
}

func priceClaudeAttempt(info *relaycommon.RelayInfo, usage dto.Usage, refused bool, category string) (claudeAttemptCost, bool) {
	creation, err := usage.CacheCreationBreakdown()
	row := claudeAttemptCost{
		Model: info.OriginModelName, Input: usage.PromptTokens, Output: usage.CompletionTokens,
		CacheRead: usage.PromptTokensDetails.CachedTokens, CacheCreation: creation.Total, Refused: refused,
	}
	if err != nil || row.Input < 0 || row.Output < 0 || row.CacheRead < 0 {
		return row, false
	}
	if refused && row.Output == 0 && !billedZeroOutputRefusal(category) {
		return row, true
	}
	p := info.PriceData
	if row.Input != 0 || row.Output != 0 || row.CacheRead != 0 || creation.Total != 0 {
		if snap := info.TieredBillingSnapshot; snap != nil {
			params := BuildTieredTokenParams(&usage, true, billingexpr.UsedVars(snap.ExprString))
			ok, quota, result := TryTieredSettle(info, params)
			if !ok || result == nil || result.SettlementFallback || quota < 0 {
				return row, false
			}
			row.Quota = quota
		} else {
			amount := ClaudeTokenQuotaAmount(row.Input, row.Output, row.CacheRead, creation, p)
			if math.IsNaN(amount) || amount < 0 || amount >= float64(math.MaxInt) {
				return row, false
			}
			row.Quota = int(amount)
		}
	}
	// Chat bills independently reported web searches. Native Messages follows
	// its existing inclusive tool policy. Never copy a tool count to both hops.
	if info.RelayFormat == types.RelayFormatOpenAI && !p.UsePrice && usage.WebSearchRequests > 0 {
		tools := ComputeToolCallQuota(ToolCallUsage{ModelName: info.OriginModelName, WebSearchCalls: usage.WebSearchRequests, WebSearchToolName: dto.BuildInToolWebSearch}, p.GroupRatioInfo.GroupRatio*p.TimeRatioInfo.EffectiveRatio())
		if tools.TotalQuota < 0 || tools.TotalQuota > math.MaxInt-row.Quota {
			return row, false
		}
		row.Quota += tools.TotalQuota
	}
	return row, true
}

// PostClaudeFallbackUsage settles the single reservation exactly once. The
// top-level counters describe the final hop, never an additional attempt.
func PostClaudeFallbackUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) bool {
	state := info.ClaudeFallback
	if state == nil || usage == nil || (state.ServingModel != relaycommon.ClaudeFallbackTarget && state.StopReason != "refusal") {
		return false
	}
	ObserveGroupHealthCacheUsage(info, usage)
	ObserveChannelAffinityUsageCacheByRelayInfo(ctx, usage, info)
	ObserveSlowTTFT(ctx, info, usage)
	attempts := state.Attempts
	if len(attempts) == 0 {
		state.IncompleteUsage = state.IncompleteUsage || state.ServingModel != info.OriginModelName
		if (state.ServingModel == info.OriginModelName || state.ServingModel == relaycommon.ClaudeFallbackTarget) && !usage.InputTokensEstimated {
			attempts = []relaycommon.ClaudeFallbackAttempt{{Model: state.ServingModel, Usage: *usage}}
		}
	}
	var rows []claudeAttemptCost
	var quota, prompt, output, read, write int
	for i, attempt := range attempts {
		price := info
		if attempt.Model == relaycommon.ClaudeFallbackTarget {
			price = state.Target
		}
		last := i == len(attempts)-1
		refused, category := true, state.PrimaryCategory
		if last {
			refused, category = state.StopReason == "refusal", state.FinalCategory
			if attempt.Usage.WebSearchRequests == 0 {
				attempt.Usage.WebSearchRequests = max(usage.WebSearchRequests, ctx.GetInt("claude_web_search_requests"))
			}
		}
		row, ok := priceClaudeAttempt(price, attempt.Usage, refused, category)
		if !ok || row.Quota > math.MaxInt-quota || row.Input > math.MaxInt-prompt || row.Output > math.MaxInt-output || row.CacheRead > math.MaxInt-read || row.CacheCreation > math.MaxInt-write {
			state.IncompleteUsage = true
			continue
		}
		rows = append(rows, row)
		quota += row.Quota
		prompt += row.Input
		output += row.Output
		read += row.CacheRead
		write += row.CacheCreation
	}
	err := SettleBilling(ctx, info, quota)
	if err != nil {
		logger.LogError(ctx, "error settling Claude fallback: "+err.Error())
	}
	p := info.PriceData
	other := GenerateTextOtherInfo(ctx, info, p.ModelRatio, p.GroupRatioInfo.GroupRatio, p.CompletionRatio, read, p.CacheRatio, p.ModelPrice, p.GroupRatioInfo.GroupSpecialRatio)
	AppendUsageTokenSemantics(other, true)
	other["cache_creation_tokens"] = write
	other["claude_fallback"] = map[string]any{
		"serving_model": state.ServingModel, "stop_reason": state.StopReason, "attempts": rows,
		"incomplete_usage": state.IncompleteUsage, "settled": err == nil,
	}
	quota, content, other := FinalizeConsumeLogAfterSettle("", other, quota, info, err)
	RecordRelayUsageStats(info, quota)
	model.RecordConsumeLog(ctx, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId, ModelName: info.OriginModelName, TokenName: ctx.GetString("token_name"),
		PromptTokens: prompt, CompletionTokens: output, Quota: quota, Content: content, TokenId: info.TokenId,
		UseTimeSeconds: int(time.Since(info.StartTime).Seconds()), IsStream: info.IsStream, Group: info.UsingGroup, Other: other,
	})
	info.PerformanceOutputTokens = int64(output)
	return true
}
