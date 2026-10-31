package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InviteCommissionStatusPending = "pending"
	InviteCommissionStatusSettled = "settled"
	InviteCommissionStatusSkipped = "skipped"
)

const (
	InviteCommissionLevelDirect   = 1
	InviteCommissionLevelIndirect = 2
)

const inviteCommissionCapCasMaxRetry = 8

const (
	// 风控原因：邀请人与被邀请人相同（自邀）。
	InviteCommissionRiskReasonSelfInvite = "self_invite"
	// 风控原因：邀请人当日返佣已达上限。
	InviteCommissionRiskReasonDailyCapReached = "daily_cap_reached"
	// 风控原因：本单返佣被当日上限截断，仅部分发放。
	InviteCommissionRiskReasonDailyCapTruncated = "daily_cap_truncated"
	// 历史兑换码返佣待结算记录不再允许发放。
	InviteCommissionRiskReasonRedemptionNotCommissionable = "redemption_not_commissionable"
	// 返佣台账找不到对应的成功支付订单。
	InviteCommissionRiskReasonPaymentSourceInvalid = "payment_source_invalid"
	// 历史订单缺少可信实付金额，无法安全计算返佣。
	InviteCommissionRiskReasonPaidMoneyMissing = "paid_money_missing"
	// 结算时邀请人已删除、禁用或不存在。
	InviteCommissionRiskReasonInviterUnavailable = "inviter_unavailable"
	// 对应支付已撤销，待结算返佣作废；已发放返佣同步回退。
	InviteCommissionRiskReasonPaymentReversed = "payment_reversed"
)

var errInviteCommissionAlreadyProcessed = errors.New("invite commission ledger already processed")

// InviteCommissionLedger 记录邀请充值返佣的完整生命周期。
//
// 生命周期：
// 1. pending：充值成功后入池，等待后台及时结算。
// 2. settled：后台任务已将返佣发放到邀请人的 aff_quota / aff_history。
// 3. skipped：被风控跳过（例如自邀、单日上限已满）。
//
// 幂等策略：
// - 同一 trade_no + inviter_user_id 只允许一条台账（唯一索引）。
// - 结算时仅处理 status=pending 的记录，重复执行不会重复入账。
type InviteCommissionLedger struct {
	Id                  int     `json:"id"`
	InviteeUserId       int     `json:"invitee_user_id" gorm:"index;not null"`
	InviterUserId       int     `json:"inviter_user_id" gorm:"index;not null;uniqueIndex:idx_invite_commission_trade_inviter"`
	DirectInviteeUserId int     `json:"direct_invitee_user_id" gorm:"index;not null;default:0"`
	CommissionLevel     int     `json:"commission_level" gorm:"index;not null;default:1"`
	TopupTradeNo        string  `json:"topup_trade_no" gorm:"type:varchar(255);not null;uniqueIndex:idx_invite_commission_trade_inviter"`
	BizDate             string  `json:"biz_date" gorm:"type:varchar(10);index;not null"` // 业务日期（YYYY-MM-DD）
	BaseQuota           int     `json:"base_quota" gorm:"type:int;not null;default:0"`
	CommissionRate      float64 `json:"commission_rate" gorm:"type:decimal(10,6);not null;default:0"`
	CommissionQuota     int     `json:"commission_quota" gorm:"type:int;not null;default:0"`
	SettledQuota        int     `json:"settled_quota" gorm:"type:int;not null;default:0"`
	Status              string  `json:"status" gorm:"type:varchar(16);index;not null;index:idx_invite_commission_ready,priority:1"`
	RiskReason          string  `json:"risk_reason" gorm:"type:varchar(64);default:''"`
	CreatedAt           int64   `json:"created_at" gorm:"index"`
	SettledAt           int64   `json:"settled_at"`
	AmountVersion       int     `json:"amount_version" gorm:"not null;default:0"`
	RetryCount          int     `json:"retry_count" gorm:"not null;default:0"`
	NextRetryAt         int64   `json:"next_retry_at" gorm:"not null;default:0;index:idx_invite_commission_ready,priority:2"`
}

// InviteCommissionDailyCapState 记录 inviter + bizDate 的当日已结算返佣额度。
type InviteCommissionDailyCapState struct {
	Id            int    `json:"id"`
	InviterUserId int    `json:"inviter_user_id" gorm:"not null;uniqueIndex:idx_invite_commission_daily_cap_inviter_date"`
	BizDate       string `json:"biz_date" gorm:"type:varchar(10);not null;uniqueIndex:idx_invite_commission_daily_cap_inviter_date"`
	SettledQuota  int    `json:"settled_quota" gorm:"type:int;not null;default:0"`
	CreatedAt     int64  `json:"created_at" gorm:"index"`
	UpdatedAt     int64  `json:"updated_at" gorm:"index"`
}

// EnqueueInviteCommissionFromTopUp 按订单实付金额折算返佣基数，不复用到账额度。
func EnqueueInviteCommissionFromTopUp(topUp *TopUp) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueInviteCommissionFromTopUpTx(tx, topUp)
	})
	if err == nil {
		NotifyInviteCommissionSettlement()
	}
	return err
}

// EnqueueInviteCommissionFromTopUpTx persists the commission in the payment
// transaction. It must not publish a wakeup until the caller commits.
func EnqueueInviteCommissionFromTopUpTx(tx *gorm.DB, topUp *TopUp) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	tx = tx.Session(&gorm.Session{NewDB: true})
	if topUp == nil || topUp.Id <= 0 {
		return nil
	}
	if !common.InviteCommissionConfigured() {
		return nil
	}
	if operation_setting.Price <= 0 || math.IsNaN(operation_setting.Price) || math.IsInf(operation_setting.Price, 0) {
		common.SysError(fmt.Sprintf("skip invite commission for top-up %d: invalid payment price setting", topUp.Id))
		return nil
	}

	// Reading the source and inserting the ledger happen in one transaction.
	// The row lock serializes payment reversal so a reversed order cannot be
	// re-enqueued in the gap between validation and ledger creation.
	dbTopUp := &TopUp{}
	query := tx.Select("id", "user_id", "trade_no", "money", "paid_money", "paid_currency", "presentment_money", "presentment_currency", "payment_method", "payment_provider", "complete_time", "status")
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(dbTopUp, "id = ?", topUp.Id).Error; err != nil {
		return err
	}
	if dbTopUp.Status != common.TopUpStatusSuccess {
		return nil
	}

	paidMoney, ok := dbTopUp.CNYPaymentAmount()
	if !ok {
		return nil
	}

	baseQuota := int(decimal.NewFromFloat(paidMoney).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Div(decimal.NewFromFloat(operation_setting.Price)).IntPart())
	if baseQuota <= 0 {
		return nil
	}
	return enqueueInviteCommissionWithDB(tx, dbTopUp.UserId, dbTopUp.TradeNo, dbTopUp.CompleteTime, baseQuota)
}

// EnqueueInviteCommissionFromSubscriptionOrderTx 将“订阅支付成功”纳入邀请返佣口径。
// 返佣基数按“实付金额折算额度”计算：
// 返佣基数公式：baseQuota = floor(order.money * QuotaPerUnit / Price)
// 其中 Price 为“充值价格（x元/美金）”。
func EnqueueInviteCommissionFromSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if order == nil || order.Id <= 0 {
		return nil
	}
	if !common.InviteCommissionConfigured() {
		return nil
	}
	if operation_setting.Price <= 0 || math.IsNaN(operation_setting.Price) || math.IsInf(operation_setting.Price, 0) {
		common.SysError(fmt.Sprintf("skip invite commission for subscription order %d: invalid payment price setting", order.Id))
		return nil
	}

	tx = tx.Session(&gorm.Session{NewDB: true})
	// 仅信任 order.id；其余字段从 DB 读取，避免调用方传入被篡改数据。
	dbOrder := &SubscriptionOrder{}
	query := tx.Select("id", "user_id", "trade_no", "money", "complete_time", "status")
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(dbOrder, "id = ?", order.Id).Error; err != nil {
		return err
	}
	// 仅已支付成功订单允许入返佣池。
	if dbOrder.Status != common.TopUpStatusSuccess {
		return nil
	}

	dBaseQuota := decimal.NewFromFloat(dbOrder.Money).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Div(decimal.NewFromFloat(operation_setting.Price))
	baseQuota := int(dBaseQuota.IntPart())
	if baseQuota <= 0 {
		return nil
	}

	return enqueueInviteCommissionWithDB(tx, dbOrder.UserId, dbOrder.TradeNo, dbOrder.CompleteTime, baseQuota)
}

func enqueueInviteCommissionWithDB(db *gorm.DB, inviteeUserID int, tradeNo string, completeTime int64, baseQuota int) error {
	if db == nil {
		return errors.New("db is nil")
	}
	// 基础防护：仅合法充值额度允许入池。
	if inviteeUserID <= 0 || tradeNo == "" || baseQuota <= 0 {
		return nil
	}
	// 功能开关与完整比例检查。使用同一份比例快照，避免设置更新时
	// 同一订单混用两个时刻的经济参数。
	configured, firstLevelRate, secondLevelRate := common.InviteCommissionConfigSnapshot()
	if !configured {
		return nil
	}

	// 在入池时快照 C -> B -> A，避免后续关系变更影响历史订单归属。
	// C 只对 B 做过一次直接绑定判断；A 由 B 的既有 inviter_id 自动继承，
	// 不进行第二次概率抽签。
	invitee := &User{}
	if err := db.Select("id", "inviter_id").First(invitee, "id = ?", inviteeUserID).Error; err != nil {
		return err
	}
	if invitee.InviterId == 0 {
		return nil
	}
	if invitee.InviterId == invitee.Id {
		return nil
	}

	if completeTime == 0 {
		completeTime = common.GetTimestamp()
	}
	bizDate := time.Unix(completeTime, 0).Format("2006-01-02")
	createdAt := common.GetTimestamp()
	ledgers := make([]*InviteCommissionLedger, 0, 2)
	appendLedger := func(inviterUserID, directInviteeUserID, level int, rate float64) {
		if inviterUserID <= 0 || directInviteeUserID <= 0 || rate <= 0 ||
			inviterUserID == invitee.Id {
			return
		}
		// 每级分别向下取整，且完整比例已限制合计不超过 100%。
		commissionQuota := int(decimal.NewFromInt(int64(baseQuota)).Mul(decimal.NewFromFloat(rate)).IntPart())
		if commissionQuota <= 0 {
			return
		}
		ledgers = append(ledgers, &InviteCommissionLedger{
			InviteeUserId:       invitee.Id,
			InviterUserId:       inviterUserID,
			DirectInviteeUserId: directInviteeUserID,
			CommissionLevel:     level,
			TopupTradeNo:        tradeNo,
			BizDate:             bizDate,
			BaseQuota:           baseQuota,
			CommissionRate:      rate,
			CommissionQuota:     commissionQuota,
			AmountVersion:       1,
			Status:              InviteCommissionStatusPending,
			CreatedAt:           createdAt,
		})
	}

	appendLedger(
		invitee.InviterId,
		invitee.Id,
		InviteCommissionLevelDirect,
		firstLevelRate,
	)

	if secondLevelRate > 0 {
		directInviter := &User{}
		err := db.Select("id", "inviter_id").First(directInviter, "id = ?", invitee.InviterId).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && directInviter.InviterId > 0 &&
			directInviter.InviterId != directInviter.Id &&
			directInviter.InviterId != invitee.Id {
			appendLedger(
				directInviter.InviterId,
				directInviter.Id,
				InviteCommissionLevelIndirect,
				secondLevelRate,
			)
		}
	}
	if len(ledgers) == 0 {
		return nil
	}

	return db.Clauses(clause.OnConflict{
		// 幂等保障：同一个订单对同一个邀请人只允许入池一次。
		Columns: []clause.Column{
			{Name: "topup_trade_no"},
			{Name: "inviter_user_id"},
		},
		DoNothing: true,
	}).Create(&ledgers).Error
}

func SettleInviteCommissionByBizDate(bizDate string, batchSize int) (settledCount int, skippedCount int, processedCount int, err error) {
	// 防御性检查：避免错误参数导致全表扫。
	if batchSize <= 0 {
		return 0, 0, 0, errors.New("batch size must be positive")
	}

	// 到期任务按 id 扫描；失败记录退避，不阻塞后续订单。
	var ledgers []*InviteCommissionLedger
	if err = DB.Where("status = ? AND biz_date <= ? AND next_retry_at <= ?", InviteCommissionStatusPending, bizDate, common.GetTimestamp()).
		Order("id asc").
		Limit(batchSize).
		Find(&ledgers).Error; err != nil {
		return 0, 0, 0, err
	}
	if len(ledgers) == 0 {
		return 0, 0, 0, nil
	}

	dailyCap := common.InviterCommissionDailyCap

	for _, ledger := range ledgers {
		if ledger == nil {
			continue
		}

		processed, settled, settleErr := settleSingleInviteCommissionLedger(ledger, dailyCap)
		if settleErr != nil {
			// Count attempted rows so a full batch can advance past failed jobs.
			processedCount++
			if retryErr := deferInviteCommissionRetry(ledger); retryErr != nil {
				return settledCount, skippedCount, processedCount, retryErr
			}
			common.SysError(fmt.Sprintf("invite commission settlement deferred: ledger=%d err=%v", ledger.Id, settleErr))
			continue
		}
		if !processed {
			continue
		}

		processedCount++
		if settled {
			settledCount++
		} else {
			skippedCount++
		}
	}

	return settledCount, skippedCount, processedCount, nil
}

// Wakeups are only an optimization; the database and periodic sweep are durable.
var inviteCommissionWakeup = make(chan struct{}, 1)

func InviteCommissionSettlementWakeups() <-chan struct{} { return inviteCommissionWakeup }

func NotifyInviteCommissionSettlement() {
	select {
	case inviteCommissionWakeup <- struct{}{}:
	default:
	}
}

func deferInviteCommissionRetry(ledger *InviteCommissionLedger) error {
	attempt := ledger.RetryCount
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 6 {
		attempt = 6
	}
	delay := int64(30) << attempt
	if delay > 1800 {
		delay = 1800
	}
	return DB.Model(&InviteCommissionLedger{}).
		Where("id = ? AND status = ? AND retry_count = ?", ledger.Id, InviteCommissionStatusPending, ledger.RetryCount).
		Updates(map[string]interface{}{
			"retry_count":   gorm.Expr("retry_count + 1"),
			"next_retry_at": common.GetTimestamp() + delay,
		}).Error
}

func settleSingleInviteCommissionLedger(ledger *InviteCommissionLedger, dailyCap int) (processed bool, settled bool, err error) {
	now := common.GetTimestamp()
	allowedQuota := 0
	riskReason := ""

	err = DB.Transaction(func(tx *gorm.DB) error {
		sourceRiskReason, reconcileErr := reconcilePendingInviteCommissionAmountTx(tx, ledger)
		if reconcileErr != nil {
			return reconcileErr
		}
		if sourceRiskReason != "" {
			riskReason = sourceRiskReason
		}
		allowedQuota = ledger.CommissionQuota

		if strings.HasPrefix(ledger.TopupTradeNo, "redeem:") {
			allowedQuota = 0
			riskReason = InviteCommissionRiskReasonRedemptionNotCommissionable
		} else if sourceRiskReason != "" {
			allowedQuota = 0
		} else if ledger.InviterUserId == ledger.InviteeUserId {
			allowedQuota = 0
			riskReason = InviteCommissionRiskReasonSelfInvite
		}
		if allowedQuota > 0 {
			inviter := &User{}
			inviterQuery := tx.Select("id", "status")
			if !common.UsingSQLite {
				inviterQuery = inviterQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			inviterErr := inviterQuery.First(inviter, "id = ?", ledger.InviterUserId).Error
			if errors.Is(inviterErr, gorm.ErrRecordNotFound) ||
				(inviterErr == nil && inviter.Status != common.UserStatusEnabled) {
				allowedQuota = 0
				riskReason = InviteCommissionRiskReasonInviterUnavailable
			} else if inviterErr != nil {
				return inviterErr
			}
		}

		// 在事务内做日上限 CAS 预占，避免并发超发。
		if allowedQuota > 0 && dailyCap > 0 {
			var truncated bool
			allowedQuota, truncated, err = reserveInviteCommissionDailyCapTx(tx, ledger.InviterUserId, ledger.BizDate, allowedQuota, dailyCap)
			if err != nil {
				return err
			}
			if allowedQuota == 0 {
				riskReason = InviteCommissionRiskReasonDailyCapReached
			} else if truncated {
				riskReason = InviteCommissionRiskReasonDailyCapTruncated
			}
		}

		targetStatus := InviteCommissionStatusSkipped
		if allowedQuota > 0 {
			targetStatus = InviteCommissionStatusSettled
		}

		// CAS 风格状态流转：只允许 pending -> settled/skipped 一次。
		updateResult := tx.Model(&InviteCommissionLedger{}).
			Where("id = ? AND status = ?", ledger.Id, InviteCommissionStatusPending).
			Updates(map[string]interface{}{
				"base_quota":       ledger.BaseQuota,
				"commission_quota": ledger.CommissionQuota,
				"status":           targetStatus,
				"settled_quota":    allowedQuota,
				"risk_reason":      riskReason,
				"settled_at":       now,
				"next_retry_at":    0,
			})
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			// 并发重跑命中时，回滚事务中的日上限预占。
			return errInviteCommissionAlreadyProcessed
		}

		processed = true
		if targetStatus != InviteCommissionStatusSettled {
			return nil
		}

		settled = true
		// 保持现有邀请体系体验：返佣先进邀请额度，用户再手动划转到余额。
		if err := tx.Model(&User{}).Where("id = ?", ledger.InviterUserId).Updates(map[string]interface{}{
			"aff_quota":   gorm.Expr("aff_quota + ?", allowedQuota),
			"aff_history": gorm.Expr("aff_history + ?", allowedQuota),
		}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errInviteCommissionAlreadyProcessed) {
			return false, false, nil
		}
		return false, false, err
	}

	if processed && settled {
		levelLabel := "一级"
		if ledger.CommissionLevel == InviteCommissionLevelIndirect {
			levelLabel = "二级"
		}
		RecordLog(
			ledger.InviterUserId,
			LogTypeSystem,
			fmt.Sprintf("%s邀请返佣到账 %s（订单:%s）", levelLabel, logger.LogQuota(allowedQuota), maskTradeNoForLog(ledger.TopupTradeNo)),
		)
	}

	return processed, settled, nil
}

// reverseInviteCommissionsByTradeNoTx invalidates every commission created by
// a reversed payment. Pending ledgers are skipped before they can settle. For
// already-settled ledgers, the payout is reclaimed from aff_quota first and
// then from wallet quota, covering the case where the inviter already moved
// the commission into the wallet.
func reverseInviteCommissionsByTradeNoTx(tx *gorm.DB, tradeNo string) (int, error) {
	if tx == nil {
		return 0, errors.New("tx is nil")
	}
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return 0, nil
	}

	var ledgers []*InviteCommissionLedger
	if err := tx.Where("topup_trade_no = ? AND status IN ?", tradeNo, []string{
		InviteCommissionStatusPending,
		InviteCommissionStatusSettled,
	}).Order("id ASC").Find(&ledgers).Error; err != nil {
		return 0, err
	}

	reversedQuota := 0
	for _, ledger := range ledgers {
		if ledger == nil {
			continue
		}
		if ledger.Status == InviteCommissionStatusPending {
			result := tx.Model(&InviteCommissionLedger{}).
				Where("id = ? AND status = ?", ledger.Id, InviteCommissionStatusPending).
				Updates(map[string]any{
					"status":        InviteCommissionStatusSkipped,
					"settled_quota": 0,
					"risk_reason":   InviteCommissionRiskReasonPaymentReversed,
					"settled_at":    common.GetTimestamp(),
				})
			if result.Error != nil {
				return 0, result.Error
			}
			continue
		}

		settledQuota := ledger.SettledQuota
		if settledQuota <= 0 {
			settledQuota = ledger.CommissionQuota
		}
		if settledQuota <= 0 {
			continue
		}

		// Keep the same lock order as settlement: inviter -> daily cap -> ledger.
		// The payment source is locked first by both settlement and reversal.
		inviterQuery := tx.Unscoped().Select("id", "quota", "transferable_quota", "aff_quota", "aff_history").Where("id = ?", ledger.InviterUserId)
		if !common.UsingSQLite {
			inviterQuery = inviterQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var inviter User
		inviterErr := inviterQuery.First(&inviter).Error
		if inviterErr != nil && !errors.Is(inviterErr, gorm.ErrRecordNotFound) {
			return 0, inviterErr
		}

		var capState InviteCommissionDailyCapState
		capQuery := tx.Where("inviter_user_id = ? AND biz_date = ?", ledger.InviterUserId, ledger.BizDate)
		if !common.UsingSQLite {
			capQuery = capQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		capErr := capQuery.First(&capState).Error
		if capErr != nil && !errors.Is(capErr, gorm.ErrRecordNotFound) {
			return 0, capErr
		}

		ledgerResult := tx.Model(&InviteCommissionLedger{}).
			Where("id = ? AND status = ?", ledger.Id, InviteCommissionStatusSettled).
			Updates(map[string]any{
				"status":        InviteCommissionStatusSkipped,
				"settled_quota": 0,
				"risk_reason":   InviteCommissionRiskReasonPaymentReversed,
				"settled_at":    common.GetTimestamp(),
			})
		if ledgerResult.Error != nil {
			return 0, ledgerResult.Error
		}
		if ledgerResult.RowsAffected != 1 {
			continue
		}

		if capErr == nil {
			newSettledQuota := capState.SettledQuota - settledQuota
			if newSettledQuota < 0 {
				newSettledQuota = 0
			}
			if err := tx.Model(&InviteCommissionDailyCapState{}).
				Where("id = ?", capState.Id).
				Update("settled_quota", newSettledQuota).Error; err != nil {
				return 0, err
			}
		}

		if errors.Is(inviterErr, gorm.ErrRecordNotFound) {
			continue
		}

		affDebit := inviter.AffQuota
		if affDebit < 0 {
			affDebit = 0
		}
		if affDebit > settledQuota {
			affDebit = settledQuota
		}
		walletDebit := settledQuota - affDebit
		newQuota := inviter.Quota - walletDebit
		newTransferableQuota := EffectiveTransferableQuota(newQuota, inviter.TransferableQuota)
		newAffHistoryQuota := inviter.AffHistoryQuota - settledQuota
		if newAffHistoryQuota < 0 {
			newAffHistoryQuota = 0
		}
		if walletDebit > 0 {
			if err := InvalidatePulsePaidFundingTx(tx, inviter.Id, "commission_clawback"); err != nil {
				return 0, err
			}
		}
		if err := tx.Unscoped().Model(&User{}).Where("id = ?", inviter.Id).Updates(map[string]any{
			"quota":              newQuota,
			"transferable_quota": newTransferableQuota,
			"aff_quota":          inviter.AffQuota - affDebit,
			"aff_history":        newAffHistoryQuota,
		}).Error; err != nil {
			return 0, err
		}
		reversedQuota += settledQuota
	}

	return reversedQuota, nil
}

func reconcilePendingInviteCommissionAmountTx(tx *gorm.DB, ledger *InviteCommissionLedger) (string, error) {
	if tx == nil || ledger == nil {
		return "", errors.New("invalid commission settlement")
	}
	if strings.HasPrefix(ledger.TopupTradeNo, "redeem:") {
		return InviteCommissionRiskReasonRedemptionNotCommissionable, nil
	}
	topUp := &TopUp{}
	query := tx.Select("id", "money", "paid_money", "paid_currency", "presentment_money", "presentment_currency", "payment_method", "payment_provider", "status")
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(topUp, "trade_no = ?", ledger.TopupTradeNo).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return InviteCommissionRiskReasonPaymentSourceInvalid, nil
	}
	if err != nil {
		return "", err
	}
	if topUp.Status != common.TopUpStatusSuccess {
		return InviteCommissionRiskReasonPaymentSourceInvalid, nil
	}
	// Version 1 is the paid-time snapshot; do not reprice it on retry.
	if ledger.AmountVersion >= 1 {
		if ledger.BaseQuota <= 0 || ledger.CommissionQuota <= 0 {
			return InviteCommissionRiskReasonPaidMoneyMissing, nil
		}
		return "", nil
	}
	// Existing unversioned pending records retain the legacy reconciliation.
	if operation_setting.Price <= 0 || math.IsNaN(operation_setting.Price) || math.IsInf(operation_setting.Price, 0) {
		return "", errors.New("invalid payment price for legacy commission")
	}

	paidMoney, ok := topUp.CNYPaymentAmount()
	if !ok {
		return InviteCommissionRiskReasonPaidMoneyMissing, nil
	}
	if ledger.CommissionRate <= 0 || math.IsNaN(ledger.CommissionRate) || math.IsInf(ledger.CommissionRate, 0) {
		return InviteCommissionRiskReasonPaymentSourceInvalid, nil
	}

	baseQuota := int(decimal.NewFromFloat(paidMoney).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Div(decimal.NewFromFloat(operation_setting.Price)).IntPart())
	if baseQuota <= 0 {
		return InviteCommissionRiskReasonPaidMoneyMissing, nil
	}
	commissionQuota := int(decimal.NewFromInt(int64(baseQuota)).
		Mul(decimal.NewFromFloat(ledger.CommissionRate)).IntPart())
	if commissionQuota <= 0 {
		return InviteCommissionRiskReasonPaidMoneyMissing, nil
	}

	ledger.BaseQuota = baseQuota
	ledger.CommissionQuota = commissionQuota
	return "", nil
}

func reserveInviteCommissionDailyCapTx(tx *gorm.DB, inviterUserId int, bizDate string, requestQuota int, dailyCap int) (grantedQuota int, truncated bool, err error) {
	if requestQuota <= 0 || dailyCap <= 0 {
		return requestQuota, false, nil
	}
	if err := ensureInviteCommissionDailyCapStateTx(tx, inviterUserId, bizDate); err != nil {
		return 0, false, err
	}

	for i := 0; i < inviteCommissionCapCasMaxRetry; i++ {
		state := &InviteCommissionDailyCapState{}
		if err := tx.Select("inviter_user_id", "biz_date", "settled_quota").First(state, "inviter_user_id = ? AND biz_date = ?", inviterUserId, bizDate).Error; err != nil {
			return 0, false, err
		}
		if state.SettledQuota >= dailyCap {
			return 0, false, nil
		}

		remain := dailyCap - state.SettledQuota
		grantedQuota = requestQuota
		if grantedQuota > remain {
			grantedQuota = remain
			truncated = true
		}

		now := common.GetTimestamp()
		updateResult := tx.Model(&InviteCommissionDailyCapState{}).
			Where("inviter_user_id = ? AND biz_date = ? AND settled_quota = ?", inviterUserId, bizDate, state.SettledQuota).
			Updates(map[string]interface{}{
				"settled_quota": state.SettledQuota + grantedQuota,
				"updated_at":    now,
			})
		if updateResult.Error != nil {
			return 0, false, updateResult.Error
		}
		if updateResult.RowsAffected > 0 {
			return grantedQuota, truncated, nil
		}
	}
	return 0, false, fmt.Errorf("reserve invite commission daily cap retry exhausted: inviter=%d biz_date=%s", inviterUserId, bizDate)
}

func ensureInviteCommissionDailyCapStateTx(tx *gorm.DB, inviterUserId int, bizDate string) error {
	now := common.GetTimestamp()
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "inviter_user_id"},
			{Name: "biz_date"},
		},
		DoNothing: true,
	}).Create(&InviteCommissionDailyCapState{
		InviterUserId: inviterUserId,
		BizDate:       bizDate,
		SettledQuota:  0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error
}

func maskTradeNoForLog(tradeNo string) string {
	if tradeNo == "" {
		return "-"
	}
	if len(tradeNo) <= 10 {
		return "***"
	}
	return tradeNo[:6] + "***" + tradeNo[len(tradeNo)-4:]
}
