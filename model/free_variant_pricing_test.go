package model

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func flintFreePricingModel() Model {
	zero := 0.0
	return Model{ModelName: "flint-v1.0:free", NameRule: NameRuleExact, OfficialPricing: OfficialPricing{
		Currency: "USD", SourceURL: "https://openrouter.ai/recursive/flint-v1.0:free",
		Tiers: []OfficialPricingTier{{Label: "OpenRouter 免费变体", Unit: "million_tokens", Input: &zero, Output: &zero}},
	}}
}

func TestFlintFreeReferencePricing(t *testing.T) {
	for _, name := range []string{"flint-v1.0:free", "recursive/flint-v1.0:free"} {
		m := flintFreePricingModel()
		m.ModelName = name
		for _, address := range []string{"https://openrouter.ai/recursive/flint-v1.0:free", "https://openrouter.ai/recursive/flint-v1.0%3Afree"} {
			m.OfficialPricing.SourceURL = address
			require.False(t, IsManufacturerPriceSource(name, address), "platform quote must not acquire manufacturer provenance")
			price := m.ManufacturerOfficialPricing()
			require.Len(t, price.Tiers, 1)
			require.Zero(t, *price.Tiers[0].Input)
			require.Zero(t, *price.Tiers[0].Output)
			encoded, err := common.Marshal(Pricing{ModelName: name, OfficialPricing: price})
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"input":0`)
			require.Contains(t, string(encoded), `"output":0`)
			require.NotContains(t, string(encoded), `"cache_read"`)
		}
	}
	for label, change := range map[string]func(*Model){
		"unknown free":    func(m *Model) { m.ModelName = "unknown:free" },
		"paid variant":    func(m *Model) { m.ModelName = "flint-v1.0" },
		"wrong namespace": func(m *Model) { m.ModelName = "other/flint-v1.0:free" },
		"prefix rule":     func(m *Model) { m.NameRule = NameRulePrefix },
		"missing input":   func(m *Model) { m.OfficialPricing.Tiers[0].Input = nil },
		"missing output":  func(m *Model) { m.OfficialPricing.Tiers[0].Output = nil },
		"paid input":      func(m *Model) { v := 1.0; m.OfficialPricing.Tiers[0].Input = &v },
		"paid output":     func(m *Model) { v := 1.0; m.OfficialPricing.Tiers[0].Output = &v },
		"negative":        func(m *Model) { v := -1.0; m.OfficialPricing.Tiers[0].Input = &v },
		"nonfinite":       func(m *Model) { v := math.NaN(); m.OfficialPricing.Tiers[0].Output = &v },
		"extra fee":       func(m *Model) { v := 1.0; m.OfficialPricing.Tiers[0].Price = &v },
		"cache fee":       func(m *Model) { v := 1.0; m.OfficialPricing.Tiers[0].CacheRead = &v },
		"write fee":       func(m *Model) { v := 1.0; m.OfficialPricing.Tiers[0].CacheWrite = &v },
		"unit":            func(m *Model) { m.OfficialPricing.Tiers[0].Unit = "request" },
		"currency":        func(m *Model) { m.OfficialPricing.Currency = "CNY" },
		"empty":           func(m *Model) { m.OfficialPricing.Tiers = nil },
		"extra tier":      func(m *Model) { m.OfficialPricing.Tiers = append(m.OfficialPricing.Tiers, m.OfficialPricing.Tiers[0]) },
	} {
		t.Run(label, func(t *testing.T) {
			m := flintFreePricingModel()
			change(&m)
			require.Empty(t, m.ManufacturerOfficialPricing().Tiers)
		})
	}
	for _, address := range []string{
		"https://openrouter.ai/recursive/another:free", "https://openrouter.ai/recursive/flint-v1.0",
		"https://openrouter.ai.evil.test/recursive/flint-v1.0:free", "https://openrouter.ai@evil.test/recursive/flint-v1.0:free",
		"http://openrouter.ai/recursive/flint-v1.0:free", "https://openrouter.ai:443/recursive/flint-v1.0:free",
		"https://openrouter.ai/recursive/flint-v1.0:free?model=other", "https://openrouter.ai/recursive/flint-v1.0:free#other",
		"https://user@openrouter.ai/recursive/flint-v1.0:free", "https://openrouter.ai/recursive/flint-v1.0:free/", "",
	} {
		m := flintFreePricingModel()
		m.OfficialPricing.SourceURL = address
		require.Empty(t, m.ManufacturerOfficialPricing().Tiers, address)
	}
}

func TestReviewedOpenRouterFreeVariantIdentities(t *testing.T) {
	for native, upstream := range verifiedOpenRouterFreeVariants {
		for _, name := range []string{native, upstream} {
			m := flintFreePricingModel()
			m.ModelName = name
			m.OfficialPricing.SourceURL = "https://openrouter.ai/" + upstream
			require.Len(t, m.ManufacturerOfficialPricing().Tiers, 1, name)
			m.OfficialPricing.SourceURL = "https://openrouter.ai/recursive/flint-v1.0:free"
			if upstream != "recursive/flint-v1.0:free" {
				require.Empty(t, m.ManufacturerOfficialPricing().Tiers, "another free model is not proof for %s", name)
			}
			m.OfficialPricing.SourceURL = "https://openrouter.ai/" + upstream
			m.ModelName = "other/" + native
			require.Empty(t, m.ManufacturerOfficialPricing().Tiers)
		}
	}
}

func TestJevNativeNamePreservesManufacturerPricing(t *testing.T) {
	for _, name := range []string{"jev-1.13", "jev-1.14"} {
		require.True(t, IsManufacturerPriceSource(name, "https://help.typesafe.ai/articles/5187202169-how-much-does-jev-cost"))
		require.Equal(t, name, ManufacturerNativeModel("typesafe/"+name))
	}
}
