package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAlibabaCommunityCatalogPreservesProviderAndResolvesAuthor(t *testing.T) {
	entries, err := parseModelCapabilities([]byte(`{
		"alibaba-token-plan":{"id":"alibaba-token-plan","models":{
			"qwen-image-2.0":{"id":"qwen-image-2.0","modalities":{"output":["image"]}},
			"happyhorse-1.1-t2v":{"id":"happyhorse-1.1-t2v","modalities":{"output":["video"]}},
			"deepseek-v4":{"id":"deepseek-v4","limit":{"context":1000}},
			"unknown":{"id":"unknown","limit":{"context":1000}}
		}}
	}`))
	require.NoError(t, err)
	for _, name := range []string{"qwen-image-2.0", "happyhorse-1.1-t2v"} {
		require.Equal(t, "阿里巴巴", entries[name].VendorName)
		require.Equal(t, "alibaba-token-plan", entries[name].Provider)
	}
	require.Equal(t, "DeepSeek", entries["deepseek-v4"].VendorName)
	require.Empty(t, entries["unknown"].VendorName)
	for _, provider := range []string{"alibaba", "alibaba-cn", "alibaba-token-plan", "alibaba-token-plan-cn"} {
		require.Equal(t, "阿里巴巴", communityMetadataVendor(provider, "wan2.7-image-pro"))
		require.Empty(t, communityMetadataVendor(provider, "wannabe"))
	}
}

func TestAlibabaLegacySyncAndPreviewUseCanonicalVendor(t *testing.T) {
	db := setupModelSyncTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Channel{}))
	t.Cleanup(func() {
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
	})
	canonical := model.Vendor{Name: "阿里巴巴", Status: 1}
	require.NoError(t, db.Create(&canonical).Error)
	for i, name := range []string{"qwen-plus", "qwen-image-2.0", "wan2.7-image-pro"} {
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: name, ChannelId: i + 1, Enabled: true}).Error)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "vendors.json") {
			_, _ = w.Write([]byte(`[{"name":"Alibaba"},{"name":"Alibaba Token Plan"}]`))
		} else {
			_, _ = w.Write([]byte(`[{"model_name":"qwen-plus","vendor_name":"Alibaba","status":1},{"model_name":"qwen-image-2.0","vendor_name":"Alibaba Token Plan","status":1},{"model_name":"wan2.7-image-pro","vendor_name":"Alibaba Token Plan","status":1}]`))
		}
	}))
	defer server.Close()
	t.Setenv("SYNC_UPSTREAM_BASE", server.URL)
	for i := 0; i < 2; i++ {
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
	}
	var rows []model.Model
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, row := range rows {
		require.Equal(t, canonical.Id, row.VendorID, row.ModelName)
	}
	var count int64
	require.NoError(t, db.Model(&model.Vendor{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/models/sync_upstream/preview", nil)
	SyncUpstreamPreview(ctx)
	var preview struct {
		Success bool `json:"success"`
		Data    struct {
			Conflicts []struct {
				Fields []struct {
					Field string `json:"field"`
				} `json:"fields"`
			} `json:"conflicts"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &preview))
	require.True(t, preview.Success, response.Body.String())
	for _, conflict := range preview.Data.Conflicts {
		for _, field := range conflict.Fields {
			require.NotEqual(t, "vendor", field.Field)
		}
	}
}
