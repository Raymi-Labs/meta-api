package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"strings"
	"time"
)

type inviteActionSnapshot struct {
	Profile   PromoterProfile `json:"profile"`
	Children  []int           `json:"children"`
	Recovered int64           `json:"recovered"`
	DebtAdded int64           `json:"debt_added"`
}

func ChangeInvitePromoter(user, admin int, kind string, tier int, expires int64, freeze bool, amount int64, remark string) (*InviteAdminAction, error) {
	if len([]rune(remark)) > 255 || user <= 0 {
		return nil, ErrInviteInvalid
	}
	var action InviteAdminAction
	err := DB.Transaction(func(tx *gorm.DB) error {
		if kind == "clear" {
			if e := lockInviteGraphTx(tx); e != nil {
				return e
			}
		}
		u, e := lockInviteUser(tx, user)
		if e != nil {
			return e
		}
		p, e := ensurePromoterTx(tx, user)
		if e != nil {
			return e
		}
		before := inviteActionSnapshot{Profile: *p}
		switch kind {
		case "tier":
			if tier < 0 || tier > 5 || (expires > 0 && expires <= common.GetTimestamp()) {
				return ErrInviteInvalid
			}
			p.OverrideActive = true
			p.AdminOverrideTier = tier
			p.AdminOverrideExpiresAt = expires
			p.AdminOverrideAssignedBy = admin
		case "freeze":
			p.WithdrawalFrozen = freeze
			if !freeze {
				if e := resumeInvitePendingTx(tx, user, "payout_frozen"); e != nil {
					return e
				}
			}
			p.WithdrawalFrozenAt = common.GetTimestamp()
		case "clear":
			if p.Cleared {
				return ErrInviteConflict
			}
			if e := tx.Model(&User{}).Where("inviter_id = ?", user).Order("id").Pluck("id", &before.Children).Error; e != nil {
				return e
			}
			if e := tx.Model(&User{}).Where("inviter_id = ?", user).Update("inviter_id", 0).Error; e != nil {
				return e
			}
			if e := tx.Model(u).Update("aff_count", 0).Error; e != nil {
				return e
			}
			p.Cleared = true
			p.WithdrawalFrozen = true
		case "clawback":
			if amount <= 0 {
				return ErrInviteInvalid
			}
			aff := min(amount, max(int64(0), u.AffQuota))
			wallet := min(amount-aff, int64(max(0, EffectiveTransferableQuota(u.Quota, u.TransferableQuota))))
			before.Recovered = aff + wallet
			before.DebtAdded = amount - before.Recovered
			if wallet > 0 {
				if e := InvalidatePulsePaidFundingTx(tx, user, "commission_clawback"); e != nil {
					return e
				}
			}
			if e := tx.Model(u).Updates(map[string]any{"aff_quota": u.AffQuota - aff, "quota": u.Quota - int(wallet), "transferable_quota": EffectiveTransferableQuota(u.Quota-int(wallet), max(0, u.TransferableQuota-int(wallet)))}).Error; e != nil {
				return e
			}
			if e := addInviteDebtTx(tx, user, before.DebtAdded, "admin"); e != nil {
				return e
			}
		default:
			return ErrInviteInvalid
		}
		if e := tx.Save(p).Error; e != nil {
			return e
		}
		raw, e := common.Marshal(before)
		if e != nil {
			return e
		}
		action = InviteAdminAction{UserId: user, AdminId: admin, Kind: kind, Before: string(raw), Remark: remark}
		return tx.Create(&action).Error
	})
	return &action, err
}
func ReverseInviteAdminAction(id, admin int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var action InviteAdminAction
		if e := tx.First(&action, id).Error; e != nil {
			return e
		}
		if action.Kind == "clear" {
			if e := lockInviteGraphTx(tx); e != nil {
				return e
			}
		}
		if _, e := lockInviteUser(tx, action.UserId); e != nil {
			return e
		}
		if e := inviteLock(tx).First(&action, id).Error; e != nil {
			return e
		}
		if action.Reversed {
			return ErrInviteConflict
		}
		var newer int64
		if e := tx.Model(&InviteAdminAction{}).Where("user_id = ? AND id > ? AND reversed = ?", action.UserId, id, false).Count(&newer).Error; e != nil {
			return e
		}
		if newer > 0 {
			return ErrInviteConflict
		}
		var before inviteActionSnapshot
		if e := common.UnmarshalJsonStr(action.Before, &before); e != nil {
			return e
		}
		if action.Kind == "clawback" {
			remaining := int64(0)
			var debt InviteCommissionDebt
			e := inviteLock(tx).Where("inviter_user_id = ?", action.UserId).First(&debt).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if e == nil {
				remaining = min(before.DebtAdded, max(int64(0), debt.DebtAmount-debt.PaidAmount))
				debt.DebtAmount -= remaining
				if debt.DebtAmount <= debt.PaidAmount {
					if e := tx.Delete(&debt).Error; e != nil {
						return e
					}
				} else if e := tx.Save(&debt).Error; e != nil {
					return e
				}
			}
			if e := tx.Model(&User{}).Where("id = ?", action.UserId).UpdateColumn("aff_quota", gorm.Expr("aff_quota + ?", before.Recovered+before.DebtAdded-remaining)).Error; e != nil {
				return e
			}
		} else {
			current, e := ensurePromoterTx(tx, action.UserId)
			if e != nil {
				return e
			}
			switch action.Kind {
			case "tier":
				current.OverrideActive = before.Profile.OverrideActive
				current.AdminOverrideTier = before.Profile.AdminOverrideTier
				current.AdminOverrideExpiresAt = before.Profile.AdminOverrideExpiresAt
				current.AdminOverrideAssignedBy = before.Profile.AdminOverrideAssignedBy
			case "freeze":
				current.WithdrawalFrozen = before.Profile.WithdrawalFrozen
				current.WithdrawalFrozenAt = before.Profile.WithdrawalFrozenAt
			case "clear":
				current.Cleared = before.Profile.Cleared
				current.WithdrawalFrozen = before.Profile.WithdrawalFrozen
			}
			if e := tx.Save(current).Error; e != nil {
				return e
			}
			if action.Kind == "clear" {
				for _, child := range before.Children {
					// A cleared descendant may since have become this promoter's ancestor.
					seen := map[int]bool{}
					node := action.UserId
					for node > 0 {
						if node == child || seen[node] {
							return ErrInviteConflict
						}
						seen[node] = true
						var ancestor User
						if e := tx.First(&ancestor, node).Error; e != nil {
							return e
						}
						node = ancestor.InviterId
					}

					if e := tx.Model(&User{}).Where("id = ? AND inviter_id = 0", child).Update("inviter_id", action.UserId).Error; e != nil {
						return e
					}
				}
				var n int64
				if e := tx.Model(&User{}).Where("inviter_id = ?", action.UserId).Count(&n).Error; e != nil {
					return e
				}
				if e := tx.Model(&User{}).Where("id = ?", action.UserId).Update("aff_count", n).Error; e != nil {
					return e
				}
			}
		}
		return tx.Model(&action).Update("reversed", true).Error
	})
}
func SetInviteStatementFrozen(id int, frozen bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var st InviteCommissionStatement
		if e := tx.First(&st, id).Error; e != nil {
			return e
		}
		if _, e := lockInviteUser(tx, st.InviterUserId); e != nil {
			return e
		}
		if e := inviteLock(tx).First(&st, id).Error; e != nil {
			return e
		}
		if frozen {
			if st.Status == 3 {
				return ErrInviteConflict
			}
			return tx.Model(&st).Update("status", 4).Error
		}
		if st.Status != 4 {
			return ErrInviteConflict
		}
		status := 1
		if st.IssuedAt > 0 {
			status = 2
		}
		if e := tx.Model(&st).Update("status", status).Error; e != nil {
			return e
		}
		if e := tx.Model(&InviteCommissionLedger{}).Where("statement_id = ? AND status = ? AND risk_reason = ?", id, InviteCommissionStatusPending, "statement_frozen").Update("next_retry_at", common.GetTimestamp()).Error; e != nil {
			return e
		}
		return refreshInviteStatementTx(tx, id, common.GetTimestamp())
	})
}
func ReviewPromoterIdentity(user, admin int, approve bool, remark string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, e := lockInviteUser(tx, user); e != nil {
			return e
		}
		var identity PromoterIdentity
		if e := tx.Where("user_id = ?", user).First(&identity).Error; e != nil {
			return e
		}
		if approve {
			var n int64
			if e := tx.Model(&InviteRiskSignal{}).Where("user_id = ? AND kind = ? AND related_user_id > 0", user, "identity").Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return ErrInviteEligibility
			}
		}
		status := "rejected"
		if approve {
			status = "approved"
			if e := resumeInvitePendingTx(tx, user, "identity_pending"); e != nil {
				return e
			}
		}
		return tx.Model(&identity).Updates(map[string]any{"status": status, "reviewer_id": admin, "remark": strings.TrimSpace(remark)}).Error
	})
}
func InviteWithdrawalMinimum(tx *gorm.DB, user int) (int64, error) {
	if !common.GetInviteTierSettings().Enabled {
		return AffWithdrawalMinimumAmountCents, nil
	}
	tier, e := getTierForCommission(tx, user, InviteBusinessTime(time.Now()).Format("2006-01"))
	if e != nil {
		return 0, e
	}
	if tier < 0 || tier > 5 {
		return 0, ErrInviteInvalid
	}
	return int64(common.GetInviteTierSettings().MinWithdrawalAmounts[tier]), nil
}

func InviteAdminSubject(action string, id int) (int, error) {
	var subject int
	query := DB
	switch action {
	case "application":
		query = query.Model(&PromoterApplication{}).Select("user_id")
	case "early":
		query = query.Model(&InviteEarlyPayout{}).Select("user_id")
	case "statement":
		query = query.Model(&InviteCommissionStatement{}).Select("inviter_user_id")
	case "reverse":
		query = query.Model(&InviteAdminAction{}).Select("user_id")
	default:
		return id, nil
	}
	e := query.Where("id = ?", id).Scan(&subject).Error
	if e == nil && subject <= 0 {
		return 0, ErrInviteInvalid
	}
	return subject, e
}

func resumeInvitePendingTx(tx *gorm.DB, user int, reason string) error {
	return tx.Model(&InviteCommissionLedger{}).Where("inviter_user_id = ? AND amount_version = 2 AND status = ? AND risk_reason = ?", user, InviteCommissionStatusPending, reason).Update("next_retry_at", common.GetTimestamp()).Error
}
