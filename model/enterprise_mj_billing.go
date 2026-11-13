package model

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const (
	// Unknown includes a crash immediately before or after sending. Lack of a reply
	// is never evidence that the upstream did not perform billable work.
	MJEnterpriseUnknown  = "unknown"
	MJEnterpriseAccepted = "accepted"
	MJEnterpriseRejected = "rejected"
)

func (task *Midjourney) EnterpriseMJSubject() EnterpriseSubject {
	return EnterpriseSubject{EnterpriseId: task.EnterpriseId, MemberId: task.EnterpriseMemberId, UserId: task.UserId, TokenId: task.BillingTokenId}
}

func validEnterpriseMJ(task *Midjourney) bool {
	return task.EnterpriseMJSubject().valid() && task.BillingTokenId > 0 && task.BillingSource == EnterpriseQuotaSource && task.BillingRequestId != "" && len(task.BillingRequestId) <= 96 && task.Quota >= 0 && int64(task.Quota) <= enterpriseMaxAmount
}

// CreateEnterpriseMJIntent persists ownership before reserving/sending. A failure
// leaves no unowned upstream task, and an interrupted send is explicitly unknown.
func CreateEnterpriseMJIntent(task *Midjourney) error {
	if !validEnterpriseMJ(task) || task.Id != 0 {
		return ErrEnterpriseSubject
	}
	task.EnterpriseBillingState = MJEnterpriseUnknown
	task.EnterpriseInitialQuota = task.Quota
	return DB.Transaction(func(tx *gorm.DB) error {
		member, err := lockEnterpriseMemberTx(tx, task.EnterpriseMemberId)
		if err != nil {
			return err
		}
		if member.EnterpriseId != task.EnterpriseId || member.UserId != task.UserId {
			return ErrEnterpriseSubject
		}
		var count int64
		if err := tx.Model(&Midjourney{}).Where("billing_request_id = ?", task.BillingRequestId).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrEnterpriseReservation
		}
		return tx.Create(task).Error
	})
}

// SaveEnterpriseMJSubmission records the response before settlement. Repeated
// upstream task IDs in the same payer/channel are one billable task, even when
// the upstream returns code 1 instead of code 21. Duplicate intents settle zero.
func SaveEnterpriseMJSubmission(task *Midjourney, accepted bool) error {
	if !validEnterpriseMJ(task) || task.Id <= 0 {
		return ErrEnterpriseSubject
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		member, err := lockEnterpriseMemberTx(tx, task.EnterpriseMemberId)
		if err != nil {
			return err
		}
		var stored Midjourney
		if err := tx.First(&stored, task.Id).Error; err != nil {
			return err
		}
		if stored.EnterpriseMJSubject() != task.EnterpriseMJSubject() || stored.BillingRequestId != task.BillingRequestId || member.EnterpriseId != task.EnterpriseId || member.UserId != task.UserId {
			return ErrEnterpriseSubject
		}
		if stored.EnterpriseBillingState != MJEnterpriseUnknown {
			return ErrEnterpriseFinalized
		}
		if stored.ChannelId != task.ChannelId || stored.Quota != task.Quota || stored.EnterpriseInitialQuota != task.EnterpriseInitialQuota || stored.PollingKey != task.PollingKey || stored.PollingBaseURL != task.PollingBaseURL {
			return ErrEnterpriseSubject
		}
		state := MJEnterpriseAccepted
		if accepted && task.MjId != "" {
			var previous Midjourney
			err := tx.Where("id <> ? AND enterprise_task_key = ?", task.Id, task.EnterpriseMJTaskIdentity()).First(&previous).Error
			if err == nil {
				if previous.UserId != task.UserId || previous.EnterpriseId != task.EnterpriseId || previous.EnterpriseMemberId != task.EnterpriseMemberId {
					return ErrEnterpriseSubject
				}
				accepted = false
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if !accepted {
			state = MJEnterpriseRejected
			task.Quota = 0
			task.EnterpriseInitialQuota = 0
			task.MjId = ""
			task.Status, task.Progress = "FAILURE", "100%"
		}
		task.EnterpriseBillingState = state
		updates := midjourneyProgressUpdates(task)
		updates["mj_id"], updates["quota"], updates["description"] = task.MjId, task.Quota, task.Description
		updates["enterprise_billing_state"] = state
		updates["enterprise_initial_quota"] = task.EnterpriseInitialQuota
		if accepted && task.MjId != "" {
			key := task.EnterpriseMJTaskIdentity()
			updates["enterprise_task_key"] = key
		}
		return tx.Model(&Midjourney{}).Where("id = ? AND enterprise_billing_state = ?", task.Id, MJEnterpriseUnknown).Updates(updates).Error
	})
}

func matchingMJReservation(task *Midjourney, r *EnterpriseReservation) bool {
	return r.EnterpriseId == task.EnterpriseId && r.MemberId == task.EnterpriseMemberId && r.UserId == task.UserId && r.TokenId == task.BillingTokenId
}

// SettleEnterpriseMJSubmission resumes only a durable, known response. The
// reservation (including token usage) is idempotent. No token secret, current
// membership or personal wallet is consulted.
func SettleEnterpriseMJSubmission(id int, expectedQuota int) error {
	var task Midjourney
	if err := DB.First(&task, id).Error; err != nil {
		return err
	}
	if !validEnterpriseMJ(&task) {
		return ErrEnterpriseSubject
	}
	if task.EnterpriseBillingSettled {
		return nil
	}
	if task.EnterpriseBillingState != MJEnterpriseAccepted && task.EnterpriseBillingState != MJEnterpriseRejected {
		return ErrEnterpriseReservation
	}
	if expectedQuota != task.EnterpriseInitialQuota || (task.EnterpriseBillingState == MJEnterpriseRejected && task.EnterpriseInitialQuota != 0) {
		return ErrEnterpriseReservation
	}
	var original EnterpriseReservation
	if err := DB.Where("request_id = ?", task.BillingRequestId).First(&original).Error; err != nil {
		return err
	}
	if !matchingMJReservation(&task, &original) {
		return ErrEnterpriseSubject
	}
	if _, err := AdjustEnterpriseReservation(task.EnterpriseMJSubject(), task.BillingRequestId, task.EnterpriseInitialQuota, EnterpriseReservationSettled, false); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockEnterpriseMemberTx(tx, task.EnterpriseMemberId); err != nil {
			return err
		}
		var current Midjourney
		if err := tx.First(&current, id).Error; err != nil {
			return err
		}
		if current.EnterpriseBillingSettled {
			return nil
		}
		if current.BillingRequestId != task.BillingRequestId || current.EnterpriseMJSubject() != task.EnterpriseMJSubject() || current.Quota != task.Quota {
			return ErrEnterpriseSubject
		}
		return tx.Model(&Midjourney{}).Where("id = ?", id).Update("enterprise_billing_settled", true).Error
	})
}

// FinalizeEnterpriseMJ commits the correction and terminal acknowledgement in one
// main-database transaction. Pollers may retry after a crash or callback race.
func FinalizeEnterpriseMJ(id int) (EnterpriseReservation, error) {
	var receipt EnterpriseReservation
	var candidate Midjourney
	if err := DB.First(&candidate, id).Error; err != nil {
		return receipt, err
	}
	if !validEnterpriseMJ(&candidate) {
		return receipt, ErrEnterpriseSubject
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockEnterpriseMemberTx(tx, candidate.EnterpriseMemberId); err != nil {
			return err
		}
		var task Midjourney
		if err := tx.First(&task, id).Error; err != nil {
			return err
		}
		if task.EnterpriseBillingFinalized {
			return nil
		}
		if !validEnterpriseMJ(&task) || task.EnterpriseMJSubject() != candidate.EnterpriseMJSubject() {
			return ErrEnterpriseSubject
		}
		if !task.EnterpriseBillingSettled || (task.Status != "SUCCESS" && task.Status != "FAILURE") {
			return ErrEnterpriseReservation
		}
		target := int64(task.EnterpriseInitialQuota)
		if task.Status == "FAILURE" {
			target = 0
		}
		event := fmt.Sprintf("mj-final:%x", sha256.Sum256([]byte(task.BillingRequestId)))
		var err error
		receipt, err = applyEnterpriseAsyncCorrectionTx(tx, task.EnterpriseMJSubject(), task.BillingRequestId, target, event)
		if err != nil {
			return err
		}
		if receipt.Changed && task.ChannelId > 0 && tx.Migrator().HasTable(&Channel{}) {
			if err := tx.Model(&Channel{}).Where("id = ?", task.ChannelId).Update("used_quota", gorm.Expr("used_quota + ?", receipt.Quota)).Error; err != nil {
				return err
			}
		}
		return tx.Model(&Midjourney{}).Where("id = ?", id).Updates(map[string]any{"quota": target, "enterprise_billing_finalized": true}).Error
	})
	return receipt, err
}

// ReconcileEnterpriseMJ is bounded and deliberately ignores unknown upstream
// outcomes. It can be called at startup and by the existing maintenance worker.
// n counts completed recoveries; errors remain persisted and retryable.
func ReconcileEnterpriseMJ(limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	if limit > 1000 {
		limit = 1000
	}
	var tasks []Midjourney
	err := DB.Where("enterprise_id > 0 AND enterprise_billing_state IN ?", []string{MJEnterpriseAccepted, MJEnterpriseRejected}).
		Where("enterprise_billing_settled = ? OR (enterprise_billing_finalized = ? AND status IN ?)", false, false, []string{"SUCCESS", "FAILURE"}).Order("id ASC").Limit(limit).Find(&tasks).Error
	if err != nil {
		return 0, err
	}
	n := 0
	var failures []error
	for _, task := range tasks {
		if err := SettleEnterpriseMJSubmission(task.Id, task.EnterpriseInitialQuota); err != nil {
			failures = append(failures, fmt.Errorf("mj %d settlement: %w", task.Id, err))
			continue
		}
		if task.Status == "SUCCESS" || task.Status == "FAILURE" {
			if _, err := FinalizeEnterpriseMJ(task.Id); err != nil {
				failures = append(failures, fmt.Errorf("mj %d correction: %w", task.Id, err))
				continue
			}
		}
		n++
	}
	return n, errors.Join(failures...)
}
