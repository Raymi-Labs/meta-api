package model

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAlibabaVendorAuthorBoundaries(t *testing.T) {
	for _, name := range []string{"qwen3.8-flash", "qwen3.7-plus", "Qwen/Qwen3-235B-A22B", "qwq-32b", "qvq-max", "qwen3-tts-vc-realtime", "qwen-image-2.0-pro", "wan2.7-image-pro", "Wan-AI/Wan2.2-T2V-A14B", "happyhorse-1.1-r2v", "happyoyster-1.0", "z-image-turbo"} {
		require.True(t, isAlibabaModel(name), name)
		require.Equal(t, defaultAlibabaVendorName, getDefaultVendorName(name), name)
		for _, alias := range []string{"Alibaba", " alibaba (CHINA) ", "Alibaba Token Plan", "Alibaba Token Plan (China)", "阿里巴巴"} {
			require.Equal(t, defaultAlibabaVendorName, ResolveVendorNameForModel(name, alias), name)
		}
	}
	for _, name := range []string{"qwenish", "wannabe", "happyhorsepower", "z-images", "qwen-deepseek", "qwen3-llama", "deepseek-r1-distill-qwen-32b", "other/qwen3", "qwen/other/qwen3", "qwen3:custom"} {
		require.False(t, isAlibabaModel(name), name)
	}
	require.Equal(t, "DeepSeek", AlibabaMetadataVendor("deepseek-v4"))
	require.Empty(t, AlibabaMetadataVendor("unknown-model"))
	require.Empty(t, ResolveVendorNameForModel("qwen-deepseek", "Alibaba Token Plan"))
	require.Equal(t, "自建厂商", ResolveVendorNameForModel("qwen3", " 自建厂商 "))
	require.Equal(t, "Alibaba Token Plan", CanonicalVendorName("Alibaba Token Plan"), "package is not an unconditional alias")
	require.Equal(t, "my Alibaba proxy", CanonicalVendorName("my Alibaba proxy"))
}

// Dedicated DSNs must point at disposable databases. If MySQL tables already
// exist, keep their DDL intact so this suite can exercise a production schema.
func alibabaVendorTestDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "mysql":
		dsn := os.Getenv("TEST_ALIBABA_MYSQL_DSN")
		if dsn == "" {
			t.Skip("disposable TEST_ALIBABA_MYSQL_DSN not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_ALIBABA_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("disposable TEST_ALIBABA_POSTGRES_DSN not configured")
		}
		driver = postgres.Open(dsn)
	default:
		driver = sqlite.Open(":memory:")
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	for _, table := range []interface{}{&Model{}, &Vendor{}} {
		if !db.Migrator().HasTable(table) {
			require.NoError(t, db.AutoMigrate(table))
		}
		require.NoError(t, db.Unscoped().Where("1 = 1").Delete(table).Error)
	}
	return db
}

func TestAlibabaVendorMigrationCompatibility(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := alibabaVendorTestDB(t, dialect)
			canonical := Vendor{Id: 16, Name: defaultAlibabaVendorName, Icon: "custom-icon", Description: "人工介绍", Status: 1}
			legacy := Vendor{Id: 25, Name: "Alibaba", Status: 1}
			plan := Vendor{Id: 26, Name: "Alibaba Token Plan", Status: 1}
			for _, v := range []*Vendor{&canonical, &legacy, &plan} {
				require.NoError(t, db.Create(v).Error)
			}
			names := []string{"qwen3.8-flash", "qwen3.7-plus", "qwen3.8-max", "qwen3.8-omni-flash", "qwen3-max", "qwen3-vl-plus", "qwen-plus", "qwen-turbo", "qwen-vl-max", "qwen3-coder-plus", "qwen3-235b-a22b", "qwen-image-2.0", "qwen-image-2.0-pro", "wan2.7-image-pro", "happyhorse-1.1-t2v", "happyhorse-1.1-i2v", "happyhorse-1.1-r2v"}
			for i, name := range names {
				vendor := legacy.Id
				if i >= 11 {
					vendor = plan.Id
				}
				state := `{"provider":"alibaba-token-plan","source":"community","future":{"preserve":true}}`
				if i == 11 {
					state = `{"vendor_id":26,"provider":"alibaba-token-plan","source":"community","future":{"preserve":true}}`
				}
				require.NoError(t, db.Create(&Model{ModelName: name, VendorID: vendor, ModelCapabilitySyncState: state, Tags: "人工标签", Description: "人工介绍", ContextLength: 256000, Status: 1, SyncOfficial: 1}).Error)
			}
			// No migration of manually assigned unrelated vendors or zero values.
			require.NoError(t, db.Create(&Model{ModelName: "qwen-manual", VendorID: 999}).Error)
			require.NoError(t, db.Create(&Model{ModelName: "qwen-zero", VendorID: 0}).Error)
			require.NoError(t, db.Model(&Model{}).Where("model_name = ?", "qwen-zero").Update("vendor_id", nil).Error)
			result, err := normalizeAlibabaVendorMetadata(db)
			require.NoError(t, err)
			require.Equal(t, 16, result.VendorID)
			require.EqualValues(t, 17, result.ReassignedModels)
			require.Equal(t, 2, result.RemovedAliases)
			var rows []Model
			require.NoError(t, db.Where("model_name IN ?", names).Find(&rows).Error)
			require.Len(t, rows, 17)
			for _, row := range rows {
				require.Equal(t, 16, row.VendorID)
				require.Equal(t, "人工标签", row.Tags)
				require.Equal(t, "人工介绍", row.Description)
				require.EqualValues(t, 256000, row.ContextLength)
				var state map[string]interface{}
				require.NoError(t, common.UnmarshalJsonStr(row.ModelCapabilitySyncState, &state))
				require.Equal(t, "alibaba-token-plan", state["provider"])
				require.Equal(t, map[string]interface{}{"preserve": true}, state["future"])
				if row.ModelName == "qwen-image-2.0" {
					require.EqualValues(t, 16, state["vendor_id"])
				} else {
					require.NotContains(t, state, "vendor_id")
				}
			}
			var saved Vendor
			require.NoError(t, db.First(&saved, 16).Error)
			require.Equal(t, canonical, saved, "existing metadata must be untouched")
			var untouched []Model
			require.NoError(t, db.Where("model_name IN ?", []string{"qwen-manual", "qwen-zero"}).Order("model_name").Find(&untouched).Error)
			require.Equal(t, 999, untouched[0].VendorID)
			require.Zero(t, untouched[1].VendorID)
			again, err := normalizeAlibabaVendorMetadata(db)
			require.NoError(t, err)
			require.Zero(t, again.ReassignedModels)
			require.Zero(t, again.RemovedAliases)
			require.False(t, again.UpdatedVendor)
		})
	}
}

func TestAlibabaMigrationProtectsMixedAndDeletedReferences(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := alibabaVendorTestDB(t, dialect)
			alias := Vendor{Name: "Alibaba (China)", Status: 1}
			plan := Vendor{Name: "Alibaba Token Plan", Status: 1}
			require.NoError(t, db.Create(&alias).Error)
			require.NoError(t, db.Create(&plan).Error)
			for _, row := range []Model{
				{ModelName: "qwen3", VendorID: plan.Id},
				{ModelName: "unknown", VendorID: plan.Id},
				{ModelName: "deepseek-v4", VendorID: plan.Id},
				{ModelName: "qwen-prefix", NameRule: NameRulePrefix, VendorID: alias.Id},
				{ModelName: "qwen-deleted", VendorID: alias.Id},
			} {
				require.NoError(t, db.Create(&row).Error)
			}
			require.NoError(t, db.Where("model_name = ?", "qwen-deleted").Delete(&Model{}).Error)
			result, err := normalizeAlibabaVendorMetadata(db)
			require.NoError(t, err)
			require.EqualValues(t, 1, result.ReassignedModels)
			require.Zero(t, result.RemovedAliases)
			require.NotEqual(t, alias.Id, result.VendorID)
			require.NotEqual(t, plan.Id, result.VendorID)
			var count int64
			require.NoError(t, db.Model(&Vendor{}).Count(&count).Error)
			require.EqualValues(t, 3, count)
			var deleted Model
			require.NoError(t, db.Unscoped().Where("model_name = ?", "qwen-deleted").First(&deleted).Error)
			require.True(t, deleted.DeletedAt.Valid)
			require.Equal(t, alias.Id, deleted.VendorID)
		})
	}
}

func TestAlibabaVendorReuseAndSync(t *testing.T) {
	db := capabilitySyncTestDB(t, "sqlite")
	require.NoError(t, db.AutoMigrate(&Vendor{}))
	alias := Vendor{Name: " alibaba ", Icon: "custom", Status: 1}
	require.NoError(t, db.Create(&alias).Error)
	result, err := normalizeAlibabaVendorMetadata(db)
	require.NoError(t, err)
	require.Equal(t, alias.Id, result.VendorID)
	require.True(t, result.UpdatedVendor)
	entries := map[string]ModelCapabilityMetadata{}
	for i, name := range []string{"qwen-image-2.0", "wan2.7-image-pro", "deepseek-v4", "unknown-model"} {
		require.NoError(t, db.Create(&Ability{Group: "default", Model: name, ChannelId: i + 1, Enabled: true}).Error)
		entries[name] = ModelCapabilityMetadata{VendorName: "Alibaba Token Plan", Provider: "alibaba-token-plan", ContextLength: 1000}
	}
	_, err = ApplyModelCapabilityMetadata(context.Background(), entries, "community")
	require.NoError(t, err)
	_, err = ApplyModelCapabilityMetadata(context.Background(), entries, "community")
	require.NoError(t, err)
	var vendors []Vendor
	require.NoError(t, db.Find(&vendors).Error)
	require.Len(t, vendors, 2, "only Alibaba and the real third-party author")
	for _, name := range []string{"qwen-image-2.0", "wan2.7-image-pro"} {
		var row Model
		require.NoError(t, db.Where("model_name = ?", name).First(&row).Error)
		require.Equal(t, alias.Id, row.VendorID)
		require.Contains(t, row.ModelCapabilitySyncState, "alibaba-token-plan")
	}
	var unknown Model
	require.NoError(t, db.Where("model_name = ?", "unknown-model").First(&unknown).Error)
	require.Zero(t, unknown.VendorID)
	for _, state := range []string{"", "invalid", `{"vendor_id":999}`, `{"vendor_id":null}`, `{"provider":"alibaba"}`} {
		require.Equal(t, state, reassignAlibabaVendorSnapshot(state, alias.Id, 999))
	}
	found, err := findVendorByCanonicalName(db, "Alibaba (China)")
	require.NoError(t, err)
	require.Equal(t, alias.Id, found.Id)
	mapVendors := map[int]*Vendor{alias.Id: {Id: alias.Id, Name: "Alibaba"}}
	require.Equal(t, alias.Id, getOrCreateVendor(defaultAlibabaVendorName, mapVendors))
	require.Equal(t, defaultAlibabaVendorName, mapVendors[alias.Id].Name)
	require.Equal(t, "Qwen.Color", GetDefaultVendorIcon("Alibaba"))
	require.Equal(t, 999, modelDisplayVendorID("qwen3", 999, mapVendors), fmt.Sprint(mapVendors))
}
