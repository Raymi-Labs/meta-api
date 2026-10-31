package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TopUp struct {
	Id     int   `json:"id"`
	UserId int   `json:"user_id" gorm:"index"`
	Amount int64 `json:"amount"`
	// Money is the benefit/grant base and is not necessarily the cash paid.
	Money float64 `json:"money"`
	// PaidMoney is the provider settlement amount; PresentmentMoney is what the customer paid.
	PaidMoney           float64 `json:"paid_money" gorm:"type:decimal(20,6);not null;default:0"`
	PaidCurrency        string  `json:"paid_currency" gorm:"type:varchar(16);not null;default:''"`
	PresentmentMoney    float64 `json:"presentment_money" gorm:"type:decimal(20,6);not null;default:0"`
	PresentmentCurrency string  `json:"presentment_currency" gorm:"type:varchar(16);not null;default:''"`
	TradeNo             string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod       string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider     string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime          int64   `json:"create_time"`
	CompleteTime        int64   `json:"complete_time"`
	Status              string  `json:"status"`
	Username            string  `json:"username,omitempty" gorm:"column:username;->"`
	DisplayName         string  `json:"display_name,omitempty" gorm:"column:display_name;->"`
}

type TopUpPaymentDetails struct {
	PaidMoney           float64
	PaidCurrency        string
	PresentmentMoney    float64
	PresentmentCurrency string
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("top-up not found")
	ErrTopUpStatusInvalid    = errors.New("top-up status invalid")
)

type TopUpSearchParams struct {
	Keyword        string
	Username       string
	Status         string
	PaymentMethod  string
	StartTimestamp int64
	EndTimestamp   int64
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	topUp, err := GetTopUpByTradeNoWithError(tradeNo)
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNoWithError(tradeNo string) (*TopUp, error) {
	if strings.TrimSpace(tradeNo) == "" {
		return nil, ErrTopUpNotFound
	}
	topUp := &TopUp{}
	if err := DB.Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopUpNotFound
		}
		return nil, err
	}
	return topUp, nil
}

func NormalizePaymentProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func NormalizePaymentMethod(method string) string {
	return strings.ToLower(strings.TrimSpace(method))
}

func NormalizePaymentCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "RMB" {
		return "CNY"
	}
	if len(currency) != 3 {
		return ""
	}
	for _, char := range currency {
		if char < 'A' || char > 'Z' {
			return ""
		}
	}
	return currency
}

func validPaymentMoney(money float64) bool {
	return money > 0 && !math.IsNaN(money) && !math.IsInf(money, 0)
}

// SettlementPaymentAmount returns the provider settlement amount used for callback reconciliation.
func (topUp *TopUp) SettlementPaymentAmount() (float64, string, bool) {
	if topUp == nil {
		return 0, "", false
	}
	if validPaymentMoney(topUp.PaidMoney) {
		currency := NormalizePaymentCurrency(topUp.PaidCurrency)
		if currency == "" {
			if InferPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod) == PaymentProviderStripe {
				return 0, "", false
			}
			currency = "CNY"
		}
		return topUp.PaidMoney, currency, true
	}
	if InferPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod) != PaymentProviderStripe && validPaymentMoney(topUp.Money) {
		return topUp.Money, "CNY", true
	}
	return 0, "", false
}

// EffectivePaymentAmount returns the customer-facing amount without confusing it with granted quota.
func (topUp *TopUp) EffectivePaymentAmount() (float64, string, bool) {
	if topUp == nil {
		return 0, "", false
	}
	if currency := NormalizePaymentCurrency(topUp.PresentmentCurrency); validPaymentMoney(topUp.PresentmentMoney) && currency != "" {
		return topUp.PresentmentMoney, currency, true
	}
	return topUp.SettlementPaymentAmount()
}

// CNYPaymentAmount only returns amounts that are safe to use for invoices and CNY-denominated commissions.
func (topUp *TopUp) CNYPaymentAmount() (float64, bool) {
	if topUp == nil {
		return 0, false
	}
	if validPaymentMoney(topUp.PresentmentMoney) {
		if NormalizePaymentCurrency(topUp.PresentmentCurrency) == "CNY" {
			return topUp.PresentmentMoney, true
		}
		return 0, false
	}
	if validPaymentMoney(topUp.PaidMoney) {
		currency := NormalizePaymentCurrency(topUp.PaidCurrency)
		if currency == "CNY" {
			return topUp.PaidMoney, true
		}
		if currency != "" || InferPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod) == PaymentProviderStripe {
			return 0, false
		}
		return topUp.PaidMoney, true
	}
	if InferPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod) != PaymentProviderStripe && validPaymentMoney(topUp.Money) {
		return topUp.Money, true
	}
	return 0, false
}

func applyTopUpPaymentDetails(topUp *TopUp, details TopUpPaymentDetails) {
	if topUp == nil {
		return
	}
	if validPaymentMoney(details.PaidMoney) {
		topUp.PaidMoney = details.PaidMoney
		if currency := NormalizePaymentCurrency(details.PaidCurrency); currency != "" {
			topUp.PaidCurrency = currency
		}
	}
	if validPaymentMoney(details.PresentmentMoney) {
		if currency := NormalizePaymentCurrency(details.PresentmentCurrency); currency != "" {
			topUp.PresentmentMoney = details.PresentmentMoney
			topUp.PresentmentCurrency = currency
		}
	}
}

// InferPaymentProvider keeps legacy pending orders usable after adding payment_provider.
func InferPaymentProvider(paymentProvider string, paymentMethod string) string {
	if provider := NormalizePaymentProvider(paymentProvider); provider != "" {
		return provider
	}
	switch NormalizePaymentMethod(paymentMethod) {
	case PaymentMethodStripe:
		return PaymentProviderStripe
	case PaymentMethodCreem:
		return PaymentProviderCreem
	case PaymentMethodWaffo:
		return PaymentProviderWaffo
	case PaymentMethodWaffoPancake:
		return PaymentProviderWaffoPancake
	case "":
		return ""
	default:
		return PaymentProviderEpay
	}
}

func PaymentProviderMatches(orderProvider string, orderMethod string, callbackProvider string, callbackMethod string) bool {
	expected := InferPaymentProvider(orderProvider, orderMethod)
	actual := InferPaymentProvider(callbackProvider, callbackMethod)
	if expected == "" || actual == "" {
		return true
	}
	return expected == actual
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		query := tx.Where(refCol+" = ?", tradeNo)
		if !common.UsingSQLite {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && !PaymentProviderMatches(topUp.PaymentProvider, topUp.PaymentMethod, expectedPaymentProvider, "") {
			return ErrPaymentMethodMismatch
		}
		if topUp.PaymentProvider == "" && expectedPaymentProvider != "" {
			topUp.PaymentProvider = expectedPaymentProvider
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func Recharge(referenceId string, customerId string, callerIp string, paidMoney float64) (err error) {
	return RechargeWithPaymentDetails(referenceId, customerId, callerIp, TopUpPaymentDetails{PaidMoney: paidMoney})
}

func RechargeWithPaymentDetails(referenceId string, customerId string, callerIp string, details TopUpPaymentDetails) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota float64
	topUp := &TopUp{}
	alreadyCompleted := false

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Where(refCol+" = ?", referenceId)
		if !common.UsingSQLite {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		err := query.First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if !PaymentProviderMatches(topUp.PaymentProvider, topUp.PaymentMethod, PaymentProviderStripe, PaymentMethodStripe) {
			return ErrPaymentMethodMismatch
		}

		applyTopUpPaymentDetails(topUp, details)
		if topUp.Status == common.TopUpStatusSuccess {
			alreadyCompleted = true
			return tx.Save(topUp).Error
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		if topUp.PaymentProvider == "" {
			topUp.PaymentProvider = PaymentProviderStripe
		}
		if topUp.PaymentMethod == "" {
			topUp.PaymentMethod = PaymentMethodStripe
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota = topUp.Money * common.QuotaPerUnit
		if customerId != "" {
			err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("stripe_customer", customerId).Error
			if err != nil {
				return err
			}
		}
		if err := GrantUserQuotaTx(tx, topUp.UserId, int(quota), int(quota)); err != nil {
			return err
		}
		if err := recordQuotaBenefitGrantTx(tx, topUp, int(quota), "stripe"); err != nil {
			return err
		}

		return EnqueueInviteCommissionFromTopUpTx(tx, topUp)
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	NotifyInviteCommissionSettlement()
	if alreadyCompleted {
		return nil
	}
	if cacheErr := cacheIncrUserQuota(topUp.UserId, int64(quota)); cacheErr != nil {
		common.SysLog("failed to update user quota cache after stripe top-up: " + cacheErr.Error())
	}

	RecordTopupLog(topUp.UserId,
		fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%s", logger.FormatQuota(int(quota)), formatTopUpPaymentAmount(topUp)),
		callerIp, topUp.PaymentMethod, "stripe", nil)

	return nil
}

func formatTopUpPaymentAmount(topUp *TopUp) string {
	money, currency, known := topUp.EffectivePaymentAmount()
	if !known {
		return "未知"
	}
	return fmt.Sprintf("%.2f %s", money, currency)
}

func topUpBaseQuery(tx *gorm.DB, includeUser bool) *gorm.DB {
	if includeUser {
		return tx.Table("top_ups").
			Select("top_ups.*, users.username AS username, users.display_name AS display_name").
			Joins("LEFT JOIN users ON users.id = top_ups.user_id")
	}
	return tx.Model(&TopUp{})
}

// searchUserTopUpCountHardLimit 普通用户 COUNT 的安全上限，
// 防止对表执行无界 COUNT 触发 DoS。
const searchUserTopUpCountHardLimit = 500

func applyTopUpSearch(query *gorm.DB, params TopUpSearchParams, includeUsername bool) (*gorm.DB, error) {
	if params.Keyword != "" {
		pattern, err := sanitizeContainsLikePattern(params.Keyword)
		if err != nil {
			return nil, err
		}
		if pattern != "" {
			query = query.Where("top_ups.trade_no LIKE ? ESCAPE '!'", pattern)
		}
	}
	if params.Status != "" {
		query = query.Where("top_ups.status = ?", params.Status)
	}
	if params.PaymentMethod != "" {
		query = query.Where("top_ups.payment_method = ?", params.PaymentMethod)
	}
	if includeUsername && params.Username != "" {
		query = applyPaymentRecordUsernameFilter(query, params.Username, "users.id", "users.username")
	}
	effectiveTimestampExpr := buildPaymentRecordEffectiveTimestampExpr("top_ups.status", "top_ups.create_time", "top_ups.complete_time")
	query = applyPaymentRecordTimeRange(query, params.StartTimestamp, params.EndTimestamp, effectiveTimestampExpr)
	return query, nil
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	return GetUserTopUpsByParams(userId, TopUpSearchParams{}, pageInfo)
}

func GetUserTopUpsByParams(userId int, params TopUpSearchParams, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	countQuery, err := applyTopUpSearch(tx.Model(&TopUp{}).Where("user_id = ?", userId), params, false)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = countQuery.Limit(searchUserTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	dataQuery, err := applyTopUpSearch(tx.Model(&TopUp{}).Where("user_id = ?", userId), params, false)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = dataQuery.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps returns all platform top-up records for admins.
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	return GetAllTopUpsByParams(TopUpSearchParams{}, pageInfo)
}

func GetAllTopUpsByParams(params TopUpSearchParams, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	countQuery, err := applyTopUpSearch(tx.Table("top_ups").Joins("LEFT JOIN users ON users.id = top_ups.user_id"), params, true)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = countQuery.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	dataQuery, err := applyTopUpSearch(topUpBaseQuery(tx, true), params, true)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = dataQuery.Order("top_ups.id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// SearchUserTopUps searches a user's top-up records by trade number.
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	return GetUserTopUpsByParams(userId, TopUpSearchParams{Keyword: keyword}, pageInfo)
}

// SearchAllTopUps searches platform top-up records by trade number.
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	return GetAllTopUpsByParams(TopUpSearchParams{Keyword: keyword}, pageInfo)
}

func CalculateGrantedQuotaForTopUp(topUp *TopUp) (int, error) {
	if topUp == nil {
		return 0, errors.New("top-up is nil")
	}
	if InferPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod) == PaymentProviderStripe {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd := int(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return 0, errors.New("invalid top-up quota")
		}
		return quotaToAdd, nil
	}

	dAmount := decimal.NewFromInt(topUp.Amount)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())
	if quotaToAdd <= 0 {
		return 0, errors.New("invalid top-up quota")
	}
	return quotaToAdd, nil
}

func recordQuotaBenefitGrantTx(tx *gorm.DB, topUp *TopUp, quotaDelta int, context string) error {
	if tx == nil || topUp == nil || quotaDelta <= 0 {
		return nil
	}
	if err := createBenefitChangeRecordTx(tx, &BenefitChangeRecord{
		BenefitType: BenefitTypeQuota,
		Action:      BenefitActionGrant,
		SourceType:  BenefitSourceTopUpOrder,
		SourceRef:   topUp.TradeNo,
		UserId:      topUp.UserId,
		TargetType:  BenefitTargetUserQuota,
		TargetId:    topUp.UserId,
		Detail: marshalBenefitDetail(&QuotaBenefitDetail{
			QuotaDelta:    quotaDelta,
			PaymentMethod: topUp.PaymentMethod,
			Context:       context,
		}),
	}); err != nil {
		return err
	}
	return creditOnlinePaidFundingTx(tx, topUp, quotaDelta, context)
}

func CompleteTopUpByTradeNo(tradeNo string, source string, callerIp string, adminExtras map[string]interface{}) error {
	return CompleteTopUpByTradeNoWithPayment(tradeNo, source, callerIp, "", "", adminExtras)
}

func CompleteTopUpByTradeNoWithPayment(tradeNo string, source string, callerIp string, actualPaymentMethod string, paymentProvider string, adminExtras map[string]interface{}) error {
	if tradeNo == "" {
		return errors.New("trade number is required")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string
	var alreadyCompleted bool

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		query := tx.Where(refCol+" = ?", tradeNo)
		if !common.UsingSQLite {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(topUp).Error; err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyCompleted = true
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("top-up order is not pending")
		}

		actualPaymentMethod = NormalizePaymentMethod(actualPaymentMethod)
		paymentProvider = InferPaymentProvider(paymentProvider, actualPaymentMethod)
		if paymentProvider != "" && !PaymentProviderMatches(topUp.PaymentProvider, topUp.PaymentMethod, paymentProvider, actualPaymentMethod) {
			return ErrPaymentMethodMismatch
		}
		if topUp.PaymentProvider == "" {
			topUp.PaymentProvider = InferPaymentProvider(paymentProvider, topUp.PaymentMethod)
		}
		if topUp.PaymentMethod == "" && actualPaymentMethod != "" {
			topUp.PaymentMethod = actualPaymentMethod
		}

		calculatedQuota, calcErr := CalculateGrantedQuotaForTopUp(topUp)
		if calcErr != nil {
			return calcErr
		}
		quotaToAdd = calculatedQuota

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := GrantUserQuotaTx(tx, topUp.UserId, quotaToAdd, quotaToAdd); err != nil {
			return err
		}
		if err := recordQuotaBenefitGrantTx(tx, topUp, quotaToAdd, source); err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return EnqueueInviteCommissionFromTopUpTx(tx, topUp)
	})

	if err != nil {
		return err
	}
	NotifyInviteCommissionSettlement()
	if alreadyCompleted {
		return nil
	}
	if cacheErr := cacheIncrUserQuota(userId, int64(quotaToAdd)); cacheErr != nil {
		common.SysLog("failed to update user quota cache after top-up: " + cacheErr.Error())
	}

	logMessage := fmt.Sprintf("online top-up succeeded, quota: %v, paid amount: %f", logger.FormatQuota(quotaToAdd), payMoney)
	if source == "manual" {
		logMessage = fmt.Sprintf("manual top-up completion succeeded, quota: %v, paid amount: %f", logger.FormatQuota(quotaToAdd), payMoney)
	}
	RecordTopupLog(userId, logMessage, callerIp, paymentMethod, source, adminExtras)

	return nil
}

// ManualCompleteTopUp completes a pending top-up order manually.
func ManualCompleteTopUp(tradeNo string, callerIp string, adminExtras map[string]interface{}) error {
	return CompleteTopUpByTradeNo(tradeNo, "manual", callerIp, adminExtras)
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {

	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}
	alreadyCompleted := false

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Where(refCol+" = ?", referenceId)
		if !common.UsingSQLite {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		err := query.First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if !PaymentProviderMatches(topUp.PaymentProvider, topUp.PaymentMethod, PaymentProviderCreem, PaymentMethodCreem) {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyCompleted = true
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		if topUp.PaymentProvider == "" {
			topUp.PaymentProvider = PaymentProviderCreem
		}
		if topUp.PaymentMethod == "" {
			topUp.PaymentMethod = PaymentMethodCreem
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota = topUp.Amount

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		if len(updateFields) > 0 {
			err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields).Error
			if err != nil {
				return err
			}
		}
		if err := GrantUserQuotaTx(tx, topUp.UserId, int(quota), int(quota)); err != nil {
			return err
		}
		if err := recordQuotaBenefitGrantTx(tx, topUp, int(quota), "creem"); err != nil {
			return err
		}

		return EnqueueInviteCommissionFromTopUpTx(tx, topUp)
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	NotifyInviteCommissionSettlement()
	if alreadyCompleted {
		return nil
	}
	if cacheErr := cacheIncrUserQuota(topUp.UserId, quota); cacheErr != nil {
		common.SysLog("failed to update user quota cache after creem top-up: " + cacheErr.Error())
	}

	RecordTopupLog(topUp.UserId,
		fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money),
		callerIp, topUp.PaymentMethod, "creem", nil)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) error {
	return CompleteTopUpByTradeNoWithPayment(tradeNo, PaymentProviderWaffo, callerIp, PaymentMethodWaffo, PaymentProviderWaffo, nil)
}

func RechargeWaffoPancake(tradeNo string) error {
	return CompleteTopUpByTradeNoWithPayment(tradeNo, PaymentProviderWaffoPancake, "", PaymentMethodWaffoPancake, PaymentProviderWaffoPancake, nil)
}
