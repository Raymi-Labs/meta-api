package model

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var inviteBusinessLocation = time.FixedZone("Asia/Shanghai", 8*3600)

// InviteCommissionAllocation is the transaction receipt, including zero-award
// payments. A callback must never reprice an existing allocation after promotion.
type InviteCommissionAllocation struct {
	TradeNo       string `gorm:"primaryKey;type:varchar(255)"`
	SecondUserID  int
	InviteeUserID int
	Month         string `gorm:"type:varchar(7)"`
	BaseCents     int64  `gorm:"type:bigint;not null;default:0"`
	BaseQuota     int64  `gorm:"type:bigint;not null;default:0"`
	Reversed      bool   `gorm:"not null;default:false"`
}

func calculateTopUpBaseQuota(topUp *TopUp) (int64, error) {
	if topUp == nil || topUp.Status != common.TopUpStatusSuccess {
		return 0, errors.New("topup not paid")
	}
	money, ok := topUp.CNYPaymentAmount()
	if !ok {
		return 0, errors.New("missing paid CNY amount")
	}
	return inviteBaseQuota(money)
}
func inviteBaseQuota(money float64) (int64, error) {
	if money <= 0 || math.IsNaN(money) || math.IsInf(money, 0) || operation_setting.Price <= 0 || math.IsNaN(operation_setting.Price) || math.IsInf(operation_setting.Price, 0) || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return 0, errors.New("invalid commission amount settings")
	}
	q := decimal.NewFromFloat(money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Div(decimal.NewFromFloat(operation_setting.Price)).Floor()
	if q.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return 0, errors.New("commission amount overflow")
	}
	return q.IntPart(), nil
}

func enqueueTierBasedCommission(tx *gorm.DB, trade string, inviteeID int, completed int64, base int64, paidCNY decimal.Decimal) error {
	s := common.GetInviteTierSettings()
	if !s.Enabled {
		return nil
	}
	if completed <= 0 {
		completed = common.GetTimestamp()
	}
	month := time.Unix(completed, 0).In(inviteBusinessLocation).Format("2006-01")
	receipt := InviteCommissionAllocation{TradeNo: trade, InviteeUserID: inviteeID, Month: month}
	inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt)
	if inserted.Error != nil {
		return inserted.Error
	}
	if inserted.RowsAffected == 0 {
		return nil
	}
	var user User
	if err := tx.First(&user, inviteeID).Error; err != nil {
		return err
	}
	if user.InviterId <= 0 || user.InviterId == user.Id {
		return nil
	}
	var first User
	if err := tx.First(&first, user.InviterId).Error; err != nil {
		return err
	}
	tier, err := getTierForCommission(tx, first.Id, month)
	if err != nil {
		return err
	}
	if tier < 0 || tier > 5 {
		return fmt.Errorf("invalid tier %d", tier)
	}
	// Serialize qualification and once-only bonus decisions across different payments.
	guardLock := tx.Model(&User{}).Where("id = ?", inviteeID).UpdateColumn("inviter_id", gorm.Expr("inviter_id"))
	if guardLock.Error != nil {
		return guardLock.Error
	}
	appendRow := func(receiver, level, snapshot int, rate float64, amountBase int64) error {
		if receiver <= 0 || receiver == inviteeID || rate <= 0 {
			return nil
		}
		quota := decimal.NewFromInt(amountBase).Mul(decimal.NewFromFloat(rate)).Floor().IntPart()
		if quota <= 0 {
			return nil
		}
		directInvitee := first.Id
		if level == InviteCommissionLevelDirect || level == InviteCommissionLevelBonus {
			directInvitee = inviteeID
		}
		row := InviteCommissionLedger{TopupTradeNo: trade, InviteeUserId: inviteeID, InviterUserId: receiver, DirectInviteeUserId: directInvitee, CommissionLevel: level, BaseQuota: amountBase, CommissionRate: rate, CommissionQuota: quota, InviterTierSnapshot: snapshot, InviteeTierSnapshot: tier, AmountVersion: 2, BizDate: time.Unix(completed, 0).In(inviteBusinessLocation).Format("2006-01-02"), Status: InviteCommissionStatusPending, CreatedAt: common.GetTimestamp()}
		return tx.Create(&row).Error
	}
	if err := appendRow(first.Id, 1, tier, s.FirstLevelRates[tier], base); err != nil {
		return err
	}
	bonusReserve := 0.0
	if tier == 3 || tier == 5 {
		bonusReserve = s.QualifiedBonusRate
		if paidCNY.GreaterThanOrEqual(decimal.NewFromFloat(s.QualifiedThresholdMoney)) && s.QualifiedBonusRate > 0 {
			guard := QualifiedBonusOnceGuard{InviteeUserId: inviteeID, InviterUserId: first.Id, FirstQualifiedTopupTradeNo: trade}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				if err := appendRow(first.Id, 3, tier, s.QualifiedBonusRate, base); err != nil {
					return err
				}
			}
		}
	}
	leaderFor := func(id int) (int, error) {
		var member DepartmentMember
		err := tx.Where("user_id = ? AND left_at = 0", id).First(&member).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		var dept Department
		if err = tx.First(&dept, member.DepartmentId).Error; err != nil {
			return 0, err
		}
		return dept.LeaderUserId, nil
	}
	if tier == 4 {
		leader, err := leaderFor(first.Id)
		if err != nil {
			return err
		}
		if leader == first.Id {
			return errors.New("department leader cannot be its own member")
		}
		if leader > 0 {
			lt, err := getTierForCommission(tx, leader, month)
			if err != nil {
				return err
			}
			if err = appendRow(leader, 4, lt, s.DepartmentCommissionRate, base); err != nil {
				return err
			}
		}
	}
	var profile PromoterProfile
	err = tx.Where("user_id = ?", first.Id).First(&profile).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if tier >= 4 || profile.UpstreamCut || first.InviterId <= 0 || first.InviterId == first.Id || first.InviterId == inviteeID {
		return nil
	}
	second := first.InviterId
	identity, err := getTierForCommission(tx, second, month)
	if err != nil {
		return err
	}
	if identity < 0 || identity > 5 {
		return errors.New("invalid second tier")
	}
	rate := s.SecondLevelMatrix[identity][min(tier, 3)]
	leader, leaderRate := 0, 0.0
	if identity == 4 {
		leader, err = leaderFor(second)
		if err != nil {
			return err
		}
		if leader == second || leader == first.Id || leader == inviteeID {
			return errors.New("cyclic department reward")
		}
		if leader > 0 {
			leaderRate = s.LeaderSlotRates[min(tier, 3)]
		}
	}
	if decimal.NewFromFloat(rate).Add(decimal.NewFromFloat(leaderRate)).Add(decimal.NewFromFloat(s.FirstLevelRates[tier])).Add(decimal.NewFromFloat(bonusReserve)).GreaterThan(decimal.NewFromFloat(s.PoolRatio)) {
		return errors.New("commission pool capacity exceeded")
	}
	if rate+leaderRate <= 0 {
		return nil
	}
	// Cents are a base cap, not a cap on the resulting commission.
	cents := paidCNY.Mul(decimal.NewFromInt(100)).Floor().IntPart()
	usage := SecondLevelMonthlyUsage{InviterUserId: second, InviteeUserId: inviteeID, Month: month}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage).Error; err != nil {
		return err
	}
	q := tx.Where("inviter_user_id = ? AND invitee_user_id = ? AND month = ?", second, inviteeID, month)
	if !common.UsingSQLite {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&usage).Error; err != nil {
		return err
	}
	if s.SecondLevelMonthlyCapMoney > 0 {
		cap := decimal.NewFromFloat(s.SecondLevelMonthlyCapMoney).Mul(decimal.NewFromInt(100)).Floor().IntPart()
		cents = min(cents, max(int64(0), cap-usage.UsedBaseCents))
	}
	if cents <= 0 {
		return nil
	}
	secondBase := decimal.NewFromInt(base).Mul(decimal.NewFromInt(cents)).Div(paidCNY.Mul(decimal.NewFromInt(100))).Floor().IntPart()
	if err := appendRow(second, 2, identity, rate, secondBase); err != nil {
		return err
	}
	if leader > 0 {
		lt, err := getTierForCommission(tx, leader, month)
		if err != nil {
			return err
		}
		if err = appendRow(leader, 5, lt, leaderRate, secondBase); err != nil {
			return err
		}
	}
	if err := tx.Model(&SecondLevelMonthlyUsage{}).Where("id = ?", usage.Id).Updates(map[string]any{"used_base_cents": gorm.Expr("used_base_cents + ?", cents), "used_quota_amount": gorm.Expr("used_quota_amount + ?", secondBase)}).Error; err != nil {
		return err
	}
	return tx.Model(&InviteCommissionAllocation{}).Where("trade_no = ?", trade).Updates(map[string]any{"second_user_id": second, "base_cents": cents, "base_quota": secondBase}).Error
}

// Reversal releases the base once, even when both member and leader received it.
func reverseInviteTierAllocationTx(tx *gorm.DB, trade string) error {
	var receipt InviteCommissionAllocation
	q := tx.Where("trade_no = ?", trade)
	if !common.UsingSQLite {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if receipt.Reversed {
		return nil
	}
	if receipt.SecondUserID > 0 && receipt.BaseCents > 0 {
		result := tx.Model(&SecondLevelMonthlyUsage{}).Where("inviter_user_id = ? AND invitee_user_id = ? AND month = ? AND used_base_cents >= ?", receipt.SecondUserID, receipt.InviteeUserID, receipt.Month, receipt.BaseCents).Updates(map[string]any{"used_base_cents": gorm.Expr("used_base_cents - ?", receipt.BaseCents), "used_quota_amount": gorm.Expr("used_quota_amount - ?", receipt.BaseQuota)})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("monthly base reversal invariant failed")
		}
	}
	if err := tx.Where("first_qualified_topup_trade_no = ?", trade).Delete(&QualifiedBonusOnceGuard{}).Error; err != nil {
		return err
	}
	if err := tx.Model(&InviteeQualification{}).Where("source_topup_trade_no = ?", trade).Update("status", 0).Error; err != nil {
		return err
	}
	return tx.Model(&InviteCommissionAllocation{}).Where("trade_no = ?", trade).Update("reversed", true).Error
}
