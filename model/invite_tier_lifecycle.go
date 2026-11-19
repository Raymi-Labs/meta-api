package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInviteEligibility = errors.New("invite.eligibility")
var ErrInviteConflict = errors.New("invite.conflict")
var ErrInviteInvalid = errors.New("invite.invalid")
var ErrInviteFrozen = errors.New("invite.frozen")
var ErrInviteIdentity = errors.New("invite.identity_pending")
var ErrInviteMinimum = errors.New("invite.minimum")

func InviteBusinessTime(t time.Time) time.Time { return t.In(inviteBusinessLocation) }
func inviteLock(tx *gorm.DB) *gorm.DB {
	if !common.UsingSQLite {
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx
}
func lockInviteUser(tx *gorm.DB, id int) (*User, error) {
	var u User
	err := inviteLock(tx).First(&u, id).Error
	return &u, err
}

// Period boundaries are exclusive. ISO year is used, not calendar year.
func invitePeriod(t time.Time, tier int) (kind int, key string, end time.Time) {
	t = InviteBusinessTime(t)
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, inviteBusinessLocation)
	if tier >= 3 {
		start = start.AddDate(0, 0, -(int(start.Weekday())+6)%7)
		year, week := start.ISOWeek()
		return 1, fmt.Sprintf("%04d-W%02d", year, week), start.AddDate(0, 0, 7)
	}
	return 2, t.Format("2006-01"), time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, inviteBusinessLocation)
}
func scheduleInviteLedger(l *InviteCommissionLedger, completed int64) {
	kind, key, end := invitePeriod(time.Unix(completed, 0), l.InviterTierSnapshot)
	l.SettlementCycle = kind
	l.PeriodKey = key
	l.IssueAt = end.AddDate(0, 0, 3).Unix()
	l.PayoutAt = l.IssueAt
	if kind == 1 {
		l.PayoutAt = end.AddDate(0, 0, 7).Unix()
	}
	l.ObservedUntil = completed + 7*86400
	l.NextRetryAt = max(l.PayoutAt, l.ObservedUntil)
}
func inviteValidity(u *User, cents, now int64) string {
	count := 0
	if cents >= 100 {
		count++
	}
	if u.CreatedAt > 0 && now >= u.CreatedAt+7*86400 {
		count++
	}
	if u.UsedQuota > 0 {
		count++
	}
	if count >= 2 {
		return "valid"
	}
	if count == 1 && u.CreatedAt > 0 && now < u.CreatedAt+7*86400 {
		return "pending_time"
	}
	if count == 0 && u.CreatedAt > 0 && now < u.CreatedAt+7*86400 {
		return "pending_usage"
	}
	return "invalid"
}
func checkInvitePayoutEligibility(tx *gorm.DB, id int) error {
	var p PromoterProfile
	err := tx.Where("user_id = ?", id).First(&p).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if p.WithdrawalFrozen || p.Cleared {
		return ErrInviteFrozen
	}
	if common.GetInviteTierSettings().RequireIdentityForPayout {
		var n int64
		if err := tx.Model(&PromoterIdentity{}).Where("user_id = ? AND status = ?", id, "approved").Count(&n).Error; err != nil {
			return err
		}
		if n != 1 {
			return ErrInviteIdentity
		}
	}
	return nil
}

type PromoterApplication struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id" gorm:"index;not null"`
	Tier         int    `json:"tier"`
	DepartmentId int    `json:"department_id"`
	Status       string `json:"status" gorm:"type:varchar(20);index"`
	Remark       string `json:"remark" gorm:"type:varchar(255)"`
	ReviewerId   int    `json:"reviewer_id"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime"`
	ReviewedAt   int64  `json:"reviewed_at"`
}
type InviteEarlyPayout struct {
	Id         int    `json:"id"`
	UserId     int    `json:"user_id" gorm:"uniqueIndex:idx_invite_early_week"`
	WeekKey    string `json:"week_key" gorm:"type:varchar(10);uniqueIndex:idx_invite_early_week"`
	Status     string `json:"status" gorm:"type:varchar(20);index"`
	Remark     string `json:"remark" gorm:"type:varchar(255)"`
	ReviewerId int    `json:"reviewer_id"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
	ReviewedAt int64  `json:"reviewed_at"`
}
type InvitePoolMonth struct {
	Month       string `json:"month" gorm:"primaryKey;type:varchar(7)"`
	Coefficient string `json:"coefficient" gorm:"type:varchar(64)"`
	AdjustedAt  int64  `json:"adjusted_at"`
}
type InvitePoolCarry struct {
	PeriodStart int64 `json:"period_start" gorm:"not null;default:0"`
	Id          int   `json:"id"`
	LedgerId    int   `json:"ledger_id" gorm:"uniqueIndex"`
	UserId      int   `json:"user_id" gorm:"index"`
	Amount      int64 `json:"amount" gorm:"type:bigint"`
	NextAt      int64 `json:"next_at" gorm:"index"`
	Paid        bool  `json:"paid" gorm:"not null;default:false"`
}

func inviteCommissionLevelLabel(level int) string {
	labels := []string{"", "一级", "二级", "达标奖金", "部门团队分成", "成员部长位", "注册奖励"}
	if level < 1 || level >= len(labels) {
		return "邀请"
	}
	return labels[level]
}

var inviteStatementWakeup = make(chan struct{}, 1)

func InviteStatementWakeup() <-chan struct{} { return inviteStatementWakeup }
func NotifyInviteStatements() {
	select {
	case inviteStatementWakeup <- struct{}{}:
	default:
	}
}
