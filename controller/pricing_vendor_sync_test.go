package controller

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPricingVendorSurvivesCapabilityMetadataCreation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Ability{}, &model.Channel{}, &model.ModelPermission{}))
	originalDB := model.DB
	model.DB = db
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() {
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = originalDB
		model.InvalidateModelPermissionCache()
		_ = sqlDB.Close()
	})

	customVendor := model.Vendor{Name: "Curated Supplier", Icon: "CustomIcon", Status: 1}
	require.NoError(t, customVendor.Insert())
	curated := model.Model{
		ModelName: "gpt-curated", VendorID: customVendor.Id, Status: 1, SyncOfficial: 1,
		Description: "Administrator description", Tags: "curated", Icon: "CustomModelIcon",
	}
	require.NoError(t, curated.Insert())
	disabled := model.Model{ModelName: "gpt-disabled", Status: 0, SyncOfficial: 1}
	require.NoError(t, disabled.Insert())
	require.NoError(t, db.Create(&model.Channel{
		Id: 1, Name: "pricing-vendor-test", Type: constant.ChannelTypeOpenAI,
		Key: "test-only", Status: common.ChannelStatusEnabled,
	}).Error)

	wantVendors := map[string]string{
		"gpt-4o":             "OpenAI",
		"claude-sonnet-4-6":  "Anthropic",
		"gemini-2.5-pro":     "Google",
		"kimi-k2":            "Moonshot",
		"qwen3-32b":          "阿里巴巴",
		"codex-auto-review":  "OpenAI",
		"minimax-m2":         "MiniMax",
		"unidentified-model": "",
		"gpt-curated":        customVendor.Name,
		"gpt-admin-only":     "OpenAI",
		"gpt-hidden":         "OpenAI",
		"gpt-disabled":       "OpenAI",
	}
	catalogModels := make(map[string]interface{}, len(wantVendors))
	for name := range wantVendors {
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: name, ChannelId: 1, Enabled: true}).Error)
		catalogModels[name] = map[string]interface{}{
			"id": name, "limit": map[string]int{"context": 128000},
			"modalities": map[string][]string{"input": {"image", "text"}, "output": {"text"}},
		}
	}
	for name, scope := range map[string]int{
		"gpt-admin-only": model.ModelPermissionScopeAdminOnly,
		"gpt-hidden":     model.ModelPermissionScopeNone,
	} {
		permission := model.ModelPermission{ModelName: name, NameRule: model.NameRuleExact, VisibilityScope: scope}
		require.NoError(t, permission.Insert())
	}
	// The catalog provider is provenance, not a replacement for a model vendor.
	catalog, err := common.Marshal(map[string]interface{}{
		"catalog-reseller": map[string]interface{}{"id": "catalog-reseller", "models": catalogModels},
	})
	require.NoError(t, err)
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/all.json" {
			http.NotFound(w, r)
			return
		}
		if unavailable.Load() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write(catalog)
	}))
	defer server.Close()
	t.Setenv("SYNC_UPSTREAM_BASE", server.URL)

	invokeSync := func() (bool, model.ModelCapabilitySyncResult) {
		t.Helper()
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_capabilities", nil)
		SyncModelCapabilities(ctx)
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool                            `json:"success"`
			Data    model.ModelCapabilitySyncResult `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		return result.Success, result.Data
	}
	type pricingResponse struct {
		Success bool                  `json:"success"`
		Data    []model.Pricing       `json:"data"`
		Vendors []model.PricingVendor `json:"vendors"`
	}
	readPricing := func(role int) pricingResponse {
		t.Helper()
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
		ctx.Set("role", role)
		GetPricing(ctx)
		require.Equal(t, http.StatusOK, response.Code)
		var result pricingResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		return result
	}
	assertPricing := func(role int, wantContext int64) map[string]int {
		t.Helper()
		response := readPricing(role)
		vendors := make(map[int]model.PricingVendor, len(response.Vendors))
		for _, vendor := range response.Vendors {
			vendors[vendor.ID] = vendor
		}
		items := make(map[string]model.Pricing, len(response.Data))
		ids := make(map[string]int, len(response.Data))
		for _, item := range response.Data {
			items[item.ModelName] = item
			ids[item.ModelName] = item.VendorID
		}
		for name, vendorName := range wantVendors {
			item, exists := items[name]
			if name == "gpt-hidden" || name == "gpt-disabled" || (name == "gpt-admin-only" && role < common.RoleAdminUser) {
				require.False(t, exists, "model must remain hidden: %s for role %d", name, role)
				continue
			}
			require.True(t, exists, "missing pricing for %s", name)
			require.Equal(t, wantContext, item.ContextLength, name)
			if wantContext != 0 {
				require.Equal(t, model.ModelModalities{"text", "image"}, item.InputModalities, name)
				require.Equal(t, model.ModelModalities{"text"}, item.OutputModalities, name)
			}
			if vendorName == "" {
				require.Zero(t, item.VendorID, "unknown models must not inherit catalog provider")
				continue
			}
			require.NotZero(t, item.VendorID, name)
			vendor, found := vendors[item.VendorID]
			require.True(t, found, "pricing vendor_id must join the vendors response: %s", name)
			require.Equal(t, vendorName, vendor.Name, name)
		}
		require.Equal(t, customVendor.Id, items[curated.ModelName].VendorID)
		require.Equal(t, curated.Description, items[curated.ModelName].Description)
		require.Equal(t, curated.Tags, items[curated.ModelName].Tags)
		require.Equal(t, curated.Icon, items[curated.ModelName].Icon)
		return ids
	}
	model.RefreshPricing()
	before := assertPricing(common.RoleAdminUser, 0)
	success, result := invokeSync()
	require.True(t, success)
	require.Equal(t, len(wantVendors)-2, result.Created)
	require.Equal(t, 2, result.Updated)
	for _, role := range []int{common.RoleGuestUser, common.RoleCommonUser, common.RoleAdminUser} {
		assertPricing(role, 128000)
	}
	require.Equal(t, before, assertPricing(common.RoleAdminUser, 128000), "creating capability metadata must preserve vendor identities")

	states := make(map[string]string, len(wantVendors))
	for name := range wantVendors {
		var saved model.Model
		require.NoError(t, db.Where("model_name = ?", name).First(&saved).Error)
		if name == curated.ModelName {
			require.Equal(t, customVendor.Id, saved.VendorID)
		} else {
			require.Zero(t, saved.VendorID, "display fallback must not rewrite stored metadata: %s", name)
		}
		require.NotEmpty(t, saved.ModelCapabilitySyncState, name)
		states[name] = saved.ModelCapabilitySyncState
	}
	var vendorCount int64
	require.NoError(t, db.Model(&model.Vendor{}).Count(&vendorCount).Error)
	success, result = invokeSync()
	require.True(t, success)
	require.Zero(t, result.Created)
	require.Zero(t, result.Updated)
	require.Equal(t, before, assertPricing(common.RoleAdminUser, 128000))

	unavailable.Store(true)
	success, _ = invokeSync()
	require.False(t, success)
	model.RefreshPricing()
	require.Equal(t, before, assertPricing(common.RoleAdminUser, 128000))
	for name, state := range states {
		var saved model.Model
		require.NoError(t, db.Where("model_name = ?", name).First(&saved).Error)
		require.Equal(t, state, saved.ModelCapabilitySyncState, name)
		require.EqualValues(t, 128000, saved.ContextLength, name)
	}
	var finalVendorCount int64
	require.NoError(t, db.Model(&model.Vendor{}).Count(&finalVendorCount).Error)
	require.Equal(t, vendorCount, finalVendorCount, "refresh and repeated sync must reuse vendor rows")
}
