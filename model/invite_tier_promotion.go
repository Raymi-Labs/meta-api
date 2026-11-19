package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"time"
)

type InviteRebind struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id" gorm:"uniqueIndex;not null"`
	OldInviterId int    `json:"old_inviter_id"`
	NewInviterId int    `json:"new_inviter_id" gorm:"index"`
	Status       string `json:"status" gorm:"type:varchar(20)"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime"`
}
type InviteAdminAction struct {
	Id        int    `json:"id"`
	UserId    int    `json:"user_id" gorm:"index"`
	AdminId   int    `json:"admin_id"`
	Kind      string `json:"kind" gorm:"type:varchar(24)"`
	Before    string `json:"-" gorm:"type:text"`
	Remark    string `json:"remark" gorm:"type:varchar(255)"`
	Reversed  bool   `json:"reversed" gorm:"not null;default:false"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func ensurePromoterTx(tx *gorm.DB, id int) (*PromoterProfile, error) {
	p := PromoterProfile{UserId: id}
	if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; e != nil {
		return nil, e
	}
	e := tx.Where("user_id = ?", id).First(&p).Error
	return &p, e
}
func ApplyPromoter(user, tier, dept int) (*PromoterApplication, error) {
	if tier < 3 || tier > 5 {
		return nil, ErrInviteInvalid
	}
	var row PromoterApplication
	e := DB.Transaction(func(tx *gorm.DB) error {
		u, e := lockInviteUser(tx, user)
		if e != nil {
			return e
		}
		if u.Status != common.UserStatusEnabled {
			return ErrInviteEligibility
		}
		p, e := ensurePromoterTx(tx, user)
		if e != nil {
			return e
		}
		if p.Cleared || p.WithdrawalFrozen {
			return ErrInviteFrozen
		}
		current, e := getTierForCommission(tx, user, InviteBusinessTime(time.Now()).Format("2006-01"))
		if e != nil {
			return e
		}
		if tier == 4 && current < 2 {
			return ErrInviteEligibility
		}
		if tier == 4 {
			var d Department
			if e := tx.First(&d, dept).Error; e != nil {
				return e
			}
			if d.LeaderUserId == user {
				return ErrInviteInvalid
			}
		}
		var n int64
		if e := tx.Model(&PromoterApplication{}).Where("user_id = ? AND status = ?", user, "pending").Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return ErrInviteConflict
		}
		row = PromoterApplication{UserId: user, Tier: tier, DepartmentId: dept, Status: "pending"}
		return tx.Create(&row).Error
	})
	return &row, e
}
func ReviewPromoterApplication(id, admin int, approve bool, remark string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var a PromoterApplication
		if e := tx.First(&a, id).Error; e != nil {
			return e
		}
		if _, e := lockInviteUser(tx, a.UserId); e != nil {
			return e
		}
		if e := inviteLock(tx).First(&a, id).Error; e != nil {
			return e
		}
		if a.Status != "pending" {
			return ErrInviteConflict
		}
		a.Status = "rejected"
		a.Remark = remark
		a.ReviewerId = admin
		a.ReviewedAt = common.GetTimestamp()
		if approve {
			p, e := ensurePromoterTx(tx, a.UserId)
			if e != nil {
				return e
			}
			if p.Cleared || p.WithdrawalFrozen {
				return ErrInviteEligibility
			}
			month := InviteBusinessTime(time.Now()).Format("2006-01")
			if a.Tier == 4 {
				current, e := getTierForCommission(tx, a.UserId, month)
				if e != nil {
					return e
				}
				if current < 2 {
					return ErrInviteEligibility
				}
				var d Department
				if e := tx.First(&d, a.DepartmentId).Error; e != nil {
					return e
				}
				if d.LeaderUserId == a.UserId {
					return ErrInviteInvalid
				}
				member := DepartmentMember{UserId: a.UserId, DepartmentId: a.DepartmentId, JoinedAt: common.GetTimestamp()}
				if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"department_id": a.DepartmentId, "joined_at": member.JoinedAt, "left_at": 0, "leave_effective_at": 0})}).Create(&member).Error; e != nil {
					return e
				}
				p.DepartmentId = a.DepartmentId
			}
			if a.Tier != 4 {
				if e := tx.Model(&DepartmentMember{}).Where("user_id = ? AND left_at = 0", a.UserId).Update("left_at", common.GetTimestamp()).Error; e != nil {
					return e
				}
				p.DepartmentId = 0
			}
			if a.Tier == 5 {
				var n int64
				if e := tx.Model(&Department{}).Where("leader_user_id = ?", a.UserId).Count(&n).Error; e != nil {
					return e
				}
				if n == 0 {
					if e := tx.Create(&Department{LeaderUserId: a.UserId, Name: remark}).Error; e != nil {
						return e
					}
				}
			}
			p.ManualTier = a.Tier
			p.ManualTierEffectiveMonth = month
			p.ExemptAssessment = false
			if a.Tier >= 4 {
				p.UpstreamCut = true
				p.UpstreamCutAt = common.GetTimestamp()
			}
			if e := tx.Save(p).Error; e != nil {
				return e
			}
			a.Status = "approved"
		}
		return tx.Save(&a).Error
	})
}
func LeaveInviteDepartment(user int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, e := lockInviteUser(tx, user); e != nil {
			return e
		}
		now := InviteBusinessTime(time.Now())
		next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, inviteBusinessLocation).Unix()
		r := tx.Model(&DepartmentMember{}).Where("user_id = ? AND left_at = 0 AND leave_effective_at = 0", user).Update("leave_effective_at", next)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrInviteConflict
		}
		return nil
	})
}
func MaintainInvitePromoters(now int64) error {
	var members []DepartmentMember
	if e := DB.Where("left_at = 0").Find(&members).Error; e != nil {
		return e
	}
	for _, m := range members {
		if e := DB.Transaction(func(tx *gorm.DB) error {
			if _, e := lockInviteUser(tx, m.UserId); e != nil {
				return e
			}
			if e := tx.First(&m, m.Id).Error; e != nil {
				return e
			}
			if m.LeftAt != 0 {
				return nil
			}
			leave := m.LeaveEffectiveAt > 0 && m.LeaveEffectiveAt <= now
			if now-m.JoinedAt >= 30*86400 {
				var n int64
				if e := tx.Model(&InviteeQualification{}).Where("inviter_id = ? AND status = 1 AND qualified_at > ?", m.UserId, now-30*86400).Count(&n).Error; e != nil {
					return e
				}
				leave = leave || n == 0
			}
			if !leave {
				return nil
			}
			if e := tx.Model(&m).Update("left_at", now).Error; e != nil {
				return e
			}
			return tx.Model(&PromoterProfile{}).Where("user_id = ?", m.UserId).Updates(map[string]any{"manual_tier": 3, "department_id": 0, "exempt_assessment": true, "upstream_cut": true}).Error
		}); e != nil {
			return e
		}
	}
	return nil
}
func RequestInviteEarlyPayout(user int) (*InviteEarlyPayout, error) {
	var a InviteEarlyPayout
	e := DB.Transaction(func(tx *gorm.DB) error {
		if _, e := lockInviteUser(tx, user); e != nil {
			return e
		}
		if e := checkInvitePayoutEligibility(tx, user); e != nil {
			return e
		}
		tier, e := getTierForCommission(tx, user, InviteBusinessTime(time.Now()).Format("2006-01"))
		if e != nil {
			return e
		}
		if tier < 3 {
			return ErrInviteEligibility
		}
		var debts int64
		if e := tx.Model(&InviteCommissionDebt{}).Where("inviter_user_id = ? AND debt_amount > paid_amount", user).Count(&debts).Error; e != nil {
			return e
		}
		if debts > 0 {
			return ErrInviteEligibility
		}
		var n int64
		if e := tx.Model(&InviteCommissionLedger{}).Where("inviter_user_id = ? AND amount_version = 2 AND status = ? AND statement_id > 0 AND observed_until <= ?", user, InviteCommissionStatusPending, common.GetTimestamp()).Count(&n).Error; e != nil {
			return e
		}
		if n == 0 {
			return ErrInviteEligibility
		}
		_, key, _ := invitePeriod(time.Now(), 3)
		a = InviteEarlyPayout{UserId: user, WeekKey: key, Status: "pending"}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&a)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return ErrInviteConflict
		}
		return nil
	})
	return &a, e
}
func ReviewInviteEarlyPayout(id, admin int, approve bool, remark string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var a InviteEarlyPayout
		if e := tx.First(&a, id).Error; e != nil {
			return e
		}
		if _, e := lockInviteUser(tx, a.UserId); e != nil {
			return e
		}
		if e := inviteLock(tx).First(&a, id).Error; e != nil {
			return e
		}
		if a.Status != "pending" {
			return ErrInviteConflict
		}
		a.Status = "rejected"
		if approve {
			if e := checkInvitePayoutEligibility(tx, a.UserId); e != nil {
				return e
			}
			tier, e := getTierForCommission(tx, a.UserId, InviteBusinessTime(time.Now()).Format("2006-01"))
			if e != nil {
				return e
			}
			if tier < 3 {
				return ErrInviteEligibility
			}
			var debt int64
			if e := tx.Model(&InviteCommissionDebt{}).Where("inviter_user_id = ? AND debt_amount > paid_amount", a.UserId).Count(&debt).Error; e != nil {
				return e
			}
			if debt > 0 {
				return ErrInviteEligibility
			}
			now := common.GetTimestamp()
			if e := tx.Model(&InviteCommissionLedger{}).Where("inviter_user_id = ? AND amount_version = 2 AND status = ? AND statement_id IN (?) AND observed_until <= ? AND created_at <= ?", a.UserId, InviteCommissionStatusPending, tx.Model(&InviteCommissionStatement{}).Select("id").Where("status = 2"), now, a.CreatedAt).Updates(map[string]any{"early_approved": true, "next_retry_at": now}).Error; e != nil {
				return e
			}
			a.Status = "approved"
		}
		a.Remark = remark
		a.ReviewerId = admin
		a.ReviewedAt = common.GetTimestamp()
		return tx.Save(&a).Error
	})
}
func RequestInviteRebind(user, newInviter int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		u, e := lockInviteUser(tx, user)
		if e != nil {
			return e
		}
		if newInviter <= 0 || newInviter == user || common.GetTimestamp() > u.CreatedAt+30*86400 {
			return ErrInviteEligibility
		}
		var p PromoterProfile
		e = tx.Where("user_id = ?", user).First(&p).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if p.UpstreamCut {
			return ErrInviteEligibility
		}
		row := InviteRebind{UserId: user, OldInviterId: u.InviterId, NewInviterId: newInviter, Status: "pending"}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return ErrInviteConflict
		}
		return nil
	})
}

// Serialize relation rewrites on a durable global row, then lock users in ID order.
func ConfirmInviteRebind(id, inviter int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if e := lockInviteGraphTx(tx); e != nil {
			return e
		}
		var a InviteRebind
		if e := tx.First(&a, id).Error; e != nil {
			return e
		}
		if a.NewInviterId != inviter || a.Status != "pending" {
			return ErrInviteEligibility
		}
		ids := []int{a.UserId, inviter}
		if a.OldInviterId > 0 {
			ids = append(ids, a.OldInviterId)
		}
		sort.Ints(ids)
		for _, uid := range ids {
			if _, e := lockInviteUser(tx, uid); e != nil {
				return e
			}
		}
		var u User
		if e := tx.First(&u, a.UserId).Error; e != nil {
			return e
		}
		if common.GetTimestamp() > u.CreatedAt+30*86400 || u.InviterId != a.OldInviterId {
			return ErrInviteConflict
		}
		var p PromoterProfile
		e := tx.Where("user_id = ?", u.Id).First(&p).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if p.UpstreamCut {
			return ErrInviteEligibility
		}
		seen := map[int]bool{u.Id: true}
		node := inviter
		for depth := 1; node > 0; depth++ {
			if depth > 3 || seen[node] {
				return ErrInviteInvalid
			}
			seen[node] = true
			var parent User
			if e := tx.First(&parent, node).Error; e != nil {
				return e
			}
			if parent.Status != common.UserStatusEnabled {
				return ErrInviteEligibility
			}
			node = parent.InviterId
		}
		if e := tx.Model(&User{}).Where("id = ?", u.Id).Update("inviter_id", inviter).Error; e != nil {
			return e
		}
		if u.InviterId > 0 {
			if e := tx.Model(&User{}).Where("id = ? AND aff_count > 0", u.InviterId).UpdateColumn("aff_count", gorm.Expr("aff_count - 1")).Error; e != nil {
				return e
			}
		}
		if e := tx.Model(&User{}).Where("id = ?", inviter).UpdateColumn("aff_count", gorm.Expr("aff_count + 1")).Error; e != nil {
			return e
		}
		return tx.Model(&a).Update("status", "approved").Error
	})
}

func lockInviteGraphTx(tx *gorm.DB) error {
	guard := InviteSubject{Hash: "invite-rebind-lock", Kind: "lock"}
	if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard).Error; e != nil {
		return e
	}
	return inviteLock(tx).First(&guard, "hash = ?", guard.Hash).Error
}
