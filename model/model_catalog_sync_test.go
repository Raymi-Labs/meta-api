package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureModelCatalog(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			manual := Model{ModelName: "manual", Status: 1, SyncOfficial: 0, Description: "curated", ContextLength: 42}
			require.NoError(t, manual.Insert())
			deleted := Model{ModelName: "deleted", Status: 1}
			require.NoError(t, deleted.Insert())
			require.NoError(t, deleted.Delete())
			for _, rule := range []Model{{ModelName: "prefix-", NameRule: NameRulePrefix}, {ModelName: "-suffix", NameRule: NameRuleSuffix}, {ModelName: "contains", NameRule: NameRuleContains}} {
				require.NoError(t, db.Create(&rule).Error)
			}
			for i, name := range []string{"unknown-custom-model", "manual", "deleted", "prefix-model", "model-suffix", "model-contains-text", "disabled", strings.Repeat("x", 129)} {
				require.NoError(t, db.Create(&Ability{Group: "default", Model: name, ChannelId: i + 1, Enabled: name != "disabled"}).Error)
			}
			require.NoError(t, db.Create(&Ability{Group: "second", Model: "unknown-custom-model", ChannelId: 100, Enabled: true}).Error)
			count, err := EnsureModelCatalog(context.Background())
			require.NoError(t, err)
			require.Equal(t, 1, count)
			var fresh Model
			require.NoError(t, db.Where("model_name = ?", "unknown-custom-model").First(&fresh).Error)
			require.Equal(t, 1, fresh.SyncOfficial)
			require.Equal(t, 1, fresh.Status)
			require.Empty(t, fresh.ModelCapabilitySyncState)
			require.Zero(t, fresh.ContextLength)
			require.Equal(t, NameRuleExact, fresh.NameRule)
			require.Equal(t, manual.Description, readCapabilityModel(t, manual.Id).Description)
			count, err = EnsureModelCatalog(context.Background())
			require.NoError(t, err)
			require.Zero(t, count)
			require.NoError(t, db.Model(&Ability{}).Where("model = ?", fresh.ModelName).Update("enabled", false).Error)
			count, err = EnsureModelCatalog(context.Background())
			require.NoError(t, err)
			require.Zero(t, count)
			require.Equal(t, fresh.Id, readCapabilityModel(t, fresh.Id).Id, "channel outages retain editable metadata")
			// A row added by local reconciliation receives official attributes later.
			entry := capabilityEntry(128000)
			entry.FieldSources = map[string]ModelAttributeSource{"context_length": {Kind: "official", URL: "https://example.com/official"}}
			result, err := ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{fresh.ModelName: entry}, "https://example.com/catalog")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated)
			require.EqualValues(t, 128000, readCapabilityModel(t, fresh.Id).ContextLength)
			_, err = ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{fresh.ModelName: capabilityEntry(10)}, "https://example.com/catalog")
			require.NoError(t, err)
			require.EqualValues(t, 128000, readCapabilityModel(t, fresh.Id).ContextLength, "community data must not replace an official value")
		})
	}
}
