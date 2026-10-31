package controller

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func kimiPricingFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/kimi-pricing-flight.txt")
	require.NoError(t, err)
	payload, err := common.Marshal([]interface{}{1, string(data)})
	require.NoError(t, err)
	return []byte(`<html><head><link rel="canonical" href="` + kimiPricingURL + `"></head><body><h1>Model Inference Pricing</h1><p>1 MTok = 1,000,000 tokens. All prices above are per MTok consumed.</p><script>self.__next_f.push(` + string(payload) + `)</script></body></html>`)
}

func TestKimiOfficialPricesFromFlightData(t *testing.T) {
	data := kimiPricingFixture(t)
	if file := os.Getenv("TEST_KIMI_PRICING_HTML"); file != "" {
		var err error
		data, err = os.ReadFile(file)
		require.NoError(t, err)
	}
	names := map[string]bool{"kimi-k3.1-pro": true, "kimi-k3.1-pro-highspeed": true, "kimi-k3": true, "kimi-k2.7-code": true, "kimi-k3.1-pro-private": true}
	entries, err := parseKimiPricing(data, names)
	require.NoError(t, err)
	require.Len(t, entries, 4)
	require.NotContains(t, entries, "kimi-k3.1-pro-private")
	p := entries["kimi-k3.1-pro"].OfficialPricing
	require.Equal(t, "USD", p.Currency)
	require.Equal(t, kimiPricingURL, p.SourceURL)
	require.Len(t, p.Tiers, 2)
	require.Equal(t, []float64{2, 10, .2, 2}, []float64{*p.Tiers[0].Input, *p.Tiers[0].Output, *p.Tiers[0].CacheRead, *p.Tiers[0].CacheWrite})
	require.Equal(t, 4.0, *p.Tiers[1].CacheWrite)
	require.Contains(t, p.Tiers[0].Label, "5m")
	require.Contains(t, p.Tiers[1].Label, "1h")
	require.Equal(t, 4.0, *entries["kimi-k3.1-pro-highspeed"].OfficialPricing.Tiers[0].Input)
	require.Nil(t, entries["kimi-k2.7-code"].OfficialPricing.Tiers[0].CacheWrite)
	require.Equal(t, "official", entries["kimi-k3.1-pro"].FieldSources["official_pricing"].Kind)
	if file := os.Getenv("TEST_KIMI_PRICING_OUTPUT"); file != "" {
		data, err := common.Marshal(p)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(file, data, 0600))
	}
}

func TestKimiPriceLayoutChangesFailClosed(t *testing.T) {
	original := string(kimiPricingFixture(t))
	for name, change := range map[string]func(string) string{
		"wrong page":        func(s string) string { return strings.ReplaceAll(s, kimiPricingURL, kimiPricingURL+"-other") },
		"missing unit":      func(s string) string { return strings.ReplaceAll(s, "1 MTok = 1,000,000 tokens.", "1K tokens") },
		"shifted columns":   func(s string) string { return strings.ReplaceAll(s, "title:`Input`", "title:`Output`") },
		"unknown retention": func(s string) string { return strings.ReplaceAll(s, "children:`5m writes`", "children:`30m writes`") },
		"currency":          func(s string) string { return strings.ReplaceAll(s, "$2.00 / MTok", "¥2.00 / MTok") },
		"wrong rate unit":   func(s string) string { return strings.ReplaceAll(s, "$2.00 / MTok", "$2.00 / 1K tokens") },
		"duplicate model": func(s string) string {
			return strings.ReplaceAll(s, "children:`kimi-k3.1-pro-highspeed`", "children:`kimi-k3.1-pro`")
		},
		"dynamic expression": func(s string) string { return strings.ReplaceAll(s, "$2.00 / MTok", "${price} / MTok") },
		"incomplete rate": func(s string) string {
			return strings.ReplaceAll(s, "`$2.00 / MTok`,`$10.00 / MTok`", "`$10.00 / MTok`")
		},
		"empty markdown table": func(string) string { return "# Model Inference Pricing\n<table><thead><tr /></thead><tbody /></table>" },
	} {
		t.Run(name, func(t *testing.T) {
			out, err := parseKimiPricing([]byte(change(original)), map[string]bool{"kimi-k3.1-pro": true})
			require.Error(t, err)
			require.Empty(t, out)
		})
	}
}

func TestKimiPriceSourceUsesFixedPublicHTML(t *testing.T) {
	original := getHTTPClient()
	copyClient := *original
	httpClient = &copyClient
	t.Cleanup(func() { httpClient = original })
	fixture := kimiPricingFixture(t)
	var failed bool
	copyClient.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "platform.kimi.ai", r.URL.Host)
		require.Empty(t, r.Header.Get("Authorization"))
		body, status := "", 200
		switch r.URL.Path {
		case "/docs/models.md":
			body = "| Model Name | Description |\n| - | - |\n| kimi-k3.1-pro | Official model description |\n"
		case "/docs/pricing/chat":
			require.Equal(t, "text/html", r.Header.Get("Accept"))
			body = string(fixture)
			if failed {
				body = "empty page"
			}
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	entries, warnings := fetchPublicModelDocuments(context.Background(), map[string]bool{"kimi-k3.1-pro": true})
	require.Empty(t, warnings)
	require.Equal(t, "Official model description", entries["kimi-k3.1-pro"].Description)
	require.Len(t, entries["kimi-k3.1-pro"].OfficialPricing.Tiers, 2)
	failed = true
	entries, warnings = fetchPublicModelDocuments(context.Background(), map[string]bool{"kimi-k3.1-pro": true})
	require.Contains(t, warnings, "moonshotai pricing")
	require.Equal(t, "Official model description", entries["kimi-k3.1-pro"].Description)
	require.Empty(t, entries["kimi-k3.1-pro"].OfficialPricing.Tiers)
	// Empty failed-source metadata is handled by the existing ownership contract;
	// it must not synthesize a replacement price or erase a prior snapshot.
	existing := model.ModelCapabilityMetadata{OfficialPricing: model.OfficialPricing{Currency: "CNY", Tiers: []model.OfficialPricingTier{{Label: "Manual"}}}}
	require.Equal(t, existing.OfficialPricing, mergeOfficialAttributes(existing, entries["kimi-k3.1-pro"]).OfficialPricing)
}

func TestKimiMetadataSyncPublishesPriceAndBundledLocales(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Vendor{}, &model.Channel{}, &model.ModelDescriptionTranslation{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1=1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = originalDB
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	var seeds []struct {
		Source       string
		Translations model.ModelDescriptions
	}
	data, err := os.ReadFile("../model/data/model_description_translations.json")
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(data, &seeds))
	var source string
	for _, seed := range seeds {
		if strings.HasPrefix(seed.Source, "The first model of the new K3.1") {
			source = seed.Source
		}
	}
	require.NotEmpty(t, source)
	m := model.Model{ModelName: "kimi-k3.1-pro", SyncOfficial: 1, Status: 1}
	require.NoError(t, m.Insert())
	require.NoError(t, db.Create(&model.Ability{Model: m.ModelName, Group: "default", ChannelId: 1, Enabled: true}).Error)
	original := getHTTPClient()
	client := *original
	httpClient = &client
	t.Cleanup(func() { httpClient = original })
	t.Setenv("SYNC_UPSTREAM_BASE", "https://community.example")
	t.Setenv("MODEL_METADATA_ANTHROPIC_API_KEY", "")
	t.Setenv("MODEL_METADATA_GEMINI_API_KEY", "")
	failed := false
	fixture := string(kimiPricingFixture(t))
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		body, status := "", 200
		switch r.URL.Host + r.URL.Path {
		case "community.example/api/prices":
			body = `{"moonshotai":{"id":"moonshotai","models":{"kimi-k3.1-pro":{"id":"kimi-k3.1-pro","limit":{"context":1048576}}}}}`
		case "platform.kimi.ai/docs/models.md":
			body = "| Model Name | Description |\n| - | - |\n| kimi-k3.1-pro | " + source + " |\n"
		case "platform.kimi.ai/docs/pricing/chat":
			body = fixture
			if failed {
				status = 503
			}
		default:
			if r.URL.Host == "community.example" {
				body = `{"moonshotai":{"id":"moonshotai","models":{"kimi-k3.1-pro":{"id":"kimi-k3.1-pro","limit":{"context":1048576}}}}}`
			} else {
				t.Fatalf("unexpected source %s", r.URL)
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	result, err := runModelCapabilitySync(context.Background())
	require.NoError(t, err)
	require.Zero(t, result.SourceFailures)
	require.Equal(t, 1, result.PricingUpdated)
	pricing := model.GetPricing()
	require.Len(t, pricing, 1)
	require.Len(t, pricing[0].OfficialPricing.Tiers, 2)
	require.Equal(t, 2.0, *pricing[0].OfficialPricing.Tiers[0].Input)
	require.Len(t, pricing[0].DescriptionTranslations, 17)
	require.Contains(t, pricing[0].DescriptionTranslations["zh-CN"], "Kimi")
	require.NotEqual(t, source, pricing[0].DescriptionTranslations["zh-CN"])
	savedPrice := pricing[0].OfficialPricing
	failed = true
	result, err = runModelCapabilitySync(context.Background())
	require.NoError(t, err)
	require.Positive(t, result.SourceFailures)
	require.Zero(t, result.PricingUpdated)
	model.RefreshPricing()
	require.Equal(t, savedPrice, model.GetPricing()[0].OfficialPricing)
}
