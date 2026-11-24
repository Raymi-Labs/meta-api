package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

func TestWalletAutoTopupThresholdAndOffSessionRecovery(t *testing.T) {
	setupTopupCallbackTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.WalletAutoTopup{}, &model.InviteCommissionLedger{}, &model.InviteCommissionDailyCapState{}))
	user := createTopupCallbackTestUser(t, "threshold_wallet")
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"quota": 1000, "stripe_customer": "cus_wallet"}).Error)
	oldKey, oldPrice, oldSecret, oldMin := setting.StripeApiSecret, setting.StripePriceId, setting.StripeWebhookSecret, setting.StripeMinTopUp
	setting.StripeApiSecret, setting.StripePriceId, setting.StripeWebhookSecret, setting.StripeMinTopUp = "sk_test_mock", "price_wallet", "whsec_wallet", 1
	t.Cleanup(func() {
		setting.StripeApiSecret, setting.StripePriceId, setting.StripeWebhookSecret, setting.StripeMinTopUp = oldKey, oldPrice, oldSecret, oldMin
	})
	var trade, setupRef string
	creates, confirms := 0, 0
	status := "requires_confirmation"
	pi := func() string {
		return fmt.Sprintf(`{"id":"pi_wallet","status":%q,"amount":1500,"amount_received":1500,"currency":"usd","customer":"cus_wallet","payment_method":"pm_wallet","metadata":{"wallet_auto_topup":%q}}`, status, trade)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/prices/price_wallet":
			fmt.Fprint(w, `{"id":"price_wallet","type":"one_time","active":true,"billing_scheme":"per_unit","unit_amount":150,"unit_amount_decimal":"150","currency":"usd"}`)
		case "/v1/checkout/sessions":
			require.Equal(t, "setup", r.FormValue("mode"))
			setupRef = r.FormValue("setup_intent_data[metadata][wallet_auto_topup]")
			require.NotEmpty(t, setupRef)
			fmt.Fprint(w, `{"id":"cs_wallet","url":"https://checkout.stripe.com/example"}`)
		case "/v1/payment_intents":
			creates++
			trade = r.FormValue("metadata[wallet_auto_topup]")
			require.NotEmpty(t, trade)
			require.Equal(t, "1500", r.FormValue("amount"))
			require.Empty(t, r.FormValue("confirm"))
			fmt.Fprint(w, pi())
		case "/v1/payment_intents/pi_wallet":
			fmt.Fprint(w, pi())
		case "/v1/payment_intents/pi_wallet/confirm":
			confirms++
			require.Equal(t, "true", r.FormValue("off_session"))
			status = "succeeded"
			http.Error(w, `{"error":{"message":"lost confirmation response"}}`, 500)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	old := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, old) })
	setup := func() {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("id", user.Id)
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"threshold":1000,"quantity":10,"price":"price_wallet","currency":"USD","unit_minor":150,"grant_unit":1}`))
		SetupWalletAutoTopup(c)
		require.Contains(t, w.Body.String(), `"success":true`)
	}
	send := func(kind, obj string) {
		payload := []byte(fmt.Sprintf(`{"object":"event","type":%q,"data":{"object":%s}}`, kind, obj))
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/stripe/webhook", bytes.NewReader(payload))
		c.Request.Header.Set("Stripe-Signature", webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: "whsec_wallet"}).Header)
		StripeWebhook(c)
		require.Equal(t, 200, w.Code)
	}
	authorize := func() {
		send("setup_intent.succeeded", fmt.Sprintf(`{"id":"seti_wallet","status":"succeeded","usage":"off_session","customer":"cus_wallet","payment_method":"pm_wallet","metadata":{"wallet_auto_topup":%q}}`, setupRef))
	}
	read := func() model.WalletAutoTopup {
		r, err := model.GetWalletAutoTopup(user.Id)
		require.NoError(t, err)
		return r
	}
	setup()
	require.NoError(t, model.DisableWalletAutoTopup(user.Id))
	authorize()
	require.Equal(t, model.AutoTopupDisabled, read().State)
	setup()
	authorize()
	require.Equal(t, model.AutoTopupArmed, read().State)
	run := func() error { return processWalletAutoTopup(context.Background(), user.Id) }
	require.NoError(t, run())
	require.Zero(t, creates)
	require.NoError(t, model.DB.Model(user).Update("quota", 900).Error)
	hold := model.PulseWalletReservation{ID: "hold", UserID: user.Id, RequestID: "hold_request", Quota: 100, Status: "reserved"}
	require.NoError(t, model.DB.Create(&hold).Error)
	require.NoError(t, run())
	require.Zero(t, creates)
	require.NoError(t, model.DB.Delete(&hold).Error)
	ids, err := model.WalletAutoTopupCandidates(0, 100)
	require.NoError(t, err)
	require.Contains(t, ids, user.Id)
	require.Error(t, run())
	require.Equal(t, "pi_wallet", read().Intent)
	require.NoError(t, model.DisableWalletAutoTopup(user.Id))
	require.NoError(t, run())
	send("payment_intent.succeeded", pi())
	require.NoError(t, run())
	require.Equal(t, 1, creates)
	require.Equal(t, 1, confirms)
	require.Equal(t, model.AutoTopupDisabled, read().State)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	require.Equal(t, 900+int(10*common.QuotaPerUnit), user.Quota)
	var orders []model.TopUp
	require.NoError(t, model.DB.Find(&orders).Error)
	require.Len(t, orders, 1)
	require.Equal(t, common.TopUpStatusSuccess, orders[0].Status)
}

func TestWalletAutoTopupOneChargePerLowBalanceEpisode(t *testing.T) {
	setupTopupCallbackTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.WalletAutoTopup{}))
	user := createTopupCallbackTestUser(t, "wallet_episode")
	rule := model.WalletAutoTopup{UserID: user.Id, Threshold: 9999999, Quantity: 1, UnitMinor: 200, GrantMoney: 1, Currency: "USD", PriceID: "price_wallet", SetupRef: "autoset_1", Customer: "cus_1"}
	require.NoError(t, model.BeginWalletAutoTopupSetup(&rule))
	require.NoError(t, model.ActivateWalletAutoTopup(rule.SetupRef, rule.Customer, "pm_1"))
	claim := func() model.WalletAutoTopup {
		r, err := model.ClaimWalletAutoTopup(user.Id, "price_wallet", 1)
		require.NoError(t, err)
		return r
	}
	first := claim()
	require.NotEmpty(t, first.Trade)
	require.Equal(t, first.Trade, claim().Trade)
	require.Error(t, model.BeginWalletAutoTopupSetup(&rule))
	require.NoError(t, model.DB.Model(&model.WalletAutoTopup{}).Where("user_id = ?", user.Id).Update("intent", "pi_1").Error)
	require.NoError(t, model.FailWalletAutoTopup(first)) // stale pre-create worker
	require.Equal(t, first.Trade, claim().Trade)
	r := claim()
	// A different customer or short payment cannot add credit.
	pi := &stripe.PaymentIntent{ID: "pi_1", Status: stripe.PaymentIntentStatusSucceeded, AmountReceived: 199, Currency: "usd", Metadata: map[string]string{"wallet_auto_topup": r.Trade}, Customer: &stripe.Customer{ID: "cus_1"}, PaymentMethod: &stripe.PaymentMethod{ID: "pm_1"}}
	require.Error(t, settleAutoTopup(r, pi))
	pi.AmountReceived = 200
	require.NoError(t, settleAutoTopup(r, pi))
	waiting := claim()
	require.Empty(t, waiting.Trade)
	require.Equal(t, model.AutoTopupWaiting, waiting.State)
	require.NoError(t, model.DB.Model(user).Update("quota", rule.Threshold).Error)
	require.Empty(t, claim().Trade)
	require.NoError(t, model.DB.Model(user).Update("quota", rule.Threshold-1).Error)
	second := claim()
	require.NotEmpty(t, second.Trade)
	require.NotEqual(t, first.Trade, second.Trade)
	require.NoError(t, model.FailWalletAutoTopup(second))
	require.Empty(t, claim().Trade)
	require.Equal(t, model.AutoTopupFailed, claim().State)
}
