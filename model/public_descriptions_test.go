package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPublicDescriptionExactBoundaries(t *testing.T) {
	for name, review := range publicDescriptionReviews {
		require.Len(t, review.Translations, 17)
		require.Equal(t, review.Source, NormalizePublicModelDescription(name, review.OldSource))
		for _, locale := range ModelDescriptionLocales {
			text := review.Translations[locale]
			require.NotEmpty(t, text)
			for _, term := range []string{"venice", "qihang-ai", "vivgrid", "威尼斯", "奏鸣曲", "俳句"} {
				require.NotContains(t, strings.ToLower(text), term)
			}
			require.Equal(t, text, normalizePublicDescriptionLocale(name, locale, review.OldTranslations[locale]))
		}
	}
	for _, text := range []string{"Venice is a provider. This comparison is intentional.", "An OpenRouter integration guide.", "Manual description with vivgrid context.", "Model is an AI model provided by foo. Additional supported features.", "Anthropic develops Claude.", "Copyright QuantumNous/new-api"} {
		require.Equal(t, text, NormalizePublicModelDescription("claude-sonnet-4-6", text))
	}
	require.Equal(t, "Future Model is an AI model.", NormalizePublicModelDescription("future", "Future Model is an AI model provided by another-provider."))
	require.Equal(t, "OpenRouter 的模型系列。可用型号、接口与价格见模型列表。", NormalizePublicVendorDescription("OpenRouter", "通过 OpenRouter 接入的 OpenRouter 模型"))
	require.Equal(t, "OpenRouter compatible API documentation", NormalizePublicVendorDescription("Example", "OpenRouter compatible API documentation"))
	require.NotContains(t, NormalizePublicVendorDescription("Example", "通过 OpenRouter 接入的 Example 模型"), "OpenRouter")
}

func TestPublicDescriptionMigrationCompatibility(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := alibabaVendorTestDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&ModelDescriptionTranslation{}))
			oldDB := DB
			DB = db
			t.Cleanup(func() { DB = oldDB })
			for name, review := range publicDescriptionReviews {
				raw, _ := common.Marshal(map[string]interface{}{"version": 1, "source": "https://catalog.example/all.json", "provider": "original-provider", "description": review.OldSource, "unknown": map[string]bool{"keep": true}, "field_sources": map[string]interface{}{"description": map[string]string{"kind": "community", "url": "https://catalog.example/all.json"}, "context_length": map[string]string{"kind": "official", "url": "https://author.example/model"}}})
				m := Model{ModelName: name, Description: review.OldSource, DescriptionOverrides: review.OldTranslations.Clone(), ModelCapabilitySyncState: string(raw), VendorID: 97, ContextLength: 128000, Status: 1}
				// A genuine human translation must survive even when the source is remediated.
				m.DescriptionOverrides["ja"] = "手動のモデル説明"
				require.NoError(t, db.Create(&m).Error)
				for locale, text := range review.OldTranslations {
					row := ModelDescriptionTranslation{CacheKey: DescriptionCacheKey(review.OldSource, locale), SourceHash: ModelDescriptionHash(review.OldSource), Locale: locale, Text: text, Completed: true}
					require.NoError(t, db.Where("cache_key = ?", row.CacheKey).Delete(&ModelDescriptionTranslation{}).Error)
					require.NoError(t, db.Create(&row).Error)
				}
			}
			manual := Model{ModelName: "custom", Description: "Vendor comparison mentions OpenRouter intentionally", DescriptionOverrides: ModelDescriptions{"zh-CN": "保留人工介绍"}, Status: 1}
			require.NoError(t, db.Create(&manual).Error)
			v := Vendor{Name: "Cohere", Description: "通过 OpenRouter 接入的 Cohere 模型", Icon: "custom-icon", Status: 2}
			require.NoError(t, db.Create(&v).Error)
			baseline := []Model{}
			require.NoError(t, db.Order("id").Find(&baseline).Error)
			result, err := normalizePublicDescriptions(db)
			require.NoError(t, err)
			require.EqualValues(t, 4, result.Models)
			require.EqualValues(t, 1, result.Vendors)
			for _, before := range baseline {
				var got Model
				require.NoError(t, db.First(&got, before.Id).Error)
				review, known := publicDescriptionReviews[got.ModelName]
				if !known {
					require.Equal(t, before, got)
					continue
				}
				require.Equal(t, review.Source, got.Description)
				require.Equal(t, "手動のモデル説明", got.DescriptionOverrides["ja"])
				var snapshot map[string]interface{}
				require.NoError(t, common.UnmarshalJsonStr(got.ModelCapabilitySyncState, &snapshot))
				require.Equal(t, "original-provider", snapshot["provider"])
				require.Contains(t, snapshot, "unknown")
				require.NotContains(t, snapshot, "description")
				require.NotContains(t, snapshot["field_sources"], "description")
				require.Contains(t, snapshot["field_sources"], "context_length")
				want := before
				want.Description = got.Description
				want.DescriptionOverrides = got.DescriptionOverrides
				want.ModelCapabilitySyncState = got.ModelCapabilitySyncState
				want.UpdatedTime = got.UpdatedTime
				require.Equal(t, want, got, "non-description metadata must be unchanged")
				require.NoError(t, FillModelDescriptionTranslations(context.Background(), []*Model{&got}))
				require.Len(t, got.DescriptionTranslations, 17)
				for locale, text := range review.Translations {
					if locale != "ja" {
						require.Equal(t, text, got.DescriptionTranslations[locale])
					}
				}
				for range 2 {
					changes, err := modelCapabilityChanges(got, ModelCapabilityMetadata{Description: review.OldSource, Provider: "other"}, "https://catalog.example/all.json")
					require.NoError(t, err)
					require.NotContains(t, changes, "description", "sync cannot restore template")
				}
			}
			var vendor Vendor
			require.NoError(t, db.First(&vendor, v.Id).Error)
			require.Equal(t, v.Icon, vendor.Icon)
			require.Equal(t, v.Status, vendor.Status)
			require.NotContains(t, vendor.Description, "OpenRouter")
			again, err := normalizePublicDescriptions(db)
			require.NoError(t, err)
			require.Equal(t, publicDescriptionCleanupResult{}, again)
			// Content failures must roll back model and vendor updates together.
			first := baseline[0]
			require.NoError(t, db.Model(&Model{}).Where("id = ?", first.Id).Update("description", first.Description).Error)
			require.NoError(t, db.Model(&Vendor{}).Where("id = ?", v.Id).Update("description", v.Description).Error)
			name := "test:description-failure"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Table == "vendors" {
					tx.AddError(errors.New("fixture database failure"))
				}
			}))
			result, err = normalizePublicDescriptions(db)
			require.Error(t, err)
			require.Equal(t, publicDescriptionCleanupResult{}, result)
			require.NoError(t, db.Callback().Update().Remove(name))
			var restored Model
			require.NoError(t, db.First(&restored, first.Id).Error)
			require.Equal(t, first.Description, restored.Description)
		})
	}
}

func TestPublicDescriptionReadFailureStillProvidesReviewedLocales(t *testing.T) {
	db := alibabaVendorTestDB(t, "sqlite")
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	review := publicDescriptionReviews["claude-sonnet-4-6"]
	m := Model{ModelName: review.Model, Description: review.OldSource, DescriptionOverrides: review.OldTranslations.Clone()}
	err := FillModelDescriptionTranslations(context.Background(), []*Model{&m})
	require.Error(t, err, "fixture deliberately lacks cache table")
	require.Equal(t, review.Source, m.Description)
	require.Equal(t, review.Translations, m.DescriptionTranslations)
}
