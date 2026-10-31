package model

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelManagementVendorParity(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&Vendor{}, &Channel{}))
			openai := Vendor{Name: "OpenAI", Status: 1}
			custom := Vendor{Name: "Custom", Status: 1}
			require.NoError(t, openai.Insert())
			require.NoError(t, custom.Insert())
			names := []string{"gpt-image-2", "gpt-6.1-sol", "gpt-5.6-sol", "gpt-6-sol", "gpt-6.1-terra", "gpt-image-2.5-flare", "gpt-6-luna", "gpt-6-astra", "gpt-image-2.5-sunburst", "gpt-5.6-terra", "gpt-5.6-luna", "codex-auto-review"}
			inferredIDs := []int{}
			for i, name := range names {
				m := Model{ModelName: name, Status: 1, SyncOfficial: 1}
				if i == 0 {
					m.VendorID = openai.Id
				}
				require.NoError(t, m.Insert())
				if i > 0 {
					inferredIDs = append(inferredIDs, m.Id)
				}
				require.NoError(t, db.Create(&Ability{Model: name, Group: "default", ChannelId: 1, Enabled: true}).Error)
			}
			overridden := Model{ModelName: "gpt-curated", VendorID: custom.Id, Status: 1, SyncOfficial: 1}
			require.NoError(t, overridden.Insert())
			unknown := Model{ModelName: "private-model", Status: 1, SyncOfficial: 1}
			require.NoError(t, unknown.Insert())
			deleted := Model{ModelName: "gpt-deleted", Status: 1, SyncOfficial: 1}
			require.NoError(t, deleted.Insert())
			require.NoError(t, deleted.Delete())
			ctx := context.Background()
			page, err := GetModelManagementPage(ctx, "", strconv.Itoa(openai.Id), 0, 10)
			require.NoError(t, err)
			require.Equal(t, 12, page.Total)
			require.Len(t, page.Items, 10)
			require.Equal(t, 12, page.VendorCounts[openai.Id])
			seen := map[int]bool{}
			for _, m := range page.Items {
				seen[m.Id] = true
				require.Equal(t, openai.Id, m.DisplayVendorID)
			}
			second, err := GetModelManagementPage(ctx, "", strconv.Itoa(openai.Id), 10, 10)
			require.NoError(t, err)
			require.Equal(t, 12, second.Total)
			require.Len(t, second.Items, 2)
			for _, m := range second.Items {
				require.False(t, seen[m.Id])
				require.Equal(t, openai.Id, m.DisplayVendorID)
			}
			named, err := GetModelManagementPage(ctx, "sol", "OpenAI", 0, 10)
			require.NoError(t, err)
			require.Equal(t, 3, named.Total)
			require.Equal(t, page.VendorCounts, named.VendorCounts)
			empty, err := GetModelManagementPage(ctx, "no-match", "OpenAI", 0, 10)
			require.NoError(t, err)
			require.Empty(t, empty.Items)
			require.Zero(t, empty.Total)
			require.Equal(t, 12, empty.VendorCounts[openai.Id])
			manual, err := GetModelManagementPage(ctx, "", "Custom", 0, 10)
			require.NoError(t, err)
			require.Len(t, manual.Items, 1)
			require.Equal(t, overridden.Id, manual.Items[0].Id)
			other, err := GetModelManagementPage(ctx, "", "0", 0, 10)
			require.NoError(t, err)
			require.Len(t, other.Items, 1)
			require.Equal(t, unknown.Id, other.Items[0].Id)
			for _, id := range inferredIDs {
				require.Zero(t, readCapabilityModel(t, id).VendorID, "reads must not persist inferred ownership")
			}
			var saved Model
			require.NoError(t, db.First(&saved, inferredIDs[0]).Error)
			saved.Description = "edited"
			saved.DisplayVendorID = openai.Id
			require.NoError(t, saved.Update())
			require.Zero(t, readCapabilityModel(t, saved.Id).VendorID, "saving unrelated metadata must not promote display ownership")
			// The public pricing resolver and admin response use the same IDs.
			meta := map[string]*Model{}
			vendors := map[int]*Vendor{openai.Id: &openai, custom.Id: &custom}
			abilities := []AbilityWithChannel{}
			for _, name := range names {
				meta[name] = &Model{ModelName: name}
				abilities = append(abilities, AbilityWithChannel{Ability: Ability{Model: name}})
			}
			initDefaultVendorMapping(meta, vendors, abilities)
			for _, row := range append(page.Items, second.Items...) {
				require.Equal(t, meta[row.ModelName].VendorID, row.DisplayVendorID)
			}
		})
	}
}
