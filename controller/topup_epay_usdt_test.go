package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var epayCryptoTestMethods = []string{"usdt.trc20", "usdt.erc20", "usdt.bep20", "usdt.polygon", "usdt.arbitrum", "usdt.solana", "usdc.erc20", "usdc.bep20", "usdc.polygon", "usdc.arbitrum", "usdc.base", "usdc.solana"}

func setupEpayUsdtTest(t *testing.T) *model.User {
	t.Helper()
	gin.SetMode(gin.TestMode)
	setupTopupCallbackTestDB(t)
	oldMethods := operation_setting.PayMethods
	oldPrice, oldMin := operation_setting.Price, operation_setting.MinTopUp
	oldDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	t.Cleanup(func() {
		operation_setting.PayMethods = oldMethods
		operation_setting.Price, operation_setting.MinTopUp = oldPrice, oldMin
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplay
	})
	operation_setting.PayMethods = []map[string]string{{"type": "usdt.trc20", "name": "USDT / TRC20"}}
	operation_setting.Price, operation_setting.MinTopUp = 7.3, 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	return createTopupCallbackTestUser(t, "usdt-user")
}

func requestEpayUsdt(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", userID)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/pay", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	RequestEpay(ctx)
	return recorder
}

func TestEpayCryptoUsesCNYOrderAndExactPaymentType(t *testing.T) {
	for _, method := range epayCryptoTestMethods {
		t.Run(method, func(t *testing.T) {
			user := setupEpayUsdtTest(t)
			operation_setting.PayMethods = []map[string]string{{"type": method, "name": method}}
			recorder := requestEpayUsdt(t, user.Id, `{"amount":2,"payment_method":"`+method+`"}`)
			var response struct {
				Message string            `json:"message"`
				URL     string            `json:"url"`
				Data    map[string]string `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "success", response.Message)
			require.Equal(t, "https://epay.example.com/submit.php", response.URL)
			require.Equal(t, method, response.Data["type"])
			require.Equal(t, "14.60", response.Data["money"])
			require.Equal(t, operation_setting.EpayId, response.Data["pid"])
			topup := model.GetTopUpByTradeNo(response.Data["out_trade_no"])
			require.NotNil(t, topup)
			require.Equal(t, "CNY", topup.PaidCurrency)
			require.Equal(t, 14.6, topup.PaidMoney)
			require.Equal(t, model.PaymentProviderEpay, topup.PaymentProvider)
			require.Equal(t, method, topup.PaymentMethod)
			require.Equal(t, common.TopUpStatusPending, topup.Status)

			// Disabling the method stops new orders but must not invalidate an in-flight payment.
			operation_setting.PayMethods = nil
			disabled := requestEpayUsdt(t, user.Id, `{"amount":2,"payment_method":"`+method+`"}`)
			require.Contains(t, disabled.Body.String(), "payment method does not exist")
			params := map[string]string{
				"pid": operation_setting.EpayId, "trade_no": "EPAY-USDT-1", "out_trade_no": topup.TradeNo,
				"type": method, "name": "USDT test", "money": "14.60", "trade_status": epay.StatusTradeSuccess,
			}
			notify := func(callbackURL string) string {
				rec := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(rec)
				ctx.Request = httptest.NewRequest(http.MethodGet, callbackURL, nil)
				EpayNotify(ctx)
				return rec.Body.String()
			}
			// The coin amount is not the CNY order amount, even with an authentic signature.
			params["money"] = "2.00"
			require.Equal(t, "fail", notify(buildSignedEpayCallbackURL(t, "/api/user/epay/notify", params)))
			params["money"] = "14.60"
			validURL := buildSignedEpayCallbackURL(t, "/api/user/epay/notify", params)
			require.Equal(t, "fail", notify(strings.Replace(validURL, "sign=", "sign=invalid", 1)))
			savedUser, err := model.GetUserById(user.Id, false)
			require.NoError(t, err)
			require.Zero(t, savedUser.Quota)
			require.Equal(t, "success", notify(validURL))
			require.Equal(t, "success", notify(validURL))
			savedUser, err = model.GetUserById(user.Id, false)
			require.NoError(t, err)
			require.Equal(t, int(2*common.QuotaPerUnit), savedUser.Quota)
			require.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(topup.TradeNo).Status)
		})
	}
}

func TestEpayUsdtRejectsUnconfiguredType(t *testing.T) {
	user := setupEpayUsdtTest(t)
	for _, method := range []string{"usdt", "usdt.erc20", "alipay"} {
		recorder := requestEpayUsdt(t, user.Id, `{"amount":2,"payment_method":"`+method+`"}`)
		require.Contains(t, recorder.Body.String(), "payment method does not exist")
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestSubscriptionEpayCryptoUsesCNYAndCompletesOnce(t *testing.T) {
	for _, method := range epayCryptoTestMethods {
		t.Run(method, func(t *testing.T) {
			user := setupEpayUsdtTest(t)
			operation_setting.PayMethods = []map[string]string{{"type": method, "name": method}}
			require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionIssuance{}))
			plan := &model.SubscriptionPlan{Id: 9876, Title: "USDT & USDC plan", PriceAmount: 7.3, Currency: "CNY", Enabled: true, DurationUnit: "month", DurationValue: 1}
			require.NoError(t, model.DB.Create(plan).Error)
			model.InvalidateSubscriptionPlanCache(plan.Id)
			t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(plan.Id) })
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Set("id", user.Id)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/epay/pay", strings.NewReader(`{"plan_id":9876,"payment_method":"`+method+`","purchase_quantity":2}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			SubscriptionRequestEpay(ctx)
			var response struct {
				Message string            `json:"message"`
				Data    map[string]string `json:"data"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
			require.Equal(t, "success", response.Message, rec.Body.String())
			require.Equal(t, "SUB:USDT ＆ USDC plan x2", response.Data["name"])
			require.Equal(t, "USDT & USDC plan", plan.Title)
			require.Equal(t, method, response.Data["type"])
			require.Equal(t, "14.60", response.Data["money"])
			tradeNo := response.Data["out_trade_no"]
			order := model.GetSubscriptionOrderByTradeNo(tradeNo)
			require.Equal(t, model.PaymentProviderEpay, order.PaymentProvider)
			require.Equal(t, 14.6, order.Money)
			operation_setting.PayMethods = nil
			params := map[string]string{"trade_no": "EPAY-SUB-USDT", "out_trade_no": tradeNo, "type": method, "money": "14.60", "name": "test", "trade_status": epay.StatusTradeSuccess}
			for i := 0; i < 2; i++ {
				rec = httptest.NewRecorder()
				ctx, _ = gin.CreateTestContext(rec)
				ctx.Request = httptest.NewRequest(http.MethodGet, buildSignedEpayCallbackURL(t, "/api/subscription/epay/notify", params), nil)
				SubscriptionEpayNotify(ctx)
				require.Equal(t, "success", rec.Body.String())
			}
			rec = httptest.NewRecorder()
			ctx, _ = gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest(http.MethodGet, buildSignedEpayCallbackURL(t, "/api/subscription/epay/return", params), nil)
			SubscriptionEpayReturn(ctx)
			require.Equal(t, http.StatusFound, rec.Code)
			require.NotContains(t, rec.Header().Get("Location"), "pay=fail")
			require.Equal(t, common.TopUpStatusSuccess, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)
			var count int64
			require.NoError(t, model.DB.Model(&model.SubscriptionIssuance{}).Where("user_id = ?", user.Id).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
