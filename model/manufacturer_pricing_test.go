package model

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestManufacturerPriceSources(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		allowed   bool
	}{
		{"cohere/command-a", "https://docs.cohere.com/v2/docs/command-a", true},
		{"command-a-03-2025", "https://cohere.com/pricing", true},
		{"gpt-example", "https://developers.openai.com/api/docs/models/gpt-example", true},
		{"anthropic/claude-test", "https://platform.claude.com/docs", true},
		{"cohere/command-a", "https://openrouter.ai/cohere/command-a", false},
		{"cohere/command-a", "https://developers.openai.com/pricing", false},
		{"cohere/command-a", "https://docs.cohere.com.evil.test/pricing", false},
		{"cohere/command-a", "https://cohere.com@evil.test/pricing", false},
		{"cohere/command-a", "", false},
		{"amazon/nova-lite-v1", "https://aws.amazon.com/nova/pricing/", true},
		{"thinkingmachines/inkling", "https://tinker-docs.thinkingmachines.ai/tinker/models/", true},
		{"writer/palmyra-x5", "https://dev.writer.com/home/pricing", true},
		{"morph/morph-v3-fast", "https://www.morphllm.com/api/models/json", true},
		{"aion-labs/aion-2.1", "https://api.aionlabs.ai/v1/models", true},
		{"amazon/nova-lite-v1", "https://openrouter.ai/amazon/nova-lite-v1", false},
		{"amazon/nova-lite-v1", "https://aws.amazon.com.evil.test/", false},
		{"cohere/command-a", "http://cohere.com/pricing", false},
		{"unknown/gpt-example", "https://openai.com/pricing", false},
		{"unknown-model", "https://cohere.com/pricing", false},
		{"cohere/command-a:free", "https://cohere.com/pricing", false},
	} {
		require.Equal(t, tc.allowed, IsManufacturerPriceSource(tc.name, tc.url), "%s %s", tc.name, tc.url)
	}
}

func TestLegacyResellerPriceReplacement(t *testing.T) {
	m := Model{ModelName: "gpt-example", OfficialPricing: officialPricingEntry(999).OfficialPricing}
	m.OfficialPricing.SourceURL = "https://openrouter.ai/openai/gpt-example"
	require.Empty(t, m.ManufacturerOfficialPricing().Tiers)
	changes, err := modelCapabilityChanges(m, officialPricingEntry(10), "community")
	require.NoError(t, err)
	require.Equal(t, officialPricingEntry(10).OfficialPricing, changes["official_pricing"])
	// Network/parse failures preserve the stored legacy sheet, without publishing it.
	changes, err = modelCapabilityChanges(m, ModelCapabilityMetadata{}, "community")
	require.NoError(t, err)
	require.NotContains(t, changes, "official_pricing")
	// A false 'official' provenance claim cannot publish a reseller price.
	entry := officialPricingEntry(10)
	entry.OfficialPricing.SourceURL = m.OfficialPricing.SourceURL
	entry.FieldSources["official_pricing"] = ModelAttributeSource{Kind: "official", URL: m.OfficialPricing.SourceURL}
	changes, err = modelCapabilityChanges(m, entry, "community")
	require.NoError(t, err)
	require.NotContains(t, changes, "official_pricing")
	// Existing first-party manual zero prices are protected.
	m.OfficialPricing = officialPricingEntry(0).OfficialPricing
	changes, err = modelCapabilityChanges(m, officialPricingEntry(10), "community")
	require.NoError(t, err)
	require.NotContains(t, changes, "official_pricing")
	require.NotEmpty(t, m.ManufacturerOfficialPricing().Tiers)
}

func TestManufacturerNativeModel(t *testing.T) {
	for _, name := range []string{"amazon/nova-lite-v1", "aion-labs/aion-2.1", "thinkingmachines/inkling", "writer/palmyra-x5", "morph/morph-v3-fast"} {
		require.Equal(t, strings.SplitN(name, "/", 2)[1], ManufacturerNativeModel(name))
	}
	require.Equal(t, "writer/gpt-6", ManufacturerNativeModel("writer/gpt-6"))
	require.Equal(t, "gpt-example", ManufacturerNativeModel("openai/gpt-example"))
	require.Equal(t, "cohere/gpt-example", ManufacturerNativeModel("cohere/gpt-example"))
	require.Equal(t, "unknown/command-a", ManufacturerNativeModel("unknown/command-a"))
}
