package model

import (
	"errors"
	"gorm.io/gorm"
)

// The new unique index must exist before removing the old, more restrictive one.
// Never delete or repair financial rows implicitly during startup.
func migrateInviteCommissionUniqueIndex(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&InviteCommissionLedger{}) {
		return nil
	}
	const old = "idx_invite_commission_trade_inviter"
	const replacement = "idx_invite_commission_trade_inviter_level"
	var duplicate struct{ Count int64 }
	err := db.Model(&InviteCommissionLedger{}).Select("COUNT(*) AS count").Group("topup_trade_no, inviter_user_id, commission_level").Having("COUNT(*) > 1").Limit(1).Scan(&duplicate).Error
	if err != nil {
		return err
	}
	if duplicate.Count > 1 {
		return errors.New("duplicate commission rows: inspect GROUP BY topup_trade_no, inviter_user_id, commission_level HAVING COUNT(*) > 1; migration refused")
	}
	if !m.HasIndex(&InviteCommissionLedger{}, replacement) {
		if err = m.CreateIndex(&InviteCommissionLedger{}, replacement); err != nil {
			return err
		}
	}
	if m.HasIndex(&InviteCommissionLedger{}, old) {
		return m.DropIndex(&InviteCommissionLedger{}, old)
	}
	return nil
}
