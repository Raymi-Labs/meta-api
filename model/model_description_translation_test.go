package model

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestDescriptionLocalesMatchFrontend(t *testing.T) {
	source, err := os.ReadFile("../web/src/helpers/localeNormalize.js")
	require.NoError(t, err)
	require.Len(t, ModelDescriptionLocales, 17)
	for _, locale := range ModelDescriptionLocales {
		require.Contains(t, string(source), "['"+locale+"',")
	}
	require.Equal(t, 17, strings.Count(strings.Split(string(source), "];")[0], "  ['"))
}
func TestDescriptionCacheOwnershipAndLease(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&ModelDescriptionTranslation{}, &ModelDescriptionWorker{}))
			ctx := context.Background()
			original := "Original 128K context."
			m := Model{ModelName: "description-example", Description: original, Status: 1}
			require.NoError(t, db.Create(&m).Error)
			for _, locale := range ModelDescriptionLocales {
				row := ModelDescriptionTranslation{CacheKey: DescriptionCacheKey(original, locale), SourceHash: ModelDescriptionHash(original), Locale: locale, Text: locale + " description", Completed: true}
				require.NoError(t, db.Create(&row).Error)
			}
			require.NoError(t, FillModelDescriptionTranslations(ctx, []*Model{&m}))
			require.Len(t, m.DescriptionTranslations, 17)
			require.Equal(t, original, m.Description)
			zh := "人工中文"
			ja := "手動の日本語"
			// Two stale editor sessions change independent locales without lost updates.
			stale := m
			require.NoError(t, m.UpdateWithLocalizedMetadata(nil, nil, map[string]*string{"zh-CN": &zh}))
			require.NoError(t, stale.UpdateWithLocalizedMetadata(nil, nil, map[string]*string{"ja": &ja}))
			var saved Model
			require.NoError(t, db.First(&saved, m.Id).Error)
			require.Equal(t, ModelDescriptions{"zh-CN": zh, "ja": ja}, saved.DescriptionOverrides)
			require.NoError(t, FillModelDescriptionTranslations(ctx, []*Model{&saved}))
			require.Equal(t, zh, saved.DescriptionTranslations["zh-CN"])
			require.Equal(t, "zh-CN description", saved.DescriptionAutomatic["zh-CN"])
			require.NoError(t, saved.UpdateWithLocalizedMetadata(nil, nil, map[string]*string{"zh-CN": nil}))
			require.NoError(t, FillModelDescriptionTranslations(ctx, []*Model{&saved}))
			require.Equal(t, "zh-CN description", saved.DescriptionTranslations["zh-CN"])
			saved.Description = "New source"
			require.NoError(t, FillModelDescriptionTranslations(ctx, []*Model{&saved}))
			require.Equal(t, ModelDescriptions{"ja": ja}, saved.DescriptionTranslations)
			// Partially translated long sources are never exposed.
			row := ModelDescriptionTranslation{CacheKey: DescriptionCacheKey(saved.Description, "en"), SourceHash: ModelDescriptionHash(saved.Description), Locale: "en", Text: "partial"}
			require.NoError(t, db.Create(&row).Error)
			require.NoError(t, FillModelDescriptionTranslations(ctx, []*Model{&saved}))
			require.NotContains(t, saved.DescriptionTranslations, "en")
			require.Error(t, ValidateDescriptionOverrides(map[string]*string{"xx": &zh}))
			require.NoError(t, ValidateDescriptionOverrides(map[string]*string{"zh-CN": nil}))
			state, claimed, err := ClaimModelDescriptionWorker(ctx, 100, "a")
			require.NoError(t, err)
			require.True(t, claimed)
			require.Equal(t, "a", state.Lease)
			_, claimed, err = ClaimModelDescriptionWorker(ctx, 101, "b")
			require.NoError(t, err)
			require.False(t, claimed)
			require.NoError(t, FinishModelDescriptionWorker(ctx, "wrong", 100, 0))
			require.NoError(t, FinishModelDescriptionWorker(ctx, "a", 2000, 3))
			_, claimed, err = ClaimModelDescriptionWorker(ctx, 1000, "restarted")
			require.NoError(t, err)
			require.False(t, claimed)
			state, claimed, err = ClaimModelDescriptionWorker(ctx, 2000, "b")
			require.NoError(t, err)
			require.True(t, claimed)
			require.Equal(t, 3, state.Failures)
			// An expired lease can be recovered, but its late owner cannot clear the new lease.
			_, claimed, err = ClaimModelDescriptionWorker(ctx, 2121, "c")
			require.NoError(t, err)
			require.True(t, claimed)
			require.NoError(t, FinishModelDescriptionWorker(ctx, "b", 0, 0))
			require.NoError(t, db.First(&state, 1).Error)
			require.Equal(t, "c", state.Lease)
			encoded, err := common.Marshal(saved)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "source_hash")
		})
	}
}

func TestDescriptionBundledTranslationsRequireExactSource(t *testing.T) {
	var entries []struct {
		Source       string            `json:"source"`
		Translations ModelDescriptions `json:"translations"`
	}
	require.NoError(t, common.Unmarshal(bundledModelDescriptionsJSON, &entries))
	require.NotEmpty(t, entries)
	for _, entry := range entries {
		require.Len(t, entry.Translations, len(ModelDescriptionLocales))
		for _, locale := range ModelDescriptionLocales {
			require.NotEmpty(t, entry.Translations[locale])
			require.Contains(t, entry.Translations[locale], "GPT-6 Astra")
			require.Contains(t, entry.Translations[locale], "OpenAI")
		}
		require.Equal(t, entry.Source, entry.Translations["en"])
		require.NotContains(t, bundledModelDescriptions, ModelDescriptionHash(entry.Source+" Changed."))
	}
}
