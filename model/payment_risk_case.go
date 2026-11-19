package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	PaymentRiskRecordTypeTopUp        = "topup"
	PaymentRiskRecordTypeSubscription = "subscription"

	PaymentRiskStatusOpen      = "open"
	PaymentRiskStatusConfirmed = "confirmed"
	PaymentRiskStatusReversed  = "reversed"
	PaymentRiskStatusVoided    = "voided"

	PaymentRiskReasonManualReview          = "manual_review"
	PaymentRiskReasonOrderNotFound         = "order_not_found"
	PaymentRiskReasonOrderStatusInvalid    = "order_status_invalid"
	PaymentRiskReasonPaymentMethodMismatch = "payment_method_mismatch"
	PaymentRiskReasonAmountMismatch        = "amount_mismatch"
	PaymentRiskReasonUnsupportedOrderType  = "unsupported_order_type"

	PaymentRiskActionConfirm = "confirm"
	PaymentRiskActionReverse = "reverse"
	PaymentRiskActionVoid    = "void"
)

var (
	ErrPaymentRiskCaseNotFound  = errors.New("payment risk case not found")
	ErrPaymentRiskCaseResolved  = errors.New("payment risk case already resolved")
	ErrPaymentRiskActionInvalid = errors.New("invalid payment risk action")
)

type PaymentRiskCase struct {
	Id int `json:"id"`

	RecordType string `json:"record_type" gorm:"type:varchar(32);not null;uniqueIndex:idx_payment_risk_record_trade"`
	TradeNo    string `json:"trade_no" gorm:"type:varchar(255);not null;uniqueIndex:idx_payment_risk_record_trade"`
	UserId     int    `json:"user_id" gorm:"index"`

	PaymentMethod         string  `json:"payment_method" gorm:"type:varchar(50);default:''"`
	ProviderPaymentMethod string  `json:"provider_payment_method" gorm:"type:varchar(50);default:''"`
	ExpectedAmount        int64   `json:"expected_amount" gorm:"type:bigint;default:0"`
	ExpectedMoney         float64 `json:"expected_money" gorm:"type:decimal(12,6);default:0"`
	ReceivedMoney         float64 `json:"received_money" gorm:"type:decimal(12,6);default:0"`
	Currency              string  `json:"currency" gorm:"type:varchar(16);default:''"`

	Source          string `json:"source" gorm:"type:varchar(32);default:''"`
	Reason          string `json:"reason" gorm:"type:varchar(64);default:'';index"`
	OrderStatus     string `json:"order_status" gorm:"type:varchar(16);default:''"`
	ProviderPayload string `json:"provider_payload" gorm:"type:text"`
	Status          string `json:"status" gorm:"type:varchar(16);default:'open';index"`

	HandlerAdminId    int    `json:"handler_admin_id" gorm:"index;default:0"`
	HandlerNote       string `json:"handler_note" gorm:"type:text"`
	AppliedQuotaDelta int    `json:"applied_quota_delta" gorm:"default:0"`
	ResolvedAt        int64  `json:"resolved_at" gorm:"bigint;default:0;index"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint;index"`

	Username    string `json:"username,omitempty" gorm:"column:username;->"`
	DisplayName string `json:"display_name,omitempty" gorm:"column:display_name;->"`
}

type PaymentRiskCaseSearchParams struct {
	Keyword    string
	Username   string
	RecordType string
	Status     string
	Reason     string
}

type PaymentRiskCaseUpsertInput struct {
	RecordType            string
	TradeNo               string
	UserId                int
	PaymentMethod         string
	ProviderPaymentMethod string
	ExpectedAmount        int64
	ExpectedMoney         float64
	ReceivedMoney         float64
	Currency              string
	Source                string
	Reason                string
	OrderStatus           string
	ProviderPayload       string
	HandlerNote           string
}

type PaymentRiskRecordRef struct {
	RecordType string
	TradeNo    string
}

func (c *PaymentRiskCase) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = PaymentRiskStatusOpen
	}
	return nil
}

func (c *PaymentRiskCase) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = common.GetTimestamp()
	return nil
}

func normalizePaymentRiskRecordType(recordType string) string {
	switch strings.ToLower(strings.TrimSpace(recordType)) {
	case PaymentRiskRecordTypeSubscription:
		return PaymentRiskRecordTypeSubscription
	default:
		return PaymentRiskRecordTypeTopUp
	}
}

func normalizePaymentRiskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case PaymentRiskStatusConfirmed:
		return PaymentRiskStatusConfirmed
	case PaymentRiskStatusReversed:
		return PaymentRiskStatusReversed
	case PaymentRiskStatusVoided:
		return PaymentRiskStatusVoided
	default:
		return PaymentRiskStatusOpen
	}
}

func normalizePaymentRiskAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case PaymentRiskActionConfirm:
		return PaymentRiskActionConfirm
	case PaymentRiskActionReverse:
		return PaymentRiskActionReverse
	case PaymentRiskActionVoid:
		return PaymentRiskActionVoid
	default:
		return ""
	}
}

func UpsertPaymentRiskCase(input PaymentRiskCaseUpsertInput) (result *PaymentRiskCase, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error { var e error; result, e = upsertPaymentRiskCaseTx(tx, input); return e })
	return
}

func upsertPaymentRiskCaseTx(tx *gorm.DB, input PaymentRiskCaseUpsertInput) (*PaymentRiskCase, error) {
	recordType := normalizePaymentRiskRecordType(input.RecordType)
	tradeNo := strings.TrimSpace(input.TradeNo)
	if tradeNo == "" {
		return nil, errors.New("trade number is required")
	}

	var result PaymentRiskCase
	err := func() error {
		var existing PaymentRiskCase
		lookup := tx.Where("record_type = ? AND trade_no = ?", recordType, tradeNo).Limit(1).Find(&existing)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			existing = PaymentRiskCase{
				RecordType: recordType,
				TradeNo:    tradeNo,
			}
		}

		existing.UserId = input.UserId
		existing.PaymentMethod = strings.TrimSpace(input.PaymentMethod)
		existing.ProviderPaymentMethod = strings.TrimSpace(input.ProviderPaymentMethod)
		existing.ExpectedAmount = input.ExpectedAmount
		existing.ExpectedMoney = input.ExpectedMoney
		existing.ReceivedMoney = input.ReceivedMoney
		existing.Currency = strings.TrimSpace(input.Currency)
		existing.Source = strings.TrimSpace(input.Source)
		existing.Reason = strings.TrimSpace(input.Reason)
		existing.OrderStatus = strings.TrimSpace(input.OrderStatus)
		existing.ProviderPayload = strings.TrimSpace(input.ProviderPayload)
		existing.HandlerNote = strings.TrimSpace(input.HandlerNote)
		existing.Status = PaymentRiskStatusOpen
		existing.HandlerAdminId = 0
		existing.ResolvedAt = 0
		existing.AppliedQuotaDelta = 0

		if existing.Id == 0 {
			if err := tx.Create(&existing).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
		}
		result = existing
		return nil
	}()
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func GetPaymentRiskCaseByID(id int) (*PaymentRiskCase, error) {
	if id <= 0 {
		return nil, ErrPaymentRiskCaseNotFound
	}
	var riskCase PaymentRiskCase
	if err := paymentRiskCaseBaseQuery(DB).Where("payment_risk_cases.id = ?", id).First(&riskCase).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentRiskCaseNotFound
		}
		return nil, err
	}
	return &riskCase, nil
}

func GetPaymentRiskCaseByRecord(recordType string, tradeNo string) (*PaymentRiskCase, error) {
	recordType = normalizePaymentRiskRecordType(recordType)
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, ErrPaymentRiskCaseNotFound
	}
	var riskCase PaymentRiskCase
	if err := paymentRiskCaseBaseQuery(DB).
		Where("payment_risk_cases.record_type = ? AND payment_risk_cases.trade_no = ?", recordType, tradeNo).
		First(&riskCase).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentRiskCaseNotFound
		}
		return nil, err
	}
	return &riskCase, nil
}

func ListPaymentRiskCasesByParams(params PaymentRiskCaseSearchParams, pageInfo *common.PageInfo) ([]*PaymentRiskCase, int64, error) {
	query := applyPaymentRiskCaseSearch(paymentRiskCaseBaseQuery(DB), params)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if pageInfo == nil {
		pageInfo = &common.PageInfo{Page: 1, PageSize: common.ItemsPerPage}
	}

	var cases []*PaymentRiskCase
	if err := query.Order("payment_risk_cases.id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&cases).Error; err != nil {
		return nil, 0, err
	}
	return cases, total, nil
}

func ListPaymentRiskCasesByRefs(refs []PaymentRiskRecordRef) (map[string]*PaymentRiskCase, error) {
	if len(refs) == 0 {
		return map[string]*PaymentRiskCase{}, nil
	}
	tradeNos := make([]string, 0, len(refs))
	recordTypes := make([]string, 0, len(refs))
	for _, ref := range refs {
		tradeNo := strings.TrimSpace(ref.TradeNo)
		if tradeNo == "" {
			continue
		}
		tradeNos = append(tradeNos, tradeNo)
		recordTypes = append(recordTypes, normalizePaymentRiskRecordType(ref.RecordType))
	}
	if len(tradeNos) == 0 {
		return map[string]*PaymentRiskCase{}, nil
	}

	var cases []*PaymentRiskCase
	if err := DB.Where("trade_no IN ? AND record_type IN ?", tradeNos, recordTypes).Find(&cases).Error; err != nil {
		return nil, err
	}
	result := make(map[string]*PaymentRiskCase, len(cases))
	for _, riskCase := range cases {
		result[paymentRiskCaseMapKey(riskCase.RecordType, riskCase.TradeNo)] = riskCase
	}
	return result, nil
}

func CreateManualPaymentRiskCase(recordType string, tradeNo string, note string) (*PaymentRiskCase, error) {
	snapshot, err := buildPaymentRiskSnapshot(recordType, tradeNo)
	if err != nil {
		return nil, err
	}
	return UpsertPaymentRiskCase(PaymentRiskCaseUpsertInput{
		RecordType:      snapshot.RecordType,
		TradeNo:         snapshot.TradeNo,
		UserId:          snapshot.UserId,
		PaymentMethod:   snapshot.PaymentMethod,
		ExpectedAmount:  snapshot.ExpectedAmount,
		ExpectedMoney:   snapshot.ExpectedMoney,
		Currency:        snapshot.Currency,
		OrderStatus:     snapshot.OrderStatus,
		ProviderPayload: snapshot.ProviderPayload,
		Source:          "manual_admin",
		Reason:          PaymentRiskReasonManualReview,
		HandlerNote:     strings.TrimSpace(note),
	})
}

func ResolvePaymentRiskCase(id int, adminId int, action string, note string) error {
	action = normalizePaymentRiskAction(action)
	if action == "" {
		return ErrPaymentRiskActionInvalid
	}

	riskCase, err := GetPaymentRiskCaseByID(id)
	if err != nil {
		return err
	}
	if riskCase.Status != PaymentRiskStatusOpen {
		return ErrPaymentRiskCaseResolved
	}

	switch action {
	case PaymentRiskActionConfirm:
		if err := confirmPaymentRiskCase(riskCase); err != nil {
			return err
		}
		return finalizePaymentRiskCase(riskCase.Id, PaymentRiskStatusConfirmed, adminId, note, 0)
	case PaymentRiskActionReverse:
		delta, err := reversePaymentRiskCase(riskCase)
		if err != nil {
			return err
		}
		return finalizePaymentRiskCase(riskCase.Id, PaymentRiskStatusReversed, adminId, note, delta)
	case PaymentRiskActionVoid:
		if err := voidPaymentRiskCase(riskCase); err != nil {
			return err
		}
		return finalizePaymentRiskCase(riskCase.Id, PaymentRiskStatusVoided, adminId, note, 0)
	default:
		return ErrPaymentRiskActionInvalid
	}
}

func PaymentRiskAvailableActions(riskCase *PaymentRiskCase) []string {
	if riskCase == nil || normalizePaymentRiskStatus(riskCase.Status) != PaymentRiskStatusOpen {
		return []string{}
	}
	if riskCase.RecordType == PaymentRiskRecordTypeSubscription {
		if riskCase.OrderStatus == common.TopUpStatusPending {
			return []string{PaymentRiskActionConfirm, PaymentRiskActionVoid}
		}
		return []string{PaymentRiskActionConfirm, PaymentRiskActionReverse}
	}
	if riskCase.OrderStatus == common.TopUpStatusPending {
		return []string{PaymentRiskActionConfirm, PaymentRiskActionVoid}
	}
	return []string{PaymentRiskActionConfirm, PaymentRiskActionReverse}
}

func paymentRiskCaseBaseQuery(tx *gorm.DB) *gorm.DB {
	return tx.Table("payment_risk_cases").
		Select("payment_risk_cases.*, users.username AS username, users.display_name AS display_name").
		Joins("LEFT JOIN users ON users.id = payment_risk_cases.user_id")
}

func applyPaymentRiskCaseSearch(query *gorm.DB, params PaymentRiskCaseSearchParams) *gorm.DB {
	if keyword := strings.TrimSpace(params.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("payment_risk_cases.trade_no LIKE ?", like)
	}
	if status := strings.TrimSpace(params.Status); status != "" {
		query = query.Where("payment_risk_cases.status = ?", normalizePaymentRiskStatus(status))
	}
	if reason := strings.TrimSpace(params.Reason); reason != "" {
		query = query.Where("payment_risk_cases.reason = ?", reason)
	}
	if recordType := strings.TrimSpace(params.RecordType); recordType != "" {
		query = query.Where("payment_risk_cases.record_type = ?", normalizePaymentRiskRecordType(recordType))
	}
	if username := strings.TrimSpace(params.Username); username != "" {
		query = applyPaymentRecordUsernameFilter(query, username, "users.id", "users.username")
	}
	return query
}

func paymentRiskCaseMapKey(recordType string, tradeNo string) string {
	return normalizePaymentRiskRecordType(recordType) + ":" + strings.TrimSpace(tradeNo)
}

type paymentRiskSnapshot struct {
	RecordType      string
	TradeNo         string
	UserId          int
	PaymentMethod   string
	ExpectedAmount  int64
	ExpectedMoney   float64
	Currency        string
	OrderStatus     string
	ProviderPayload string
}

func buildPaymentRiskSnapshot(recordType string, tradeNo string) (*paymentRiskSnapshot, error) {
	recordType = normalizePaymentRiskRecordType(recordType)
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, errors.New("trade number is required")
	}
	if recordType == PaymentRiskRecordTypeSubscription {
		order := GetSubscriptionOrderByTradeNo(tradeNo)
		if order == nil {
			return nil, ErrPaymentRiskCaseNotFound
		}
		return &paymentRiskSnapshot{
			RecordType:      recordType,
			TradeNo:         tradeNo,
			UserId:          order.UserId,
			PaymentMethod:   order.PaymentMethod,
			ExpectedMoney:   order.Money,
			OrderStatus:     order.Status,
			ProviderPayload: order.ProviderPayload,
		}, nil
	}
	topUp := GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		return nil, ErrPaymentRiskCaseNotFound
	}
	expectedMoney, currency, _ := topUp.SettlementPaymentAmount()
	return &paymentRiskSnapshot{
		RecordType:     recordType,
		TradeNo:        tradeNo,
		UserId:         topUp.UserId,
		PaymentMethod:  topUp.PaymentMethod,
		ExpectedAmount: topUp.Amount,
		ExpectedMoney:  expectedMoney,
		Currency:       currency,
		OrderStatus:    topUp.Status,
	}, nil
}

func finalizePaymentRiskCase(id int, status string, adminId int, note string, quotaDelta int) error {
	return DB.Model(&PaymentRiskCase{}).
		Where("id = ? AND status = ?", id, PaymentRiskStatusOpen).
		Updates(map[string]any{
			"status":              normalizePaymentRiskStatus(status),
			"handler_admin_id":    adminId,
			"handler_note":        strings.TrimSpace(note),
			"applied_quota_delta": quotaDelta,
			"resolved_at":         common.GetTimestamp(),
		}).Error
}

func confirmPaymentRiskCase(riskCase *PaymentRiskCase) error {
	if riskCase == nil {
		return ErrPaymentRiskCaseNotFound
	}
	if riskCase.RecordType == PaymentRiskRecordTypeSubscription {
		order := GetSubscriptionOrderByTradeNo(riskCase.TradeNo)
		if order == nil {
			return ErrPaymentRiskCaseNotFound
		}
		if order.Status == common.TopUpStatusPending {
			payload := strings.TrimSpace(riskCase.ProviderPayload)
			if payload == "" {
				payload = strings.TrimSpace(order.ProviderPayload)
			}
			if payload == "" {
				payload = "{}"
			}
			return CompleteSubscriptionOrder(riskCase.TradeNo, payload)
		}
		return nil
	}
	topUp := GetTopUpByTradeNo(riskCase.TradeNo)
	if topUp == nil {
		return ErrPaymentRiskCaseNotFound
	}
	if topUp.Status == common.TopUpStatusPending {
		return CompleteTopUpByTradeNo(riskCase.TradeNo, "risk_confirm", "",
			map[string]interface{}{"risk_case_id": riskCase.Id})
	}
	return nil
}

func reversePaymentRiskCase(riskCase *PaymentRiskCase) (int, error) {
	if riskCase == nil {
		return 0, ErrPaymentRiskCaseNotFound
	}
	sourceType := BenefitSourceTopUpOrder
	if riskCase.RecordType == PaymentRiskRecordTypeSubscription {
		sourceType = BenefitSourceSubscriptionOrder
	}
	quotaDelta, summary, err := RollbackBenefitsBySource(
		BenefitRollbackBusinessPaymentRiskCase,
		riskCase.Id,
		sourceType,
		riskCase.TradeNo,
	)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(summary) != "" && riskCase.UserId > 0 {
		RecordTopupLog(riskCase.UserId,
			"payment risk reversal succeeded: "+summary,
			"", "", "risk_reversal",
			map[string]interface{}{"risk_case_id": riskCase.Id, "trade_no": riskCase.TradeNo})
	}
	return quotaDelta, nil
}

func voidPaymentRiskCase(riskCase *PaymentRiskCase) error {
	if riskCase == nil {
		return ErrPaymentRiskCaseNotFound
	}
	if riskCase.RecordType == PaymentRiskRecordTypeSubscription {
		return ExpireSubscriptionOrder(riskCase.TradeNo)
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Where("trade_no = ?", riskCase.TradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentRiskCaseNotFound
			}
			return err
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("only pending top-up orders can be voided")
		}
		topUp.Status = common.TopUpStatusExpired
		topUp.CompleteTime = common.GetTimestamp()
		return tx.Save(topUp).Error
	})
}
