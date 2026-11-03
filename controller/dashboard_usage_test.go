package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDashboardSelfUsesAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	old := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = old })
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	for _, row := range []model.Log{
		{UserId: 417, Username: "owner", CreatedAt: 1700000100, Type: model.LogTypeConsume, Quota: 7},
		{UserId: 418, Username: "other", CreatedAt: 1700000100, Type: model.LogTypeConsume, Quota: 999},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/data/self?start_timestamp=1700000000&end_timestamp=1700000200&username=other&user_id=418", nil)
	c.Set("id", 417)
	GetUserQuotaDates(c)
	var body struct {
		Success bool
		Summary model.DashboardUsageSummary
		Data    []model.QuotaData
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success, w.Body.String())
	require.EqualValues(t, 7, body.Summary.Quota)
	require.Len(t, body.Data, 1)
	require.Empty(t, body.Data[0].Username)
	require.Zero(t, body.Data[0].UserID)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/data/self?start_timestamp=1700000000&end_timestamp=1700000200", nil)
	GetUserQuotaDates(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDashboardRejectsInvalidRangesBeforeQuery(t *testing.T) {
	for _, query := range []string{
		"start_timestamp=no&end_timestamp=2", "start_timestamp=0&end_timestamp=2",
		"start_timestamp=10&end_timestamp=9", "start_timestamp=1&end_timestamp=2678402",
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/data/?"+query, nil)
		GetAllQuotaDates(c)
		var body struct{ Success bool }
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
		require.False(t, body.Success)
	}
}
