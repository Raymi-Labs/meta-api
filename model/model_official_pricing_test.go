package model

import (
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestOfficialPricingValidation(t *testing.T) {
	valid := `{"currency":"usd","source_url":"https://example.com/pricing","tiers":[{"label":"Standard","unit":"million_tokens","input":10,"output":50,"cache_read":0}]}`
	var p OfficialPricing
	require.NoError(t, common.UnmarshalJsonStr(valid, &p))
	require.NoError(t, p.Normalize(NameRuleExact))
	require.Equal(t, "USD", p.Currency)
	require.NotNil(t, p.Tiers[0].CacheRead)
	require.Zero(t, *p.Tiers[0].CacheRead)
	require.Nil(t, p.Tiers[0].CacheWrite)
	stored, err := p.Value()
	require.NoError(t, err)
	for _, value := range []interface{}{stored, []byte(stored.(string))} {
		var got OfficialPricing
		require.NoError(t, got.Scan(value))
		require.Equal(t, p, got)
	}
	for _, value := range []interface{}{nil, "", []byte{}} {
		got := p
		require.NoError(t, got.Scan(value))
		require.Empty(t, got.Tiers)
	}
	for _, replacement := range [][2]string{
		{`"usd"`, `"GBP"`}, {`https://example.com/pricing`, `javascript:alert(1)`},
		{`https://example.com/pricing`, `https://user:pass@example.com`},
		{`"input":10`, `"input":-1`}, {`"input":10`, `"input":1000000001`},
		{`"Standard"`, `""`}, {`"million_tokens"`, `"unknown"`},
	} {
		var invalid OfficialPricing
		require.NoError(t, common.UnmarshalJsonStr(strings.Replace(valid, replacement[0], replacement[1], 1), &invalid))
		require.Error(t, invalid.Normalize(NameRuleExact))
	}
	require.Error(t, p.Normalize(NameRulePrefix))
	for _, v := range []float64{math.NaN(), math.Inf(1)} {
		p.Tiers[0].Input = &v
		require.Error(t, p.Normalize(NameRuleExact))
	}
	for _, unit := range []string{"request", "image", "second", "minute", "hour", "million_characters", "million_audio_tokens", "million_image_tokens", "million_video_tokens"} {
		zero := 0.0
		p = OfficialPricing{Currency: "CNY", Tiers: []OfficialPricingTier{{Label: "Standard", Unit: unit, Price: &zero}}}
		require.NoError(t, p.Normalize(NameRuleExact))
	}
	p = OfficialPricing{Currency: "USD", Tiers: []OfficialPricingTier{{Label: "Empty", Unit: "million_tokens"}}}
	require.Error(t, p.Normalize(NameRuleExact))
	p = OfficialPricing{Currency: "USD"}
	require.NoError(t, p.Normalize(NameRulePrefix))
	require.Equal(t, OfficialPricing{}, p)
}
