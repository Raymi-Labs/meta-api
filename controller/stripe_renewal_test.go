package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

func TestStripePlanCheckoutRequiresExplicitRenewal(t *testing.T) {
	plan := &model.SubscriptionPlan{PriceAmount: 12, Currency: "USD", DurationUnit: "month", DurationValue: 1}
	price := &stripe.Price{ID: "price_1", Currency: "usd", Active: true, BillingScheme: "per_unit", UnitAmount: 1200, UnitAmountDecimal: 1200,
		Product: &stripe.Product{ID: "prod_1"}, Recurring: &stripe.PriceRecurring{UsageType: "licensed", Interval: "month", IntervalCount: 1}}
	params, err := stripePlanCheckoutParams("ref", &model.User{}, plan, price, 2, false)
	require.NoError(t, err)
	require.Equal(t, "payment", *params.Mode)
	require.Nil(t, params.LineItems[0].Price)
	require.Nil(t, params.LineItems[0].PriceData.Recurring)
	require.Equal(t, int64(1200), *params.LineItems[0].PriceData.UnitAmount)
	params, err = stripePlanCheckoutParams("ref", &model.User{Email: "test@example.com"}, plan, price, 2, true)
	require.NoError(t, err)
	require.Equal(t, "subscription", *params.Mode)
	require.Nil(t, params.CustomerCreation)
	require.Equal(t, "price_1", *params.LineItems[0].Price)
	price.Recurring.Interval = "day"
	_, err = stripePlanCheckoutParams("ref", &model.User{}, plan, price, 1, true)
	require.Error(t, err)
	price.Recurring = nil
	_, err = stripePlanCheckoutParams("ref", &model.User{}, plan, price, 1, true)
	require.Error(t, err)
	price.Currency = "eur"
	_, err = stripePlanCheckoutParams("ref", &model.User{}, plan, price, 1, false)
	require.Error(t, err)
}

func TestStripeRenewalSignedWebhookAndCancellation(t *testing.T) {
	setupTopupCallbackTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionIssuance{}, &model.UserSubscription{}, &model.InviteCommissionLedger{}, &model.InviteCommissionDailyCapState{}))
	user := createTopupCallbackTestUser(t, "stripe_cycle")
	plan := &model.SubscriptionPlan{Id: 22341, Title: "月卡", PriceAmount: 12, Currency: "USD", DurationUnit: "month", DurationValue: 1, TotalAmount: 1000}
	require.NoError(t, model.DB.Create(plan).Error)
	order := &model.SubscriptionOrder{UserId: user.Id, PlanId: plan.Id, Money: 12, TradeNo: "sub_auto_checkout", PurchaseQuantity: 1, StripeRecurringPrice: "price_test", StripeCurrency: "USD",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.NoError(t, order.Insert())
	key, secret := setting.StripeApiSecret, setting.StripeWebhookSecret
	setting.StripeApiSecret, setting.StripeWebhookSecret = "sk_test_example", "whsec_test"
	t.Cleanup(func() { setting.StripeApiSecret, setting.StripeWebhookSecret = key, secret })
	send := func(kind, object string) int {
		raw := []byte(fmt.Sprintf(`{"id":"evt_test","object":"event","type":%q,"data":{"object":%s}}`, kind, object))
		req := httptest.NewRequest("POST", "/api/stripe/webhook", bytes.NewReader(raw))
		req.Header.Set("Stripe-Signature", webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: raw, Secret: "whsec_test"}).Header)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		StripeWebhook(c)
		return w.Code
	}
	cancels := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/checkout/sessions":
			subID := r.URL.Query().Get("subscription")
			ref := map[string]string{"sub_test": "sub_auto_checkout", "sub_legacy": "sub_ref_legacy"}[subID]
			if ref == "" {
				http.Error(w, "unexpected subscription", 400)
				return
			}
			fmt.Fprintf(w, `{"object":"list","data":[{"id":"cs_test","subscription":%q,"client_reference_id":%q,"line_items":{"data":[{"price":{"id":"price_test","currency":"usd"},"quantity":1}]}}]}`, subID, ref)
		case "/v1/subscriptions/sub_test":
			require.Equal(t, "true", r.FormValue("cancel_at_period_end"))
			cancels++
			fmt.Fprint(w, `{"id":"sub_test","cancel_at_period_end":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	old := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, old) })

	now := time.Now().Unix()
	initial := fmt.Sprintf(`{"id":"in_first","subscription":"sub_test","status":"paid","billing_reason":"subscription_create","currency":"usd","amount_paid":1200,"lines":{"data":[{"price":{"id":"price_test"},"quantity":1,"period":{"start":%d,"end":%d}}]}}`, now-3600, now-5)
	cycle := fmt.Sprintf(`{"id":"in_second","parent":{"subscription_details":{"subscription":"sub_test"}},"status":"paid","billing_reason":"subscription_cycle","currency":"usd","amount_paid":1200,"lines":{"data":[{"pricing":{"price_details":{"price":"price_test"}},"quantity":1,"period":{"start":%d,"end":%d}}]}}`, now-5, now+3600)
	require.Equal(t, 200, send("invoice.payment_failed", cycle))
	require.Equal(t, 200, send("invoice.paid", cycle)) // before Checkout and before the older invoice
	require.Equal(t, 200, send("invoice.paid", initial))
	require.Equal(t, 200, send("checkout.session.completed", `{"client_reference_id":"sub_auto_checkout","subscription":"sub_test","status":"complete","payment_status":"paid"}`))
	require.Equal(t, 200, send("invoice.paid", cycle))
	require.Equal(t, 200, send("invoice.paid", initial))
	var granted []model.UserSubscription
	require.NoError(t, model.DB.Order("end_time").Find(&granted).Error)
	require.Len(t, granted, 2)
	require.Equal(t, "expired", granted[0].Status)
	require.Equal(t, now+3600, granted[1].EndTime)
	require.Equal(t, "active", granted[1].Status)
	var charges []model.TopUp
	require.NoError(t, model.DB.Find(&charges).Error)
	require.Len(t, charges, 2)
	require.Equal(t, "USD", charges[0].PaidCurrency)
	cancel := func(userID int) string {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Set("id", userID)
		c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(order.Id)}}
		CancelStripeRenewal(c)
		return w.Body.String()
	}
	require.Contains(t, cancel(user.Id+1), `"success":false`)
	require.Zero(t, cancels)
	require.Contains(t, cancel(user.Id), `"success":true`)
	require.Equal(t, 1, cancels)
	rows, err := model.ListStripeRenewals(user.Id)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].StripeCancelAtEnd)
	require.Equal(t, 200, send("customer.subscription.deleted", `{"id":"sub_test"}`))
	rows, err = model.ListStripeRenewals(user.Id)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, model.DB.First(&granted[1], granted[1].Id).Error)
	require.Equal(t, "active", granted[1].Status)
	legacy := &model.SubscriptionOrder{UserId: user.Id, PlanId: plan.Id, Money: 12, TradeNo: "sub_ref_legacy", PurchaseQuantity: 1,
		PaymentMethod: PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusSuccess}
	require.NoError(t, legacy.Insert())
	require.Equal(t, 200, send("invoice.paid", strings.ReplaceAll(initial, "sub_test", "sub_legacy")))
	legacyCycle := strings.NewReplacer("sub_test", "sub_legacy", "in_second", "in_legacy_cycle").Replace(cycle)
	shortPaid := strings.NewReplacer("in_legacy_cycle", "in_short", "1200", "1100").Replace(legacyCycle)
	require.Equal(t, 200, send("invoice.paid", shortPaid))
	require.Equal(t, 200, send("invoice.paid", legacyCycle))
	require.Equal(t, 200, send("invoice.paid", legacyCycle))
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
	require.NoError(t, model.DB.Model(&model.PaymentRiskCase{}).Where("trade_no = ?", "sub_cycle_in_short").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
