package controller

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	stripeclient "github.com/stripe/stripe-go/v81/client"
	"github.com/thanhpk/randstr"
)

type SubscriptionStripePayRequest struct {
	AutoRenew        bool   `json:"auto_renew"`
	PlanId           int    `json:"plan_id"`
	PurchaseMode     string `json:"purchase_mode"`
	PurchaseQuantity int    `json:"purchase_quantity"`
	// RenewTargetSubscriptionId 仅在 purchase_mode=renew 时生效。
	RenewTargetSubscriptionId int `json:"renew_target_subscription_id"`
}

func SubscriptionRequestStripePay(c *gin.Context) {
	var req SubscriptionStripePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.StripePriceId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 StripePriceId")
		return
	}
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		common.ApiErrorMsg(c, "Stripe 未配置或密钥无效")
		return
	}
	if setting.StripeWebhookSecret == "" {
		common.ApiErrorMsg(c, "Stripe Webhook 未配置")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	purchaseQuantity, err := normalizeSubscriptionPurchaseQuantity(userId, req.PurchaseQuantity, plan)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	// 续费模式支持手动指定目标订阅；未指定时按规则自动命中（唯一或最早到期）。
	purchaseMode, renewTargetSubId, err := resolveSubscriptionPurchaseModeAndTarget(
		userId, plan.Id, req.PurchaseMode, req.RenewTargetSubscriptionId,
	)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := checkSubscriptionOrderLimits(userId, plan, purchaseMode, purchaseQuantity); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	reference := fmt.Sprintf("sub-stripe-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "sub_ref_" + common.Sha1([]byte(reference))

	if req.AutoRenew && !plan.StripeAutoRenew {
		common.ApiErrorMsg(c, "该套餐未开启 Stripe 自动续费")
		return
	}
	if req.AutoRenew && purchaseMode != model.SubscriptionPurchaseModeStack {
		common.ApiErrorMsg(c, "自动续费仅适用于按周期购买的新套餐")
		return
	}
	if req.AutoRenew {
		referenceId = "sub_auto_" + common.Sha1([]byte(reference))
	}
	api := stripeclient.New(setting.StripeApiSecret, nil)
	priceParams := &stripe.PriceParams{}
	priceParams.Context = c.Request.Context()
	price, err := api.Prices.Get(plan.StripePriceId, priceParams)
	if err != nil {
		log.Printf("Stripe 套餐价格读取失败: %v", err)
		common.ApiErrorMsg(c, "无法读取 Stripe 套餐价格")
		return
	}
	params, err := stripePlanCheckoutParams(referenceId, user, plan, price, purchaseQuantity, req.AutoRenew)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	totalMoney := plan.PriceAmount * float64(purchaseQuantity)

	order := &model.SubscriptionOrder{
		UserId:                    userId,
		PlanId:                    plan.Id,
		Money:                     totalMoney,
		PurchaseQuantity:          purchaseQuantity,
		TradeNo:                   referenceId,
		PaymentMethod:             PaymentMethodStripe,
		PaymentProvider:           model.PaymentProviderStripe,
		PurchaseMode:              purchaseMode,
		RenewTargetSubscriptionId: renewTargetSubId,
		CreateTime:                time.Now().Unix(),
		Status:                    common.TopUpStatusPending,
	}
	order.StripeCurrency = strings.ToUpper(string(price.Currency))
	if req.AutoRenew {
		order.StripeRecurringPrice = price.ID
	}
	if err := order.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	params.Context = c.Request.Context()
	params.SetIdempotencyKey(referenceId)
	result, err := api.CheckoutSessions.New(params)
	if err != nil {
		log.Printf("Stripe Checkout 创建失败: %v", err)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": result.URL,
		},
	})
}

func stripePlanCheckoutParams(ref string, user *model.User, plan *model.SubscriptionPlan, price *stripe.Price, quantity int, recurring bool) (*stripe.CheckoutSessionParams, error) {
	if !price.Active || price.Product == nil || price.BillingScheme != stripe.PriceBillingSchemePerUnit ||
		price.TransformQuantity != nil || price.UnitAmount <= 0 || price.UnitAmountDecimal != float64(price.UnitAmount) ||
		!strings.EqualFold(string(price.Currency), plan.Currency) || stripeAmountFromMinorUnits(price.UnitAmount, plan.Currency) != plan.PriceAmount {
		return nil, errors.New("Stripe 价格的金额或币种与套餐不符，或并非固定单价")
	}
	line := &stripe.CheckoutSessionLineItemParams{Price: stripe.String(price.ID), Quantity: stripe.Int64(int64(quantity))}
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(ref),
		SuccessURL:        stripe.String(system_setting.ServerAddress + "/console/topup"),
		CancelURL:         stripe.String(system_setting.ServerAddress + "/console/topup"),
		LineItems:         []*stripe.CheckoutSessionLineItemParams{line},
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
	}
	if user.StripeCustomer != "" {
		params.Customer = stripe.String(user.StripeCustomer)
	} else if user.Email != "" {
		params.CustomerEmail = stripe.String(user.Email)
	}
	if recurring {
		r := price.Recurring
		if r == nil || r.UsageType != stripe.PriceRecurringUsageTypeLicensed {
			return nil, errors.New("自动续费需要固定周期的 Stripe 价格")
		}
		unit, count := string(r.Interval), r.IntervalCount
		if unit == "week" {
			unit, count = "day", count*7
		}
		if unit != plan.DurationUnit || count != int64(plan.DurationValue) {
			return nil, errors.New("Stripe 扣费周期与套餐有效期不符")
		}
		params.Mode = stripe.String(string(stripe.CheckoutSessionModeSubscription))
		params.AdaptivePricing = &stripe.CheckoutSessionAdaptivePricingParams{Enabled: stripe.Bool(false)}
		params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{Metadata: map[string]string{"meta_api_order": ref}}
	} else if price.Recurring != nil {
		// A recurring Price cannot be passed directly to payment-mode Checkout.
		line.Price = nil
		line.PriceData = &stripe.CheckoutSessionLineItemPriceDataParams{Product: stripe.String(price.Product.ID), Currency: stripe.String(string(price.Currency)), UnitAmount: stripe.Int64(price.UnitAmount)}
		if price.TaxBehavior != "" {
			line.PriceData.TaxBehavior = stripe.String(string(price.TaxBehavior))
		}
	}
	return params, nil
}
