package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestInceptionModelBoundariesAndSync(t *testing.T) {
	for _, name := range []string{"mercury-3", "mercury-2.5", "mercury-coder", "mercury-edit-2", "mercury-decide", "inception/Mercury-Voice", "vendor:mercury-mini"} {
		require.Equal(t, defaultInceptionVendorName, getDefaultVendorName(name), name)
		require.Equal(t, defaultInceptionVendorName, ResolveVendorNameForModel(name, ""), name)
	}
	for _, name := range []string{"", "notmercury-3", "mercurial", "inception/unknown", "mercury-gpt-distill", "qwen/mercury-3", "mimo-mercury"} {
		require.NotEqual(t, defaultInceptionVendorName, getDefaultVendorName(name), name)
		require.Equal(t, "Custom Vendor", ResolveVendorNameForModel(name, "Custom Vendor"), name)
	}
}

func TestInceptionCategoryCountsAndOverrides(t *testing.T) {
	protocol, explicit := "protocol", "inception"
	channels := []*Channel{
		{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "mercury-3,mercury-2.5"},
		{Id: 2, Type: constant.ChannelTypeCustom, Models: "mercury-3", ChannelVendor: &protocol},
		{Id: 3, Type: constant.ChannelTypeCustom, Models: "unknown", ChannelVendor: &explicit},
		{Id: 4, Type: constant.ChannelTypeOpenAI, Models: "mercury-3,claude-sonnet"},
	}
	category, err := ParseChannelCategory("vendor:Inception Labs")
	require.NoError(t, err)
	require.Equal(t, "vendor:inception", category.Key)
	require.EqualValues(t, 2, CountChannelCategories(channels)[category.Key])
	require.EqualValues(t, 2, CountChannelVendors(channels)["inception"])
	filtered := FilterChannelsByCategory(channels, category)
	require.Equal(t, []*Channel{channels[0], channels[2]}, filtered)
	require.Equal(t, constant.ChannelTypeCustom, filtered[1].Type)
}

func TestInceptionPricingFallbackPreservesSharedRulesAndExplicitVendor(t *testing.T) {
	shared := &Model{ModelName: "", NameRule: NameRulePrefix, Status: 1, Description: "custom model description"}
	explicit := &Model{ModelName: "mercury-2.5", VendorID: 99, Status: 1}
	meta := map[string]*Model{"mercury-3": shared, "unknown": shared, "mercury-2.5": explicit}
	vendors := map[int]*Vendor{7: {Id: 7, Name: "Inception Labs"}, 99: {Id: 99, Name: "Custom Vendor"}}
	abilities := []AbilityWithChannel{{Ability: Ability{Model: "mercury-3"}}, {Ability: Ability{Model: "unknown"}}, {Ability: Ability{Model: "mercury-2.5"}}}
	initDefaultVendorMapping(meta, vendors, abilities)
	require.Equal(t, 7, meta["mercury-3"].VendorID)
	require.Equal(t, shared.Description, meta["mercury-3"].Description)
	require.Zero(t, shared.VendorID)
	require.Same(t, shared, meta["unknown"])
	require.Same(t, explicit, meta["mercury-2.5"])
	require.Equal(t, 99, explicit.VendorID)
	require.Equal(t, "Inception", vendors[7].Name)
	require.Equal(t, "Inception", vendors[7].Icon)
	require.Equal(t, defaultInceptionVendorDescription, vendors[7].Description)
	initDefaultVendorMapping(meta, vendors, abilities)
	require.Len(t, vendors, 2)
}

func TestInceptionVendorCreationAndAliasReuse(t *testing.T) {
	db := newMiMoVendorTestDB(t)
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	vendors := map[int]*Vendor{}
	id := getOrCreateVendor("Inception Labs", vendors)
	require.NotZero(t, id)
	require.Equal(t, id, getOrCreateVendor("inceptionlabs", vendors))
	var persisted Vendor
	require.NoError(t, db.First(&persisted, id).Error)
	require.Equal(t, defaultInceptionVendorName, persisted.Name)
	require.Equal(t, defaultInceptionVendorDescription, persisted.Description)
	require.Equal(t, "Inception", persisted.Icon)
	persisted.Icon, persisted.Description = "CustomIcon", "Custom description"
	fillInceptionVendorDefaults(&persisted)
	require.Equal(t, "CustomIcon", persisted.Icon)
	require.Equal(t, "Custom description", persisted.Description)

	legacy := Vendor{Name: " Inception Labs ", Status: 1}
	require.NoError(t, db.Create(&legacy).Error)
	found, err := FindVendorByCanonicalName("inceptionlabs")
	require.NoError(t, err)
	require.Equal(t, id, found.Id, "canonical record wins")
	require.NoError(t, db.Delete(&persisted).Error)
	found, err = FindVendorByCanonicalName("INCEPTION")
	require.NoError(t, err)
	require.Equal(t, legacy.Id, found.Id)
}
