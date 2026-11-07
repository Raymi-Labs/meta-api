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

func TestWalletSummaryIdentityAndPartialFailure(t *testing.T) {
	db := useControllerCapacityTestDB(t)
	old := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = old })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "fixture-one", AffCode: "wallet-one", Quota: 1234}).Error)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "fixture-two", AffCode: "wallet-two", Quota: 9999}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 1, Type: model.LogTypeConsume, CreatedAt: time.Now().Unix() - 2, Quota: 10, Other: `{"billing_source":"wallet"}`}).Error)
	call := func(id int) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/data/self/wallet?user_id=2&username=fixture-two&start_timestamp=0", nil)
		c.Set("id", id)
		c.Set("role", common.RoleRootUser)
		GetMyWalletSummary(c)
		return w
	}
	require.Equal(t, 401, call(0).Code)
	var body struct {
		Data struct {
			UserID  int               `json:"user_id"`
			Balance int               `json:"balance"`
			Usage   model.WalletUsage `json:"usage"`
		} `json:"data"`
	}
	w := call(1)
	require.Equal(t, 200, w.Code)
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, 1, body.Data.UserID)
	require.Equal(t, 1234, body.Data.Balance)
	require.EqualValues(t, 10, *body.Data.Usage.NetQuota)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	require.NoError(t, db.Migrator().DropTable(&model.Log{}))
	w = call(1)
	require.Equal(t, 200, w.Code)
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, 1234, body.Data.Balance)
	require.Equal(t, "unavailable", body.Data.Usage.Status)
	require.Nil(t, body.Data.Usage.NetQuota)
}
