package controller

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

var officialPriceFixture = []string{"Pricing", "Text tokens", "Per 1M tokens", "Input", "$10.00", "Cached input", "$1.00", "Cache writes", "$12.50", "Output", "$50.00", "Prompts with more than 272K input tokens are priced at 2x input and cache rates and 1.5x output for the full request.", "Batch and Flex are priced at 50% of Standard rates. Fast mode is priced at 2x the applicable rates.", "Modalities"}

const officialPriceAddress = "https://developers.openai.com/api/docs/models/gpt-example"

// Visible text in Terra's official model card, including the two cache-write
// retention periods. A price-bearing note is part of the standard rate sheet.
const terraCacheRetention = "Cache-write prices shown use 30-minute retention. Standard cache writes cost $1.25 per million tokens for 30-minute retention and $2.50 for 12-hour retention, before long-context adjustments."

func terraOfficialPriceFixture() []string {
	return []string{"Pricing", "Text tokens", "Per 1M tokens", "Input", "$1.00", "Cached input", "$0.05", "Cache writes", "$1.25", "Output", "$5.00", terraCacheRetention, officialPriceFixture[11], "Modalities"}
}

func TestOpenAIOfficialPricingCacheRetention(t *testing.T) {
	pricing, err := parseOpenAIPriceLines(terraOfficialPriceFixture(), officialPriceAddress)
	require.NoError(t, err)
	require.Len(t, pricing.Tiers, 4)
	var values [][]float64
	for _, tier := range pricing.Tiers {
		values = append(values, []float64{*tier.Input, *tier.Output, *tier.CacheRead, *tier.CacheWrite})
	}
	require.ElementsMatch(t, [][]float64{{1, 5, 0.05, 1.25}, {1, 5, 0.05, 2.5}, {2, 7.5, 0.1, 2.5}, {2, 7.5, 0.1, 5}}, values)
	require.Contains(t, pricing.Notes, terraCacheRetention)
	for _, tier := range pricing.Tiers {
		require.Contains(t, tier.Label, "272K input tokens")
		require.Regexp(t, `cache (30m|12h)$`, tier.Label)
	}
	withoutContext := terraOfficialPriceFixture()
	withoutContext = append(withoutContext[:12], "Modalities")
	pricing, err = parseOpenAIPriceLines(withoutContext, officialPriceAddress)
	require.NoError(t, err)
	require.Len(t, pricing.Tiers, 2)
	for _, tier := range pricing.Tiers {
		require.NotContains(t, tier.Label, "input tokens")
	}
	defaultSecond := terraOfficialPriceFixture()
	defaultSecond[8] = "$2.50"
	defaultSecond[11] = strings.Replace(defaultSecond[11], "shown use 30-minute", "shown use 12-hour", 1)
	pricing, err = parseOpenAIPriceLines(defaultSecond, officialPriceAddress)
	require.NoError(t, err)
	require.Contains(t, pricing.Tiers[0].Label, "cache 12h")
	require.Equal(t, 2.5, *pricing.Tiers[0].CacheWrite)
	for name, change := range map[string]func([]string) []string{
		"card mismatch":      func(v []string) []string { v[8] = "$9"; return v },
		"missing write":      func(v []string) []string { v[8] = "-"; return v },
		"duplicate rule":     func(v []string) []string { return append(v[:13], terraCacheRetention, "Modalities") },
		"duplicate duration": func(v []string) []string { v[11] = strings.ReplaceAll(v[11], "12-hour", "30-minute"); return v },
		"equivalent duration": func(v []string) []string {
			v[11] = strings.ReplaceAll(strings.ReplaceAll(v[11], "30-minute", "60-minute"), "12-hour", "1-hour")
			return v
		},
		"missing default": func(v []string) []string {
			v[11] = strings.Replace(v[11], "shown use 30-minute", "shown use 1-hour", 1)
			return v
		},
		"unsupported condition": func(v []string) []string { v[11] += " Additional writes cost $7."; return v },
		"wrong currency":        func(v []string) []string { v[11] = strings.Replace(v[11], "$2.50", "€2.50", 1); return v },
		"invalid duration": func(v []string) []string {
			v[11] = strings.Replace(v[11], "12-hour", "999999999999999999999-hour", 1)
			return v
		},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := parseOpenAIPriceLines(change(terraOfficialPriceFixture()), officialPriceAddress)
			require.Error(t, err)
			require.Empty(t, p.Tiers)
		})
	}
}

func TestOpenAIOfficialPricing(t *testing.T) {
	price, err := parseOpenAIPriceLines(officialPriceFixture, officialPriceAddress)
	require.NoError(t, err)
	require.Len(t, price.Tiers, 2)
	require.Equal(t, "USD", price.Currency)
	require.Equal(t, officialPriceAddress, price.SourceURL)
	require.Equal(t, []float64{10, 50, 1, 12.5, 20, 75, 2, 25}, []float64{*price.Tiers[0].Input, *price.Tiers[0].Output, *price.Tiers[0].CacheRead, *price.Tiers[0].CacheWrite, *price.Tiers[1].Input, *price.Tiers[1].Output, *price.Tiers[1].CacheRead, *price.Tiers[1].CacheWrite})
	require.Contains(t, price.Notes, "Fast mode")
	for _, tier := range price.Tiers {
		require.Equal(t, "million_tokens", tier.Unit)
	}
	zero, err := parseOpenAIPriceLines([]string{"Pricing", "Text tokens", "Per 1M tokens", "∙", "Batch API price", "Input", "$0", "Cached input", "-", "Output", "$0.10", "Quick comparison", "Input", "$999"}, officialPriceAddress)
	require.NoError(t, err)
	require.Zero(t, *zero.Tiers[0].Input)
	require.Nil(t, zero.Tiers[0].CacheRead)
	require.Equal(t, 0.1, *zero.Tiers[0].Output)
	for name, change := range map[string]func([]string) []string{
		"unit":      func(v []string) []string { v[2] = "Per 1K tokens"; return v },
		"missing":   func(v []string) []string { return v[:9] },
		"invalid":   func(v []string) []string { v[4] = "$NaN"; return v },
		"negative":  func(v []string) []string { v[4] = "$-1"; return v },
		"duplicate": func(v []string) []string { return append(v[:11], "Input", "$5", "Modalities") },
		"condition": func(v []string) []string {
			v[11] = "Prompts with more than 272K input tokens have different prices."
			return v
		},
		"wrong currency": func(v []string) []string { v[4] = "€10"; return v },
	} {
		t.Run(name, func(t *testing.T) {
			p, err := parseOpenAIPriceLines(change(append([]string{}, officialPriceFixture...)), officialPriceAddress)
			require.Error(t, err)
			require.Empty(t, p.Tiers)
		})
	}
	p, err := parseOpenAIPriceLines([]string{"Pricing", "Audio tokens", "Per 1M tokens", "Input", "$5"}, officialPriceAddress)
	require.NoError(t, err)
	require.Empty(t, p.Tiers)
	// The identity-bearing component alone can supply prices, not nearby cards.
	html := strings.Replace(officialAttributeFixture, "</main>", "<div>Pricing</div><div>Text tokens</div><div>Per 1M tokens</div><div>Input</div><div>$99</div><div>Output</div><div>$99</div></main>", 1)
	lines, err := openAIModelLines([]byte(html), "gpt-example")
	require.NoError(t, err)
	p, err = parseOpenAIPriceLines(lines, officialPriceAddress)
	require.NoError(t, err)
	require.Empty(t, p.Tiers)
}

// Opt-in vendor acceptance; ordinary unit tests remain independent of the network.
func TestLiveOfficialModelPrices(t *testing.T) {
	if os.Getenv("TEST_LIVE_OFFICIAL_PRICES") != "1" {
		t.Skip("explicit live pricing acceptance only")
	}
	for _, id := range []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-6.1-terra", "gpt-4o-mini"} {
		t.Run(id, func(t *testing.T) {
			address := "https://developers.openai.com/api/docs/models/" + id
			data, err := fetchOfficialAttributeData(context.Background(), address, nil)
			require.NoError(t, err)
			lines, err := openAIModelLines(data, id)
			require.NoError(t, err)
			p, err := parseOpenAIPriceLines(lines, address)
			require.NoError(t, err)
			require.NotEmpty(t, p.Tiers)
			require.Equal(t, "USD", p.Currency)
			require.NoError(t, p.Normalize(model.NameRuleExact))
			t.Logf("%s input=%g output=%g tiers=%d", id, *p.Tiers[0].Input, *p.Tiers[0].Output, len(p.Tiers))
		})
	}
}
