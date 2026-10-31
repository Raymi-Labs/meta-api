package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func catalogControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Channel{}, &model.Vendor{}))
	previous := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = previous
		conn, _ := db.DB()
		_ = conn.Close()
		select {
		case <-modelCatalogChanged:
		default:
		}
	})
	return db
}

func TestModelCatalogListsAndSearchReconcileBeforePagination(t *testing.T) {
	db := catalogControllerDB(t)
	for i, name := range []string{"unlisted-a", "unlisted-b", "disabled"} {
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: name, ChannelId: i + 1, Enabled: name != "disabled"}).Error)
	}
	invoke := func(path string, handler gin.HandlerFunc) (int, []model.Model) {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		handler(c)
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Total int           `json:"total"`
				Items []model.Model `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success, w.Body.String())
		return response.Data.Total, response.Data.Items
	}
	total, rows := invoke("/api/models/?p=1&page_size=1", GetAllModelsMeta)
	require.Equal(t, 2, total)
	require.Len(t, rows, 1)
	_, other := invoke("/api/models/?p=2&page_size=1", GetAllModelsMeta)
	require.Len(t, other, 1)
	require.NotEqual(t, rows[0].Id, other[0].Id)
	require.Len(t, model.GetPricing(), 2)
	// Search as the first request after a channel change also discovers models.
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "new-search", ChannelId: 4, Enabled: true}).Error)
	total, rows = invoke("/api/models/search?keyword=new-search&p=1&page_size=1", SearchModelsMeta)
	require.Equal(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, "new-search", rows[0].ModelName)
	require.Len(t, model.GetPricing(), 3)
	// Source requests are queued, never performed on the list request.
	select {
	case <-modelCatalogChanged:
	default:
		t.Fatal("new rows must notify background sync")
	}
	total, _ = invoke("/api/models/?p=1&page_size=10", GetAllModelsMeta)
	require.Equal(t, 3, total)
	select {
	case <-modelCatalogChanged:
		t.Fatal("unchanged list must not trigger remote sync")
	default:
	}
}

func TestModelCatalogCreatedWhenMetadataSourceFails(t *testing.T) {
	db := catalogControllerDB(t)
	isolateOfficialAttributeRequests(t)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "private-unlisted", ChannelId: 1, Enabled: true}).Error)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }))
	defer source.Close()
	t.Setenv("SYNC_UPSTREAM_BASE", source.URL)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_capabilities", nil)
	SyncModelCapabilities(c)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.False(t, response.Success, "failed source must not be reported as success")
	var row model.Model
	require.NoError(t, db.Where("model_name = ?", "private-unlisted").First(&row).Error)
	require.Zero(t, row.ContextLength)
	require.Len(t, model.GetPricing(), 1)
}

func TestModelCatalogBackgroundDiscoversAndEnrichesNewModels(t *testing.T) {
	db := catalogControllerDB(t)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	isolateOfficialAttributeRequests(t)
	select {
	case <-modelCatalogChanged:
	default:
	}
	var requests atomic.Int64
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(capabilityTestCatalog))
	}))
	defer source.Close()
	t.Setenv("SYNC_UPSTREAM_BASE", source.URL)
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runModelCatalogSyncTask(ctx, ticks, 24*time.Hour) }()
	defer func() { cancel(); <-done }()
	// The first tick refreshes existing models even when the local inventory is unchanged.
	ticks <- time.Now()
	ticks <- time.Now()
	require.EqualValues(t, 1, requests.Load())
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "exact-model", ChannelId: 1, Enabled: true}).Error)
	ticks <- time.Now()
	// Receiving the next tick proves the previous iteration completed.
	ticks <- time.Now()
	require.EqualValues(t, 2, requests.Load())
	var row model.Model
	require.NoError(t, db.Where("model_name = ?", "exact-model").First(&row).Error)
	require.EqualValues(t, 128000, row.ContextLength)
	ticks <- time.Now()
	require.EqualValues(t, 2, requests.Load(), "unchanged inventory must not fetch every minute")
}

func TestModelManagementVendorResponseIncludesDiscoveredModels(t *testing.T) {
	db := catalogControllerDB(t)
	vendor := model.Vendor{Name: "OpenAI", Icon: "OpenAI", Status: 1}
	require.NoError(t, vendor.Insert())
	require.NoError(t, db.Create(&model.Model{ModelName: "gpt-image-2", VendorID: vendor.Id, Status: 1}).Error)
	for i := 0; i < 11; i++ {
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-qa-" + strconv.Itoa(i), ChannelId: i + 1, Enabled: true}).Error)
	}
	invoke := func(path string, handler gin.HandlerFunc) model.ModelManagementPage {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		handler(c)
		var response struct {
			Success bool                      `json:"success"`
			Data    model.ModelManagementPage `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success, w.Body.String())
		return response.Data
	}
	page := invoke("/api/models/search?vendor="+strconv.Itoa(vendor.Id)+"&p=1&page_size=10", SearchModelsMeta)
	require.Equal(t, 12, page.Total)
	require.Len(t, page.Items, 10)
	require.Equal(t, 12, page.VendorCounts[vendor.Id])
	require.Len(t, page.Vendors, 1)
	require.Equal(t, vendor.Id, page.Vendors[0].Id)
	for _, row := range page.Items {
		require.Equal(t, vendor.Id, row.DisplayVendorID)
		require.Zero(t, row.VendorID, "new catalog entries retain unset ownership")
	}
	second := invoke("/api/models/search?vendor="+strconv.Itoa(vendor.Id)+"&p=2&page_size=10", SearchModelsMeta)
	require.Equal(t, 12, second.Total)
	require.Len(t, second.Items, 2)
	all := invoke("/api/models/?p=1&page_size=20", GetAllModelsMeta)
	require.Equal(t, page.VendorCounts, all.VendorCounts)
	require.Equal(t, all.Total, len(all.Items))
}
