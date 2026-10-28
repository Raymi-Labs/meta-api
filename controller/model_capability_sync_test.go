package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const capabilityTestCatalog = `{"native":{"id":"native","models":{"exact-model":{"id":"exact-model","release_date":"2026-10-24","limit":{"context":128000},"modalities":{"input":["image","text","text"],"output":["text"]}}}}}`

func TestParseModelCapabilitiesPriorityAndExactIDs(t *testing.T) {
	entries, err := parseModelCapabilities([]byte(`{
		"b":{"id":"b","billing":{"priority":20},"models":{
			"same-model":{"id":"same-model","limit":{"context":200000}},
			"regional-model":{"id":"regional-model","limit":{"context":200000}}
		}},
		"a":{"id":"a","billing":{"priority":20},"models":{
			"same-model":{"id":"same-model","limit":{"context":100000}},
			"wrong-alias":{"id":"actual-model","limit":{"context":100000}},
			"fractional":{"id":"fractional","limit":{"context":1.5},"modalities":{"input":["text"]}},
			"invalid-modalities":{"id":"invalid-modalities","limit":{"context":128000},"modalities":{"input":["unknown"],"output":["image"]}},
			"oversized-context":{"id":"oversized-context","limit":{"context":9007199254740992}},
			"string-context":{"id":"string-context","limit":{"context":"128000"}},
			"without-id":{"limit":{"context":100000}},
			" padded ":{"id":" padded ","limit":{"context":100000}}
		}},
		"c":{"id":"c","billing":{"priority":30},"models":{
			"regional-model":{"id":"regional-model","limit":{"context":300000}}
		}},
		"wrong-provider":{"id":"another","models":{"wrong":{"id":"wrong","limit":{"context":999999}}}}
	}`))
	require.NoError(t, err)
	require.Len(t, entries, 4)
	require.EqualValues(t, 100000, entries["same-model"].ContextLength)
	require.Equal(t, "a", entries["same-model"].Provider)
	require.EqualValues(t, 300000, entries["regional-model"].ContextLength)
	require.Equal(t, "c", entries["regional-model"].Provider)
	require.Zero(t, entries["fractional"].ContextLength)
	require.Equal(t, model.ModelModalities{"text"}, entries["fractional"].InputModalities)
	require.Empty(t, entries["invalid-modalities"].InputModalities)
	require.Equal(t, model.ModelModalities{"image"}, entries["invalid-modalities"].OutputModalities)
	for _, payload := range []string{`null`, `{}`, `[]`, `{"success":false,"message":"no"}`, `<html>bad gateway</html>`} {
		_, err := parseModelCapabilities([]byte(payload))
		require.Error(t, err, payload)
	}
	entries, err = parseModelCapabilities([]byte(capabilityTestCatalog))
	require.NoError(t, err)
	require.Equal(t, model.ModelModalities{"text", "image"}, entries["exact-model"].InputModalities)
}

func TestParseModelCapabilitiesNormalizesPDFAndRejectsUnknownModalities(t *testing.T) {
	entries, err := parseModelCapabilities([]byte(`{
		"openai":{"id":"openai","models":{
			"gpt-6.1-sol":{"id":"gpt-6.1-sol","limit":{"context":1050000},"modalities":{"input":["text","image","pdf"],"output":["text"]}},
			"unknown-file-type":{"id":"unknown-file-type","limit":{"context":128000},"modalities":{"input":["text","pdf","unknown-format"],"output":["text"]}}
		}}
	}`))
	require.NoError(t, err)
	require.Equal(t, model.ModelModalities{"text", "image", "file"}, entries["gpt-6.1-sol"].InputModalities)
	require.Equal(t, model.ModelModalities{"text"}, entries["gpt-6.1-sol"].OutputModalities)
	require.Empty(t, entries["unknown-file-type"].InputModalities, "do not silently discard an unknown modality")
	require.EqualValues(t, 128000, entries["unknown-file-type"].ContextLength)
	require.Equal(t, model.ModelModalities{"text", "file"}, decodeCapabilityModalities([]byte(`[" PDF ","text","file","pdf"]`)))
}

func TestFetchModelCapabilitiesIsBoundedAndDoesNotRetry(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/limited":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/oversize":
			_, _ = w.Write([]byte(strings.Repeat(" ", (1<<20)+1)))
		case "/invalid":
			_, _ = w.Write([]byte(`{"native":{"id":"native","models":{}}}`))
		case "/wait":
			<-r.Context().Done()
		default:
			_, _ = w.Write([]byte(capabilityTestCatalog))
		}
	}))
	defer server.Close()
	t.Setenv("SYNC_HTTP_MAX_MB", "1")
	for _, path := range []string{"/limited", "/oversize", "/invalid"} {
		before := requests.Load()
		_, err := fetchModelCapabilities(context.Background(), server.URL+path)
		require.Error(t, err)
		require.EqualValues(t, 1, requests.Load()-before, "no retries for "+path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := fetchModelCapabilities(ctx, server.URL+"/wait")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	entries, err := fetchModelCapabilities(context.Background(), server.URL+"/valid")
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestModelCapabilitySyncBusyAndInterval(t *testing.T) {
	modelCapabilitySyncMutex.Lock()
	t.Cleanup(modelCapabilitySyncMutex.Unlock)
	_, err := runModelCapabilitySync(context.Background())
	require.ErrorIs(t, err, errModelCapabilityBusy)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_capabilities", nil)
	SyncModelCapabilities(ctx)
	require.Equal(t, http.StatusConflict, response.Code)
	for value, want := range map[string]time.Duration{"": 24 * time.Hour, "0": 24 * time.Hour, "-1": 24 * time.Hour, "1": time.Hour, "48": 48 * time.Hour, "1000000": 24 * time.Hour} {
		t.Setenv("MODEL_METADATA_SYNC_INTERVAL_HOURS", value)
		require.Equal(t, want, modelCapabilitySyncInterval())
	}
}

func TestModelSyncLocaleMapsToPublishedMetadataLocales(t *testing.T) {
	t.Setenv("SYNC_UPSTREAM_BASE", "https://metadata.example/base/")
	for locale, part := range map[string]string{
		"zh": "/i18n/zh", "zh-CN": "/i18n/zh", "zh-TW": "/i18n/zh", "zh-Hans": "/i18n/zh", "ZH-hant": "/i18n/zh",
		"en": "/i18n/en", "ja": "/i18n/ja", "": "", "invalid/../path": "",
	} {
		models, vendors := getUpstreamURLs(locale)
		require.Equal(t, "https://metadata.example/base/api"+part+"/newapi/models.json", models)
		require.Equal(t, "https://metadata.example/base/api"+part+"/newapi/vendors.json", vendors)
	}
	require.Equal(t, "https://metadata.example/base/api/all.json", modelCapabilitySourceURL())
}

func TestSyncUpstreamCreatesModelsWithSyncEnabled(t *testing.T) {
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
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "sync-new-model", ChannelId: 1, Enabled: true}).Error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "vendors.json") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[{"model_name":"sync-new-model","description":"upstream","status":1}]`))
	}))
	defer server.Close()
	t.Setenv("SYNC_UPSTREAM_BASE", server.URL)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_upstream", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	SyncUpstreamModels(ctx)
	var result struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	var saved model.Model
	require.NoError(t, db.Where("model_name = ?", "sync-new-model").First(&saved).Error)
	require.Equal(t, 1, saved.SyncOfficial)
}

func TestModelCapabilitySyncHandlerRefreshesPricingAndPreservesOnFailure(t *testing.T) {
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
	local := model.Model{ModelName: "exact-model", Status: 1, SyncOfficial: 1, Description: "curated description"}
	require.NoError(t, local.Insert())
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "exact-model", ChannelId: 1, Enabled: true}).Error)
	model.RefreshPricing()
	require.Len(t, model.GetPricing(), 1)
	require.Zero(t, model.GetPricing()[0].ContextLength)
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/all.json", r.URL.Path)
		if unavailable.Load() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(capabilityTestCatalog))
	}))
	defer server.Close()
	t.Setenv("SYNC_UPSTREAM_BASE", server.URL)
	invoke := func() (bool, model.ModelCapabilitySyncResult) {
		t.Helper()
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_capabilities", nil)
		SyncModelCapabilities(ctx)
		var result struct {
			Success bool                            `json:"success"`
			Data    model.ModelCapabilitySyncResult `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		return result.Success, result.Data
	}
	success, result := invoke()
	require.True(t, success)
	require.Equal(t, 1, result.Updated)
	pricing := model.GetPricing()
	require.Equal(t, "2026-10-24", pricing[0].ReleaseDate)
	require.EqualValues(t, 128000, pricing[0].ContextLength)
	require.Equal(t, model.ModelModalities{"text", "image"}, pricing[0].InputModalities)
	require.Equal(t, "curated description", pricing[0].Description)
	require.NoError(t, db.First(&local, local.Id).Error)
	state := local.ModelCapabilitySyncState
	require.NotEmpty(t, state)
	unavailable.Store(true)
	success, _ = invoke()
	require.False(t, success)
	require.NoError(t, db.First(&local, local.Id).Error)
	require.Equal(t, "2026-10-24", local.ReleaseDate)
	require.EqualValues(t, 128000, local.ContextLength)
	require.Equal(t, state, local.ModelCapabilitySyncState)
	require.EqualValues(t, 128000, model.GetPricing()[0].ContextLength)
}

func TestParseModelCapabilitiesReleaseDates(t *testing.T) {
	entries, err := parseModelCapabilities([]byte(`{
		"a":{"id":"a","billing":{"priority":20},"models":{
			"same-model":{"id":"same-model","release_date":"2025-01"},
			"priority-model":{"id":"priority-model","release_date":"2026-10-24"},
			"invalid-day":{"id":"invalid-day","release_date":"2025-02-29","limit":{"context":128000}},
			"number-date":{"id":"number-date","release_date":1792800000,"modalities":{"input":["text"]}},
			"date-only":{"id":"date-only","release_date":"2024-02-29","limit":{"context":"invalid"}},
			"no-date":{"id":"no-date","release_date":null,"limit":{"context":128000}},
			"invalid-only":{"id":"invalid-only","release_date":"2026-13"},
			"missing-only":{"id":"missing-only","last_updated":"2026-10-24"}
		}},
		"b":{"id":"b","billing":{"priority":20},"models":{
			"same-model":{"id":"same-model","release_date":"2026-10-24"}
		}},
		"c":{"id":"c","billing":{"priority":30},"models":{
			"priority-model":{"id":"priority-model","release_date":"2025-09-23"}
		}}
	}`))
	require.NoError(t, err)
	require.Len(t, entries, 6)
	require.Equal(t, "2025-01", entries["same-model"].ReleaseDate, "equal priority uses provider ID, not latest date")
	require.Equal(t, "2025-09-23", entries["priority-model"].ReleaseDate, "higher priority wins even with older date")
	require.Equal(t, "2024-02-29", entries["date-only"].ReleaseDate)
	require.Zero(t, entries["date-only"].ContextLength)
	require.Empty(t, entries["invalid-day"].ReleaseDate)
	require.EqualValues(t, 128000, entries["invalid-day"].ContextLength)
	require.Empty(t, entries["number-date"].ReleaseDate)
	require.Equal(t, model.ModelModalities{"text"}, entries["number-date"].InputModalities)
	require.Empty(t, entries["no-date"].ReleaseDate)
	require.NotContains(t, entries, "invalid-only")
	require.NotContains(t, entries, "missing-only", "last_updated is not a release date")
}
