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
	for _, id := range []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-4o-mini"} {
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
