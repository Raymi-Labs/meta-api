package controller

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekExactPublishedVersionPrices(t *testing.T) {
	body := priceHTML([]string{"MODEL", "MODEL", "deepseek-flash (1)"}, []string{"MODEL VERSION", "MODEL VERSION", "DeepSeek-V4.1-Flash-1017"}, []string{"PEAK", "1M INPUT TOKENS (CACHE MISS)", "$0.3"}, []string{"PEAK", "1M OUTPUT TOKENS", "$1.2"}, []string{"PEAK", "1M INPUT TOKENS (CACHE HIT)", "$0.006"}, []string{"OFF-PEAK", "1M INPUT TOKENS (CACHE MISS)", "$0.15"}, []string{"OFF-PEAK", "1M OUTPUT TOKENS", "$0.6"}, []string{"OFF-PEAK", "1M INPUT TOKENS (CACHE HIT)", "$0.003"}) + "<p>Peak hours are 01:00–04:00 UTC; official schedule applies.</p>"
	names := map[string]bool{"deepseek-v4.1-flash-1017": true, "deepseek-v4.1-flash": true, "deepseek-v4-flash-0731": true}
	got := parseDeepSeekDocumentWithPrices([]byte(body), names, "https://api-docs.deepseek.com/quick_start/pricing/")
	require.Len(t, got, 1)
	p := got["deepseek-v4.1-flash-1017"].OfficialPricing
	require.Len(t, p.Tiers, 2)
	require.Equal(t, .15, *p.Tiers[0].Input)
	require.Equal(t, 1.2, *p.Tiers[1].Output)
	require.Contains(t, p.Notes, "API model: deepseek-flash")
	require.Empty(t, parseDeepSeekDocumentWithPrices([]byte(strings.ReplaceAll(body, "DeepSeek-V4.1-Flash-1017", "DeepSeek-V4.1-Flash-1117")), names, "url"))
}

func TestElorianOfficialRates(t *testing.T) {
	body := `<h1>Introducing Glimpse</h1><p>Input tokens cost $0.60 per million and output tokens $3.00 per million</p>`
	names := map[string]bool{"glimpse": true, "glimpse-other": true}
	got := parseElorianPricing([]byte(body), names, elorianPricingURL)
	require.Len(t, got, 1)
	p := got["glimpse"].OfficialPricing
	require.Equal(t, .6, *p.Tiers[0].Input)
	require.Equal(t, 3.0, *p.Tiers[0].Output)
	require.Nil(t, p.Tiers[0].CacheRead)
	require.True(t, model.IsManufacturerPriceSource("glimpse", p.SourceURL))
	require.Empty(t, parseElorianPricing([]byte(body+body), names, elorianPricingURL))
	require.Empty(t, parseElorianPricing([]byte(strings.ReplaceAll(body, "Introducing Glimpse", "Other Model")), names, elorianPricingURL))
	require.Empty(t, parseElorianPricing([]byte(body), names, "https://other.example/pricing"))
}

func TestDownloadedGapPricingDocuments(t *testing.T) {
	if os.Getenv("TEST_PRICE_GAPS_LIVE_FILES") != "1" {
		t.Skip("opt-in downloaded official documents")
	}
	data, err := os.ReadFile("/tmp/meta-deepseek-pricing.html")
	require.NoError(t, err)
	got := parseDeepSeekDocumentWithPrices(data, map[string]bool{"deepseek-v4.1-flash-1017": true}, "https://api-docs.deepseek.com/quick_start/pricing/")
	require.Len(t, got["deepseek-v4.1-flash-1017"].OfficialPricing.Tiers, 2)
	data, err = os.ReadFile("/tmp/meta-elorian-glimpse.html")
	require.NoError(t, err)
	require.Len(t, parseElorianPricing(data, map[string]bool{"glimpse": true}, elorianPricingURL)["glimpse"].OfficialPricing.Tiers, 1)
}
