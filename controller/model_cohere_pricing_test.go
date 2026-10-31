package controller

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const coherePriceFixture = `<main><h1>Command A</h1><h2>Pricing</h2><div>Input</div><span>$</span><span>2.5</span><span>/ 1M tokens</span><div>Output</div><span>$</span><span>10</span><span>/ 1M tokens</span><h2>Specifications</h2><h3>Model ID</h3><code>command-a-03-2025</code></main>`

func TestCohereManufacturerPricing(t *testing.T) {
	for _, id := range []string{"command-a", "command-a-03-2025"} {
		e, err := parseCoherePricing([]byte(coherePriceFixture), id, "https://docs.cohere.com/v2/docs/command-a")
		require.NoError(t, err)
		require.Equal(t, 2.5, *e.OfficialPricing.Tiers[0].Input)
		require.Equal(t, 10.0, *e.OfficialPricing.Tiers[0].Output)
		require.Equal(t, "official", e.FieldSources["official_pricing"].Kind)
	}
	for _, fixture := range []string{
		strings.ReplaceAll(coherePriceFixture, "command-a-03-2025", "command-other"),
		strings.ReplaceAll(coherePriceFixture, "1M", "1K"),
		strings.ReplaceAll(coherePriceFixture, "Output", "Cached input"),
		strings.ReplaceAll(coherePriceFixture, "2.5", "-1"),
		strings.ReplaceAll(coherePriceFixture, "Specifications", "Unknown"),
	} {
		_, err := parseCoherePricing([]byte(fixture), "command-a", "https://docs.cohere.com/v2/docs/command-a")
		require.Error(t, err)
	}
	_, err := parseCoherePricing([]byte(coherePriceFixture), "command-a:free", "https://docs.cohere.com/v2/docs/command-a")
	require.Error(t, err)
}

func TestCohereNamespacedSyncAndPublicPrice(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Vendor{}, &model.Channel{}, &model.ModelDescriptionTranslation{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1=1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = originalDB
		d, _ := db.DB()
		_ = d.Close()
	})
	input, output := 999.0, 999.0
	m := model.Model{ModelName: "cohere/command-a", SyncOfficial: 1, Status: 1, OfficialPricing: model.OfficialPricing{Currency: "USD", SourceURL: "https://openrouter.ai/cohere/command-a", Tiers: []model.OfficialPricingTier{{Label: "OpenRouter Standard", Unit: "million_tokens", Input: &input, Output: &output}}}}
	require.NoError(t, m.Insert())
	require.NoError(t, db.Create(&model.Ability{Model: m.ModelName, Group: "default", ChannelId: 1, Enabled: true}).Error)
	model.RefreshPricing()
	require.Empty(t, model.GetPricing()[0].OfficialPricing.Tiers)
	original := getHTTPClient()
	client := *original
	httpClient = &client
	t.Cleanup(func() { httpClient = original })
	t.Setenv("MODEL_METADATA_ANTHROPIC_API_KEY", "")
	t.Setenv("MODEL_METADATA_GEMINI_API_KEY", "")
	fail := false
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "https://docs.cohere.com/v2/docs/command-a", r.URL.String())
		status := 200
		if fail {
			status = 503
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(coherePriceFixture)), Request: r}, nil
	})
	names := map[string]bool{m.ModelName: true, "cohere/command-a:free": true}
	entries, warnings, err := fetchOfficialModelAttributesFor(context.Background(), names)
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.NotContains(t, entries, "cohere/command-a:free")
	result, err := model.ApplyModelCapabilityMetadata(context.Background(), entries, "https://community.example")
	require.NoError(t, err)
	require.Equal(t, 1, result.PricingUpdated)
	model.RefreshPricing()
	p := model.GetPricing()[0]
	require.Equal(t, 2.5, *p.OfficialPricing.Tiers[0].Input)
	require.Equal(t, "https://docs.cohere.com/v2/docs/command-a", p.OfficialPricing.SourceURL)
	require.Equal(t, "official", p.AttributeSources["official_pricing"].Kind)
	fail = true
	entries, warnings, err = fetchOfficialModelAttributesFor(context.Background(), names)
	require.NoError(t, err)
	require.NotEmpty(t, warnings)
	_, err = model.ApplyModelCapabilityMetadata(context.Background(), entries, "https://community.example")
	require.NoError(t, err)
	model.RefreshPricing()
	require.Equal(t, p.OfficialPricing, model.GetPricing()[0].OfficialPricing)
}

func TestDownloadedCoherePrice(t *testing.T) {
	dir := os.Getenv("TEST_COHERE_PRICE_DIR")
	if dir == "" {
		t.Skip("opt-in downloaded official pages")
	}
	for _, tc := range []struct {
		slug, id      string
		input, output float64
	}{
		{"command-a", "command-a", 2.5, 10}, {"command-r", "command-r-08-2024", .15, .6},
		{"command-r-plus", "command-r-plus-08-2024", 2.5, 10}, {"command-r7b", "command-r7b-12-2024", .0375, .15},
	} {
		t.Run(tc.id, func(t *testing.T) {
			data, err := os.ReadFile(dir + "/cohere-" + tc.slug + ".html")
			require.NoError(t, err)
			e, err := parseCoherePricing(data, tc.id, "https://docs.cohere.com/v2/docs/"+tc.slug)
			require.NoError(t, err)
			require.Equal(t, tc.input, *e.OfficialPricing.Tiers[0].Input)
			require.Equal(t, tc.output, *e.OfficialPricing.Tiers[0].Output)
		})
	}
}
