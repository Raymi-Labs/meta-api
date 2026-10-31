package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func descriptionTestDB(t *testing.T) {
	db := catalogControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.ModelDescriptionTranslation{}, &model.ModelDescriptionWorker{}))
	m := model.Model{ModelName: "gpt-description", Description: "A model with 128K context and JSON output.", Status: 1}
	require.NoError(t, db.Create(&m).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: m.ModelName, ChannelId: 1, Enabled: true}).Error)
	model.RefreshPricing()
}
func TestDescriptionTranslationAllLocalesAndNoRequestOnRead(t *testing.T) {
	descriptionTestDB(t)
	old := descriptionHTTPClient
	t.Cleanup(func() { descriptionHTTPClient = old })
	calls := 0
	descriptionHTTPClient = &http.Client{Transport: attributeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "translate.googleapis.com", r.URL.Host)
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Equal(t, "auto", r.URL.Query().Get("sl"))
		query := r.URL.Query().Get("q")
		require.NotContains(t, query, "128K")
		require.NotContains(t, query, "JSON")
		body, _ := common.Marshal([]interface{}{[][]interface{}{{r.URL.Query().Get("tl") + ": " + query}}, nil, "en"})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	for range model.ModelDescriptionLocales {
		require.NoError(t, model.DB.Model(&model.ModelDescriptionWorker{}).Where("id = ?", 1).Update("next_run_at", 0).Error)
		require.NoError(t, runDescriptionTranslationStep(context.Background()))
	}
	require.Equal(t, 17, calls)
	models := model.GetPricing()
	require.Len(t, models, 1)
	require.Len(t, models[0].DescriptionTranslations, 17)
	for _, locale := range model.ModelDescriptionLocales {
		require.Equal(t, locale+": A model with 128K context and JSON output.", models[0].DescriptionTranslations[locale])
	}
	// Public reads use stored translations and never trigger a translator.
	for range 3 {
		model.GetPricing()
	}
	require.Equal(t, 17, calls)
	require.NoError(t, model.DB.Model(&model.ModelDescriptionWorker{}).Where("id = ?", 1).Update("next_run_at", 0).Error)
	require.NoError(t, runDescriptionTranslationStep(context.Background()))
	require.Equal(t, 17, calls)
}
func TestDescriptionTranslationRateLimitPersistsAndHonorsRetryAfter(t *testing.T) {
	descriptionTestDB(t)
	old := descriptionHTTPClient
	t.Cleanup(func() { descriptionHTTPClient = old })
	calls := 0
	descriptionHTTPClient = &http.Client{Transport: attributeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"7200"}}, Body: io.NopCloser(strings.NewReader("limited"))}, nil
	})}
	require.Error(t, runDescriptionTranslationStep(context.Background()))
	var state model.ModelDescriptionWorker
	require.NoError(t, model.DB.First(&state, 1).Error)
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
	old := descriptionHTTPClient
	t.Cleanup(func() { descriptionHTTPClient = old })
	descriptionHTTPClient = &http.Client{Transport: attributeTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[[["lost protected numbers"]],null,"en"]`))}, nil
	})}
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
	old := descriptionHTTPClient
	t.Cleanup(func() { descriptionHTTPClient = old })
	calls := 0
	descriptionHTTPClient = &http.Client{Transport: attributeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := common.Marshal([]interface{}{[][]interface{}{{r.URL.Query().Get("q")}}, nil, "zh-CN"})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	parts := descriptionParts(source)
	require.Greater(t, len(parts), 1)
	for index := range parts {
		require.NoError(t, model.DB.Model(&model.ModelDescriptionWorker{}).Where("id = ?", 1).Update("next_run_at", 0).Error)
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

// Explicit opt-in smoke test: one public sample, fixed existing free endpoint,
// sequential 5s pacing. A rate limit immediately ends the run (no alternate host).
func TestDescriptionTranslationLive(t *testing.T) {
	sourcePath := os.Getenv("TEST_DESCRIPTION_LIVE_SOURCE")
	if sourcePath == "" {
		t.Skip("live free-service smoke test is opt-in")
	}
	source, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	translated := map[string]string{}
	parts := descriptionParts(strings.TrimSpace(string(source)))
	for _, locale := range model.ModelDescriptionLocales {
		var out strings.Builder
		for _, part := range parts {
			value, err := translateDescriptionPart(context.Background(), part, locale)
			require.NoError(t, err)
			out.WriteString(value)
			time.Sleep(5 * time.Second)
		}
		translated[locale] = out.String()
		t.Log("translated", locale)
	}
	body, err := common.Marshal(translated)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(sourcePath), "translations.json"), body, 0600))
}
