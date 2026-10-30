package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestModelModalitiesSQLAndJSON(t *testing.T) {
	for _, value := range []interface{}{`["text","image"]`, []byte(`["text","image"]`)} {
		var modalities ModelModalities
		require.NoError(t, modalities.Scan(value))
		require.Equal(t, ModelModalities{"text", "image"}, modalities)
		stored, err := modalities.Value()
		require.NoError(t, err)
		require.Equal(t, `["text","image"]`, stored)
	}
	for _, value := range []interface{}{nil, "", []byte{}, "null", "[]"} {
		var modalities ModelModalities
		require.NoError(t, modalities.Scan(value))
		encoded, err := common.Marshal(modalities)
		require.NoError(t, err)
		require.Equal(t, "[]", string(encoded))
	}
	for _, value := range []interface{}{123, `{"text":true}`, `[1]`, `invalid`} {
		var modalities ModelModalities
		require.Error(t, modalities.Scan(value))
	}
}

func TestModelDisplayMetadataNormalization(t *testing.T) {
	m := Model{DisplayName: "  展示名称  ", ContextLength: 256000,
		InputModalities: ModelModalities{" IMAGE ", "text", "text", "file"}, OutputModalities: ModelModalities{"audio"}}
	require.NoError(t, m.NormalizeDisplayMetadata())
	require.Equal(t, "展示名称", m.DisplayName)
	require.Equal(t, ModelModalities{"text", "image", "file"}, m.InputModalities)
	for _, invalid := range []Model{
		{DisplayName: strings.Repeat("字", 129)},
		{ContextLength: -1},
		{ContextLength: 9007199254740992},
		{InputModalities: ModelModalities{"embedding"}},
		{OutputModalities: ModelModalities{""}},
	} {
		require.Error(t, invalid.NormalizeDisplayMetadata())
	}
}

func TestModelReleaseDateValidation(t *testing.T) {
	for _, value := range []string{"1970-01-01", "2024-02-29", "2026-10-24", "2025-01", "2026-12"} {
		require.True(t, ValidModelReleaseDate(value), value)
		m := Model{ReleaseDate: value}
		require.NoError(t, m.NormalizeDisplayMetadata())
		require.Equal(t, value, m.ReleaseDate, "preserve source precision")
	}
	for _, value := range []string{"", "1969-12-31", "2025-02-29", "2026-04-31", "2026-13", "2026-00", "2026-10-00", "2026-1-01", "2026-10-1", "2026", " 2026-10-24", "2026-10-24 ", "2026-10-24T00:00:00Z", "1792800000"} {
		require.False(t, ValidModelReleaseDate(value), value)
		if value != "" {
			m := Model{ReleaseDate: value}
			require.Error(t, m.NormalizeDisplayMetadata(), value)
		}
	}
	for _, rule := range []int{NameRulePrefix, NameRuleContains, NameRuleSuffix} {
		m := Model{ReleaseDate: "2026-10-24", NameRule: rule}
		require.ErrorContains(t, m.NormalizeDisplayMetadata(), "发布日期仅支持精确匹配模型")
		m.ReleaseDate = ""
		require.NoError(t, m.NormalizeDisplayMetadata())
	}
}

func TestModelDisplayMetadataMigrationCompatibility(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
				if dsn == "" {
					t.Skip("disposable TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
				if dsn == "" {
					t.Skip("disposable TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			tableName := fmt.Sprintf("model_display_%d", time.Now().UnixNano())
			if dialect == "postgres" {
				db = db.Begin()
				require.NoError(t, db.Error)
				t.Cleanup(func() { _ = db.Rollback().Error })
				require.NoError(t, db.Exec("CREATE SCHEMA ?", clause.Table{Name: tableName}).Error)
				require.NoError(t, db.Exec("SET LOCAL search_path TO ?", clause.Table{Name: tableName}).Error)
			}
			tableDB := db.Table(tableName).Session(&gorm.Session{})
			t.Cleanup(func() { _ = tableDB.Migrator().DropTable(tableName) })
			type legacyModel struct {
				Id          int
				ModelName   string
				Description string
			}
			require.NoError(t, tableDB.AutoMigrate(&legacyModel{}))
			require.NoError(t, tableDB.Create(&legacyModel{ModelName: "legacy", Description: "preserved"}).Error)
			require.NoError(t, tableDB.AutoMigrate(&Model{}))
			var restored Model
			require.NoError(t, tableDB.Where("model_name = ?", "legacy").First(&restored).Error)
			require.Equal(t, "preserved", restored.Description)
			require.Empty(t, restored.ReleaseDate)
			require.Zero(t, restored.ContextLength)
			require.Zero(t, restored.MaxOutputTokens)
			require.Empty(t, restored.Capabilities)
			require.Empty(t, restored.FAQ)
			require.Empty(t, restored.InputModalities)
			originalDB := DB
			DB = tableDB
			t.Cleanup(func() { DB = originalDB })
			require.Empty(t, restored.OfficialPricing.Tiers)
			require.NoError(t, common.UnmarshalJsonStr(`{"currency":"USD","tiers":[{"label":"Standard","unit":"million_tokens","input":10,"cache_read":0}]}`, &restored.OfficialPricing))
			restored.FAQ = ModelFAQ{{Question: "问题一", Answer: "回答一\n第二行"}, {Question: "问题二", Answer: "回答二"}}
			restored.MaxOutputTokens = 128000
			restored.Capabilities = ModelCapabilities{"streaming": false, "reasoning": true}
			restored.ReleaseDate = "2025-01"
			restored.DisplayName, restored.ContextLength = "Long context", 256000
			restored.InputModalities = ModelModalities{"text", "image"}
			restored.OutputModalities = ModelModalities{"text"}
			require.NoError(t, restored.Update())
			var saved Model
			require.NoError(t, tableDB.First(&saved, restored.Id).Error)
			require.Equal(t, restored.OfficialPricing, saved.OfficialPricing)
			require.Equal(t, restored.DisplayName, saved.DisplayName)
			require.Equal(t, restored.ReleaseDate, saved.ReleaseDate)
			require.Equal(t, restored.ContextLength, saved.ContextLength)
			require.Equal(t, restored.MaxOutputTokens, saved.MaxOutputTokens)
			require.Equal(t, restored.Capabilities, saved.Capabilities)
			require.Equal(t, restored.FAQ, saved.FAQ)
			require.Equal(t, restored.InputModalities, saved.InputModalities)
			require.Equal(t, restored.OutputModalities, saved.OutputModalities)
			saved.OfficialPricing = OfficialPricing{}
			saved.MaxOutputTokens = 0
			saved.Capabilities = ModelCapabilities{}
			saved.FAQ = ModelFAQ{}
			saved.ReleaseDate = ""
			saved.DisplayName, saved.ContextLength = "", 0
			saved.InputModalities, saved.OutputModalities = ModelModalities{}, ModelModalities{}
			require.NoError(t, saved.Update())
			require.NoError(t, tableDB.First(&restored, restored.Id).Error)
			require.Empty(t, restored.OfficialPricing.Tiers)
			require.Empty(t, restored.DisplayName)
			require.Empty(t, restored.ReleaseDate)
			require.Zero(t, restored.ContextLength)
			require.Zero(t, restored.MaxOutputTokens)
			require.Empty(t, restored.Capabilities)
			require.Empty(t, restored.FAQ)
			require.Empty(t, restored.InputModalities)
			require.Empty(t, restored.OutputModalities)
			require.NoError(t, tableDB.AutoMigrate(&Model{}))
		})
	}
}
