package controller

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestGeminiMigratedPricing(t *testing.T) {
	data, err := os.ReadFile("testdata/gemini-carbon-pricing.markdoc")
	require.NoError(t, err)
	names := map[string]bool{"gemini-4-carbon": true, "gemini-4-carbon-other": true, "gemini-4-argon": true}
	for _, tc := range []struct {
		date                 string
		input, output, cache float64
	}{
		{"2026-11-15", 2, 10, .1}, {"2026-11-30", 2, 10, .1}, {"2026-12-01", 4, 20, .2},
	} {
		t.Run(tc.date, func(t *testing.T) {
			now, err := time.Parse("2006-01-02", tc.date)
			require.NoError(t, err)
			entries := parseGeminiPricingAt(data, names, geminiPricingURL, now)
			require.Len(t, entries, 1)
			p := entries["gemini-4-carbon"].OfficialPricing
			require.Len(t, p.Tiers, 1)
			require.Equal(t, tc.input, *p.Tiers[0].Input)
			require.Equal(t, tc.output, *p.Tiers[0].Output)
			require.NotNil(t, p.Tiers[0].CacheRead)
			require.Equal(t, tc.cache, *p.Tiers[0].CacheRead)
			require.Contains(t, p.Notes, "Implicit caching only")
			require.NoError(t, p.Normalize(model.NameRuleExact))
			require.True(t, model.IsManufacturerPriceSource("gemini-4-carbon", p.SourceURL))
		})
	}
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	for _, broken := range []string{
		strings.Replace(string(data), "/docs/models/gemini-4-carbon", "/docs/models/gemini-4-argon", 1),
		strings.Replace(string(data), `tab title="Standard"`, `tab title="Batch"`, 1),
		strings.Replace(string(data), "Paid Tier, per 1M tokens in USD", "Paid Tier, per image in USD", 1),
	} {
		require.Empty(t, parseGeminiPricingAt([]byte(broken), names, geminiPricingURL, now))
	}
	adjacent := string(data) + strings.ReplaceAll(string(data), "gemini-4-carbon", "gemini-4-argon")
	require.Len(t, parseGeminiPricingAt([]byte(adjacent), names, geminiPricingURL, now), 2)
	require.False(t, model.IsManufacturerPriceSource("gemini-4-carbon", "https://aistudio.google.com.evil.example/docs/pricing"))
}

func TestGeminiPricingFixedSource(t *testing.T) {
	data, err := os.ReadFile("testdata/gemini-carbon-pricing.markdoc")
	require.NoError(t, err)
	original := getHTTPClient()
	copyClient := *original
	httpClient = &copyClient
	t.Cleanup(func() { httpClient = original })
	copyClient.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("x-goog-api-key"))
		body := ""
		if r.URL.String() == geminiPricingURL {
			require.Contains(t, r.Header.Get("Accept"), "text/markdown")
			body = string(data)
		} else {
			require.Equal(t, "ai.google.dev", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	entries, _ := fetchPublicModelDocuments(context.Background(), map[string]bool{"gemini-4-carbon": true})
	require.Len(t, entries["gemini-4-carbon"].OfficialPricing.Tiers, 1)
	require.Equal(t, geminiPricingURL, entries["gemini-4-carbon"].FieldSources["official_pricing"].URL)
}

func TestDownloadedGeminiPricing(t *testing.T) {
	path := os.Getenv("TEST_GEMINI_PRICING_DOCUMENT")
	if path == "" {
		t.Skip("opt-in real document check")
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	entries := parseGeminiPricingAt(data, map[string]bool{"gemini-4-carbon": true, "gemini-4-argon": true}, geminiPricingURL, time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
	require.Len(t, entries, 2)
	for _, e := range entries {
		require.Equal(t, 2.0, *e.OfficialPricing.Tiers[0].Input)
		require.Equal(t, .1, *e.OfficialPricing.Tiers[0].CacheRead)
	}
}
