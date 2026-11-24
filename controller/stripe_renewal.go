package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	stripeclient "github.com/stripe/stripe-go/v81/client"
	"gorm.io/gorm"
)

// Stripe moved the subscription and price fields under parent/pricing in
// Basil. Webhook endpoints can be pinned independently of the outbound SDK.
type stripePaidInvoice struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	BillingReason string `json:"billing_reason"`
	AmountPaid    int64  `json:"amount_paid"`
	Currency      string `json:"currency"`
	Subscription  any    `json:"subscription"`
	Parent        struct {
		SubscriptionDetails struct {
			Subscription any `json:"subscription"`
		} `json:"subscription_details"`
	} `json:"parent"`
	Lines struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			Price   any `json:"price"`
			Pricing struct {
				PriceDetails struct {
					Price any `json:"price"`
				} `json:"price_details"`
			} `json:"pricing"`
			Quantity  int                        `json:"quantity"`
			Period    struct{ Start, End int64 } `json:"period"`
			Proration bool                       `json:"proration"`
			Parent    struct {
				SubscriptionItemDetails struct {
					Proration bool `json:"proration"`
				} `json:"subscription_item_details"`
			} `json:"parent"`
		} `json:"data"`
	} `json:"lines"`
}

// A missing binding is normal for pre-upgrade subscriptions and when the
// invoice wins the race against checkout.session.completed. Resolve ownership
// using the original Checkout reference, never email or the customer's plan.
func resolveStripeSubscriptionOrder(ctx context.Context, subID string) (*model.SubscriptionOrder, error) {
	var bound model.SubscriptionOrder
	err := model.DB.Where("stripe_subscription_id = ?", subID).First(&bound).Error
	if err == nil {
		return &bound, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	api := stripeclient.New(setting.StripeApiSecret, nil)
	params := &stripe.CheckoutSessionListParams{Subscription: stripe.String(subID)}
	params.Context = ctx
	params.AddExpand("data.line_items")
	iter := api.CheckoutSessions.List(params)
	for iter.Next() {
		session := iter.CheckoutSession()
		ref := session.ClientReferenceID
		if session.Subscription == nil || session.Subscription.ID != subID || (!strings.HasPrefix(ref, "sub_auto_") && !strings.HasPrefix(ref, "sub_ref_")) {
			continue
		}
		order, err := model.GetSubscriptionOrderByTradeNoWithError(ref)
		if errors.Is(err, model.ErrSubscriptionOrderNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if session.LineItems == nil || len(session.LineItems.Data) != 1 || session.LineItems.Data[0].Price == nil {
			return nil, errors.New("Stripe Checkout 缺少原始套餐价格")
		}
		item := session.LineItems.Data[0]
		price := item.Price
		if int64(order.PurchaseQuantity) != item.Quantity || (order.StripeRecurringPrice != "" && order.StripeRecurringPrice != price.ID) {
			return nil, errors.New("Stripe Checkout 与套餐订单不符")
		}
		if err := model.BindStripeSubscription(order, subID, price.ID, strings.ToUpper(string(price.Currency))); err != nil {
			return nil, err
		}
		order.StripeRecurringPrice, order.StripeCurrency = price.ID, strings.ToUpper(string(price.Currency))
		return order, nil
	}
	return nil, iter.Err()
}

func stripeInvoicePaid(ctx context.Context, event stripe.Event) error {
	if event.Data == nil {
		return errors.New("missing Stripe invoice")
	}
	var inv stripePaidInvoice
	if err := common.Unmarshal(event.Data.Raw, &inv); err != nil {
		return err
	}
	first := inv.BillingReason == "subscription_create"
	if inv.Status != "paid" || (!first && inv.BillingReason != "subscription_cycle") {
		return nil
	}
	subID := stripeCheckoutCustomerID(inv.Parent.SubscriptionDetails.Subscription)
	if subID == "" {
		subID = stripeCheckoutCustomerID(inv.Subscription)
	}
	if subID == "" || inv.ID == "" {
		return errors.New("Stripe 周期账单缺少标识")
	}
	origin, err := resolveStripeSubscriptionOrder(ctx, subID)
	if err != nil || origin == nil {
		return err
	}
	if first && !strings.HasPrefix(origin.TradeNo, "sub_auto_") {
		return nil
	} // legacy first payment uses Checkout
	if len(inv.Lines.Data) != 1 || inv.Lines.HasMore {
		return fmt.Errorf("Stripe invoice %s: expected a single complete subscription line", inv.ID)
	}
	line := inv.Lines.Data[0]
	price := stripeCheckoutCustomerID(line.Pricing.PriceDetails.Price)
	if price == "" {
		price = stripeCheckoutCustomerID(line.Price)
	}
	if price != origin.StripeRecurringPrice || line.Quantity != origin.PurchaseQuantity || line.Proration || line.Parent.SubscriptionItemDetails.Proration ||
		line.Period.Start <= 0 || line.Period.End <= line.Period.Start || line.Period.Start > common.GetTimestamp() ||
		!strings.EqualFold(inv.Currency, origin.StripeCurrency) {
		return fmt.Errorf("Stripe invoice %s: recurring line does not match original order or period", inv.ID)
	}

	trade, err := model.PrepareStripeInvoice(origin, inv.ID, first, line.Period.Start, line.Period.End)
	if err != nil {
		return err
	}
	payload := common.GetJsonString(map[string]any{"invoice": inv.ID, "subscription": subID, "amount_paid": inv.AmountPaid, "currency": inv.Currency})
	input := service.PaymentCallbackValidationInput{TradeNo: trade, PaymentMethod: PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe,
		ProviderAmount: stripeAmountFromMinorUnits(inv.AmountPaid, inv.Currency), Currency: inv.Currency, Source: "stripe_invoice_paid", ProviderPayload: payload}
	check, err := service.ValidateSubscriptionCallback(input)
	if errors.Is(err, service.ErrPaymentCallbackRejected) {
		return nil
	}
	if err != nil || check.AlreadyCompleted {
		return err
	}
	return model.CompleteSubscriptionOrderWithPayment(trade, payload, PaymentMethodStripe, model.PaymentProviderStripe)
}

func syncStripeSubscription(ctx context.Context, event stripe.Event) error {
	if event.Data == nil {
		return errors.New("missing Stripe subscription")
	}
	id := event.GetObjectValue("id")
	if id == "" {
		return errors.New("missing Stripe subscription ID")
	}
	order, err := resolveStripeSubscriptionOrder(ctx, id)
	if err != nil || order == nil || order.StripeEnded {
		return err
	}
	query := model.DB.Model(&model.SubscriptionOrder{}).Where("id = ? AND stripe_ended = ?", order.Id, false)
	if event.Type == stripe.EventTypeCustomerSubscriptionDeleted {
		return query.Update("stripe_ended", true).Error
	}
	// An old update can arrive after a new one. Read Stripe's current state.
	params := &stripe.SubscriptionParams{}
	params.Context = ctx
	sub, err := stripeclient.New(setting.StripeApiSecret, nil).Subscriptions.Get(id, params)
	if err != nil {
		return err
	}
	return query.Where("stripe_cancel_at_end = ?", order.StripeCancelAtEnd).Updates(map[string]any{"stripe_cancel_at_end": sub.CancelAtPeriodEnd, "stripe_ended": sub.Status == stripe.SubscriptionStatusCanceled}).Error
}

func CancelStripeRenewal(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	order, err := model.GetStripeRenewalForUser(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params := &stripe.SubscriptionParams{CancelAtPeriodEnd: stripe.Bool(true)}
	params.Context = c.Request.Context()
	_, err = stripeclient.New(setting.StripeApiSecret, nil).Subscriptions.Update(*order.StripeSubscriptionID, params)
	if err != nil {
		common.ApiErrorMsg(c, "Stripe 未能确认期末取消，请重试")
		return
	}
	if err = model.DB.Model(&model.SubscriptionOrder{}).Where("id = ?", order.Id).Update("stripe_cancel_at_end", true).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
