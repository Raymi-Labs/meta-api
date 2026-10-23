package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestClaudeTieredCachePartitionAndBoundary(t *testing.T) {
	expression := `len <= 100000 ? tier("base", p*0.5+c*2.5+cr*0.05+cc*0.625+cc1h) : tier("long", p*2.5+c*12.5+cr*0.25+cc*3.125+cc1h*5)`
	for _, semantic := range []string{"", "anthropic", "Anthropic"} {
		for _, total := range []int{99999, 100000, 100001} {
			u := &dto.Usage{UsageSemantic: semantic, PromptTokens: 10, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: total - 210, CachedCreationTokens: 200}, ClaudeCacheCreation5mTokens: 50, ClaudeCacheCreation1hTokens: 150}
			params := BuildTieredTokenParams(u, true, billingexpr.UsedVars(expression))
			require.EqualValues(t, total, params.Len)
			require.EqualValues(t, 50, params.CC)
			require.EqualValues(t, 150, params.CC1h)
			_, trace, err := billingexpr.RunExpr(expression, params)
			require.NoError(t, err)
			want := "base"
			if total > 100000 {
				want = "long"
			}
			require.Equal(t, want, trace.MatchedTier)
		}
		// A total without TTL detail must survive semantic normalization.
		u := &dto.Usage{UsageSemantic: semantic, PromptTokens: 90000, PromptTokensDetails: dto.InputTokenDetails{CachedCreationTokens: 20000}}
		params := BuildTieredTokenParams(u, true, billingexpr.UsedVars(expression))
		require.EqualValues(t, 110000, params.Len)
		require.EqualValues(t, 20000, params.CC)
		require.Zero(t, params.CC1h)
	}
	u := &dto.Usage{UsageSemantic: "anthropic", PromptTokens: 10, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 30, CachedCreationTokens: 200}, ClaudeCacheCreation5mTokens: 50, ClaudeCacheCreation1hTokens: 100}
	params := BuildTieredTokenParams(u, false, map[string]bool{"cr": true, "cc": true, "cc1h": true})
	require.EqualValues(t, 100, params.CC)
	require.EqualValues(t, 240, params.Len)
	require.EqualValues(t, 10, params.P)
	fallback := BuildTieredTokenParams(u, true, map[string]bool{})
	require.EqualValues(t, 240, fallback.P)
	require.EqualValues(t, 240, fallback.Len)
	creation, err := u.CacheCreationBreakdown()
	require.NoError(t, err)
	price := types.PriceData{ModelRatio: 1, CacheRatio: 0.1, CompletionRatio: 5, CacheCreationRatio: 1.25, CacheCreation5mRatio: 1.25, CacheCreation1hRatio: 2, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0}}
	require.Zero(t, ClaudeTokenQuotaAmount(10, 1, 30, creation, price), "free group stays free")
	require.Equal(t, billingexpr.TokenParams{}, BuildTieredTokenParams(nil, true, nil))
	invalid := &dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CachedCreationTokens: 1}, ClaudeCacheCreation1hTokens: 2}
	require.NotNil(t, ValidateClaudeCacheUsage(&relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}, invalid))
	require.Nil(t, ValidateClaudeCacheUsage(&relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI}, invalid), "do not reinterpret another provider's untagged usage")
	invalid.UsageSemantic = "anthropic"
	require.NotNil(t, ValidateClaudeCacheUsage(&relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI}, invalid))
}

func TestClaudeCacheValidationUsesActualUsageSemantics(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter}}
	usage := &dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CachedCreationTokens: 1}, ClaudeCacheCreation1hTokens: 2}
	require.Nil(t, ValidateClaudeCacheUsage(info, usage), "untagged inclusive usage is not an Anthropic partition")
	usage.UsageSemantic = "anthropic"
	require.NotNil(t, ValidateClaudeCacheUsage(info, usage), "explicit parser semantic takes precedence")
}
