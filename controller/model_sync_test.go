package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Vendor{}))

	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
	})
	return db
}

func TestEnsureVendorIDReusesLegacyMiMoAlias(t *testing.T) {
	db := setupModelSyncTestDB(t)
	legacy := model.Vendor{Name: " mImO ", Status: 1}
	require.NoError(t, db.Create(&legacy).Error)

	canonicalName := model.CanonicalVendorName("MiMo")
	createdVendors := 0
	vendorID := ensureVendorID(
		canonicalName,
		map[string]upstreamVendor{},
		map[string]int{},
		&createdVendors,
	)

	require.Equal(t, legacy.Id, vendorID)
	require.Zero(t, createdVendors)
	var vendorCount int64
	require.NoError(t, db.Model(&model.Vendor{}).Count(&vendorCount).Error)
	require.EqualValues(t, 1, vendorCount)
}

func TestEnsureVendorIDCreatesCanonicalMiMoVendorWithIcon(t *testing.T) {
	db := setupModelSyncTestDB(t)
	canonicalName := model.CanonicalVendorName("MiMo")
	createdVendors := 0
	vendorID := ensureVendorID(
		canonicalName,
		map[string]upstreamVendor{
			canonicalName: {Name: canonicalName, Status: 1},
		},
		map[string]int{},
		&createdVendors,
	)

	require.NotZero(t, vendorID)
	require.Equal(t, 1, createdVendors)
	var persisted model.Vendor
	require.NoError(t, db.First(&persisted, vendorID).Error)
	require.Equal(t, canonicalName, persisted.Name)
	require.Equal(t, "Xiaomi.color='#FF6900'", persisted.Icon)
}

func TestEnsureVendorIDInceptionDefaultsAndAliases(t *testing.T) {
	db := setupModelSyncTestDB(t)
	created := 0
	id := ensureVendorID("Inception Labs", map[string]upstreamVendor{}, map[string]int{}, &created)
	require.NotZero(t, id)
	require.Equal(t, 1, created)
	var vendor model.Vendor
	require.NoError(t, db.First(&vendor, id).Error)
	require.Equal(t, "Inception", vendor.Name)
	require.Equal(t, "Inception", vendor.Icon)
	require.NotEmpty(t, vendor.Description)
	require.Equal(t, id, ensureVendorID("inceptionlabs", map[string]upstreamVendor{}, map[string]int{}, &created))
	require.Equal(t, 1, created)
}
