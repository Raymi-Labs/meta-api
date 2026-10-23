package service

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP string) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, channelAffinityMeta{
		CacheKey:       fmt.Sprintf("test:%s:%s:%s", ruleName, usingGroup, keyFP),
		TTLSeconds:     600,
		RuleName:       ruleName,
		UsingGroup:     usingGroup,
		KeyFingerprint: keyFP,
	})
	return ctx
}

func TestChannelAffinityCacheUsesActualUpstreamUsageSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, semantic, wantMode string
		format                   types.RelayFormat
		channel                  int
	}{
		{"converted Chat", "", cacheTokenRateModeCachedOverPrompt, types.RelayFormatOpenAI, constant.ChannelTypeOpenAI},
		{"OpenRouter Messages", "", cacheTokenRateModeCachedOverPrompt, types.RelayFormatClaude, constant.ChannelTypeOpenRouter},
		{"explicit Anthropic", "anthropic", cacheTokenRateModeCachedOverPromptPlusCached, types.RelayFormatOpenAI, constant.ChannelTypeOpenAI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, group, fp := setupChannelAffinityUsageCacheTest(t)
			ctx := buildChannelAffinityStatsContextForTest(rule, group, fp)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, FinalRequestRelayFormat: tc.format,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channel}}
			ObserveChannelAffinityUsageCacheByRelayInfo(ctx, &dto.Usage{PromptTokens: 100, UsageSemantic: tc.semantic,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 80}}, info)
			require.Equal(t, tc.wantMode, GetChannelAffinityUsageCacheStats(rule, group, fp).CachedTokenRateMode)
		})
	}
}

func setupChannelAffinityUsageCacheTest(t *testing.T) (string, string, string) {
	t.Helper()
	resetChannelAffinityUsageCacheStatsForTest()
	t.Cleanup(resetChannelAffinityUsageCacheStatsForTest)
	ruleName := "rule_" + t.Name()
	usingGroup := "default"
	keyFP := "fp_" + t.Name()
	return ruleName, usingGroup, keyFP
}

func TestObserveChannelAffinityUsageCacheByRelayFormat_ClaudeMode(t *testing.T) {
	ruleName, usingGroup, keyFP := setupChannelAffinityUsageCacheTest(t)
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)

	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 40,
		TotalTokens:      140,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 30,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, usage, types.RelayFormatClaude)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 1, stats.Total)
	require.EqualValues(t, 1, stats.Hit)
	require.EqualValues(t, 100, stats.PromptTokens)
	require.EqualValues(t, 40, stats.CompletionTokens)
	require.EqualValues(t, 140, stats.TotalTokens)
	require.EqualValues(t, 30, stats.CachedTokens)
	require.Equal(t, cacheTokenRateModeCachedOverPromptPlusCached, stats.CachedTokenRateMode)
}

func TestObserveChannelAffinityUsageCacheByRelayFormat_MixedMode(t *testing.T) {
	ruleName, usingGroup, keyFP := setupChannelAffinityUsageCacheTest(t)
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)

	openAIUsage := &dto.Usage{
		PromptTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 10,
		},
	}
	claudeUsage := &dto.Usage{
		PromptTokens: 80,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 20,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, openAIUsage, types.RelayFormatOpenAI)
	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, claudeUsage, types.RelayFormatClaude)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 2, stats.Total)
	require.EqualValues(t, 2, stats.Hit)
	require.EqualValues(t, 180, stats.PromptTokens)
	require.EqualValues(t, 30, stats.CachedTokens)
	require.Equal(t, cacheTokenRateModeMixed, stats.CachedTokenRateMode)
}

func TestObserveChannelAffinityUsageCacheByRelayFormat_UnsupportedModeKeepsEmpty(t *testing.T) {
	ruleName, usingGroup, keyFP := setupChannelAffinityUsageCacheTest(t)
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)

	usage := &dto.Usage{
		PromptTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 25,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, usage, types.RelayFormatGemini)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 1, stats.Total)
	require.EqualValues(t, 1, stats.Hit)
	require.EqualValues(t, 25, stats.CachedTokens)
	require.Equal(t, "", stats.CachedTokenRateMode)
}

func TestObserveChannelAffinityUsageCacheByRelayFormat_ResetClearsStats(t *testing.T) {
	ruleName, usingGroup, keyFP := setupChannelAffinityUsageCacheTest(t)
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, &dto.Usage{
		PromptTokens: 10,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 3,
		},
	}, types.RelayFormatOpenAI)

	resetChannelAffinityUsageCacheStatsForTest()
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 0, stats.Total)
	require.EqualValues(t, 0, stats.Hit)
	require.EqualValues(t, 0, stats.CachedTokens)
	require.Equal(t, "", stats.CachedTokenRateMode)
}
