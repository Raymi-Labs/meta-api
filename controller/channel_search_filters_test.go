package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSearchControllerTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	originDB := model.DB
	originLogDB := model.LOG_DB
	originRedisEnabled := common.RedisEnabled
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL

	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.RedisEnabled = originRedisEnabled
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
	})

	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
}

func TestInceptionCategoryListAndSearchPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	explicit, protocol := "inception", "protocol"
	for _, channel := range []*model.Channel{
		{Id: 201, Name: "inception-auto", Type: 1, Models: "mercury-3,mercury-2.5"},
		{Id: 202, Name: "inception-explicit", Type: 8, Models: "local-alias", ChannelVendor: &explicit},
		{Id: 203, Name: "inception-protocol", Type: 1, Models: "mercury-3", ChannelVendor: &protocol},
		{Id: 204, Name: "inception-mixed", Type: 1, Models: "mercury-3,gpt-4o"},
	} {
		channel.Status, channel.Group, channel.Key = common.ChannelStatusEnabled, "Inception · 标准", "fixture-only"
		require.NoError(t, channel.Insert())
	}
	for _, endpoint := range []string{"/api/channel/", "/api/channel/search"} {
		seen := map[int]bool{}
		for _, page := range []string{"1", "2"} {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, endpoint+"?category=vendor:inception&p="+page+"&page_size=1", nil)
			if endpoint == "/api/channel/" {
				GetAllChannels(ctx)
			} else {
				SearchChannels(ctx)
			}
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Items          []model.Channel  `json:"items"`
					Total          int              `json:"total"`
					CategoryCounts map[string]int64 `json:"category_counts"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			require.Equal(t, 2, response.Data.Total)
			require.EqualValues(t, 2, response.Data.CategoryCounts["vendor:inception"])
			require.Len(t, response.Data.Items, 1)
			seen[response.Data.Items[0].Id] = true
		}
		require.Equal(t, map[int]bool{201: true, 202: true}, seen)
	}
}

func seedChannelSearchControllerTestData(t *testing.T) {
	t.Helper()

	alphaTag := "alpha"
	betaTag := "beta"
	gammaTag := "gamma"
	priority := int64(0)
	weight := uint(10)

	channels := []*model.Channel{
		{
			Id:       1,
			Name:     "alpha-openai",
			Key:      "alpha-openai-key",
			Type:     1,
			Status:   common.ChannelStatusEnabled,
			Group:    "default,vip",
			Models:   "gpt-4o,claude-3-5-sonnet",
			Tag:      &alphaTag,
			Priority: &priority,
			Weight:   &weight,
		},
		{
			Id:       2,
			Name:     "alpha-gemini-disabled",
			Key:      "alpha-gemini-key",
			Type:     2,
			Status:   common.ChannelStatusManuallyDisabled,
			Group:    "vip",
			Models:   "gemini-1.5-pro",
			Tag:      &alphaTag,
			Priority: &priority,
			Weight:   &weight,
		},
		{
			Id:       3,
			Name:     "beta-mini",
			Key:      "beta-mini-key",
			Type:     2,
			Status:   common.ChannelStatusEnabled,
			Group:    "vip",
			Models:   "gpt-4o-mini",
			Tag:      &betaTag,
			Priority: &priority,
			Weight:   &weight,
		},
		{
			Id:       4,
			Name:     "gamma-basic",
			Key:      "gamma-basic-key",
			Type:     1,
			Status:   common.ChannelStatusEnabled,
			Group:    "default",
			Models:   "gemini-1.5-flash",
			Tag:      &gammaTag,
			Priority: &priority,
			Weight:   &weight,
		},
	}

	for _, channel := range channels {
		require.NoError(t, channel.Insert())
	}
}

func TestSearchChannels_TypeCountsIgnoreTypeFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelSearchControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/search?group=vip&model=gpt-4&status=enabled&type=1&p=1&page_size=20",
		nil,
	)

	SearchChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items      []model.Channel  `json:"items"`
			Total      int              `json:"total"`
			TypeCounts map[string]int64 `json:"type_counts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, 1, resp.Data.Total)
	require.Equal(t, 1, resp.Data.Items[0].Type)
	require.EqualValues(t, 1, resp.Data.TypeCounts["1"])
	require.EqualValues(t, 1, resp.Data.TypeCounts["2"])
}

func TestSearchChannels_CategoryCountsIgnoreGroupFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelSearchControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/search?group=default&status=enabled&p=1&page_size=20",
		nil,
	)

	SearchChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items          []model.Channel  `json:"items"`
			Total          int              `json:"total"`
			CategoryCounts map[string]int64 `json:"category_counts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 2, resp.Data.Total)
	require.Len(t, resp.Data.Items, 2)
	require.EqualValues(t, 1, resp.Data.CategoryCounts["type:1"])
	require.EqualValues(t, 1, resp.Data.CategoryCounts["vendor:openai"])
	require.EqualValues(t, 1, resp.Data.CategoryCounts["vendor:google"])
}

func seedChannelVendorControllerTestData(t *testing.T) {
	t.Helper()
	priority := int64(0)
	weight := uint(10)
	channels := []*model.Channel{
		{
			Id:       101,
			Name:     "official-mimo",
			Key:      "mimo-key",
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Group:    "default",
			Models:   "mimo-v2.5,mimo-v2.5-pro",
			Priority: &priority,
			Weight:   &weight,
		},
		{
			Id:       102,
			Name:     "adversarial-mimosa",
			Key:      "mimosa-key",
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Group:    "default",
			Models:   "mimosa,notmimo-model",
			Priority: &priority,
			Weight:   &weight,
		},
		{
			Id:       103,
			Name:     "xiaomi-compatible",
			Key:      "xiaomi-key",
			Type:     constant.ChannelTypeAnthropic,
			Status:   common.ChannelStatusEnabled,
			Group:    "default",
			Models:   "xiaomi/mimo-v2.5",
			Priority: &priority,
			Weight:   &weight,
		},
	}
	for _, channel := range channels {
		require.NoError(t, channel.Insert())
	}
}

func TestGetAllChannels_VendorFacetFiltersBeforePagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelVendorControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/?vendor=mimo&type=1&p=1&page_size=1",
		nil,
	)

	GetAllChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items          []model.Channel  `json:"items"`
			Total          int              `json:"total"`
			TypeCounts     map[string]int64 `json:"type_counts"`
			VendorCounts   map[string]int64 `json:"vendor_counts"`
			CategoryCounts map[string]int64 `json:"category_counts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 1, resp.Data.Total)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, 101, resp.Data.Items[0].Id)
	require.EqualValues(t, 2, resp.Data.VendorCounts[model.ChannelVendorMiMo])
	require.EqualValues(t, 3, resp.Data.VendorCounts[model.ChannelVendorAll])
	require.EqualValues(t, 2, resp.Data.TypeCounts["1"])
	require.EqualValues(t, 1, resp.Data.TypeCounts["14"])
	require.EqualValues(t, 1, resp.Data.CategoryCounts["type:1"])
	require.EqualValues(t, 2, resp.Data.CategoryCounts["vendor:mimo"])
}

func TestGetAllChannels_TypeFilterKeepsProtocolSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelVendorControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/?type=1&p=1&page_size=20",
		nil,
	)

	GetAllChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items        []model.Channel  `json:"items"`
			Total        int              `json:"total"`
			TypeCounts   map[string]int64 `json:"type_counts"`
			VendorCounts map[string]int64 `json:"vendor_counts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 2, resp.Data.Total)
	require.Len(t, resp.Data.Items, 2)
	require.ElementsMatch(t, []int{101, 102}, []int{resp.Data.Items[0].Id, resp.Data.Items[1].Id})
	require.EqualValues(t, 2, resp.Data.TypeCounts["1"])
	require.EqualValues(t, 2, resp.Data.VendorCounts[model.ChannelVendorMiMo])
}

func TestGetAllChannels_CategorySeparatesMiMoFromOpenAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelVendorControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/?category=type:1&p=1&page_size=20",
		nil,
	)

	GetAllChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 1, resp.Data.Total)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, 102, resp.Data.Items[0].Id)
}

func TestSearchChannels_VendorFacetExcludesEmbeddedNameFalsePositives(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelVendorControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/search?vendor=mimo&p=1&page_size=20",
		nil,
	)

	SearchChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 2, resp.Data.Total)
	require.Len(t, resp.Data.Items, 2)
	require.ElementsMatch(t, []int{101, 103}, []int{resp.Data.Items[0].Id, resp.Data.Items[1].Id})
}

func TestSearchChannels_TypeFilterKeepsProtocolSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelVendorControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/search?type=1&p=1&page_size=20",
		nil,
	)

	SearchChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 2, resp.Data.Total)
	require.Len(t, resp.Data.Items, 2)
	require.ElementsMatch(t, []int{101, 102}, []int{resp.Data.Items[0].Id, resp.Data.Items[1].Id})
}

func TestSearchChannels_CategorySeparatesMiMoFromOpenAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelVendorControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/search?category=vendor:mimo&p=1&page_size=20",
		nil,
	)

	SearchChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 2, resp.Data.Total)
	require.ElementsMatch(t, []int{101, 103}, []int{resp.Data.Items[0].Id, resp.Data.Items[1].Id})
}

func TestGetAllChannels_InvalidCategoryFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelVendorControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/?category=vendor:unknown&p=1&page_size=20",
		nil,
	)

	GetAllChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.False(t, resp.Success)
}

func TestGetAllChannels_TagModeCountsFilteredTags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedChannelSearchControllerTestData(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/?tag_mode=true&status=enabled&type=1&p=1&page_size=20",
		nil,
	)

	GetAllChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data.Items, 2)
	require.Equal(t, 2, resp.Data.Total)
}

func TestGetAllChannels_ExposesEffectiveAvailabilityAndPendingDisableState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)

	pendingTag := "pending"
	priority := int64(0)
	weight := uint(10)
	pendingUntil := int64(1_800_000_000)
	channel := &model.Channel{
		Id:       10,
		Name:     "pending-claude",
		Key:      "pending-key",
		Type:     constant.ChannelTypeAnthropic,
		Status:   common.ChannelStatusEnabled,
		Group:    "vip",
		Models:   "claude-opus-4-6",
		Tag:      &pendingTag,
		Priority: &priority,
		Weight:   &weight,
	}
	channel.SetPendingDisable(pendingUntil, "upstream unstable")
	require.NoError(t, channel.Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/channel/?p=1&page_size=20",
		nil,
	)

	GetAllChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				ID                          int    `json:"id"`
				EffectiveAvailable          bool   `json:"effective_available"`
				PendingDisableUntil         int64  `json:"pending_disable_until"`
				PendingDisableReason        string `json:"pending_disable_reason"`
				MultiKeyPendingDisableCount int    `json:"multi_key_pending_disable_count"`
				MultiKeyCooldownKeyCount    int    `json:"multi_key_cooldown_key_count"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, 10, resp.Data.Items[0].ID)
	require.False(t, resp.Data.Items[0].EffectiveAvailable)
	require.Equal(t, pendingUntil, resp.Data.Items[0].PendingDisableUntil)
	require.Equal(t, "upstream unstable", resp.Data.Items[0].PendingDisableReason)
	require.Equal(t, 0, resp.Data.Items[0].MultiKeyPendingDisableCount)
	require.Equal(t, 0, resp.Data.Items[0].MultiKeyCooldownKeyCount)
}
