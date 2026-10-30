package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func officialPricingEntry(input float64) ModelCapabilityMetadata {
	output := input * 5
	price := OfficialPricing{Currency: "USD", SourceURL: "https://developers.openai.com/api/docs/models/gpt-example", Tiers: []OfficialPricingTier{{Label: "Standard", Unit: "million_tokens", Input: &input, Output: &output}}}
	return ModelCapabilityMetadata{OfficialPricing: price, FieldSources: map[string]ModelAttributeSource{"official_pricing": {Kind: "official", URL: price.SourceURL}}}
}

func TestOfficialPricingSyncOwnership(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			m := Model{ModelName: "gpt-example", SyncOfficial: 1, Status: 1}
			require.NoError(t, m.Insert())
			run := func(entry ModelCapabilityMetadata) ModelCapabilitySyncResult {
				t.Helper()
				result, err := ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{m.ModelName: entry}, "https://community.example")
				require.NoError(t, err)
				return result
			}
			require.Equal(t, 1, run(officialPricingEntry(10)).PricingUpdated)
			saved := readCapabilityModel(t, m.Id)
			require.True(t, saved.OfficialPricing.Equal(officialPricingEntry(10).OfficialPricing))
			origin := saved.DisplayAttributeSources()["official_pricing"]
			require.Equal(t, "official", origin.Kind)
			require.Positive(t, origin.SyncedAt)
			require.Zero(t, run(officialPricingEntry(10)).Updated) // Idempotent; timestamp means price update, not arbitrary attribute check.
			require.Equal(t, origin, readCapabilityModel(t, m.Id).DisplayAttributeSources()["official_pricing"])
			require.Equal(t, 1, run(officialPricingEntry(11)).PricingUpdated)
			require.True(t, readCapabilityModel(t, m.Id).OfficialPricing.Equal(officialPricingEntry(11).OfficialPricing))
			// Missing/failed source and community costs cannot delete or replace the last official sheet.
			require.Zero(t, run(ModelCapabilityMetadata{ContextLength: 1000}).PricingUpdated)
			candidate := officialPricingEntry(999)
			candidate.FieldSources["official_pricing"] = ModelAttributeSource{Kind: "community", URL: candidate.OfficialPricing.SourceURL}
			require.Zero(t, run(candidate).PricingUpdated)
			require.Equal(t, 11.0, *readCapabilityModel(t, m.Id).OfficialPricing.Tiers[0].Input)
			saved = readCapabilityModel(t, m.Id)
			saved.OfficialPricing = officialPricingEntry(0).OfficialPricing
			require.NoError(t, saved.Update()) // Real administrator CRUD; zero is a manual price, not empty.
			require.NotContains(t, readCapabilityModel(t, m.Id).DisplayAttributeSources(), "official_pricing")
			require.Zero(t, run(officialPricingEntry(12)).PricingUpdated)
			require.Zero(t, *readCapabilityModel(t, m.Id).OfficialPricing.Tiers[0].Input)
			require.NoError(t, db.Model(&m).Update("official_pricing", OfficialPricing{}).Error)
			require.Equal(t, 1, run(officialPricingEntry(12)).PricingUpdated)
			// Compare the raw serialized column: a concurrent manual save wins on every database.
			var snapshot modelCapabilitySnapshot
			require.NoError(t, db.Model(&Model{}).Select("*, official_pricing AS capability_pricing_snapshot, release_date AS capability_release_snapshot, context_length AS capability_context_snapshot, max_output_tokens AS capability_max_output_snapshot, capabilities AS capability_flags_snapshot, input_modalities AS capability_input_snapshot, output_modalities AS capability_output_snapshot, model_capability_sync_state AS capability_state_snapshot").First(&snapshot, m.Id).Error)
			manual := officialPricingEntry(12).OfficialPricing
			manual.Notes = "Manual"
			require.NoError(t, db.Model(&m).Update("official_pricing", manual).Error)
			changed, priceChanged, err := applyModelCapabilitySnapshot(context.Background(), snapshot, officialPricingEntry(13), "source")
			require.NoError(t, err)
			require.False(t, changed)
			require.False(t, priceChanged)
			require.Equal(t, "Manual", readCapabilityModel(t, m.Id).OfficialPricing.Notes)
			require.NoError(t, db.Model(&m).Updates(map[string]interface{}{"sync_official": 0, "official_pricing": OfficialPricing{}}).Error)
			require.Zero(t, run(officialPricingEntry(15)).PricingUpdated)
			require.Empty(t, readCapabilityModel(t, m.Id).OfficialPricing.Tiers)
			require.NoError(t, db.Model(&m).Update("sync_official", 1).Error)
			require.NoError(t, db.Delete(&m).Error)
			require.Zero(t, run(officialPricingEntry(15)).PricingUpdated)
			// Missing enabled exact models receive prices through the same creation path.
			require.NoError(t, db.Create(&Ability{Model: "gpt-new", Group: "default", ChannelId: 1, Enabled: true}).Error)
			result, err := ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{"gpt-new": officialPricingEntry(2)}, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Created)
			require.Equal(t, 1, result.PricingUpdated)
			var fresh Model
			require.NoError(t, db.Where("model_name = ?", "gpt-new").First(&fresh).Error)
			require.Equal(t, 2.0, *fresh.OfficialPricing.Tiers[0].Input)
		})
	}
}
