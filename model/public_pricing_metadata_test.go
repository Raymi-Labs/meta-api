package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestPublicPricingMetadataSanitizesWithoutMutatingOrRepricing(t *testing.T) {
	price := 2.5
	in := []Pricing{{ModelName: "gpt-4o", VendorID: 1, ModelRatio: 42, EnableGroup: []string{"优选"}, BillingExpr: "p * 5 + c * 10",
		Description: "Fast text model. Served by OpenRouter.", DescriptionTranslations: ModelDescriptions{"zh-CN": "支持工具。通过 OpenRouter 接入。"},
		FAQ:              ModelFAQ{{Question: "服务条件", Answer: "缓存有效期5分钟；OpenRouter 当前账号实测接入"}},
		OfficialPricing:  OfficialPricing{Currency: "USD", SourceURL: "https://developers.openai.com/api/docs/pricing", Notes: "缓存有效期5分钟；仅在 OpenRouter 当前账号实测成功后接入", Tiers: []OfficialPricingTier{{Label: "Standard ≤272K", Unit: "million_tokens", Input: &price}}},
		AttributeSources: map[string]ModelAttributeSource{"description": {Kind: "community", URL: "https://openrouter.ai/openai/gpt-4o"}, "official_pricing": {Kind: "official", URL: "https://developers.openai.com/api/docs/pricing", SyncedAt: 123}},
	}}
	vendors := []PricingVendor{{ID: 1, Name: "OpenAI", Description: "通过 OpenRouter 接入。OpenAI 模型。"}}
	before, _ := common.Marshal(in)
	vb, _ := common.Marshal(vendors)
	out, vo := PublicPricingMetadata(in, vendors)
	raw, _ := common.Marshal(out)
	require.NotContains(t, strings.ToLower(string(raw)), "openrouter")
	require.Equal(t, "缓存有效期5分钟", out[0].OfficialPricing.Notes)
	require.Equal(t, price, *out[0].OfficialPricing.Tiers[0].Input)
	require.Equal(t, in[0].BillingExpr, out[0].BillingExpr)
	require.Equal(t, in[0].ModelRatio, out[0].ModelRatio)
	require.Equal(t, in[0].EnableGroup, out[0].EnableGroup)
	require.Equal(t, "OpenAI", vo[0].Name)
	require.Contains(t, vo[0].Description, "OpenAI")
	require.EqualValues(t, 123, out[0].AttributeSources["official_pricing"].SyncedAt)
	after, _ := common.Marshal(in)
	va, _ := common.Marshal(vendors)
	require.Equal(t, string(before), string(after))
	require.Equal(t, string(vb), string(va))
	out[0].DescriptionTranslations["zh-CN"] = "changed"
	out[0].OfficialPricing.Tiers[0].Label = "changed"
	delete(out[0].AttributeSources, "official_pricing")
	require.Equal(t, "Standard ≤272K", in[0].OfficialPricing.Tiers[0].Label)
	require.Len(t, in[0].AttributeSources, 2)
}

func TestPublicPricingMetadataFreeVariantIsNotManufacturerPrice(t *testing.T) {
	m := flintFreePricingModel()
	in := []Pricing{{ModelName: m.ModelName, OfficialPricing: m.ManufacturerOfficialPricing(), ModelPrice: 9.9, VendorID: 63}}
	out, v := PublicPricingMetadata(in, []PricingVendor{{ID: 63, Name: "Venice"}})
	require.Empty(t, out[0].OfficialPricing.Tiers)
	require.Equal(t, 9.9, out[0].ModelPrice)
	require.Equal(t, m.ModelName, out[0].ModelName)
	require.Zero(t, out[0].VendorID)
	require.Empty(t, v)
	require.NotNil(t, v)
	// Fireworks is the actual manufacturer here, not a reseller to erase.
	out, v = PublicPricingMetadata([]Pricing{{ModelName: "fireworks/ember-1", Description: "Fireworks Ember", VendorID: 4}}, []PricingVendor{{ID: 4, Name: "Fireworks", Description: "https://fireworks.ai/"}})
	require.Equal(t, "Fireworks Ember", out[0].Description)
	require.Equal(t, "Fireworks", v[0].Name)
}

func TestPublicPricingMetadataHostAndTextAdversaries(t *testing.T) {
	// Legitimate manufacturer query parameters are not a reason to erase its
	// prices, but queries/fragments must not disclose internal navigation data.
	p := 2.0
	out, _ := PublicPricingMetadata([]Pricing{{ModelName: "morph-v3-large", OfficialPricing: OfficialPricing{Currency: "USD", SourceURL: "https://www.morphllm.com/api/models/json?prices=list", Tiers: []OfficialPricingTier{{Label: "List price", Input: &p}}}}}, nil)
	require.Len(t, out[0].OfficialPricing.Tiers, 1)
	require.Equal(t, "https://www.morphllm.com/api/models/json", out[0].OfficialPricing.SourceURL)
	for _, text := range []string{"OPENROUTER", "Open\u200bRouter", "Open%52outer", "Open&#82;outer", "[link](https://openrouter.ai/openai/gpt-4o)", "https://openai.com.evil.test/", "https://openai.com@evil.test/", "https://developers.openai.com/?redirect=https://openrouter.ai", "https://unreviewed-platform.test/a", "www.venice.ai", "//openrouter.ai", "接入muzeai平台"} {
		require.True(t, privatePublicMetadata(text), text)
		require.Empty(t, publicMetadataText(text), text)
	}
	for _, text := range []string{"OpenAI GPT; Anthropic Claude; Google Gemini", "https://developers.openai.com/api/docs/pricing", "https://meta-api.vip/model/gpt-4o", "Copyright QuantumNous/new-api", "Cache writes 5m: 1.25x; 1h: 2x."} {
		require.Equal(t, text, publicMetadataText(text))
	}
	for _, address := range []string{"https://openai.com.evil.test/p", "https://user@openai.com/p", "http://openai.com/p", "https://openai.com:443/p", "https://openrouter.ai/openai/gpt-4o"} {
		p := 1.0
		out, _ := PublicPricingMetadata([]Pricing{{ModelName: "gpt-4o", OfficialPricing: OfficialPricing{SourceURL: address, Tiers: []OfficialPricingTier{{Label: "Standard", Input: &p}}}}}, nil)
		require.Empty(t, out[0].OfficialPricing.Tiers, address)
	}
}

// Optional read-only public snapshot audit; fixture path is provided explicitly
// and never embeds production accounts, credentials, or mutable price defaults.
func TestPublicPricingMetadataSnapshot(t *testing.T) {
	path := os.Getenv("PUBLIC_PRICING_SNAPSHOT")
	if path == "" {
		t.Skip("no public snapshot supplied")
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var payload struct {
		Data    []Pricing       `json:"data"`
		Vendors []PricingVendor `json:"vendors"`
	}
	require.NoError(t, common.Unmarshal(data, &payload))
	out, vendors := PublicPricingMetadata(payload.Data, payload.Vendors)
	data, err = common.Marshal(struct {
		Data    []Pricing       `json:"data"`
		Vendors []PricingVendor `json:"vendors"`
	}{out, vendors})
	require.NoError(t, err)
	require.NotRegexp(t, publicResellerMention, string(data))
	for i, p := range out {
		require.Equal(t, payload.Data[i].ModelName, p.ModelName)
		require.Equal(t, payload.Data[i].ModelRatio, p.ModelRatio)
		require.Equal(t, payload.Data[i].BillingExpr, p.BillingExpr)
		require.Equal(t, payload.Data[i].ModelPrice, p.ModelPrice)
		require.Equal(t, payload.Data[i].EnableGroup, p.EnableGroup)
	}
	if dest := os.Getenv("PUBLIC_PRICING_AUDIT_OUTPUT"); dest != "" {
		require.NoError(t, os.WriteFile(dest, data, 0600))
	}
	t.Logf("public snapshot audited: %d models, %d vendors; model identities and billing unchanged", len(out), len(vendors))
}
