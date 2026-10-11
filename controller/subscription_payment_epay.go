package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type SubscriptionEpayPayRequest struct {
	PlanId           int    `json:"plan_id"`
	PaymentMethod    string `json:"payment_method"`
	PurchaseMode     string `json:"purchase_mode"`
	PurchaseQuantity int    `json:"purchase_quantity"`
	// RenewTargetSubscriptionId 仅在 purchase_mode=renew 时生效。
	RenewTargetSubscriptionId int `json:"renew_target_subscription_id"`
}

func SubscriptionRequestEpay(c *gin.Context) {
	var req SubscriptionEpayPayRequest
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
	if plan.PriceAmount < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}

	userId := c.GetInt("id")
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

	callBackAddress := service.GetCallbackAddress()
	returnUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/return")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}
	notifyUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/notify")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}

	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("SUBUSR%dNO%s", userId, tradeNo)

	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置支付信息")
		return
	}

	totalMoney := plan.PriceAmount * float64(purchaseQuantity)

	order := &model.SubscriptionOrder{
		UserId:                    userId,
		PlanId:                    plan.Id,
		Money:                     totalMoney,
		PurchaseQuantity:          purchaseQuantity,
		TradeNo:                   tradeNo,
		PaymentMethod:             req.PaymentMethod,
		PaymentProvider:           model.PaymentProviderEpay,
		PurchaseMode:              purchaseMode,
		RenewTargetSubscriptionId: renewTargetSubId,
		CreateTime:                time.Now().Unix(),
		Status:                    common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		// Epay signs raw values joined with '&'; normalize only the display name.
		Name:      fmt.Sprintf("SUB:%s x%d", strings.ReplaceAll(plan.Title, "&", "＆"), purchaseQuantity),
		Money:     strconv.FormatFloat(totalMoney, 'f', 2, 64),
		Device:    epay.PC,
		NotifyUrl: notifyUrl,
		ReturnUrl: returnUrl,
	})
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func SubscriptionEpayNotify(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}

	if len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	client := GetEpayClient()
	if client == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	payAmount, _ := strconv.ParseFloat(verifyInfo.Money, 64)
	LockOrder(verifyInfo.ServiceTradeNo)
	defer UnlockOrder(verifyInfo.ServiceTradeNo)
	validationInput := service.PaymentCallbackValidationInput{
		TradeNo:               verifyInfo.ServiceTradeNo,
		PaymentMethod:         verifyInfo.Type,
		PaymentProvider:       model.PaymentProviderEpay,
		ProviderAmount:        payAmount,
		Source:                "subscription_epay_notify",
		ProviderPayload:       common.GetJsonString(verifyInfo),
		ProviderPaymentMethod: verifyInfo.Type,
	}
	checkResult, err := service.ValidateSubscriptionCallback(validationInput)
	if err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if checkResult.AlreadyCompleted {
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	if err := model.CompleteSubscriptionOrderWithPayment(verifyInfo.ServiceTradeNo, common.GetJsonString(verifyInfo), verifyInfo.Type, model.PaymentProviderEpay); err != nil {
		service.RecordSubscriptionProcessingRiskCase(validationInput, err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))
}

// SubscriptionEpayReturn handles browser return after payment.
// It verifies the payload and completes the order, then redirects to console.
func SubscriptionEpayReturn(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=fail")
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}

	if len(params) == 0 {
		c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=fail")
		return
	}

	client := GetEpayClient()
	if client == nil {
		c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=fail")
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=fail")
		return
	}
	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		payAmount, _ := strconv.ParseFloat(verifyInfo.Money, 64)
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		validationInput := service.PaymentCallbackValidationInput{
			TradeNo:               verifyInfo.ServiceTradeNo,
			PaymentMethod:         verifyInfo.Type,
			PaymentProvider:       model.PaymentProviderEpay,
			ProviderAmount:        payAmount,
			Source:                "subscription_epay_return",
			ProviderPayload:       common.GetJsonString(verifyInfo),
			ProviderPaymentMethod: verifyInfo.Type,
		}
		checkResult, err := service.ValidateSubscriptionCallback(validationInput)
		if err != nil {
			c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=fail")
			return
		}
		if checkResult.AlreadyCompleted {
			c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=success")
			return
		}
		if err := model.CompleteSubscriptionOrderWithPayment(verifyInfo.ServiceTradeNo, common.GetJsonString(verifyInfo), verifyInfo.Type, model.PaymentProviderEpay); err != nil {
			service.RecordSubscriptionProcessingRiskCase(validationInput, err)
			c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=fail")
			return
		}
		c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=success")
		return
	}
	c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/subscription?pay=pending")
}
