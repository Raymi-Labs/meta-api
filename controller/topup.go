package controller

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	enableStripe := isStripeTopUpEnabled()
	payMethods := stripeTopUpPayMethods(operation_setting.PayMethods, enableStripe,
		setting.StripeDisplayName, setting.StripeDisplayIcon, setting.StripeMinTopUp)
	common.OptionMapRWMutex.RUnlock()

	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}
		if !hasWaffo {
			payMethods = append(payMethods, map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      model.PaymentMethodWaffo,
				"color":     "rgba(var(--semi-blue-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoMinTopUp),
			})
		}
	}

	enableWaffoPancake := isWaffoPancakeTopUpEnabled()

	data := gin.H{
		"enable_online_topup":        isEpayTopUpEnabled(),
		"enable_stripe_topup":        enableStripe,
		"enable_creem_topup":         isCreemTopUpEnabled(),
		"enable_waffo_topup":         enableWaffo,
		"enable_waffo_pancake_topup": enableWaffoPancake,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products":          setting.CreemProducts,
		"pay_methods":             payMethods,
		"min_topup":               operation_setting.MinTopUp,
		"stripe_min_topup":        setting.StripeMinTopUp,
		"waffo_min_topup":         setting.WaffoMinTopUp,
		"waffo_pancake_min_topup": setting.WaffoPancakeMinTopUp,
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                operation_setting.GetPaymentSetting().AmountDiscount,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type AmountRequest struct {
	Amount int64 `json:"amount"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPayMoney(amount int64, group string) float64 {
	return service.CalculateEpayTopUpPayMoney(amount, group)
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "invalid parameters"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("top-up amount must be at least %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "failed to get user group"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "top-up amount is too low"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(200, gin.H{"message": "error", "data": "payment method does not exist"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, err := url.Parse(callBackAddress + "/api/user/epay/return")
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "callback address configuration error"})
		return
	}
	notifyUrl, err := url.Parse(callBackAddress + "/api/user/epay/notify")
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "callback address configuration error"})
		return
	}
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(200, gin.H{"message": "error", "data": "payment config is not set"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "failed to start payment"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:              id,
		Amount:              amount,
		Money:               payMoney,
		PaidMoney:           payMoney,
		PaidCurrency:        "CNY",
		PresentmentMoney:    payMoney,
		PresentmentCurrency: "CNY",
		TradeNo:             tradeNo,
		PaymentMethod:       req.PaymentMethod,
		PaymentProvider:     model.PaymentProviderEpay,
		CreateTime:          time.Now().Unix(),
		Status:              common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "failed to create order"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// LockOrder tries to lock a trade number in-process.
func LockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if !ok {
		createLock.Lock()
		defer createLock.Unlock()
		lock, ok = orderLocks.Load(tradeNo)
		if !ok {
			lock = new(sync.Mutex)
			orderLocks.Store(tradeNo, lock)
		}
	}
	lock.(*sync.Mutex).Lock()
}

// UnlockOrder releases an in-process lock for a trade number.
func UnlockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if ok {
		lock.(*sync.Mutex).Unlock()
	}
}

func parseEpayCallbackParams(c *gin.Context) (map[string]string, error) {
	if c.Request.Method == http.MethodPost {
		if err := c.Request.ParseForm(); err != nil {
			return nil, err
		}
		params := lo.Reduce(lo.Keys(c.Request.PostForm), func(result map[string]string, key string, index int) map[string]string {
			result[key] = c.Request.PostForm.Get(key)
			return result
		}, map[string]string{})
		if len(params) == 0 {
			return nil, fmt.Errorf("empty callback params")
		}
		return params, nil
	}

	params := lo.Reduce(lo.Keys(c.Request.URL.Query()), func(result map[string]string, key string, index int) map[string]string {
		result[key] = c.Request.URL.Query().Get(key)
		return result
	}, map[string]string{})
	if len(params) == 0 {
		return nil, fmt.Errorf("empty callback params")
	}
	return params, nil
}

func EpayNotify(c *gin.Context) {
	params, err := parseEpayCallbackParams(c)
	if err != nil {
		log.Println("epay notify parse failed:", err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	client := GetEpayClient()
	if client == nil {
		log.Println("epay notify failed: payment config missing")
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		log.Println("epay notify signature verification failed")
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		log.Printf("epay notify unexpected trade status: %v", verifyInfo)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	payAmount, _ := strconv.ParseFloat(verifyInfo.Money, 64)
	LockOrder(verifyInfo.ServiceTradeNo)
	defer UnlockOrder(verifyInfo.ServiceTradeNo)
	checkResult, err := service.ValidateTopUpCallback(service.PaymentCallbackValidationInput{
		TradeNo:               verifyInfo.ServiceTradeNo,
		PaymentMethod:         verifyInfo.Type,
		PaymentProvider:       model.PaymentProviderEpay,
		ProviderAmount:        payAmount,
		Source:                "epay_notify",
		ProviderPayload:       common.GetJsonString(verifyInfo),
		ProviderPaymentMethod: verifyInfo.Type,
	})
	if err != nil {
		log.Printf("epay notify validation failed: trade_no=%s err=%s", verifyInfo.ServiceTradeNo, err.Error())
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if checkResult.AlreadyCompleted {
		_, _ = c.Writer.Write([]byte("success"))
		return
	}
	if err := model.CompleteTopUpByTradeNoWithPayment(verifyInfo.ServiceTradeNo, "epay", c.ClientIP(), verifyInfo.Type, model.PaymentProviderEpay, nil); err != nil {
		log.Printf("epay notify complete order failed: trade_no=%s err=%s", verifyInfo.ServiceTradeNo, err.Error())
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))
}

func EpayReturn(c *gin.Context) {
	failURL := system_setting.ServerAddress + "/console/topup?pay=fail"
	successURL := system_setting.ServerAddress + "/console/topup?pay=success"
	pendingURL := system_setting.ServerAddress + "/console/topup?pay=pending"

	params, err := parseEpayCallbackParams(c)
	if err != nil {
		c.Redirect(http.StatusFound, failURL)
		return
	}

	client := GetEpayClient()
	if client == nil {
		c.Redirect(http.StatusFound, failURL)
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		c.Redirect(http.StatusFound, failURL)
		return
	}
	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		payAmount, _ := strconv.ParseFloat(verifyInfo.Money, 64)
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		checkResult, err := service.ValidateTopUpCallback(service.PaymentCallbackValidationInput{
			TradeNo:               verifyInfo.ServiceTradeNo,
			PaymentMethod:         verifyInfo.Type,
			PaymentProvider:       model.PaymentProviderEpay,
			ProviderAmount:        payAmount,
			Source:                "epay_return",
			ProviderPayload:       common.GetJsonString(verifyInfo),
			ProviderPaymentMethod: verifyInfo.Type,
		})
		if err != nil {
			log.Printf("epay return validation failed: trade_no=%s err=%s", verifyInfo.ServiceTradeNo, err.Error())
			c.Redirect(http.StatusFound, failURL)
			return
		}
		if checkResult.AlreadyCompleted {
			c.Redirect(http.StatusFound, successURL)
			return
		}
		if err := model.CompleteTopUpByTradeNoWithPayment(verifyInfo.ServiceTradeNo, "epay_return", c.ClientIP(), verifyInfo.Type, model.PaymentProviderEpay, nil); err != nil {
			log.Printf("epay return complete order failed: trade_no=%s err=%s", verifyInfo.ServiceTradeNo, err.Error())
			c.Redirect(http.StatusFound, failURL)
			return
		}
		c.Redirect(http.StatusFound, successURL)
		return
	}
	c.Redirect(http.StatusFound, pendingURL)
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	writePaymentQuote(c, payMoney, "CNY")
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	params := model.TopUpSearchParams{
		Keyword:       c.Query("keyword"),
		Status:        c.Query("status"),
		PaymentMethod: c.Query("payment_method"),
	}

	topups, total, err := model.GetUserTopUpsByParams(userId, params, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps returns all platform top-up records for admins.
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	params := model.TopUpSearchParams{
		Keyword:       c.Query("keyword"),
		Username:      c.Query("username"),
		Status:        c.Query("status"),
		PaymentMethod: c.Query("payment_method"),
	}

	topups, total, err := model.GetAllTopUpsByParams(params, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	adminExtras := map[string]interface{}{
		"admin_id":       c.GetInt("id"),
		"admin_username": c.GetString("username"),
	}
	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP(), adminExtras); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
