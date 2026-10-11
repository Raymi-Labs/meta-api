package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopupCallbackTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	originDB := model.DB
	originLogDB := model.LOG_DB
	originRedisEnabled := common.RedisEnabled
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originPayAddress := operation_setting.PayAddress
	originEpayID := operation_setting.EpayId
	originEpayKey := operation_setting.EpayKey

	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	operation_setting.PayAddress = "https://epay.example.com"
	operation_setting.EpayId = "1000"
	operation_setting.EpayKey = "callback-secret"

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.RedisEnabled = originRedisEnabled
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		operation_setting.PayAddress = originPayAddress
		operation_setting.EpayId = originEpayID
		operation_setting.EpayKey = originEpayKey
	})

	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.TopUp{},
		&model.PaymentRiskCase{},
		&model.SubscriptionOrder{},
		&model.BenefitChangeRecord{},
		&model.PulseFundingLedger{},
		&model.PulseWalletReservation{},
		&model.Log{},
	))
}

func createTopupCallbackTestUser(t *testing.T, username string) *model.User {
	t.Helper()

	user := &model.User{
		Username: username,
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  username + "-aff",
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func buildSignedEpayCallbackURL(t *testing.T, path string, params map[string]string) string {
	t.Helper()

	signed, err := epay.GenerateParamsWithError(params, operation_setting.EpayKey)
	require.NoError(t, err)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}
	return path + "?" + query.Encode()
}

func TestEpayNotify_RejectsAmountMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTopupCallbackTestDB(t)

	user := createTopupCallbackTestUser(t, "alice")
	topup := &model.TopUp{
		UserId:        user.Id,
		Amount:        1000,
		Money:         160,
		TradeNo:       "USR1NOAMOUNTMISMATCH",
		PaymentMethod: "alipay",
		CreateTime:    1_760_000_000,
		Status:        common.TopUpStatusPending,
	}
	require.NoError(t, topup.Insert())

	callbackURL := buildSignedEpayCallbackURL(t, "/api/user/epay/notify", map[string]string{
		"trade_no":     "EPAY-ORDER-001",
		"out_trade_no": topup.TradeNo,
		"type":         "alipay",
		"name":         "test",
		"money":        "0.01",
		"trade_status": epay.StatusTradeSuccess,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, callbackURL, nil)

	EpayNotify(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "fail", recorder.Body.String())

	savedTopup := model.GetTopUpByTradeNo(topup.TradeNo)
	require.NotNil(t, savedTopup)
	require.Equal(t, common.TopUpStatusPending, savedTopup.Status)

	savedUser, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	require.NotNil(t, savedUser)
	require.Equal(t, 0, savedUser.Quota)

	riskCase, err := model.GetPaymentRiskCaseByRecord(model.PaymentRiskRecordTypeTopUp, topup.TradeNo)
	require.NoError(t, err)
	require.Equal(t, model.PaymentRiskStatusOpen, riskCase.Status)
	require.Equal(t, model.PaymentRiskReasonAmountMismatch, riskCase.Reason)
	require.Equal(t, topup.UserId, riskCase.UserId)
	require.Equal(t, topup.PaymentMethod, riskCase.PaymentMethod)
	require.Equal(t, 0.01, riskCase.ReceivedMoney)
}

func TestSubscriptionEpayNotify_ProcessingFailureCreatesRiskCase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTopupCallbackTestDB(t)

	user := createTopupCallbackTestUser(t, "sub-risk")
	order := &model.SubscriptionOrder{
		UserId:        user.Id,
		PlanId:        999999,
		Money:         88,
		TradeNo:       "SUBUSR1NOPROCESSFAIL",
		PaymentMethod: "alipay",
		CreateTime:    1_760_000_100,
		Status:        common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	callbackURL := buildSignedEpayCallbackURL(t, "/api/subscription/epay/notify", map[string]string{
		"trade_no":     "EPAY-SUB-001",
		"out_trade_no": order.TradeNo,
		"type":         "alipay",
		"name":         "subscription",
		"money":        "88.00",
		"trade_status": epay.StatusTradeSuccess,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, callbackURL, nil)

	SubscriptionEpayNotify(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "fail", recorder.Body.String())

	savedOrder := model.GetSubscriptionOrderByTradeNo(order.TradeNo)
	require.NotNil(t, savedOrder)
	require.Equal(t, common.TopUpStatusPending, savedOrder.Status)

	riskCase, err := model.GetPaymentRiskCaseByRecord(model.PaymentRiskRecordTypeSubscription, order.TradeNo)
	require.NoError(t, err)
	require.Equal(t, model.PaymentRiskReasonManualReview, riskCase.Reason)
	require.Equal(t, order.UserId, riskCase.UserId)
	require.Equal(t, order.PaymentMethod, riskCase.PaymentMethod)
	require.Equal(t, 88.0, riskCase.ReceivedMoney)
}

func TestStripeWebhook_ForbiddenWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originStripeAPISecret := setting.StripeApiSecret
	originStripeWebhookSecret := setting.StripeWebhookSecret
	originStripePriceID := setting.StripePriceId
	setting.StripeApiSecret = ""
	setting.StripeWebhookSecret = "whsec_test"
	setting.StripePriceId = ""
	t.Cleanup(func() {
		setting.StripeApiSecret = originStripeAPISecret
		setting.StripeWebhookSecret = originStripeWebhookSecret
		setting.StripePriceId = originStripePriceID
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewBufferString(`{"id":"evt_test"}`))
	ctx.Request.Header.Set("Stripe-Signature", "t=123,v1=invalid")

	StripeWebhook(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}
