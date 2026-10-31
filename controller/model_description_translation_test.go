package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func descriptionTestDB(t *testing.T) {
	db := catalogControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.ModelDescriptionTranslation{}, &model.ModelDescriptionWorker{}, &model.Token{}, &model.User{}, &model.Option{}))
	require.NoError(t, db.Create(&model.User{Id: 100, Username: "translation-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 100, UserId: 100, Name: "System translation", Key: "testsystemtranslation", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: model.SystemTranslationModel}).Error)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	previous := common.OptionMap[model.SystemTranslationTokenOption]
	common.OptionMap[model.SystemTranslationTokenOption] = "100"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap[model.SystemTranslationTokenOption] = previous
		common.OptionMapRWMutex.Unlock()
	})
	m := model.Model{ModelName: "gpt-description", Description: "A model with 128K context and JSON output.", Status: 1}
	require.NoError(t, db.Create(&m).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: m.ModelName, ChannelId: 1, Enabled: true}).Error)
	model.RefreshPricing()
}

// Exercise the real loopback HTTP boundary without invoking a paid provider.
func descriptionMockRelay(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("PORT", strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
}
func descriptionRequest(t *testing.T, r *http.Request) (string, string) {
	t.Helper()
	require.Equal(t, "POST", r.Method)
	require.Equal(t, "/v1/chat/completions", r.URL.Path)
	require.Equal(t, "Bearer sk-testsystemtranslation", r.Header.Get("Authorization"))
	require.Empty(t, r.Header.Get("Cookie"))
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.DecodeJson(r.Body, &request))
	require.Equal(t, model.SystemTranslationModel, request.Model)
	require.False(t, request.Stream)
	require.Len(t, request.Messages, 1)
	require.Equal(t, "user", request.Messages[0].Role)
	var options map[string]string
	require.NoError(t, common.Unmarshal(request.TranslationOptions, &options))
	require.Equal(t, "auto", options["source_lang"])
	return request.Messages[0].StringContent(), options["target_lang"]
}
func descriptionResponse(w http.ResponseWriter, value string) {
	body, _ := common.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": value}, "finish_reason": "stop"}}})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
func TestDescriptionTranslationAllLocalesAndNoRequestOnRead(t *testing.T) {
	descriptionTestDB(t)
	calls := 0
	descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		query, target := descriptionRequest(t, r)
		require.NotContains(t, query, "128K")
		require.NotContains(t, query, "JSON")
		descriptionResponse(w, target+": "+query)
	})
	for range model.ModelDescriptionLocales {
		require.NoError(t, model.DB.Model(&model.ModelDescriptionWorker{}).Where("id = ?", model.ModelDescriptionWorkerID).Update("next_run_at", 0).Error)
		require.NoError(t, runDescriptionTranslationStep(context.Background()))
	}
	require.Equal(t, 17, calls)
	models := model.GetPricing()
	require.Len(t, models, 1)
	require.Len(t, models[0].DescriptionTranslations, 17)
	targets := map[string]string{"en": "English", "zh-CN": "Chinese", "zh-TW": "Traditional Chinese", "ja": "Japanese", "ko": "Korean", "de": "German", "es": "Spanish", "fr": "French", "pt": "Portuguese", "ru": "Russian", "tr": "Turkish", "pl": "Polish", "nl": "Dutch", "it": "Italian", "vi": "Vietnamese", "uk": "Ukrainian", "bg": "Bulgarian"}
	for _, locale := range model.ModelDescriptionLocales {
		require.Equal(t, targets[locale]+": A model with 128K context and JSON output.", models[0].DescriptionTranslations[locale])
	}
	// Public reads use stored translations and never trigger a translator.
	for range 3 {
		model.GetPricing()
	}
	require.Equal(t, 17, calls)
	require.NoError(t, model.DB.Model(&model.ModelDescriptionWorker{}).Where("id = ?", model.ModelDescriptionWorkerID).Update("next_run_at", 0).Error)
	require.NoError(t, runDescriptionTranslationStep(context.Background()))
	require.Equal(t, 17, calls)
}
func TestDescriptionTranslationRateLimitPersistsAndHonorsRetryAfter(t *testing.T) {
	descriptionTestDB(t)
	calls := 0
	descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "7200")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	require.Error(t, runDescriptionTranslationStep(context.Background()))
	var state model.ModelDescriptionWorker
	require.NoError(t, model.DB.First(&state, model.ModelDescriptionWorkerID).Error)
	require.GreaterOrEqual(t, state.NextRunAt, time.Now().Unix()+7198)
	require.Zero(t, state.LeaseUntil)
	require.NoError(t, runDescriptionTranslationStep(context.Background()))
	require.Equal(t, 1, calls)
	require.Empty(t, model.GetPricing()[0].DescriptionTranslations)
	require.Equal(t, 30*time.Minute, descriptionBackoff(1, true))
	require.Equal(t, 6*time.Hour, descriptionBackoff(10, true))
}
func TestDescriptionProtectedTokensChunkingAndRejection(t *testing.T) {
	source := "GPT gpt-6-astra supports 1,050,000 tokens.\nSee https://example.com/a?x=1 and {{name}}. `print(128)`\n" + strings.Repeat("Plain sentence. ", 200) + "\n```python\nprint(2)\n```"
	parts := descriptionParts(source)
	require.Greater(t, len(parts), 2)
	var restored strings.Builder
	for _, part := range parts {
		text, err := restoreDescriptionPart(part.Text, part)
		require.NoError(t, err)
		restored.WriteString(text)
	}
	require.Equal(t, source, restored.String())
	_, err := restoreDescriptionPart("oops", parts[0])
	require.Error(t, err)
	_, err = restoreDescriptionPart(parts[0].Text+"ZXV00000ZXV", parts[0])
	require.Error(t, err)
}

func TestDescriptionMalformedTranslationDoesNotStarveOtherLocales(t *testing.T) {
	descriptionTestDB(t)
	descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) { descriptionResponse(w, "lost protected numbers") })
	require.Error(t, runDescriptionTranslationStep(context.Background()))
	var row model.ModelDescriptionTranslation
	require.NoError(t, model.DB.First(&row).Error)
	require.False(t, row.Completed)
	require.Greater(t, row.NextAttemptAt, time.Now().Unix()+23*3600)
	next, _, err := nextDescriptionTranslation(context.Background(), time.Now().Unix())
	require.NoError(t, err)
	require.NotNil(t, next)
	require.NotEqual(t, row.Locale, next.Locale)
	require.Empty(t, model.GetPricing()[0].DescriptionTranslations)
}

func TestDescriptionPartialTranslationResumesWithoutPublishingIncompleteText(t *testing.T) {
	descriptionTestDB(t)
	source := strings.Repeat("A detailed model description. ", 80) + "128K JSON"
	require.NoError(t, model.DB.Model(&model.Model{}).Where("id = ?", 1).Update("description", source).Error)
	model.RefreshPricing()
	calls := 0
	descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		text, _ := descriptionRequest(t, r)
		descriptionResponse(w, text)
	})
	parts := descriptionParts(source)
	require.Greater(t, len(parts), 1)
	for index := range parts {
		require.NoError(t, model.DB.Model(&model.ModelDescriptionWorker{}).Where("id = ?", model.ModelDescriptionWorkerID).Update("next_run_at", 0).Error)
		require.NoError(t, runDescriptionTranslationStep(context.Background()))
		var row model.ModelDescriptionTranslation
		require.NoError(t, model.DB.Where("locale = ?", "zh-CN").First(&row).Error)
		require.Equal(t, index+1, row.Chunk)
		if index < len(parts)-1 {
			model.RefreshPricing()
			require.Empty(t, model.GetPricing()[0].DescriptionTranslations)
		} else {
			require.Equal(t, source, model.GetPricing()[0].DescriptionTranslations["zh-CN"])
		}
	}
	require.LessOrEqual(t, calls, len(parts))
}
func TestDescriptionAdminPatchValidationAndLegacyPreservation(t *testing.T) {
	descriptionTestDB(t)
	var m model.Model
	require.NoError(t, model.DB.First(&m).Error)
	m.DescriptionOverrides = model.ModelDescriptions{"ja": "日本語"}
	require.NoError(t, model.DB.Save(&m).Error)
	for _, payload := range []string{
		`{"id":1,"model_name":"gpt-description","status":1,"description_overrides":{"zh-CN":"中文介绍"}}`,
		`{"id":1,"model_name":"gpt-description","status":1,"icon":"OpenAI"}`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/models/", strings.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		UpdateModelMeta(c)
		require.Contains(t, w.Body.String(), `"success":true`)
	}
	require.NoError(t, model.DB.First(&m).Error)
	require.Equal(t, model.ModelDescriptions{"zh-CN": "中文介绍", "ja": "日本語"}, m.DescriptionOverrides)
	var req modelMetaRequest
	require.NoError(t, common.Unmarshal([]byte(`{"description_overrides":{"xx":"bad"}}`), &req))
	_, err := req.withDisplayMetadata(&m)
	require.Error(t, err)
}

func TestDescriptionTranslationIsolatesLegacyPartialAndCooldown(t *testing.T) {
	descriptionTestDB(t)
	source := model.GetPricing()[0].Description
	legacy := model.ModelDescriptionTranslation{CacheKey: model.DescriptionCacheKey(source, "zh-CN"), SourceHash: model.ModelDescriptionHash(source), Locale: "zh-CN", Text: "old partial", Chunk: 1, NextAttemptAt: time.Now().Unix() + 86400}
	require.NoError(t, model.DB.Create(&legacy).Error)
	require.NoError(t, model.DB.Create(&model.ModelDescriptionWorker{ID: 1, NextRunAt: time.Now().Unix() + 86400, Failures: 5}).Error)
	descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) {
		text, _ := descriptionRequest(t, r)
		descriptionResponse(w, "new "+text)
	})
	require.NoError(t, runDescriptionTranslationStep(context.Background()))
	require.Equal(t, "new "+source, model.GetPricing()[0].DescriptionTranslations["zh-CN"])
	require.NoError(t, model.DB.First(&legacy, "cache_key = ?", legacy.CacheKey).Error)
	require.Equal(t, "old partial", legacy.Text)
}

func TestDescriptionTranslationUnconfiguredAndInvalidTokenDoNotCallRelay(t *testing.T) {
	descriptionTestDB(t)
	calls := 0
	descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) })
	require.NoError(t, model.UpdateOption(model.SystemTranslationTokenOption, "0"))
	require.NoError(t, runDescriptionTranslationStep(context.Background()))
	require.NoError(t, model.UpdateOption(model.SystemTranslationTokenOption, "100"))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 100).Update("status", common.TokenStatusDisabled).Error)
	require.Error(t, runDescriptionTranslationStep(context.Background()))
	require.Zero(t, calls)
	var count int64
	require.NoError(t, model.DB.Model(&model.ModelDescriptionTranslation{}).Count(&count).Error)
	require.Zero(t, count)
	var worker model.ModelDescriptionWorker
	require.NoError(t, model.DB.First(&worker, model.ModelDescriptionWorkerID).Error)
	require.Greater(t, worker.NextRunAt, time.Now().Unix()+290)
}

func TestDescriptionTranslationAuthFailureBacksOffGlobally(t *testing.T) {
	descriptionTestDB(t)
	calls := 0
	descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) })
	require.Error(t, runDescriptionTranslationStep(context.Background()))
	require.NoError(t, runDescriptionTranslationStep(context.Background()))
	require.Equal(t, 1, calls)
	var worker model.ModelDescriptionWorker
	require.NoError(t, model.DB.First(&worker, model.ModelDescriptionWorkerID).Error)
	require.Greater(t, worker.NextRunAt, time.Now().Unix()+1700)
}
