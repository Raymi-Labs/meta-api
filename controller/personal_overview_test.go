package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPersonalOverviewOnlyUsesAuthenticatedIdentity(t *testing.T) {
	db := useControllerCapacityTestDB(t)
	old := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = old })
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Token{}))
	require.NoError(t, db.Create(&model.Log{UserId: 1, Type: model.LogTypeConsume, Quota: 12, CreatedAt: time.Now().Unix()}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 2, Type: model.LogTypeConsume, Quota: 999, CreatedAt: time.Now().Unix()}).Error)
	require.NoError(t, db.Create(&model.Token{UserId: 1, Key: "secret-account-one", Status: 1, ExpiredTime: -1}).Error)
	call := func(fn gin.HandlerFunc, id int, query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/data/self/month"+query, nil)
		c.Set("id", id)
		c.Set("role", common.RoleRootUser)
		fn(c)
		return w
	}
	for _, fn := range []gin.HandlerFunc{GetMyMonthUsage, GetMyTokenSummary} {
		require.Equal(t, 401, call(fn, 0, "?user_id=1").Code)
	}
	require.Equal(t, 400, call(GetMyMonthUsage, 1, "?timezone=invalid").Code)
	w := call(GetMyMonthUsage, 1, "?timezone=UTC&user_id=2&username=admin")
	require.Equal(t, 200, w.Code)
	var month struct {
		Data model.PersonalMonthUsage `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &month))
	require.EqualValues(t, 12, month.Data.Summary.Quota)
	w = call(GetMyTokenSummary, 2, "?user_id=1")
	require.Equal(t, 200, w.Code)
	var tokens struct {
		Data model.PersonalTokenSummary `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &tokens))
	require.Zero(t, tokens.Data.Total)
	require.NotContains(t, w.Body.String(), "secret-account-one")
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
}
