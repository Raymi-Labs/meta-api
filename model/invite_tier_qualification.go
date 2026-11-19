package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func tryMarkInviteeQualified(tx *gorm.DB, inviteeUserId int, sourceTopupTradeNo string) error {
	if !common.GetInviteTierSettings().Enabled {
		return nil
	}
	var user User
	if err := tx.First(&user, inviteeUserId).Error; err != nil {
		return err
	}
	if user.InviterId <= 0 || user.InviterId == user.Id || (common.GetInviteTierSettings().ActivatedAt > 0 && user.CreatedAt < common.GetInviteTierSettings().ActivatedAt) {
		return nil
	}
	var related int64
	if e := tx.Model(&InviteRiskSignal{}).Where("user_id = ? AND related_user_id > 0", inviteeUserId).Count(&related).Error; e != nil {
		return e
	}
	if related > 0 {
		return nil
	}
	var orders []TopUp
	if err := tx.Where("user_id = ? AND status = ?", inviteeUserId, common.TopUpStatusSuccess).Find(&orders).Error; err != nil {
		return err
	}
	total := decimal.Zero
	for _, order := range orders {
		if money, ok := order.CNYPaymentAmount(); ok {
			total = total.Add(decimal.NewFromFloat(money))
		}
	}
	now := common.GetTimestamp()
	if total.LessThan(decimal.NewFromInt(1)) && !(user.CreatedAt > 0 && now >= user.CreatedAt+7*86400 && user.UsedQuota > 0) {
		return nil
	}
	qualification := InviteeQualification{InviteeId: inviteeUserId, InviterId: user.InviterId, SourceTopupTradeNo: sourceTopupTradeNo, QualifiedAt: now, QualifiedMonth: time.Unix(now, 0).In(inviteBusinessLocation).Format("2006-01"), Status: 1}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&qualification)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return tx.Model(&InviteeQualification{}).Where("invitee_id = ? AND status = 0", inviteeUserId).Updates(map[string]any{"status": 1, "inviter_id": user.InviterId, "source_topup_trade_no": sourceTopupTradeNo, "qualified_at": now, "qualified_month": qualification.QualifiedMonth}).Error
	}
	return createInviteRegisterReward(tx, &user, sourceTopupTradeNo, now)
}
func checkInviteeQualified(tx *gorm.DB, inviteeUserId int) (bool, error) {
	var count int64
	err := tx.Model(&InviteeQualification{}).Where("invitee_id = ? AND status = 1", inviteeUserId).Count(&count).Error
	return count > 0, err
}

func createInviteRegisterReward(tx *gorm.DB, u *User, source string, now int64) error {
	tier, e := getTierForCommission(tx, u.InviterId, InviteBusinessTime(time.Unix(now, 0)).Format("2006-01"))
	if e != nil {
		return e
	}
	if tier < 0 || tier > 5 {
		return ErrInviteInvalid
	}
	q := decimal.NewFromFloat(common.GetInviteTierSettings().RegisterRewards[tier]).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Round(0)
	if !q.BigInt().IsInt64() {
		return ErrInviteInvalid
	}
	if q.IntPart() <= 0 {
		return nil
	}
	l := InviteCommissionLedger{TopupTradeNo: fmt.Sprintf("invite-register:%d", u.Id), SourceTradeNo: source, InviteeUserId: u.Id, DirectInviteeUserId: u.Id, InviterUserId: u.InviterId, CommissionLevel: 6, CommissionQuota: q.IntPart(), BaseQuota: q.IntPart(), AmountVersion: 2, InviterTierSnapshot: tier, Status: InviteCommissionStatusPending, ValidityState: "valid", CreatedAt: now, BizDate: InviteBusinessTime(time.Unix(now, 0)).Format("2006-01-02")}
	scheduleInviteLedger(&l, now)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&l).Error
}

func RefreshInviteQualifications(now int64) error {
	s := common.GetInviteTierSettings()
	if !s.Enabled {
		return nil
	}
	last := 0
	for {
		var users []User
		if e := DB.Select("id").Where("id > ? AND inviter_id > 0 AND created_at >= ? AND created_at <= ? AND used_quota > 0", last, s.ActivatedAt, now-7*86400).Order("id").Limit(500).Find(&users).Error; e != nil {
			return e
		}
		if len(users) == 0 {
			return nil
		}
		for _, u := range users {
			last = u.Id
			if e := DB.Transaction(func(tx *gorm.DB) error {
				if _, e := lockInviteUser(tx, u.Id); e != nil {
					return e
				}
				return tryMarkInviteeQualified(tx, u.Id, "")
			}); e != nil {
				return e
			}
		}
	}
}
