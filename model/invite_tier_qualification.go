package model

import (
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
	if user.InviterId <= 0 || user.InviterId == user.Id {
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
	return nil
}
func checkInviteeQualified(tx *gorm.DB, inviteeUserId int) (bool, error) {
	var count int64
	err := tx.Model(&InviteeQualification{}).Where("invitee_id = ? AND status = 1", inviteeUserId).Count(&count).Error
	return count > 0, err
}
