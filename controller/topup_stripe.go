package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	stripeclient "github.com/stripe/stripe-go/v81/client"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

const (
	PaymentMethodStripe = model.PaymentMethodStripe
)

var stripeAdaptor = &StripeAdaptor{}

type stripePriceCurrencyLoaderFunc func(ctx context.Context, apiSecret string, priceID string) (string, error)

var (
	stripePriceCurrencyCacheMu sync.RWMutex
	stripePriceCurrencyCache                                 = make(map[string]string)
	stripePriceCurrencyLoader  stripePriceCurrencyLoaderFunc = loadStripePriceCurrency
)

// StripePayRequest represents a payment request for Stripe checkout.
type StripePayRequest struct {
	// Amount is the quantity of units to purchase.
	Amount int64 `json:"amount"`
	// PaymentMethod specifies the payment method (e.g., "stripe").
	PaymentMethod string `json:"payment_method"`
	// SuccessURL is the optional custom URL to redirect after successful payment.
	// If empty, defaults to the server's console log page.
	SuccessURL string `json:"success_url,omitempty"`
	// CancelURL is the optional custom URL to redirect when payment is canceled.
	// If empty, defaults to the server's console topup page.
	CancelURL string `json:"cancel_url,omitempty"`
}

type StripeAdaptor struct {
}

type stripeCheckoutResult struct {
	URL      string
	Money    float64
	Currency string
}

type stripeCheckoutSessionEvent struct {
	Subscription      any    `json:"subscription"`
	Customer          any    `json:"customer"`
	ClientReferenceID string `json:"client_reference_id"`
	Status            string `json:"status"`
	PaymentStatus     string `json:"payment_status"`
	AmountTotal       int64  `json:"amount_total"`
	AmountSubtotal    int64  `json:"amount_subtotal"`
	Currency          string `json:"currency"`
	TotalDetails      *struct {
		AmountDiscount int64 `json:"amount_discount"`
	} `json:"total_details"`
	CurrencyConversion *struct {
		AmountSubtotal int64  `json:"amount_subtotal"`
		AmountTotal    int64  `json:"amount_total"`
		SourceCurrency string `json:"source_currency"`
	} `json:"currency_conversion"`
	PresentmentDetails *struct {
		PresentmentAmount   int64  `json:"presentment_amount"`
		PresentmentCurrency string `json:"presentment_currency"`
	} `json:"presentment_details"`
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	if req.Amount < getStripeMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getStripeMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getStripePayMoney(float64(req.Amount), group)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	currency, err := resolveStripePriceCurrency(c.Request.Context())
	if err != nil {
		log.Printf("获取 Stripe Price 币种失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取 Stripe 支付币种失败，请联系管理员检查价格配置"})
		return
	}
	writePaymentQuote(c, payMoney, currency)
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if req.PaymentMethod != PaymentMethodStripe {
		c.JSON(200, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if req.Amount < getStripeMinTopup() {
		c.JSON(200, gin.H{"message": fmt.Sprintf("充值数量不能小于 %d", getStripeMinTopup()), "data": 10})
		return
	}
	if req.Amount > 10000 {
		c.JSON(200, gin.H{"message": "充值数量不能大于 10000", "data": 10})
		return
	}

	if req.SuccessURL != "" && common.ValidateRedirectURL(req.SuccessURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付成功重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	if req.CancelURL != "" && common.ValidateRedirectURL(req.CancelURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付取消重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	id := c.GetInt("id")
	user, _ := model.GetUserById(id, false)
	chargedMoney := GetChargedAmount(float64(req.Amount), *user)
	paidMoney := getStripePayMoney(float64(req.Amount), user.Group)

	reference := fmt.Sprintf("new-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	checkout, err := genStripeLink(c.Request.Context(), referenceId, user.StripeCustomer, user.Email, req.Amount, req.SuccessURL, req.CancelURL)
	if err != nil {
		log.Println("获取Stripe Checkout支付链接失败", err)
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if checkout.Money > 0 {
		paidMoney = checkout.Money
	}

	topUp := &model.TopUp{
		UserId:          id,
		Amount:          req.Amount,
		Money:           chargedMoney,
		PaidMoney:       paidMoney,
		PaidCurrency:    checkout.Currency,
		TradeNo:         referenceId,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": checkout.URL,
		},
	})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	if !isStripeWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("解析Stripe Webhook参数失败: %v\n", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	endpointSecret := setting.StripeWebhookSecret
	event, err := webhook.ConstructEventWithOptions(payload, signature, endpointSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		log.Printf("Stripe Webhook验签失败: %v\n", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// A plan can have a Stripe price without enabling wallet top-ups.
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted, stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded, stripe.EventTypeCheckoutSessionExpired:
		if event.Data == nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		if strings.HasPrefix(event.GetObjectValue("client_reference_id"), "autoset_") {
			if err := autoTopupCheckout(c.Request.Context(), event); err != nil {
				log.Printf("Stripe 自动充值授权回调失败 event=%s", event.ID)
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			c.Status(http.StatusOK)
			return
		}
		if !isStripeTopUpEnabled() && !strings.HasPrefix(event.GetObjectValue("client_reference_id"), "sub_") {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
	}
	var processingErr error
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted, stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		processingErr = sessionCompleted(event, c.ClientIP())
	case stripe.EventTypeSetupIntentSucceeded:
		processingErr = autoTopupSetupSucceeded(event)
	case stripe.EventTypePaymentIntentSucceeded:
		processingErr = autoTopupPaymentSucceeded(event)
	case stripe.EventTypeInvoicePaid:
		processingErr = stripeInvoicePaid(c.Request.Context(), event)
	case stripe.EventTypeCustomerSubscriptionDeleted, stripe.EventTypeCustomerSubscriptionUpdated:
		processingErr = syncStripeSubscription(c.Request.Context(), event)
	case stripe.EventTypeCheckoutSessionExpired:
		sessionExpired(event)
	default:
		log.Printf("不支持的Stripe Webhook事件类型: %s\n", event.Type)
	}
	if processingErr != nil {
		log.Printf("Stripe Webhook处理失败: event=%s err=%v\n", event.ID, processingErr)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}

func sessionCompleted(event stripe.Event, callerIp string) error {
	if event.Data == nil || len(event.Data.Raw) == 0 {
		return errors.New("Stripe Checkout完成事件缺少支付数据")
	}
	checkout := stripeCheckoutSessionEvent{}
	if err := common.Unmarshal(event.Data.Raw, &checkout); err != nil {
		log.Println("解析Stripe Checkout完成事件失败:", err)
		return err
	}
	referenceId := strings.TrimSpace(checkout.ClientReferenceID)
	if checkout.Status == "complete" && strings.HasPrefix(referenceId, "sub_auto_") {
		order, err := model.GetSubscriptionOrderByTradeNoWithError(referenceId)
		if err != nil {
			return err
		}
		// The invoice is the sole fulfillment event for every paid period.
		return model.BindStripeSubscription(order, stripeCheckoutCustomerID(checkout.Subscription), order.StripeRecurringPrice, order.StripeCurrency)
	}
	if checkout.Status != "complete" || checkout.PaymentStatus != "paid" {
		log.Println("错误的Stripe Checkout支付状态:", checkout.Status, checkout.PaymentStatus, ",", referenceId)
		return nil
	}
	if referenceId == "" {
		log.Println("Stripe Checkout完成事件未提供支付单号")
		return nil
	}

	paymentDetails, err := stripePaymentDetailsFromCheckout(checkout)
	if err != nil {
		log.Println("Stripe Checkout完成事件金额或币种无效:", checkout.AmountTotal, checkout.Currency, ",", referenceId)
		return err
	}
	paidAmount := paymentDetails.PaidMoney
	paidCurrency := paymentDetails.PaidCurrency
	subtotalAmount := stripeSettlementSubtotalFromCheckout(checkout, paidCurrency)
	discountAmount := float64(0)
	discountCurrency := paidCurrency
	if checkout.TotalDetails != nil {
		if checkout.CurrencyConversion != nil && checkout.PresentmentDetails == nil {
			discountCurrency = model.NormalizePaymentCurrency(checkout.Currency)
		}
		discountAmount = stripeAmountFromMinorUnits(checkout.TotalDetails.AmountDiscount, discountCurrency)
	}
	customerId := stripeCheckoutCustomerID(checkout.Customer)

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	payload := map[string]any{
		"customer":             customerId,
		"amount_total":         checkout.AmountTotal,
		"amount_subtotal":      checkout.AmountSubtotal,
		"amount_discount":      discountAmount,
		"discount_currency":    discountCurrency,
		"currency":             paidCurrency,
		"presentment_amount":   paymentDetails.PresentmentMoney,
		"presentment_currency": paymentDetails.PresentmentCurrency,
		"event_type":           string(event.Type),
	}
	subscriptionOrder, lookupErr := model.GetSubscriptionOrderByTradeNoWithError(referenceId)
	if lookupErr != nil && !errors.Is(lookupErr, model.ErrSubscriptionOrderNotFound) {
		return lookupErr
	}
	if subscriptionOrder != nil {
		validationInput := service.PaymentCallbackValidationInput{
			TradeNo:          referenceId,
			PaymentMethod:    PaymentMethodStripe,
			PaymentProvider:  model.PaymentProviderStripe,
			ProviderAmount:   paidAmount,
			ProviderSubtotal: subtotalAmount,
			Currency:         paidCurrency,
			Source:           "subscription_stripe_webhook",
			ProviderPayload:  common.GetJsonString(payload),
		}
		checkResult, err := service.ValidateSubscriptionCallback(validationInput)
		if err != nil {
			log.Println("stripe subscription validation failed:", err.Error(), referenceId)
			if errors.Is(err, service.ErrPaymentCallbackRejected) {
				return nil
			}
			return err
		}
		if !checkResult.AlreadyCompleted {
			if err := model.CompleteSubscriptionOrderWithPayment(referenceId, common.GetJsonString(payload), PaymentMethodStripe, model.PaymentProviderStripe); err != nil {
				service.RecordSubscriptionProcessingRiskCase(validationInput, err)
				log.Println("complete subscription order failed:", err.Error(), referenceId)
				return err
			}
		}
		return nil
	}

	_, err = service.ValidateTopUpCallback(service.PaymentCallbackValidationInput{
		TradeNo:                      referenceId,
		PaymentMethod:                PaymentMethodStripe,
		PaymentProvider:              model.PaymentProviderStripe,
		ProviderAmount:               paidAmount,
		ProviderSubtotal:             subtotalAmount,
		Currency:                     paidCurrency,
		Source:                       "stripe_webhook",
		ProviderPayload:              common.GetJsonString(payload),
		AllowCompletedAmountBackfill: true,
	})
	if err != nil {
		log.Println("stripe top-up validation failed:", err.Error(), referenceId)
		if errors.Is(err, service.ErrPaymentCallbackRejected) {
			return nil
		}
		return err
	}
	if err := model.RechargeWithPaymentDetails(referenceId, customerId, callerIp, paymentDetails); err != nil {
		log.Println(err.Error(), referenceId)
		return err
	}

	log.Printf("收到款项：%s, %.2f(%s)", referenceId, paidAmount, paidCurrency)
	return nil
}

func sessionExpired(event stripe.Event) {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		log.Println("错误的Stripe Checkout过期状态:", status, ",", referenceId)
		return
	}

	if len(referenceId) == 0 {
		log.Println("未提供支付单号")
		return
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	if subscriptionOrder := model.GetSubscriptionOrderByTradeNo(referenceId); subscriptionOrder != nil {
		if err := model.ExpireSubscriptionOrder(referenceId); err != nil {
			log.Println("过期订阅订单失败", referenceId, ", err:", err.Error())
		}
		return
	}

	topUp := model.GetTopUpByTradeNo(referenceId)
	if topUp == nil {
		log.Println("充值订单不存在", referenceId)
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		log.Println("充值订单状态错误", referenceId)
	}

	topUp.Status = common.TopUpStatusExpired
	err := topUp.Update()
	if err != nil {
		log.Println("过期充值订单失败", referenceId, ", err:", err.Error())
		return
	}

	log.Println("充值订单已过期", referenceId)
}

// genStripeLink generates a Stripe Checkout session URL for payment.
// It creates a new checkout session with the specified parameters and returns the payment URL.
//
// Parameters:
//   - referenceId: unique reference identifier for the transaction
//   - customerId: existing Stripe customer ID (empty string if new customer)
//   - email: customer email address for new customer creation
//   - amount: quantity of units to purchase
//   - successURL: custom URL to redirect after successful payment (empty for default)
//   - cancelURL: custom URL to redirect when payment is canceled (empty for default)
//
// Returns the checkout session URL or an error if the session creation fails.
func genStripeLink(ctx context.Context, referenceId string, customerId string, email string, amount int64, successURL string, cancelURL string) (stripeCheckoutResult, error) {
	checkout := stripeCheckoutResult{}
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return checkout, fmt.Errorf("无效的Stripe API密钥")
	}

	// Use custom URLs if provided, otherwise use defaults
	if successURL == "" {
		successURL = system_setting.ServerAddress + "/console/log"
	}
	if cancelURL == "" {
		cancelURL = system_setting.ServerAddress + "/console/topup"
	}

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(successURL),
		CancelURL:         stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(setting.StripePriceId),
				Quantity: stripe.Int64(amount),
			},
		},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(setting.StripePromotionCodesEnabled),
	}
	params.Context = ctx

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}

		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	stripeClient := stripeclient.New(setting.StripeApiSecret, nil)
	result, err := stripeClient.CheckoutSessions.New(params)
	if err != nil {
		return checkout, err
	}

	checkout.URL = result.URL
	checkout.Currency = model.NormalizePaymentCurrency(string(result.Currency))
	checkout.Money = stripeAmountFromMinorUnits(result.AmountTotal, checkout.Currency)
	return checkout, nil
}

func loadStripePriceCurrency(ctx context.Context, apiSecret string, priceID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	stripeClient := stripeclient.New(apiSecret, nil)
	params := &stripe.PriceParams{}
	params.Context = ctx
	price, err := stripeClient.Prices.Get(priceID, params)
	if err != nil {
		return "", err
	}
	if price == nil || price.Deleted {
		return "", fmt.Errorf("Stripe Price 不存在")
	}
	currency := model.NormalizePaymentCurrency(string(price.Currency))
	if currency == "" {
		return "", fmt.Errorf("Stripe Price 币种无效")
	}
	return currency, nil
}

func resolveStripePriceCurrency(ctx context.Context) (string, error) {
	apiSecret := strings.TrimSpace(setting.StripeApiSecret)
	priceID := strings.TrimSpace(setting.StripePriceId)
	if (!strings.HasPrefix(apiSecret, "sk_") && !strings.HasPrefix(apiSecret, "rk_")) || priceID == "" {
		return "", fmt.Errorf("Stripe API 密钥或 Price ID 未正确配置")
	}

	cacheKey := fmt.Sprintf("%x:%s", common.Sha256Raw([]byte(apiSecret)), priceID)
	stripePriceCurrencyCacheMu.RLock()
	currency, ok := stripePriceCurrencyCache[cacheKey]
	stripePriceCurrencyCacheMu.RUnlock()
	if ok {
		return currency, nil
	}

	currency, err := stripePriceCurrencyLoader(ctx, apiSecret, priceID)
	if err != nil {
		return "", err
	}
	currency = model.NormalizePaymentCurrency(currency)
	if currency == "" {
		return "", fmt.Errorf("Stripe Price 币种无效")
	}

	stripePriceCurrencyCacheMu.Lock()
	stripePriceCurrencyCache[cacheKey] = currency
	stripePriceCurrencyCacheMu.Unlock()
	return currency, nil
}

func stripeAmountFromMinorUnits(amount int64, currency string) float64 {
	return model.StripeMoneyFromMinor(amount, currency)
}

func stripePaymentDetailsFromCheckout(checkout stripeCheckoutSessionEvent) (model.TopUpPaymentDetails, error) {
	if checkout.PresentmentDetails == nil && checkout.CurrencyConversion != nil {
		paidCurrency := model.NormalizePaymentCurrency(checkout.CurrencyConversion.SourceCurrency)
		paidAmount := stripeAmountFromMinorUnits(checkout.CurrencyConversion.AmountTotal, paidCurrency)
		presentmentCurrency := model.NormalizePaymentCurrency(checkout.Currency)
		presentmentAmount := stripeAmountFromMinorUnits(checkout.AmountTotal, presentmentCurrency)
		if paidAmount <= 0 || paidCurrency == "" || presentmentAmount <= 0 || presentmentCurrency == "" {
			return model.TopUpPaymentDetails{}, fmt.Errorf("invalid legacy Stripe adaptive payment amount or currency")
		}
		return model.TopUpPaymentDetails{
			PaidMoney:           paidAmount,
			PaidCurrency:        paidCurrency,
			PresentmentMoney:    presentmentAmount,
			PresentmentCurrency: presentmentCurrency,
		}, nil
	}

	paidCurrency := model.NormalizePaymentCurrency(checkout.Currency)
	paidAmount := stripeAmountFromMinorUnits(checkout.AmountTotal, paidCurrency)
	if paidAmount <= 0 || paidCurrency == "" {
		return model.TopUpPaymentDetails{}, fmt.Errorf("invalid Stripe payment amount or currency")
	}
	details := model.TopUpPaymentDetails{
		PaidMoney:    paidAmount,
		PaidCurrency: paidCurrency,
	}
	if checkout.PresentmentDetails == nil {
		return details, nil
	}
	presentmentCurrency := model.NormalizePaymentCurrency(checkout.PresentmentDetails.PresentmentCurrency)
	presentmentAmount := stripeAmountFromMinorUnits(checkout.PresentmentDetails.PresentmentAmount, presentmentCurrency)
	if presentmentAmount > 0 && presentmentCurrency != "" {
		details.PresentmentMoney = presentmentAmount
		details.PresentmentCurrency = presentmentCurrency
	}
	return details, nil
}

func stripeSettlementSubtotalFromCheckout(checkout stripeCheckoutSessionEvent, paidCurrency string) float64 {
	if checkout.PresentmentDetails == nil && checkout.CurrencyConversion != nil {
		sourceCurrency := model.NormalizePaymentCurrency(checkout.CurrencyConversion.SourceCurrency)
		return stripeAmountFromMinorUnits(checkout.CurrencyConversion.AmountSubtotal, sourceCurrency)
	}
	return stripeAmountFromMinorUnits(checkout.AmountSubtotal, paidCurrency)
}

func stripeCheckoutCustomerID(customer any) string {
	switch value := customer.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		id, _ := value["id"].(string)
		return strings.TrimSpace(id)
	default:
		return ""
	}
}

func GetChargedAmount(count float64, user model.User) float64 {
	topUpGroupRatio := common.GetTopupGroupRatio(user.Group)
	if topUpGroupRatio == 0 {
		topUpGroupRatio = 1
	}

	return count * topUpGroupRatio
}

func getStripePayMoney(amount float64, group string) float64 {
	return service.CalculateStripeTopUpPayMoney(amount, group)
}

func getStripeMinTopup() int64 {
	minTopup := setting.StripeMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		minTopup = minTopup * int(common.QuotaPerUnit)
	}
	return int64(minTopup)
}
