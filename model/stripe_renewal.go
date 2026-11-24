package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Bind only the originating Stripe Checkout order. A subscription can never
// move between accounts, including when two webhook deliveries race.
func BindStripeSubscription(order *SubscriptionOrder, subscription, price, currency string) error {
	if subscription == "" || price == "" || currency == "" || order.PaymentMethod != PaymentMethodStripe {
		return errors.New("invalid Stripe subscription binding")
	}
	result := DB.Model(&SubscriptionOrder{}).Where("id = ? AND (stripe_subscription_id IS NULL OR stripe_subscription_id = ?)", order.Id, subscription).
		Updates(map[string]any{"stripe_subscription_id": subscription, "stripe_recurring_price": price, "stripe_currency": currency})
	if result.Error != nil {
		return result.Error
	}
	var stored SubscriptionOrder
	if err := DB.Select("stripe_subscription_id").First(&stored, order.Id).Error; err != nil {
		return err
	}
	if stored.StripeSubscriptionID == nil || *stored.StripeSubscriptionID != subscription {
		return errors.New("conflicting Stripe subscription")
	}
	return nil
}

type StripeRenewalSummary struct {
	Id                int    `json:"id"`
	PlanId            int    `json:"plan_id"`
	Title             string `json:"title"`
	StripeCancelAtEnd bool   `json:"cancel_at_period_end"`
}

func ListStripeRenewals(userID int) ([]StripeRenewalSummary, error) {
	var rows []StripeRenewalSummary
	err := DB.Model(&SubscriptionOrder{}).
		Select("subscription_orders.id, subscription_orders.plan_id, subscription_orders.stripe_cancel_at_end, subscription_plans.title").
		Joins("LEFT JOIN subscription_plans ON subscription_plans.id = subscription_orders.plan_id").
		Where("subscription_orders.user_id = ? AND stripe_subscription_id IS NOT NULL AND stripe_ended = ?", userID, false).
		Order("subscription_orders.id desc").Scan(&rows).Error
	return rows, err
}

// Stage an invoice for the normal payment ledger. The original order is
// settled by the first invoice. Later invoices each own a unique trade number.
func PrepareStripeInvoice(origin *SubscriptionOrder, invoiceID string, first bool, start, end int64) (string, error) {
	if first {
		return origin.TradeNo, DB.Model(&SubscriptionOrder{}).Where("id = ? AND status = ?", origin.Id, common.TopUpStatusPending).
			Updates(map[string]any{"stripe_period_start": start, "stripe_period_end": end}).Error
	}
	order := SubscriptionOrder{
		TradeNo: fmt.Sprintf("sub_cycle_%s", invoiceID), UserId: origin.UserId, PlanId: origin.PlanId,
		Money: origin.Money, PurchaseQuantity: origin.PurchaseQuantity, PurchaseMode: SubscriptionPurchaseModeStack,
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
		CreateTime: common.GetTimestamp(), StripePeriodStart: start, StripePeriodEnd: end, StripeCurrency: origin.StripeCurrency,
	}
	err := DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "trade_no"}}, DoNothing: true}).Create(&order).Error
	return order.TradeNo, err
}

func GetStripeRenewalForUser(userID, orderID int) (*SubscriptionOrder, error) {
	var order SubscriptionOrder
	err := DB.Where("id = ? AND user_id = ? AND stripe_subscription_id IS NOT NULL AND stripe_ended = ?", orderID, userID, false).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("未找到可管理的 Stripe 订阅")
	}
	return &order, err
}
