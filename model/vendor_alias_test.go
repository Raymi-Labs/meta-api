package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIdentityVendorCanonicalBoundaries(t *testing.T) {
	for canonical, aliases := range identityVendorAliases {
		for _, name := range aliases {
			require.Equal(t, canonical, CanonicalVendorName("  "+name+"  "))
		}
	}
	for _, name := range []string{"Moondream", "my Moonshot proxy", "Moonshot AI Plan", "Xiaomi", "Tencent TokenHub", "OpenRouter", "Alibaba Token Plan", "z-ai-custom", "ChatGLM hosted"} {
		require.Equal(t, name, CanonicalVendorName(name))
	}
	require.Equal(t, "Moonshot AI", getDefaultVendorName("kimi-k3"))
	require.Equal(t, "Moonshot", getDefaultVendorIcon("MoonshotAI"))
	require.Equal(t, "智谱", getDefaultVendorName("glm-5"))
}

func TestIdentityVendorMigrationCompatibility(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := alibabaVendorTestDB(t, dialect)
			canonical := Vendor{Id: 23, Name: "Moonshot AI", Icon: "CustomIcon", Description: "人工介绍", Status: 2}
			for _, v := range []Vendor{{Id: 19, Name: "Moonshot", Description: "旧人工说明", Status: 1}, canonical, {Id: 59, Name: "MoonshotAI", Description: "通过 OpenRouter 接入的 MoonshotAI 模型", Status: 1}, {Id: 10, Name: "ChatGLM", Status: 1}, {Id: 22, Name: "Z.AI", Status: 1}, {Id: 18, Name: "智谱", Status: 1}, {Id: 80, Name: "Moondream", Status: 1}, {Id: 81, Name: "Tencent TokenHub", Status: 1}, {Id: 82, Name: "Xiaomi", Status: 1}} {
				require.NoError(t, db.Create(&v).Error)
			}
			models := []Model{
				{Id: 1, ModelName: "k3", VendorID: 19, NameRule: NameRuleExact, Status: 1, Description: "人工模型", ModelCapabilitySyncState: `{"vendor_id":19,"source":"openrouter","unknown":{"keep":true}}`},
				{Id: 2, ModelName: "kimi-k3", VendorID: 23, NameRule: NameRuleExact, Status: 1},
				{Id: 3, ModelName: "moonshotai/kimi-k2", VendorID: 59, NameRule: NameRuleExact, Status: 1, ModelCapabilitySyncState: `{"vendor_id":999,"keep":"manual override"}`},
				{Id: 4, ModelName: "kimi-prefix", VendorID: 19, NameRule: NameRulePrefix, Status: 0, ModelCapabilitySyncState: `{"future":true}`},
				{Id: 5, ModelName: "removed", VendorID: 59, NameRule: NameRuleExact, Status: 1, ModelCapabilitySyncState: `invalid`},
				{Id: 6, ModelName: "glm-5", VendorID: 10, Status: 1},
				{Id: 7, ModelName: "glm-5.3", VendorID: 22, Status: 1},
				{Id: 8, ModelName: "custom", VendorID: 80, Status: 1},
			}
			for i := range models {
				require.NoError(t, db.Create(&models[i]).Error)
			}
			require.NoError(t, db.Delete(&models[4]).Error)
			var before []Model
			require.NoError(t, db.Unscoped().Order("id").Find(&before).Error)
			result, err := normalizeIdentityVendorMetadata(db)
			require.NoError(t, err)
			require.EqualValues(t, 6, result.ReassignedModels)
			require.Equal(t, 4, result.RemovedAliases)
			var got Vendor
			require.NoError(t, db.First(&got, 23).Error)
			require.Equal(t, canonical.Name, got.Name)
			require.Equal(t, canonical.Icon, got.Icon)
			require.Equal(t, canonical.Description, got.Description)
			require.Equal(t, canonical.Status, got.Status)
			var after []Model
			require.NoError(t, db.Unscoped().Order("id").Find(&after).Error)
			require.Len(t, after, len(before))
			for i := range before {
				want := before[i]
				switch want.VendorID {
				case 19, 59:
					want.VendorID = 23
				case 10, 22:
					want.VendorID = 18
				}
				if i == 0 {
					var fields map[string]interface{}
					require.NoError(t, common.UnmarshalJsonStr(after[i].ModelCapabilitySyncState, &fields))
					require.EqualValues(t, 23, fields["vendor_id"])
					require.Equal(t, "openrouter", fields["source"])
					require.Equal(t, map[string]interface{}{"keep": true}, fields["unknown"])
					want.ModelCapabilitySyncState = after[i].ModelCapabilitySyncState
				}
				want.UpdatedTime = after[i].UpdatedTime
				require.Equal(t, want, after[i], "only ownership and matching sync snapshot may change")
			}
			var alias Vendor
			require.NoError(t, db.Unscoped().First(&alias, 19).Error)
			require.True(t, alias.DeletedAt.Valid)
			require.Equal(t, "旧人工说明", alias.Description)
			again, err := normalizeIdentityVendorMetadata(db)
			require.NoError(t, err)
			require.Equal(t, vendorAliasNormalizationResult{}, again)
			for _, id := range []int{80, 81, 82} {
				require.NoError(t, db.First(&Vendor{}, id).Error)
			}
			original := DB
			DB = db
			t.Cleanup(func() { DB = original })
			v, err := FindVendorByCanonicalName("  moonshotai ")
			require.NoError(t, err)
			require.Equal(t, 23, v.Id)
			duplicate, err := IsVendorNameDuplicated(0, " z.ai ")
			require.NoError(t, err)
			require.True(t, duplicate)
			duplicate, err = IsVendorNameDuplicated(18, "zhipu")
			require.NoError(t, err)
			require.False(t, duplicate)
		})
	}
}

func TestIdentityVendorMigrationRollbackAndRename(t *testing.T) {
	db := alibabaVendorTestDB(t, "sqlite")
	require.NoError(t, db.Create(&Vendor{Id: 1, Name: "moonshotai", Status: 1}).Error)
	require.NoError(t, db.Create(&Vendor{Id: 2, Name: "Moonshot", Status: 1}).Error)
	require.NoError(t, db.Create(&Model{Id: 1, ModelName: "k3", VendorID: 2}).Error)
	callback := "test:alias-failure"
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "vendors" {
			tx.AddError(errors.New("fixture failure"))
		}
	}))
	result, err := normalizeIdentityVendorMetadata(db)
	require.Error(t, err)
	require.Equal(t, vendorAliasNormalizationResult{}, result)
	var v Vendor
	var m Model
	require.NoError(t, db.First(&v, 1).Error)
	require.Equal(t, "moonshotai", v.Name)
	require.NoError(t, db.First(&m, 1).Error)
	require.Equal(t, 2, m.VendorID)
	require.NoError(t, db.First(&Vendor{}, 2).Error)
	require.NoError(t, db.Callback().Delete().Remove(callback))
	_, err = normalizeIdentityVendorMetadata(db)
	require.NoError(t, err)
	require.NoError(t, db.First(&v, 1).Error)
	require.Equal(t, "Moonshot AI", v.Name)
	require.Equal(t, "Moonshot", v.Icon)
}

func TestIdentityVendorFallbackDoesNotCreateDuplicate(t *testing.T) {
	db := alibabaVendorTestDB(t, "sqlite")
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	require.NoError(t, db.Create(&Vendor{Id: 3, Name: " MoonshotAI ", Status: 1}).Error)
	found, err := FindVendorByCanonicalName("Moonshot")
	require.NoError(t, err)
	require.Equal(t, 3, found.Id)
	vendors := map[int]*Vendor{3: found, 7: {Id: 7, Name: "moonshot"}}
	require.Equal(t, 3, getOrCreateVendor("Moonshot AI", vendors))
	var count int64
	require.NoError(t, db.Model(&Vendor{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
