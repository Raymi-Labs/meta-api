package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelMetadataDefaultsOwnershipAndConcurrentSave(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&Vendor{}))
			m := Model{ModelName: "official-example", Status: 1, SyncOfficial: 1}
			require.NoError(t, db.Create(&m).Error)
			source := ModelAttributeSource{Kind: "official", URL: "https://example.com/models/official-example"}
			entry := ModelCapabilityMetadata{Description: "Official introduction", Tags: "text", VendorName: "OpenAI", Provider: "openai", ContextLength: 100000, FieldSources: map[string]ModelAttributeSource{"description": source, "tags": source, "vendor_id": source, "context_length": source}}
			run := func() {
				_, err := ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{m.ModelName: entry}, "https://community.example")
				require.NoError(t, err)
			}
			run()
			saved := readCapabilityModel(t, m.Id)
			require.Equal(t, entry.Description, saved.Description)
			require.Positive(t, saved.VendorID)
			require.Equal(t, "official", saved.DisplayAttributeSources()["vendor_id"].Kind)
			// An editor opened before a refresh only saves fields it actually edited.
			stale := saved
			entry.Description = "New official introduction"
			entry.ContextLength = 200000
			run()
			stale.Icon = "Custom"
			require.NoError(t, stale.UpdateOmittingMetadata([]string{"description", "tags", "vendor_id", "context_length"}))
			saved = readCapabilityModel(t, m.Id)
			require.Equal(t, entry.Description, saved.Description)
			require.EqualValues(t, 200000, saved.ContextLength)
			// An individual capability override preserves independently refreshed keys.
			require.NoError(t, db.Model(&Model{}).Where("id = ?", m.Id).Update("capabilities", ModelCapabilities{"reasoning": true, "memory": true}).Error)
			no := false
			stale.Capabilities = ModelCapabilities{"reasoning": true, "memory": false}
			require.NoError(t, stale.UpdateWithMetadataOverrides([]string{"description", "tags", "vendor_id", "context_length"}, map[string]*bool{"reasoning": &no}))
			saved = readCapabilityModel(t, m.Id)
			require.False(t, saved.Capabilities["reasoning"])
			require.True(t, saved.Capabilities["memory"])
			// Manual values and case-only edits survive both automatic and racing writes.
			var snapshot modelCapabilitySnapshot
			require.NoError(t, db.Model(&Model{}).Select(modelCapabilitySnapshotColumns).First(&snapshot, m.Id).Error)
			require.NoError(t, db.Model(&Model{}).Where("id = ?", m.Id).Updates(map[string]interface{}{"description": "new official introduction", "tags": "manual", "vendor_id": 777}).Error)
			entry.Description = "Next update"
			entry.VendorID = saved.VendorID
			changed, _, err := applyModelCapabilitySnapshot(context.Background(), snapshot, entry, "source")
			require.NoError(t, err)
			require.False(t, changed)
			run()
			saved = readCapabilityModel(t, m.Id)
			require.Equal(t, "new official introduction", saved.Description)
			require.Equal(t, "manual", saved.Tags)
			require.Equal(t, 777, saved.VendorID)
			require.NotContains(t, saved.DisplayAttributeSources(), "description")
		})
	}
}
