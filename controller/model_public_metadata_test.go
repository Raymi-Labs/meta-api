package controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestPublicDocumentIdentityAndFields(t *testing.T) {
	claude := []byte("Summary for work\n\nModel ID: `claude-example`\n\n| Platform | Model ID |\n| --- | --- |\n| Claude API | `claude-example` |\n\n| Feature | Value |\n| --- | --- |\n| Context window | 1M tokens |\n| Max output | 128K tokens |\n| Max output (Batch API, beta) | 300K tokens |\n| Modalities | Text and images → text |\n| Released | October 7, 2026 |\n| Thinking | Adaptive (always on) |\n| Input | $4 / MTok |\n| Output | $20 / MTok |\n| 5m cache write | $5 / MTok |\n| 1h cache write | $8 / MTok |\n| Cache read | $0.20 / MTok |\n")
	c, err := parseClaudeDocument(claude, "claude-example", "https://platform.claude.com/example")
	require.NoError(t, err)
	require.EqualValues(t, 1000000, c.ContextLength)
	require.EqualValues(t, 128000, c.MaxOutputTokens)
	require.Equal(t, "2026-10-07", c.ReleaseDate)
	require.Equal(t, "Summary for work", c.Description)
	require.Equal(t, model.ModelModalities{"text", "image"}, c.InputModalities)
	require.Len(t, c.OfficialPricing.Tiers, 2)
	require.Equal(t, 8.0, *c.OfficialPricing.Tiers[1].CacheWrite)
	_, err = parseClaudeDocument(claude, "claude-example-new", "url")
	require.Error(t, err)
	g := []byte(`<nav>Computer use Supported</nav><table><tr><th>Property</th><th>Description</th></tr><tr><td>id_card Model code</td><td>gemini-test</td></tr><tr><td>save Supported data types</td><td>Inputs Text, Image, Video, Audio, and PDF Output Text</td></tr><tr><td>token_auto Token limits</td><td>Input token limit 1,048,576 Output token limit 65,536</td></tr><tr><td>handyman Capabilities</td><td>Thinking Supported Computer use Not supported URL context Supported</td></tr><tr><td>Latest update</td><td>October 2026</td></tr></table>`)
	e, err := parseGeminiDocument(g, "gemini-test", "url")
	require.NoError(t, err)
	require.EqualValues(t, 1048576, e.ContextLength)
	require.False(t, e.Capabilities["computer_use"])
	require.True(t, e.Capabilities["url_context"])
	require.Len(t, e.InputModalities, 5)
	require.Empty(t, e.ReleaseDate)
	_, err = parseGeminiDocument(g, "gemini-other", "url")
	require.Error(t, err)
	require.EqualValues(t, 0, documentTokens("up to 1M or 2M"))
	require.Nil(t, documentModalities("text, unknown"))
}

func TestOfficialCatalogSpansAndExactIDs(t *testing.T) {
	d := []byte(`<table><tr><th rowspan="2">Model ID (Model ID)</th><th>Capability Support</th><th>Length Limit (token)</th></tr><tr><td>not a model</td><td>ignored</td></tr><tr><td>mimo-example<br>mimo-example-2</td><td rowspan="2">Deep Thinking Streaming Output Function Call</td><td rowspan="2">Context Window: 1M Maximum Output: 128K</td></tr><tr><td>mimo-example-fast</td></tr></table>`)
	names := map[string]bool{"mimo-example": true, "mimo-example-fast": true, "mimo-example-private": true}
	entries := parseCatalogTables(d, names, "xiaomi", "https://mimo.mi.com/docs")
	require.Len(t, entries, 2)
	require.EqualValues(t, 128000, entries["mimo-example-fast"].MaxOutputTokens)
	require.True(t, entries["mimo-example"].Capabilities["streaming"])
	deep := []byte(`<table><tr><th colspan="2">MODEL</th><td>deepseek-one (1)</td><td>deepseek-two</td></tr><tr><th colspan="2">CONTEXT LENGTH</th><td colspan="2">1M</td></tr><tr><th rowspan="2">FEATURES</th><td>Json Output</td><td>✓</td><td>Not supported</td></tr><tr><td>Tool Calls</td><td>✓</td><td>✓</td></tr></table>`)
	entries = parseDeepSeekDocument(deep, map[string]bool{"deepseek-one": true, "deepseek-two": true}, "url")
	require.EqualValues(t, 1000000, entries["deepseek-two"].ContextLength)
	require.True(t, entries["deepseek-one"].Capabilities["structured_output"])
	require.False(t, entries["deepseek-two"].Capabilities["structured_output"])
	require.True(t, entries["deepseek-two"].Capabilities["function_calling"])
	index := []byte(`<table><tr><th>Model</th><th>Description</th><th>Endpoint</th></tr><tr><td>Gemini Test</td><td>Official description</td><td>gemini-test</td></tr></table>`)
	entries = parseCatalogTables(index, map[string]bool{"gemini-test": true}, "google", "url")
	require.Equal(t, "Official description", entries["gemini-test"].Description)
}

func TestMetadataRetryBackoff(t *testing.T) {
	require.Equal(t, 24*time.Hour, modelMetadataRetryDelay(24*time.Hour, 0))
	require.Equal(t, 5*time.Minute, modelMetadataRetryDelay(24*time.Hour, 1))
	require.Equal(t, 10*time.Minute, modelMetadataRetryDelay(24*time.Hour, 2))
	require.Equal(t, time.Hour, modelMetadataRetryDelay(24*time.Hour, 99))
}

func TestLivePublicModelDocuments(t *testing.T) {
	if os.Getenv("TEST_LIVE_PUBLIC_METADATA") != "1" {
		t.Skip("opt-in public documentation check")
	}
	names := map[string]bool{"claude-opus-5-5": true, "gemini-3.6-flash": true, "grok-4.7": true, "glm-5.3": true, "mimo-v2.6-pro": true, "deepseek-flash": true, "qwen3.8-max": true, "kimi-k3": true, "MiniMax-M3.1-Flash": true}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	entries, warnings := fetchPublicModelDocuments(ctx, names)
	t.Logf("warnings: %v", warnings)
	for id := range names {
		e, ok := entries[id]
		t.Logf("%s found=%v context=%d output=%d description=%v caps=%v price_tiers=%d", id, ok, e.ContextLength, e.MaxOutputTokens, e.Description != "", e.Capabilities, len(e.OfficialPricing.Tiers))
	}
	require.GreaterOrEqual(t, len(entries), 6, "public sources should be usable without metadata API credentials")
}

func TestDownloadedPublicDocuments(t *testing.T) {
	dir := os.Getenv("TEST_PUBLIC_METADATA_DIR")
	if dir == "" {
		t.Skip("opt-in downloaded official pages")
	}
	for _, x := range []struct {
		name, id string
		parse    func([]byte, string, string) (model.ModelCapabilityMetadata, error)
	}{{"claude-detail", "claude-opus-5-5", parseClaudeDocument}, {"gemini-detail", "gemini-3.6-flash", parseGeminiDocument}, {"xai-detail", "grok-4.7", parseXAIModelDocument}, {"zai-markdown", "glm-5.3", parseZAIModelDocument}} {
		t.Run(x.id, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, "official-"+x.name))
			require.NoError(t, err)
			e, err := x.parse(data, x.id, "https://example.com/model")
			require.NoError(t, err)
			require.Positive(t, e.ContextLength)
			t.Logf("context=%d output=%d description=%v caps=%v price=%d", e.ContextLength, e.MaxOutputTokens, e.Description != "", e.Capabilities, len(e.OfficialPricing.Tiers))
		})
	}
	for name, provider := range map[string]string{"mimo-model": "xiaomi", "qwen-text": "alibaba", "minimax-markdown": "minimax"} {
		data, err := os.ReadFile(filepath.Join(dir, "official-"+name))
		require.NoError(t, err)
		e := parseCatalogTables(data, map[string]bool{"mimo-v2.6-pro": true, "qwen3.8-max": true, "MiniMax-M3.1-Flash": true}, provider, "https://example.com/model")
		require.NotEmpty(t, e, name)
		for id, v := range e {
			require.Positive(t, v.ContextLength, id)
			require.False(t, strings.Contains(v.Description, "<script"))
		}
	}
}

func TestPublicOfficialPricingConditions(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	parts, ok := geminiPriceParts("$0.75 through December 31, 2026. $1.50 starting January 1, 2027.", now)
	require.True(t, ok)
	require.Equal(t, .75, *parts[""])
	parts, ok = geminiPriceParts("$0.75 through December 31, 2026. $1.50 starting January 1, 2027.", now.AddDate(1, 0, 0))
	require.True(t, ok)
	require.Equal(t, 1.5, *parts[""])
	_, ok = geminiPriceParts("$0.05 per image", now)
	require.False(t, ok)
	data := []byte(`<div class="models-section"><h2>Model</h2><code>gemini-test</code></div><div><section><h3>Standard</h3><table><tr><th></th><th>Free Tier</th><th>Paid Tier, per 1M tokens in USD</th></tr><tr><td>Input price</td><td>Free of charge</td><td>$0.75</td></tr><tr><td>Output price (including thinking tokens)</td><td>Free of charge</td><td>$3.75</td></tr></table></section><section><h3>Batch</h3><table><tr><th></th><th>Free Tier</th><th>Paid Tier, per 1M tokens in USD</th></tr><tr><td>Input price</td><td>Free</td><td>$0.1</td></tr><tr><td>Output price</td><td>Free</td><td>$0.2</td></tr></table></section></div>`)
	entries := parseGeminiPricing(data, map[string]bool{"gemini-test": true, "gemini-other": true}, "https://ai.google.dev/pricing")
	require.Len(t, entries, 1)
	require.Equal(t, .75, *entries["gemini-test"].OfficialPricing.Tiers[0].Input)
}

func TestOfficialPriceTablesKeepCurrencyAndTimeBands(t *testing.T) {
	data := []byte(`<table><tr><th>Inference Type</th><th>Model Name</th><th>Input (Cache Hit)</th><th>Input (Cache Miss)</th><th>Output</th></tr><tr><td>Real-time API</td><td>mimo-test</td><td>¥1</td><td>¥2</td><td>¥3</td></tr><tr><td>Real-time API</td><td>mimo-test</td><td>$0.01</td><td>$0.2</td><td>$0.3</td></tr><tr><td>Batch API</td><td>mimo-test</td><td>$0.005</td><td>$0.1</td><td>$0.15</td></tr></table>`)
	entries := parseMiMoPricing(data, map[string]bool{"mimo-test": true}, "https://mimo.mi.com/pricing")
	p := entries["mimo-test"].OfficialPricing
	require.Equal(t, "USD", p.Currency)
	require.Len(t, p.Tiers, 2)
	require.Equal(t, .2, *p.Tiers[0].Input)
	require.Equal(t, .1, *p.Tiers[1].Input)
	data = []byte("All prices are in USD. Prices per 1M tokens.\n\n| Model | Input | Cached Input | Cached Input Storage | Output |\n| - | - | - | - | - |\n| GLM-TEST | $1 | $0.2 | Limited-time Free | $3 |\n")
	entries = parseZAIPricing(data, map[string]bool{"glm-test": true, "glm-test-new": true}, "https://docs.z.ai/pricing")
	require.Len(t, entries, 1)
	require.Equal(t, 3.0, *entries["glm-test"].OfficialPricing.Tiers[0].Output)
	require.Empty(t, communityMetadataVendor("alibaba", "deepseek-test"))
	require.Equal(t, "阿里巴巴", communityMetadataVendor("alibaba", "qwen-test"))
}
