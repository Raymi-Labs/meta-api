package controller

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// Opt-in, read-only audit of a downloaded public catalog. Never writes model
// metadata or billing settings; normal test runs remain offline.
func TestOfficialPricingCatalogCoverage(t *testing.T) {
	path := os.Getenv("TEST_OFFICIAL_PRICING_CATALOG")
	if path == "" {
		t.Skip("opt-in catalog coverage audit")
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var catalog struct {
		Data []struct {
			Name  string                `json:"model_name"`
			Price model.OfficialPricing `json:"official_pricing"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(data, &catalog))
	names := map[string]bool{}
	for _, m := range catalog.Data {
		if len(m.Price.Tiers) == 0 || os.Getenv("TEST_OFFICIAL_PRICING_ALL") == "1" {
			names[m.Name] = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	entries, warnings, err := fetchOfficialModelAttributesFor(ctx, names)
	require.NoError(t, err)
	result := struct {
		Prices   map[string]model.OfficialPricing `json:"prices"`
		Missing  []string                         `json:"missing"`
		Warnings []string                         `json:"warnings"`
	}{Prices: map[string]model.OfficialPricing{}, Warnings: warnings}
	for _, m := range catalog.Data {
		if !names[m.Name] {
			continue
		}
		p := entries[m.Name].OfficialPricing
		if len(p.Tiers) > 0 {
			require.True(t, model.IsManufacturerPriceSource(m.Name, p.SourceURL), m.Name)
			require.NoError(t, p.Normalize(model.NameRuleExact), m.Name)
			result.Prices[m.Name] = p
		} else {
			result.Missing = append(result.Missing, m.Name)
		}
	}
	if out := os.Getenv("TEST_OFFICIAL_PRICING_REPORT"); out != "" {
		b, err := common.Marshal(result)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(out, b, 0600))
	}
	t.Logf("missing before=%d fetched=%d unresolved=%d warnings=%d", len(names), len(result.Prices), len(result.Missing), len(warnings))
}
