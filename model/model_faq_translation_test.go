package model

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFAQTranslationCacheOwnership(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&ModelDescriptionTranslation{}))
			m := Model{ModelName: "faq-translations", FAQ: ModelFAQ{{Question: "Question", Answer: "Answer", QuestionOverrides: ModelDescriptions{"en": "Manual"}}}}
			require.NoError(t, db.Create(&m).Error)
			for _, source := range []string{"Question", "Answer"} {
				row := ModelDescriptionTranslation{CacheKey: SystemTranslationModel + ":" + DescriptionCacheKey(source, "en"), SourceHash: ModelDescriptionHash(source), Locale: "en", Text: "Auto " + source, Completed: true}
				require.NoError(t, db.Create(&row).Error)
			}
			ctx := context.Background()
			require.NoError(t, FillModelFAQTranslations(ctx, []*Model{nil, &m}))
			require.Equal(t, "Manual", m.FAQ[0].QuestionTranslations["en"])
			require.Equal(t, "Auto Question", m.FAQ[0].QuestionAutomatic["en"])
			require.Equal(t, "Auto Answer", m.FAQ[0].AnswerTranslations["en"])
			require.NoError(t, db.Save(&m).Error)
			var saved Model
			require.NoError(t, db.First(&saved, m.Id).Error)
			require.Nil(t, saved.FAQ[0].QuestionAutomatic)
			delete(m.FAQ[0].QuestionOverrides, "en")
			require.NoError(t, FillModelFAQTranslations(ctx, []*Model{&m}))
			require.Equal(t, "Auto Question", m.FAQ[0].QuestionTranslations["en"])
			m.FAQ[0].Question = "New question"
			require.NoError(t, FillModelFAQTranslations(ctx, []*Model{&m}))
			require.Empty(t, m.FAQ[0].QuestionTranslations)
			require.Equal(t, "Auto Answer", m.FAQ[0].AnswerTranslations["en"])
		})
	}
}
