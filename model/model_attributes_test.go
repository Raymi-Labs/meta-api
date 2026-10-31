package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestModelAttributesSourcesOwnershipAndDatabaseSafety(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			m := Model{ModelName: "attributes", Status: 1, SyncOfficial: 1, Capabilities: ModelCapabilities{"background": false}}
			require.NoError(t, db.Create(&m).Error)
			official := ModelAttributeSource{Kind: "official", URL: "https://developers.openai.com/api/docs/models/example"}
			entry := ModelCapabilityMetadata{MaxOutputTokens: 128000, Capabilities: ModelCapabilities{"streaming": true, "background": true, "reasoning": false}, FieldSources: map[string]ModelAttributeSource{"max_output_tokens": official, "capabilities.streaming": official}}
			entries := map[string]ModelCapabilityMetadata{m.ModelName: entry}
			apply := func() Model {
				t.Helper()
				_, err := ApplyModelCapabilityMetadata(context.Background(), entries, "https://community.example/api/all.json")
				require.NoError(t, err)
				return readCapabilityModel(t, m.Id)
			}
			saved := apply()
			require.EqualValues(t, 128000, saved.MaxOutputTokens)
			require.False(t, saved.Capabilities["background"])
			require.Equal(t, official, saved.DisplayAttributeSources()["max_output_tokens"])
			require.Equal(t, "community", saved.DisplayAttributeSources()["capabilities.reasoning"].Kind)
			require.NotContains(t, saved.DisplayAttributeSources(), "capabilities.background")
			// A failed official fetch yields only community data, without downgrading stored official values.
			entry.MaxOutputTokens = 64000
			entry.Capabilities = ModelCapabilities{"streaming": false, "reasoning": true}
			entry.FieldSources = nil
			entries[m.ModelName] = entry
			saved = apply()
			require.EqualValues(t, 128000, saved.MaxOutputTokens)
			require.True(t, saved.Capabilities["streaming"])
			require.True(t, saved.Capabilities["reasoning"])
			// Manual false beats subsequent official true, independently for each key.
			require.NoError(t, db.Model(&m).Update("capabilities", ModelCapabilities{"streaming": false, "reasoning": true, "background": false}).Error)
			entry.Capabilities["streaming"] = true
			entry.FieldSources = map[string]ModelAttributeSource{"capabilities.streaming": official}
			entries[m.ModelName] = entry
			saved = apply()
			require.False(t, saved.Capabilities["streaming"])
			require.NotContains(t, saved.DisplayAttributeSources(), "capabilities.streaming")
			// A concurrent save to either new field must invalidate the old snapshot.
			var snapshot modelCapabilitySnapshot
			require.NoError(t, db.Model(&Model{}).Select(modelCapabilitySnapshotColumns).First(&snapshot, m.Id).Error)
			require.NoError(t, db.Model(&m).Update("max_output_tokens", 8000).Error)
			changed, _, err := applyModelCapabilitySnapshot(context.Background(), snapshot, ModelCapabilityMetadata{ContextLength: 1000}, "source")
			require.NoError(t, err)
			require.False(t, changed)
			require.NoError(t, db.Model(&m).Update("max_output_tokens", snapshot.MaxOutputTokens).Error)
			require.NoError(t, db.Model(&m).Update("capabilities", ModelCapabilities{"streaming": true}).Error)
			changed, _, err = applyModelCapabilitySnapshot(context.Background(), snapshot, ModelCapabilityMetadata{ContextLength: 1000}, "source")
			require.NoError(t, err)
			require.False(t, changed)
		})
	}
}

func TestModelCapabilitiesSerialization(t *testing.T) {
	for _, raw := range []string{`{"streaming":false}`, `{}`, `null`} {
		var flags ModelCapabilities
		require.NoError(t, common.UnmarshalJsonStr(raw, &flags))
		stored, err := flags.Value()
		require.NoError(t, err)
		var restored ModelCapabilities
		require.NoError(t, restored.Scan(stored))
		require.Equal(t, flags, restored)
	}
	for _, raw := range []string{`{"streaming":null}`, `{"streaming":0}`, `{"streaming":"true"}`, `{"unknown":false}`, `[]`} {
		var flags ModelCapabilities
		require.Error(t, common.UnmarshalJsonStr(raw, &flags))
	}
}
