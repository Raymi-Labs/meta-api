package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAffWithdrawalControllerTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	originDB := model.DB
	originLogDB := model.LOG_DB
	originRedisEnabled := common.RedisEnabled
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price

	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.QuotaPerUnit = 100
	operation_setting.Price = 20

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.RedisEnabled = originRedisEnabled
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
	})

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AffWithdrawal{}))
}

func createAffWithdrawalControllerTestUser(t *testing.T, username string, affQuota int) *model.User {
	t.Helper()
	user := &model.User{
		Username: username,
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  username + "-aff",
		AffQuota: int64(affQuota),
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func newAffWithdrawalTestContext(t *testing.T, method string, target string, body any, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var payload []byte
	if body != nil {
		var err error
		payload, err = common.Marshal(body)
		require.NoError(t, err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	return ctx, recorder
}

func TestCreateAffWithdrawalAPI_FreezesQuota(t *testing.T) {
	setupAffWithdrawalControllerTestDB(t)
	user := createAffWithdrawalControllerTestUser(t, "api_withdraw_create", 500)

	ctx, recorder := newAffWithdrawalTestContext(t, http.MethodPost, "/api/user/aff-withdrawals", gin.H{
		"quota":          model.AffWithdrawalMinimumQuota,
		"alipay_account": "user@example.com",
		"alipay_name":    "张三",
	}, user.Id)

	CreateAffWithdrawal(ctx)

	var response struct {
		Success bool                `json:"success"`
		Message string              `json:"message"`
		Data    model.AffWithdrawal `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success, response.Message)
	assert.Equal(t, model.AffWithdrawalStatusPending, response.Data.Status)
	assert.EqualValues(t, model.AffWithdrawalMinimumAmountCents, response.Data.AmountCents)

	var updated model.User
	require.NoError(t, model.DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(250), updated.AffQuota)
}

func TestCreateAffWithdrawalAPI_RejectsBelowMinimumWithoutMutation(t *testing.T) {
	setupAffWithdrawalControllerTestDB(t)
	user := createAffWithdrawalControllerTestUser(t, "api_withdraw_below_minimum", 500)

	ctx, recorder := newAffWithdrawalTestContext(t, http.MethodPost, "/api/user/aff-withdrawals", gin.H{
		"quota":          model.AffWithdrawalMinimumQuota - 1,
		"alipay_account": "user@example.com",
		"alipay_name":    "张三",
	}, user.Id)

	CreateAffWithdrawal(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "250")

	var updated model.User
	require.NoError(t, model.DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(500), updated.AffQuota)

	var count int64
	require.NoError(t, model.DB.Model(&model.AffWithdrawal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestGetUserAffWithdrawalsAPI_ReturnsOnlyCurrentUser(t *testing.T) {
	setupAffWithdrawalControllerTestDB(t)
	user := createAffWithdrawalControllerTestUser(t, "api_withdraw_self", 500)
	other := createAffWithdrawalControllerTestUser(t, "api_withdraw_other", 500)
	_, err := model.CreateAffWithdrawal(user.Id, model.AffWithdrawalMinimumQuota, "user@example.com", "张三")
	require.NoError(t, err)
	_, err = model.CreateAffWithdrawal(other.Id, model.AffWithdrawalMinimumQuota, "other@example.com", "李四")
	require.NoError(t, err)

	ctx, recorder := newAffWithdrawalTestContext(t, http.MethodGet, "/api/user/aff-withdrawals/self?p=1&page_size=10", nil, user.Id)

	GetUserAffWithdrawals(ctx)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Total int                   `json:"total"`
			Items []model.AffWithdrawal `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, user.Id, response.Data.Items[0].UserId)
}

func TestReviewAffWithdrawalAPI_ApproveAndReject(t *testing.T) {
	setupAffWithdrawalControllerTestDB(t)
	user := createAffWithdrawalControllerTestUser(t, "api_withdraw_review", 1000)
	approved, err := model.CreateAffWithdrawal(user.Id, model.AffWithdrawalMinimumQuota, "user@example.com", "张三")
	require.NoError(t, err)
	rejected, err := model.CreateAffWithdrawal(user.Id, model.AffWithdrawalMinimumQuota, "user@example.com", "张三")
	require.NoError(t, err)

	approveCtx, approveRecorder := newAffWithdrawalTestContext(t, http.MethodPost, "/api/user/aff-withdrawals/1/approve", gin.H{
		"admin_remark": "已转账",
	}, 99)
	approveCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(approved.Id)}}
	ApproveAffWithdrawal(approveCtx)

	var approveResponse struct {
		Success bool                `json:"success"`
		Data    model.AffWithdrawal `json:"data"`
	}
	require.NoError(t, common.Unmarshal(approveRecorder.Body.Bytes(), &approveResponse))
	assert.True(t, approveResponse.Success)
	assert.Equal(t, model.AffWithdrawalStatusApproved, approveResponse.Data.Status)

	rejectCtx, rejectRecorder := newAffWithdrawalTestContext(t, http.MethodPost, "/api/user/aff-withdrawals/2/reject", gin.H{
		"admin_remark": "信息不匹配",
	}, 99)
	rejectCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(rejected.Id)}}
	RejectAffWithdrawal(rejectCtx)

	var rejectResponse struct {
		Success bool                `json:"success"`
		Data    model.AffWithdrawal `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rejectRecorder.Body.Bytes(), &rejectResponse))
	assert.True(t, rejectResponse.Success)
	assert.Equal(t, model.AffWithdrawalStatusRejected, rejectResponse.Data.Status)

	var updated model.User
	require.NoError(t, model.DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(750), updated.AffQuota)
}
