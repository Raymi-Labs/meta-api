package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestStoredPromptTokenSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, other   string
		prompt, total int
	}{
		{"new inclusive overrides stale Claude flag", `{"prompt_tokens_semantic":"inclusive","claude":true}`, 100, 100},
		{"new non-cache overrides conversion", `{"prompt_tokens_semantic":"non_cache","request_conversion":["Claude Messages","OpenAI Compatible"]}`, 30, 100},
		{"native Anthropic", `{"usage_semantic":"anthropic"}`, 30, 100},
		{"legacy mislabelled Chat", `{"claude":true,"usage_semantic":"anthropic","billing_mode":"tiered_expr","request_conversion":["Claude Messages","OpenAI Compatible"],"tiered_params":{"len":100}}`, 100, 100},
		{"legacy mislabelled Responses", `{"claude":true,"billing_mode":"tiered_expr","request_conversion":["Claude Messages","OpenAI Compatible","OpenAI Responses"],"tiered_params":{"len":100}}`, 100, 100},
		{"legacy OpenRouter already normalized", `{"claude":true,"billing_mode":"tiered_expr","request_conversion":["Claude Messages","OpenAI Compatible"],"tiered_params":{"len":100}}`, 30, 100},
		{"legacy ordinary ambiguous", `{"claude":true,"request_conversion":["Claude Messages","OpenAI Compatible"]}`, 100, 170},
		{"unknown tier length", `{"claude":true,"billing_mode":"tiered_expr","request_conversion":["Claude Messages","OpenAI Compatible"],"tiered_params":{"len":150}}`, 100, 170},
		{"missing tier length", `{"claude":true,"billing_mode":"tiered_expr","request_conversion":["Claude Messages","OpenAI Compatible"],"tiered_params":{"p":100}}`, 100, 170},
		{"no final upstream evidence", `{"claude":true,"billing_mode":"tiered_expr","request_conversion":["Claude Messages"],"tiered_params":{"len":100}}`, 100, 170},
		{"ordinary does not trust tier snapshot", `{"claude":true,"request_conversion":["Claude Messages","OpenAI Compatible"],"tiered_params":{"len":100}}`, 100, 170},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]interface{}
			require.NoError(t, common.UnmarshalJsonStr(tc.other, &data))
			data["cache_tokens"] = 50
			data["cache_creation_tokens"] = 20
			raw, err := common.Marshal(data)
			require.NoError(t, err)
			hit, global := calculateCacheRates([]logCacheRateRow{{PromptTokens: tc.prompt, Other: string(raw)}})
			require.InDelta(t, 50.0/float64(tc.total), global, 1e-12)
			require.Equal(t, hit, global)
			distribution := aggregatePublicTokenDistributionRows([]tokenUsageLogRow{{PromptTokens: tc.prompt, CompletionTokens: 8, Other: string(raw)}})
			require.Equal(t, tc.total-70, distribution.InputTokens)
			require.Equal(t, tc.total+8, distribution.TotalTokens)
		})
	}
}

func TestSumUsedQuotaCorrectsLegacyTieredCacheDenominatorWithoutChangingQuota(t *testing.T) {
	setupCacheRateScanDB(t)
	setCacheRateScanLimit(t, 0)
	const prompt, cached = 23840639, 19972736
	log := Log{Type: LogTypeConsume, UserId: 1456, Username: "fixture", ChannelId: 1546, PromptTokens: prompt, Quota: 12345,
		Other: `{"claude":true,"usage_semantic":"anthropic","cache_tokens":19972736,"billing_mode":"tiered_expr","request_conversion":["Claude Messages","OpenAI Compatible"],"tiered_params":{"len":23840639}}`}
	require.NoError(t, LOG_DB.Create(&log).Error)
	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "fixture", "", 1546, "", "", false)
	require.NoError(t, err)
	require.Equal(t, 12345, stat.Quota)
	require.InDelta(t, float64(cached)/float64(prompt), stat.CacheGlobalRate, 1e-12)
	var unchanged Log
	require.NoError(t, LOG_DB.First(&unchanged, log.Id).Error)
	require.Equal(t, log.Other, unchanged.Other)
	require.Equal(t, log.Quota, unchanged.Quota)
}
