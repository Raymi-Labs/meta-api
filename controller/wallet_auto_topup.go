package controller

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	stripeclient "github.com/stripe/stripe-go/v81/client"
	"gorm.io/gorm"
)

type autoTopupQuote struct {
	Price     string  `json:"price"`
	UnitMinor int64   `json:"unit_minor"`
	UnitMoney float64 `json:"unit_money"`
	Currency  string  `json:"currency"`
	GrantUnit float64 `json:"grant_unit"`
	Minimum   int64   `json:"minimum"`
}

func loadAutoTopupQuote(ctx context.Context, user *model.User) (autoTopupQuote, error) {
	if !isStripeTopUpEnabled() {
		return autoTopupQuote{}, errors.New("Stripe 充值未配置")
	}
	params := &stripe.PriceParams{}
	params.Context = ctx
	price, err := stripeclient.New(setting.StripeApiSecret, nil).Prices.Get(setting.StripePriceId, params)
	if err != nil {
		return autoTopupQuote{}, errors.New("无法获取 Stripe 充值价格")
	}
	if !price.Active || price.Type != stripe.PriceTypeOneTime || price.BillingScheme != stripe.PriceBillingSchemePerUnit || price.TransformQuantity != nil || price.CustomUnitAmount != nil || price.UnitAmount <= 0 || price.UnitAmountDecimal != float64(price.UnitAmount) {
		return autoTopupQuote{}, errors.New("自动充值需要固定单价的 Stripe 充值价格")
	}
	return autoTopupQuote{Price: price.ID, UnitMinor: price.UnitAmount, UnitMoney: model.StripeMoneyFromMinor(price.UnitAmount, string(price.Currency)), Currency: strings.ToUpper(string(price.Currency)), GrantUnit: GetChargedAmount(1, *user), Minimum: int64(max(setting.StripeMinTopUp, 1))}, nil
}

func GetWalletAutoTopup(c *gin.Context) {
	userID := c.GetInt("id")
	rule, err := model.GetWalletAutoTopup(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := gin.H{"rule": rule, "charge_money": rule.PaidMoney(), "processing": rule.Trade != ""}
	if c.Query("quote") == "true" && isStripeTopUpEnabled() {
		user, err := model.GetUserById(userID, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		quote, err := loadAutoTopupQuote(c.Request.Context(), user)
		if err != nil {
			result["quote_error"] = err.Error()
		} else {
			result["quote"] = quote
		}
	}
	common.ApiSuccess(c, result)
}

func SetupWalletAutoTopup(c *gin.Context) {
	var req struct {
		Threshold int64 `json:"threshold"`
		Quantity  int64 `json:"quantity"`
		autoTopupQuote
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Threshold <= 0 || req.Threshold > 1e15 || req.Quantity < 1 || req.Quantity > 10000 {
		common.ApiErrorMsg(c, "阈值或充值数量无效")
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx := c.Request.Context()
	quote, err := loadAutoTopupQuote(ctx, user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Price != quote.Price || req.UnitMinor != quote.UnitMinor || req.Currency != quote.Currency || req.GrantUnit != quote.GrantUnit || req.Quantity < quote.Minimum || quote.UnitMinor > 99999999/req.Quantity {
		common.ApiErrorMsg(c, "充值报价已变更或超过支付限额，请刷新后重新确认")
		return
	}
	if float64(req.Quantity)*quote.GrantUnit*common.QuotaPerUnit < 1 {
		common.ApiErrorMsg(c, "到账额度无效")
		return
	}
	api := stripeclient.New(setting.StripeApiSecret, nil)
	customer := user.StripeCustomer
	if customer == "" {
		params := &stripe.CustomerParams{}
		params.Context = ctx
		if user.Email != "" {
			params.Email = stripe.String(user.Email)
		}
		cust, err := api.Customers.New(params)
		if err != nil {
			common.ApiErrorMsg(c, "Stripe 无法创建授权客户")
			return
		}
		customer = cust.ID
	}
	ref := "autoset_" + common.GetUUID()
	rule := model.WalletAutoTopup{UserID: user.Id, Threshold: req.Threshold, Quantity: req.Quantity, GrantMoney: GetChargedAmount(float64(req.Quantity), *user), UnitMinor: quote.UnitMinor, Currency: quote.Currency, PriceID: quote.Price, Customer: customer, SetupRef: ref}
	params := &stripe.CheckoutSessionParams{ClientReferenceID: stripe.String(ref), Mode: stripe.String("setup"), Currency: stripe.String(strings.ToLower(quote.Currency)), Customer: stripe.String(customer),
		SuccessURL: stripe.String(system_setting.ServerAddress + "/console/topup"), CancelURL: stripe.String(system_setting.ServerAddress + "/console/topup"),
		SetupIntentData: &stripe.CheckoutSessionSetupIntentDataParams{Metadata: map[string]string{"wallet_auto_topup": ref, "threshold_quota": strconv.FormatInt(req.Threshold, 10), "charge_minor": strconv.FormatInt(quote.UnitMinor*req.Quantity, 10), "currency": quote.Currency}}}
	params.Context = ctx
	session, err := api.CheckoutSessions.New(params)
	if err != nil {
		common.ApiErrorMsg(c, "无法打开 Stripe 授权页面")
		return
	}
	if err = model.BeginWalletAutoTopupSetup(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"url": session.URL})
}

func DisableWalletAutoTopup(c *gin.Context) {
	if err := model.DisableWalletAutoTopup(c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func autoTopupSetupSucceeded(event stripe.Event) error {
	if event.Data == nil {
		return errors.New("missing setup intent")
	}
	var intent stripe.SetupIntent
	if err := common.Unmarshal(event.Data.Raw, &intent); err != nil {
		return err
	}
	return applyAutoTopupAuthorization(&intent)
}

func applyAutoTopupAuthorization(intent *stripe.SetupIntent) error {
	ref := intent.Metadata["wallet_auto_topup"]
	if !strings.HasPrefix(ref, "autoset_") {
		return nil
	}
	if intent.Status != stripe.SetupIntentStatusSucceeded || intent.Usage != stripe.SetupIntentUsageOffSession || intent.Customer == nil || intent.PaymentMethod == nil {
		return errors.New("incomplete Stripe off-session authorization")
	}
	return model.ActivateWalletAutoTopup(ref, intent.Customer.ID, intent.PaymentMethod.ID)
}

func autoTopupCheckout(ctx context.Context, event stripe.Event) error {
	ref := event.GetObjectValue("client_reference_id")
	if event.Type == stripe.EventTypeCheckoutSessionExpired {
		return model.DB.Model(&model.WalletAutoTopup{}).Where("setup_ref = ? AND state = ?", ref, model.AutoTopupSetup).
			Update("state", model.AutoTopupDisabled).Error
	}
	if event.GetObjectValue("mode") != "setup" || event.GetObjectValue("status") != "complete" {
		return nil
	}
	id := stripeCheckoutCustomerID(event.Data.Object["setup_intent"])
	if id == "" {
		return errors.New("missing auto top-up setup intent")
	}
	params := &stripe.SetupIntentParams{}
	params.Context = ctx
	intent, err := stripeclient.New(setting.StripeApiSecret, nil).SetupIntents.Get(id, params)
	if err != nil {
		return err
	}
	if intent.Metadata["wallet_auto_topup"] != ref {
		return errors.New("Stripe setup reference mismatch")
	}
	return applyAutoTopupAuthorization(intent)
}

func autoTopupPaymentSucceeded(event stripe.Event) error {
	if event.Data == nil {
		return errors.New("missing payment intent")
	}
	var pi stripe.PaymentIntent
	if err := common.Unmarshal(event.Data.Raw, &pi); err != nil {
		return err
	}
	trade := pi.Metadata["wallet_auto_topup"]
	if !strings.HasPrefix(trade, "auto_") {
		return nil
	}
	var rule model.WalletAutoTopup
	err := model.DB.Where("trade = ?", trade).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} // a completed order is already credited
	if err != nil {
		return err
	}
	return settleAutoTopup(rule, &pi)
}

func settleAutoTopup(rule model.WalletAutoTopup, pi *stripe.PaymentIntent) error {
	if pi.ID != rule.Intent || pi.Status != stripe.PaymentIntentStatusSucceeded || pi.Customer == nil || pi.Customer.ID != rule.Customer || pi.PaymentMethod == nil || pi.PaymentMethod.ID != rule.Method ||
		pi.Metadata["wallet_auto_topup"] != rule.Trade || pi.AmountReceived != rule.UnitMinor*rule.Quantity || !strings.EqualFold(string(pi.Currency), rule.Currency) {
		return errors.New("Stripe 自动充值扣款与授权订单不符")
	}
	details := model.TopUpPaymentDetails{PaidMoney: rule.PaidMoney(), PaidCurrency: rule.Currency}
	_, err := service.ValidateTopUpCallback(service.PaymentCallbackValidationInput{TradeNo: rule.Trade, PaymentMethod: PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe, ProviderAmount: details.PaidMoney, Currency: rule.Currency, Source: "stripe_wallet_auto_topup", ProviderPayload: pi.ID})
	if err != nil {
		return err
	}
	return model.RechargeWithPaymentDetails(rule.Trade, "", "", details)
}

func processWalletAutoTopup(ctx context.Context, userID int) error {
	price := ""
	if isStripeTopUpEnabled() {
		price = setting.StripePriceId
	}
	rule, err := model.ClaimWalletAutoTopup(userID, price, int64(max(setting.StripeMinTopUp, 1)))
	if err != nil || rule.Trade == "" {
		return err
	}
	api := stripeclient.New(setting.StripeApiSecret, nil)
	if rule.Intent == "" {
		params := &stripe.PaymentIntentParams{Amount: stripe.Int64(rule.UnitMinor * rule.Quantity), Currency: stripe.String(strings.ToLower(rule.Currency)), Customer: stripe.String(rule.Customer), PaymentMethod: stripe.String(rule.Method),
			Metadata: map[string]string{"wallet_auto_topup": rule.Trade}, AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{Enabled: stripe.Bool(true), AllowRedirects: stripe.String("never")}}
		params.Context = ctx
		params.SetIdempotencyKey(rule.Trade + "_create")
		pi, err := api.PaymentIntents.New(params)
		if err != nil {
			// An invalid or detached method cannot have been charged by this unconfirmed creation.
			var se *stripe.Error
			if errors.As(err, &se) && (se.HTTPStatusCode == http.StatusBadRequest || se.HTTPStatusCode == http.StatusPaymentRequired) {
				return model.FailWalletAutoTopup(rule)
			}
			return err
		}
		res := model.DB.Model(&model.WalletAutoTopup{}).Where("user_id = ? AND trade = ? AND intent = ?", userID, rule.Trade, "").Update("intent", pi.ID)
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		rule.Intent = pi.ID
	}
	get := &stripe.PaymentIntentParams{}
	get.Context = ctx
	pi, err := api.PaymentIntents.Get(rule.Intent, get)
	if err != nil {
		return err
	}
	if pi.Status == stripe.PaymentIntentStatusRequiresConfirmation {
		p := &stripe.PaymentIntentConfirmParams{OffSession: stripe.Bool(true)}
		p.Context = ctx
		p.SetIdempotencyKey(rule.Trade + "_confirm")
		pi, err = api.PaymentIntents.Confirm(rule.Intent, p)
		if err != nil {
			var se *stripe.Error
			if !errors.As(err, &se) || (se.HTTPStatusCode != http.StatusBadRequest && se.HTTPStatusCode != http.StatusPaymentRequired) {
				return err
			}
			// A declined or authentication-required charge needs fresh consent.
			pi, err = api.PaymentIntents.Get(rule.Intent, get)
			if err != nil {
				return err
			}
			if pi.Status == stripe.PaymentIntentStatusRequiresConfirmation {
				cancel := &stripe.PaymentIntentCancelParams{}
				cancel.Context = ctx
				if _, err = api.PaymentIntents.Cancel(rule.Intent, cancel); err != nil {
					return err
				}
				return model.FailWalletAutoTopup(rule)
			}
		}
	}
	switch pi.Status {
	case stripe.PaymentIntentStatusSucceeded:
		return settleAutoTopup(rule, pi)
	case stripe.PaymentIntentStatusRequiresAction, stripe.PaymentIntentStatusRequiresPaymentMethod:
		p := &stripe.PaymentIntentCancelParams{}
		p.Context = ctx
		if _, err := api.PaymentIntents.Cancel(rule.Intent, p); err != nil {
			return err
		}
		return model.FailWalletAutoTopup(rule)
	case stripe.PaymentIntentStatusCanceled:
		return model.FailWalletAutoTopup(rule)
	default:
		return nil // processing, including delayed bank debits
	}
}

func StartWalletAutoTopupTask() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for ; ; <-ticker.C {
			if !isStripeWebhookEnabled() {
				continue
			}
			after := 0
			for {
				ids, err := model.WalletAutoTopupCandidates(after, 100)
				if err != nil {
					log.Printf("自动充值扫描失败: %v", err)
					break
				}
				for _, id := range ids {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					err := processWalletAutoTopup(ctx, id)
					cancel()
					if err != nil {
						var se *stripe.Error
						if errors.As(err, &se) {
							log.Printf("自动充值 Stripe 请求失败 user=%d status=%d code=%s", id, se.HTTPStatusCode, se.Code)
						} else {
							log.Printf("自动充值处理失败 user=%d: %v", id, err)
						}
					}
					after = id
				}
				if len(ids) < 100 {
					break
				}
			}
		}
	}()
}
