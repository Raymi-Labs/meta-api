package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelMetaRequestDisplayMetadataPresence(t *testing.T) {
	existing := model.Model{DisplayName: "Local title", ReleaseDate: "2025-01", ContextLength: 128000, MaxOutputTokens: 32000, Capabilities: model.ModelCapabilities{"streaming": false},
		InputModalities: model.ModelModalities{"text", "image"}, OutputModalities: model.ModelModalities{"text"}}
	var omitted modelMetaRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"id":1,"model_name":"example","description":"new"}`, &omitted))
	got, err := omitted.withDisplayMetadata(&existing)
	require.NoError(t, err)
	require.Equal(t, existing.DisplayName, got.DisplayName)
	require.Equal(t, existing.ReleaseDate, got.ReleaseDate)
	require.Equal(t, existing.ContextLength, got.ContextLength)
	require.Equal(t, existing.MaxOutputTokens, got.MaxOutputTokens)
	require.Equal(t, existing.Capabilities, got.Capabilities)
	require.Equal(t, existing.InputModalities, got.InputModalities)
	var clear modelMetaRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"official_pricing":{},"display_name":"","release_date":"","context_length":0,"max_output_tokens":0,"capabilities":{},"input_modalities":[],"output_modalities":[]}`, &clear))
	got, err = clear.withDisplayMetadata(&existing)
	require.NoError(t, err)
	require.Empty(t, got.DisplayName)
	require.Empty(t, got.ReleaseDate)
	require.Zero(t, got.ContextLength)
	require.Zero(t, got.MaxOutputTokens)
	require.Empty(t, got.Capabilities)
	require.Empty(t, got.InputModalities)
	require.Empty(t, got.OutputModalities)
	for _, payload := range []string{`{"max_output_tokens":1.5}`, `{"capabilities":{"streaming":null}}`, `{"capabilities":{"unknown":true}}`, `{"capabilities":{"reasoning":"true"}}`, `{"release_date":1792800000}`, `{"release_date":{}}`, `{"context_length":1.5}`, `{"context_length":"128000"}`, `{"input_modalities":"text"}`, `{"output_modalities":[1]}`} {
		var invalid modelMetaRequest
		require.Error(t, common.UnmarshalJsonStr(payload, &invalid))
	}
}

func TestModelMetaDisplayMetadataControllerAndPricing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Ability{}, &model.Channel{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		// Do not leave the synthetic public catalog cached for subsequent tests.
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = originalDB
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	invoke := func(method, path, body string, handler gin.HandlerFunc) bool {
		t.Helper()
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		handler(ctx)
		var result struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		return result.Success
	}
	require.True(t, invoke(http.MethodPost, "/api/models/", `{"official_pricing":{"currency":"USD","tiers":[{"label":"Standard","unit":"million_tokens","input":10,"cache_read":0}]},"model_name":"metadata-model","description":"Local description","display_name":" Local title ","release_date":"2026-10-24","context_length":256000,"max_output_tokens":64000,"capabilities":{"reasoning":true,"streaming":false},"input_modalities":["image","text","text"],"output_modalities":["text"],"status":1,"sync_official":1}`, CreateModelMeta))
	var saved model.Model
	require.NoError(t, db.Where("model_name = ?", "metadata-model").First(&saved).Error)
	require.Len(t, saved.OfficialPricing.Tiers, 1)
	require.Equal(t, float64(10), *saved.OfficialPricing.Tiers[0].Input)
	require.Equal(t, "Local title", saved.DisplayName)
	require.Equal(t, "2026-10-24", saved.ReleaseDate)
	require.Equal(t, model.ModelModalities{"text", "image"}, saved.InputModalities)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "metadata-model", ChannelId: 1, Enabled: true}).Error)
	model.RefreshPricing()
	pricing := model.GetPricing()
	require.Len(t, pricing, 1)
	require.Equal(t, "metadata-model", pricing[0].ModelName)
	require.Equal(t, "Local title", pricing[0].DisplayName)
	require.Equal(t, "2026-10-24", pricing[0].ReleaseDate)
	require.EqualValues(t, 256000, pricing[0].ContextLength)
	require.EqualValues(t, 64000, pricing[0].MaxOutputTokens)
	require.Equal(t, model.ModelCapabilities{"reasoning": true, "streaming": false}, pricing[0].Capabilities)
	require.Equal(t, saved.InputModalities, pricing[0].InputModalities)
	// An old full-update payload may edit descriptions without losing the added fields.
	require.True(t, invoke(http.MethodPut, "/api/models/", `{"id":1,"model_name":"metadata-model","description":"Updated description","status":1,"sync_official":1}`, UpdateModelMeta))
	require.Equal(t, "Local title", model.GetPricing()[0].DisplayName)
	require.Equal(t, saved.OfficialPricing, model.GetPricing()[0].OfficialPricing)
	require.Equal(t, "Updated description", model.GetPricing()[0].Description)
	require.Equal(t, "2026-10-24", model.GetPricing()[0].ReleaseDate)
	// Upstream metadata can update its known fields, but cannot overwrite locally
	// curated display metadata even when an overwrite request includes those names.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "vendors.json") {
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"metadata-model","description":"Upstream description","display_name":"Wrong upstream title","release_date":"2025-01-01","context_length":1,"input_modalities":[],"output_modalities":[]}]}`))
	}))
	t.Cleanup(upstream.Close)
	t.Setenv("SYNC_UPSTREAM_BASE", upstream.URL)
	require.True(t, invoke(http.MethodPost, "/api/models/sync_upstream", `{"overwrite":[{"model_name":"metadata-model","fields":["official_pricing","description","display_name","release_date","context_length","input_modalities","output_modalities"]}]}`, SyncUpstreamModels))
	pricing = model.GetPricing()
	require.Equal(t, saved.OfficialPricing, pricing[0].OfficialPricing)
	require.Equal(t, "Upstream description", pricing[0].Description)
	require.Equal(t, "Local title", pricing[0].DisplayName)
	require.Equal(t, "2026-10-24", pricing[0].ReleaseDate)
	require.EqualValues(t, 256000, pricing[0].ContextLength)
	require.EqualValues(t, 64000, pricing[0].MaxOutputTokens)
	require.Equal(t, model.ModelCapabilities{"reasoning": true, "streaming": false}, pricing[0].Capabilities)
	require.Equal(t, model.ModelModalities{"text", "image"}, pricing[0].InputModalities)
	require.Equal(t, model.ModelModalities{"text"}, pricing[0].OutputModalities)
	for _, body := range []string{
		`{"id":1,"model_name":"metadata-model","release_date":"2025-02-29"}`,
		`{"id":1,"model_name":"metadata-model","release_date":"2026-10-24","name_rule":1}`,
		`{"id":1,"model_name":"metadata-model","max_output_tokens":-1}`,
		`{"id":1,"model_name":"metadata-model","max_output_tokens":9007199254740992}`,
		`{"id":1,"model_name":"metadata-model","context_length":-1}`,
		`{"id":1,"model_name":"metadata-model","context_length":1.5}`,
		`{"id":1,"model_name":"metadata-model","input_modalities":["unsupported"]}`,
		`{"id":1,"model_name":"metadata-model","output_modalities":"image"}`,
	} {
		require.False(t, invoke(http.MethodPut, "/api/models/", body, UpdateModelMeta))
	}
	require.True(t, invoke(http.MethodPut, "/api/models/?status_only=true", `{"id":1,"status":0}`, UpdateModelMeta))
	require.Empty(t, model.GetPricing(), "disabled metadata remains hidden from public pricing")
	require.NoError(t, db.First(&saved, 1).Error)
	require.Equal(t, "Local title", saved.DisplayName)
	require.True(t, invoke(http.MethodPut, "/api/models/", `{"id":1,"model_name":"metadata-model","official_pricing":{},"display_name":"","release_date":"","context_length":0,"max_output_tokens":0,"capabilities":{},"input_modalities":[],"output_modalities":[],"status":1}`, UpdateModelMeta))
	pricing = model.GetPricing()
	require.Len(t, pricing, 1)
	require.Empty(t, pricing[0].OfficialPricing.Tiers)
	require.Empty(t, pricing[0].DisplayName)
	require.Empty(t, pricing[0].ReleaseDate)
	require.Zero(t, pricing[0].ContextLength)
	require.Zero(t, pricing[0].MaxOutputTokens)
	require.Empty(t, pricing[0].Capabilities)
	encoded, err := common.Marshal(pricing[0])
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"release_date":""`)
	require.Contains(t, string(encoded), `"input_modalities":[]`)
	require.Contains(t, string(encoded), `"output_modalities":[]`)
}

func TestModelReleaseDatePricingDoesNotInheritPatternDates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Ability{}, &model.Channel{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = originalDB
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	// Defend even against legacy/imported pattern records bypassing validation.
	for _, m := range []model.Model{
		{ModelName: "prefix-", NameRule: model.NameRulePrefix, ReleaseDate: "2026-10-24", Status: 1},
		{ModelName: "-suffix", NameRule: model.NameRuleSuffix, ReleaseDate: "2026-10-24", Status: 1},
		{ModelName: "middle", NameRule: model.NameRuleContains, ReleaseDate: "2026-10-24", Status: 1},
		{ModelName: "exact", NameRule: model.NameRuleExact, ReleaseDate: "2025-01", Status: 1},
		{ModelName: "invalid", NameRule: model.NameRuleExact, ReleaseDate: "2025-02-29", Status: 1},
	} {
		require.NoError(t, db.Create(&m).Error)
	}
	for _, name := range []string{"prefix-child", "child-suffix", "has-middle-name", "exact", "invalid"} {
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: name, ChannelId: 1, Enabled: true}).Error)
	}
	model.RefreshPricing()
	pricing := model.GetPricing()
	require.Len(t, pricing, 5)
	for _, item := range pricing {
		if item.ModelName == "exact" {
			require.Equal(t, "2025-01", item.ReleaseDate)
		} else {
			require.Empty(t, item.ReleaseDate, item.ModelName)
		}
	}
}
