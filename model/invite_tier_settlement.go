package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func settleTierCommission(id int, dailyCap, now int64) (processed, settled bool, err error) {
	var recipient, level int
	var net int64
	err = DB.Transaction(func(tx *gorm.DB) error {
		var l InviteCommissionLedger
		if err := tx.First(&l, id).Error; err != nil {
			return err
		}
		if l.Status != InviteCommissionStatusPending || l.NextRetryAt > now {
			return nil
		}
		// Payment source precedes beneficiary locks, matching payment reversal.
		source := l
		if l.SourceTradeNo != "" {
			source.TopupTradeNo = l.SourceTradeNo
		}
		risk := ""
		if l.CommissionLevel != InviteCommissionLevelRegister || l.SourceTradeNo != "" {
			r, e := reconcilePendingInviteCommissionAmountTx(tx, &source)
			if e != nil {
				return e
			}
			risk = r
		}
		u, e := lockInviteUser(tx, l.InviterUserId)
		if e != nil {
			return e
		}
		if err := inviteLock(tx).First(&l, id).Error; err != nil {
			return err
		}
		if l.Status != InviteCommissionStatusPending || l.NextRetryAt > now {
			return nil
		}
		deferUntil := func(reason string, at int64) error {
			return tx.Model(&l).Updates(map[string]any{"risk_reason": reason, "next_retry_at": at}).Error
		}
		if u.Status != common.UserStatusEnabled {
			risk = InviteCommissionRiskReasonInviterUnavailable
		}
		if l.InviterUserId == l.InviteeUserId {
			risk = InviteCommissionRiskReasonSelfInvite
		}
		if risk == "" {
			if e := checkInvitePayoutEligibility(tx, u.Id); e != nil {
				if errors.Is(e, ErrInviteFrozen) {
					return deferUntil("payout_frozen", now+86400)
				}
				if errors.Is(e, ErrInviteIdentity) {
					return deferUntil("identity_pending", now+86400)
				}
				return e
			}
			if l.ValidityState != "valid" && l.CommissionLevel != InviteCommissionLevelRegister && !strings.HasPrefix(l.TopupTradeNo, "invite-carry:") {
				var buyer User
				if e := tx.First(&buyer, l.InviteeUserId).Error; e != nil {
					return e
				}
				state := inviteValidity(&buyer, l.PaidCents, now)
				l.ValidityState = state
				if state == "pending_time" || state == "pending_usage" {
					return deferUntil("pending_time", buyer.CreatedAt+7*86400)
				}
				if state != "valid" {
					risk = "invalid_consumption"
				}
			}
		}
		if risk == "" {
			if now < l.ObservedUntil {
				return deferUntil("observation_period", l.ObservedUntil)
			}
			if !l.EarlyApproved && now < l.PayoutAt {
				return deferUntil("awaiting_period", l.PayoutAt)
			}
			if l.StatementId == 0 {
				return deferUntil("awaiting_statement", now+3600)
			}
			var statement InviteCommissionStatement
			if e := tx.First(&statement, l.StatementId).Error; e != nil {
				return e
			}
			if statement.Status == 4 {
				return deferUntil("statement_frozen", now+86400)
			}
			if statement.Status != 2 {
				return deferUntil("awaiting_statement", now+3600)
			}
		}
		granted := l.CommissionQuota
		if risk != "" {
			granted = 0
		}
		if granted > 0 && dailyCap > 0 {
			var truncated bool
			var e error
			granted, truncated, e = reserveInviteCommissionDailyCapTx(tx, l.InviterUserId, l.BizDate, granted, dailyCap)
			if e != nil {
				return e
			}
			if truncated {
				risk = InviteCommissionRiskReasonDailyCapTruncated
			}
			if granted == 0 {
				risk = InviteCommissionRiskReasonDailyCapReached
			}
		}
		debtOffset := int64(0)
		if granted > 0 {
			var debt InviteCommissionDebt
			e := inviteLock(tx).Where("inviter_user_id = ?", u.Id).First(&debt).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if e == nil {
				debtOffset = min(granted, max(int64(0), debt.DebtAmount-debt.PaidAmount))
				debt.PaidAmount += debtOffset
				if debt.PaidAmount >= debt.DebtAmount {
					if e = tx.Delete(&debt).Error; e != nil {
						return e
					}
				} else if e = tx.Save(&debt).Error; e != nil {
					return e
				}
			}
		}
		status := InviteCommissionStatusSkipped
		if granted > 0 {
			status = InviteCommissionStatusSettled
		}
		result := tx.Model(&InviteCommissionLedger{}).Where("id = ? AND status = ?", l.Id, InviteCommissionStatusPending).Updates(map[string]any{"status": status, "settled_quota": granted - debtOffset, "debt_offset": debtOffset, "settled_at": now, "risk_reason": risk, "next_retry_at": 0, "validity_state": l.ValidityState})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errInviteCommissionAlreadyProcessed
		}
		if granted > debtOffset {
			if e := tx.Model(&User{}).Where("id = ?", u.Id).Updates(map[string]any{"aff_quota": gorm.Expr("aff_quota + ?", granted-debtOffset), "aff_history": gorm.Expr("aff_history + ?", granted-debtOffset)}).Error; e != nil {
				return e
			}
		}
		if l.StatementId > 0 {
			if e := refreshInviteStatementTx(tx, l.StatementId, now); e != nil {
				return e
			}
		}
		processed = true
		recipient, level, net = l.InviterUserId, l.CommissionLevel, granted-debtOffset
		settled = granted > 0
		return nil
	})
	if err != nil {
		processed = false
		settled = false
	}
	if errors.Is(err, errInviteCommissionAlreadyProcessed) {
		err = nil
	}
	if processed && settled {
		RecordLog(recipient, LogTypeSystem, fmt.Sprintf("%s周期台账 %d 已结算，实发额度 %d", inviteCommissionLevelLabel(level), id, net))
	}
	return
}

func refreshInviteStatementTx(tx *gorm.DB, id int, now int64) error {
	var rows []InviteCommissionLedger
	if e := tx.Where("statement_id = ?", id).Find(&rows).Error; e != nil {
		return e
	}
	var due, paid, deduct int64
	pending := false
	for _, l := range rows {
		due += l.CommissionQuota + l.PoolDeduction
		paid += l.SettledQuota
		deduct += l.DebtOffset + l.PoolDeduction
		if l.Status == InviteCommissionStatusPending {
			pending = true
		}
		if l.Status == InviteCommissionStatusSkipped {
			deduct += l.CommissionQuota
		}
		if l.Status == InviteCommissionStatusSettled {
			// A daily cap may grant only part of the original snapshot.
			deduct += max(int64(0), l.CommissionQuota-l.SettledQuota-l.DebtOffset)
		}
	}
	updates := map[string]any{"due_amount": due, "paid_amount": paid, "deducted_amount": deduct}
	if !pending && len(rows) > 0 {
		updates["status"] = 3
		updates["released_at"] = now
	}
	return tx.Model(&InviteCommissionStatement{}).Where("id = ? AND status <> 4", id).Updates(updates).Error
}

func IssueInviteStatements(now int64) error {
	last := 0
	for {
		var rows []InviteCommissionLedger
		if e := DB.Where("amount_version = 2 AND statement_id = 0 AND issue_at <= ? AND id > ?", now, last).Order("id").Limit(1000).Find(&rows).Error; e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			last = row.Id
			if e := DB.Transaction(func(tx *gorm.DB) error {
				if _, e := lockInviteUser(tx, row.InviterUserId); e != nil {
					return e
				}
				st := InviteCommissionStatement{InviterUserId: row.InviterUserId, PeriodType: row.SettlementCycle, PeriodKey: row.PeriodKey, Status: 1}
				if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&st).Error; e != nil {
					return e
				}
				if e := tx.Where("inviter_user_id = ? AND period_type = ? AND period_key = ?", row.InviterUserId, row.SettlementCycle, row.PeriodKey).First(&st).Error; e != nil {
					return e
				}
				if st.Status == 4 {
					return nil
				}
				if e := tx.Model(&InviteCommissionLedger{}).Where("inviter_user_id = ? AND settlement_cycle = ? AND period_key = ? AND amount_version = 2 AND statement_id = 0 AND issue_at <= ?", row.InviterUserId, row.SettlementCycle, row.PeriodKey, now).Update("statement_id", st.Id).Error; e != nil {
					return e
				}
				if st.Status == 1 {
					if e := tx.Model(&st).Where("status = 1").Updates(map[string]any{"status": 2, "issued_at": now, "dispute_deadline": now + 15*86400}).Error; e != nil {
						return e
					}
				}
				if st.Status == 3 {
					if e := tx.Model(&st).Update("status", 2).Error; e != nil {
						return e
					}
				}
				if e := tx.Model(&InviteCommissionLedger{}).Where("statement_id = ? AND status = ? AND risk_reason = ?", st.Id, InviteCommissionStatusPending, "awaiting_statement").Update("next_retry_at", now).Error; e != nil {
					return e
				}
				return refreshInviteStatementTx(tx, st.Id, now)
			}); e != nil {
				return e
			}
		}
	}
}

func addInviteDebtTx(tx *gorm.DB, id int, amount int64, ref string) error {
	if amount <= 0 {
		return nil
	}
	debt := InviteCommissionDebt{InviterUserId: id, DebtAmount: amount, SourceType: 1, SourceRef: ref}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "inviter_user_id"}}, DoUpdates: clause.Assignments(map[string]any{"debt_amount": gorm.Expr("invite_commission_debts.debt_amount + ?", amount)})}).Create(&debt).Error
}

// Month adjustment is guarded by a durable month receipt. Payouts and mutations
// lock the beneficiary; the payment source is never acquired after that lock.
func AdjustInvitePoolMonth(month string, now int64) error {
	start, e := time.ParseInLocation("2006-01", month, inviteBusinessLocation)
	if e != nil {
		return e
	}
	end := start.AddDate(0, 1, 0)
	if now < end.Unix() {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		mark := InvitePoolMonth{Month: month}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&mark).Error; e != nil {
			return e
		}
		if e := inviteLock(tx).First(&mark, "month = ?", month).Error; e != nil {
			return e
		}
		if mark.AdjustedAt > 0 {
			return nil
		}
		var orders []TopUp
		if e := tx.Where("status = ? AND complete_time >= ? AND complete_time < ?", common.TopUpStatusSuccess, start.Unix(), end.Unix()).Find(&orders).Error; e != nil {
			return e
		}
		base := decimal.Zero
		for _, o := range orders {
			if m, ok := o.CNYPaymentAmount(); ok {
				q, e := inviteBaseQuota(m)
				if e != nil {
					return e
				}
				base = base.Add(decimal.NewFromInt(q))
			}
		}
		var rows []InviteCommissionLedger
		if e := tx.Where("amount_version = 2 AND biz_date >= ? AND biz_date < ? AND topup_trade_no NOT LIKE ? AND status <> ?", start.Format("2006-01-02"), end.Format("2006-01-02"), "invite-carry:%", InviteCommissionStatusSkipped).Order("inviter_user_id,id").Find(&rows).Error; e != nil {
			return e
		}
		actual := decimal.Zero
		for _, l := range rows {
			actual = actual.Add(decimal.NewFromInt(l.CommissionQuota))
		}
		coef := decimal.NewFromInt(1)
		cap := base.Mul(decimal.NewFromFloat(common.GetInviteTierSettings().PoolRatio))
		if actual.GreaterThan(cap) && actual.IsPositive() {
			coef = cap.Div(actual)
		}
		if coef.LessThan(decimal.NewFromInt(1)) {
			for _, l := range rows {
				if _, e := lockInviteUser(tx, l.InviterUserId); e != nil {
					return e
				}
				var current InviteCommissionLedger
				if e := inviteLock(tx).First(&current, l.Id).Error; e != nil {
					return e
				}
				if current.Status != InviteCommissionStatusPending {
					continue
				}
				reduced := decimal.NewFromInt(current.CommissionQuota).Mul(coef).Floor().IntPart()
				diff := current.CommissionQuota - reduced
				if diff <= 0 {
					continue
				}
				if e := tx.Model(&current).Updates(map[string]any{"commission_quota": reduced, "pool_deduction": diff}).Error; e != nil {
					return e
				}
				_, _, carryEnd := invitePeriod(end, current.InviterTierSnapshot)
				carry := InvitePoolCarry{LedgerId: current.Id, UserId: current.InviterUserId, Amount: diff, NextAt: carryEnd.AddDate(0, 0, 3).Unix(), PeriodStart: end.Unix()}
				if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&carry).Error; e != nil {
					return e
				}
				kind, key, _ := invitePeriod(end, current.InviterTierSnapshot)
				st := InviteCommissionStatement{InviterUserId: current.InviterUserId, PeriodType: kind, PeriodKey: key, Status: 1}
				if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&st).Error; e != nil {
					return e
				}
				if e := tx.Model(&InviteCommissionStatement{}).Where("inviter_user_id = ? AND period_type = ? AND period_key = ?", st.InviterUserId, kind, key).UpdateColumn("carry_over_amount", gorm.Expr("carry_over_amount + ?", diff)).Error; e != nil {
					return e
				}
				if current.StatementId > 0 {
					if e := refreshInviteStatementTx(tx, current.StatementId, now); e != nil {
						return e
					}
				}
			}
		}
		return tx.Model(&mark).Updates(map[string]any{"coefficient": coef.String(), "adjusted_at": now}).Error
	})
}

func ReleaseInviteCarry(now int64) error {
	var items []InvitePoolCarry
	if e := DB.Where("paid = ? AND next_at <= ?", false, now).Order("id").Limit(500).Find(&items).Error; e != nil {
		return e
	}
	for _, item := range items {
		if e := DB.Transaction(func(tx *gorm.DB) error {
			if _, e := lockInviteUser(tx, item.UserId); e != nil {
				return e
			}
			var original InviteCommissionLedger
			if e := tx.First(&original, item.LedgerId).Error; e != nil {
				return e
			}
			result := tx.Model(&InvitePoolCarry{}).Where("id = ? AND paid = ?", item.Id, false).Update("paid", true)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			if original.Status == InviteCommissionStatusSkipped && !(original.PoolDeduction > 0 && original.RiskReason == "") {
				return nil
			}
			l := InviteCommissionLedger{TopupTradeNo: fmt.Sprintf("invite-carry:%d", item.Id), SourceTradeNo: original.SourceTradeNo, InviteeUserId: original.InviteeUserId, InviterUserId: item.UserId, CommissionLevel: original.CommissionLevel, CommissionQuota: item.Amount, BaseQuota: item.Amount, AmountVersion: 2, Status: InviteCommissionStatusPending, ValidityState: "valid", CreatedAt: now, InviterTierSnapshot: original.InviterTierSnapshot, BizDate: InviteBusinessTime(time.Unix(item.NextAt, 0)).Format("2006-01-02")}
			scheduleInviteLedger(&l, item.PeriodStart)
			l.ObservedUntil = 0
			l.NextRetryAt = max(now, l.PayoutAt)
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&l).Error
		}); e != nil {
			return e
		}
	}
	return nil
}

func AdjustOutstandingInvitePools(now int64) error {
	current := InviteBusinessTime(time.Unix(now, 0)).Format("2006-01")
	var months []string
	if e := DB.Model(&InviteCommissionAllocation{}).Distinct("month").Where("month < ?", current).Order("month").Pluck("month", &months).Error; e != nil {
		return e
	}
	for _, month := range months {
		if e := AdjustInvitePoolMonth(month, now); e != nil {
			return e
		}
	}
	return nil
}
