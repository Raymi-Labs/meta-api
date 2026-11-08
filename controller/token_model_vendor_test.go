package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestTokenModelOptionsCarryPricingVendor(t *testing.T) {
	isolateOfficialAttributeRequests(t)
	db := setupModelSyncTestDB(t)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Channel{}, &model.ModelPermission{}))
	t.Cleanup(func() {
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
	})
	require.NoError(t, db.Create(&model.Vendor{Id: 1, Name: "OpenAI"}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "vendor-fixture", VendorID: 1, Status: 1}).Error)
	for i, name := range []string{"vendor-fixture", "no-meta-fixture"} {
		require.NoError(t, db.Create(&model.Channel{Id: i + 1, Name: "fixture", Type: 1, Key: "fixture", Status: 1, Group: "default", Models: name}).Error)
		require.NoError(t, db.Create(&model.Ability{ChannelId: i + 1, Model: name, Group: "default", Enabled: true}).Error)
	}
	model.RefreshPricing()

	options := []tokenModelOption{{Name: "vendor-fixture"}, {Name: "no-meta-fixture"}, {Name: "unknown-fixture"}}
	fillTokenModelVendors(options)
	require.Equal(t, "OpenAI", options[0].Vendor)
	require.Empty(t, options[1].Vendor, "models without vendor metadata stay unassigned")
	require.Empty(t, options[2].Vendor)
	fillTokenModelVendors(nil)
}
