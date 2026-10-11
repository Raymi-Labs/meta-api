package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStripeTopUpDisplayPreservesGatewayAndLegacyConfiguration(t *testing.T) {
	methods := []map[string]string{
		{"type": "wxpay", "name": "微信直付", "custom": "preserved"},
		{"type": "stripe", "name": "旧收款入口", "icon": "https://example.com/icon.svg", "min_topup": "20"},
	}
	before, err := common.Marshal(methods)
	require.NoError(t, err)
	result := stripeTopUpPayMethods(methods, true, " 微信支付 ", "wxpay", 1)
	require.Len(t, result, 2)
	require.Equal(t, methods[0], result[0])
	require.Equal(t, "stripe", result[1]["type"])
	require.Equal(t, "微信支付", result[1]["name"])
	require.Equal(t, "wxpay", result[1]["display_icon"])
	require.Equal(t, "20", result[1]["min_topup"])
	result[0]["custom"] = "response-only"
	after, err := common.Marshal(methods)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))

	legacy := stripeTopUpPayMethods(methods, true, "", "", 1)
	require.Equal(t, methods, legacy)
	defaultMethod := stripeTopUpPayMethods(nil, true, "   ", "", 3)
	require.Len(t, defaultMethod, 1)
	require.Equal(t, "Stripe", defaultMethod[0]["name"])
	require.Equal(t, "stripe", defaultMethod[0]["type"])
	require.Equal(t, "3", defaultMethod[0]["min_topup"])
	require.Empty(t, stripeTopUpPayMethods(nil, false, "微信支付", "wxpay", 3))
}

func TestTopUpInfoExposesStripePresentationWithoutCredentials(t *testing.T) {
	oldKey, oldWebhook, oldPrice := setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId
	oldName, oldIcon, oldMethods := setting.StripeDisplayName, setting.StripeDisplayIcon, operation_setting.PayMethods
	t.Cleanup(func() {
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = oldKey, oldWebhook, oldPrice
		setting.StripeDisplayName, setting.StripeDisplayIcon, operation_setting.PayMethods = oldName, oldIcon, oldMethods
	})
	setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = "sk_test_private", "whsec_private", "price_private"
	setting.StripeDisplayName, setting.StripeDisplayIcon = "微信支付", "wxpay"
	operation_setting.PayMethods = nil
	request := func() (bool, []map[string]string) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
		GetTopUpInfo(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		for _, secret := range []string{"sk_test_private", "whsec_private", "price_private"} {
			require.NotContains(t, recorder.Body.String(), secret)
		}
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Enabled bool                `json:"enable_stripe_topup"`
				Methods []map[string]string `json:"pay_methods"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success)
		return response.Data.Enabled, response.Data.Methods
	}
	enabled, methods := request()
	require.True(t, enabled)
	var stripeMethod map[string]string
	for _, method := range methods {
		if method["type"] == "stripe" {
			stripeMethod = method
		}
	}
	require.Equal(t, "微信支付", stripeMethod["name"])
	require.Equal(t, "wxpay", stripeMethod["display_icon"])
	setting.StripeWebhookSecret = ""
	enabled, methods = request()
	require.False(t, enabled)
	for _, method := range methods {
		require.NotEqual(t, "stripe", method["type"])
	}
}
