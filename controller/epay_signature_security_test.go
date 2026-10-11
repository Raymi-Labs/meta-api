package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEpayCallbacksRejectAmbiguousSignedValues(t *testing.T) {
	setupTopupCallbackTestDB(t)
	user := createTopupCallbackTestUser(t, "signature-security")
	topup := &model.TopUp{UserId: user.Id, Amount: 2, Money: 14.6, TradeNo: "SECURITY-TOPUP", PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, topup.Insert())
	order := &model.SubscriptionOrder{UserId: user.Id, PlanId: 1, Money: 14.6, TradeNo: "SECURITY-SUB", PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, order.Insert())
	for _, entry := range []struct {
		endpoint, tradeNo string
		handler           gin.HandlerFunc
	}{
		{"/api/user/epay/notify", topup.TradeNo, EpayNotify},
		{"/api/user/epay/return", topup.TradeNo, EpayReturn},
		{"/api/subscription/epay/notify", order.TradeNo, SubscriptionEpayNotify},
		{"/api/subscription/epay/return", order.TradeNo, SubscriptionEpayReturn},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			params := map[string]string{"pid": operation_setting.EpayId, "name": "item&out_trade_no=other", "out_trade_no": entry.tradeNo, "trade_no": "EPAY-SECURITY", "type": "usdt.trc20", "money": "14.60", "trade_status": epay.StatusTradeSuccess}
			// A genuine legacy signature must not make ambiguous raw values acceptable.
			keys, values := epay.ParamsSort(params)
			parts := make([]string, len(keys))
			for i, key := range keys {
				parts[i] = key + "=" + values[i]
			}
			params["sign"] = epay.MD5String(strings.Join(parts, "&"), operation_setting.EpayKey)
			params["sign_type"] = "MD5"
			query := url.Values{}
			for key, value := range params {
				query.Set(key, value)
			}
			rec := httptest.NewRecorder()
			router := gin.New()
			router.Any(entry.endpoint, entry.handler)
			var req *http.Request
			if method == http.MethodGet {
				req = httptest.NewRequest(method, entry.endpoint+"?"+query.Encode(), nil)
			} else {
				req = httptest.NewRequest(method, entry.endpoint, strings.NewReader(query.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			router.ServeHTTP(rec, req)
			if strings.HasSuffix(entry.endpoint, "/notify") {
				require.Equal(t, "fail", rec.Body.String())
			} else {
				require.Equal(t, http.StatusFound, rec.Code)
				require.Contains(t, rec.Header().Get("Location"), "fail")
			}
		}
	}
	require.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(topup.TradeNo).Status)
	require.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	require.Zero(t, stored.Quota)
}

func TestEpayVerifyDoesNotRepairInvalidSignature(t *testing.T) {
	setupTopupCallbackTestDB(t)
	params := map[string]string{"money": "14.60", "sign": "invalid", "sign_type": "MD5"}
	for i := 0; i < 2; i++ {
		result, err := GetEpayClient().Verify(params)
		require.NoError(t, err)
		require.False(t, result.VerifyStatus)
		require.Equal(t, "invalid", params["sign"])
	}
}
