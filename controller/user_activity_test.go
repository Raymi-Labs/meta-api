package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMyActivityAuthenticatedScope(t *testing.T) {
	db := useControllerCapacityTestDB(t)
	old := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = old })
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	now := time.Now().Unix()
	for _, id := range []int{71001, 71002} {
		require.NoError(t, db.Create(&model.Log{UserId: id, Type: model.LogTypeConsume, CreatedAt: now}).Error)
	}
	call := func(id int, query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/data/self/activity"+query, nil)
		c.Set("id", id)
		GetMyActivity(c)
		return w
	}
	require.Equal(t, 401, call(0, "?user_id=71001").Code)
	require.Equal(t, 400, call(71001, "?timezone=invalid").Code)
	w := call(71001, "?timezone=UTC&user_id=71002&username=someone&start_timestamp=1")
	require.Equal(t, 200, w.Code)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	var data struct {
		Data model.UserActivity `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &data))
	require.EqualValues(t, 1, data.Data.Total)
	require.Len(t, data.Data.Days, 365)
	require.NotContains(t, w.Body.String(), "user_id")
	require.NotContains(t, w.Body.String(), "username")
	w = call(71003, "?timezone=UTC&user_id=71001")
	require.Equal(t, 200, w.Code)
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &data))
	require.Zero(t, data.Data.Total)
}
