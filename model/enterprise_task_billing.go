package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// applyEnterpriseAsyncCorrectionTx locks the original member and records one
// immutable correction to a settled charge. It never consults current membership,
// user prices or personal wallets. Callers acknowledge their job in this transaction.
func applyEnterpriseAsyncCorrectionTx(tx *gorm.DB, sub EnterpriseSubject, requestID string, target int64, eventKey string) (EnterpriseReservation, error) {
	var receipt EnterpriseReservation
	if !sub.valid() || requestID == "" || eventKey == "" || len(eventKey) > 96 || target < 0 || target > enterpriseMaxAmount {
		return receipt, ErrEnterpriseReservation
	}
	member, err := lockEnterpriseMemberTx(tx, sub.MemberId)
	if err != nil {
		return receipt, err
	}
	if member.EnterpriseId != sub.EnterpriseId || member.UserId != sub.UserId {
		return receipt, ErrEnterpriseSubject
	}
	var original EnterpriseReservation
	if err := tx.Where("request_id = ?", requestID).First(&original).Error; err != nil {
		return receipt, err
	}
	if original.EnterpriseId != sub.EnterpriseId || original.MemberId != sub.MemberId || original.UserId != sub.UserId || original.TokenId != sub.TokenId || original.Status != EnterpriseReservationSettled {
		return receipt, ErrEnterpriseSubject
	}
	delta := target - original.Quota
	err = tx.Where("request_id = ?", eventKey).First(&receipt).Error
	if err == nil {
		if receipt.EnterpriseId != sub.EnterpriseId || receipt.MemberId != sub.MemberId || receipt.TokenId != sub.TokenId || receipt.Quota != delta {
			return receipt, ErrEnterpriseFinalized
		}
		return receipt, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return receipt, err
	}
	if err := applyEnterpriseTokenBudgetTx(tx, sub, delta, false); err != nil {
		return receipt, err
	}
	if err := tx.Model(&EnterpriseMember{}).Where("id = ?", member.Id).Updates(map[string]any{"available": member.Available - delta, "consumed": member.Consumed + delta, "updated_at": time.Now().Unix()}).Error; err != nil {
		return receipt, err
	}
	receipt = EnterpriseReservation{RequestId: eventKey, EnterpriseId: sub.EnterpriseId, MemberId: member.Id, UserId: sub.UserId, TokenId: sub.TokenId, Kind: EnterpriseChargeTaskAdjust, Quota: delta, Status: EnterpriseReservationSettled, ModelName: original.ModelName, TokenName: original.TokenName, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(), Changed: true}
	if err := tx.Create(&receipt).Error; err != nil {
		return receipt, err
	}
	if sub.TokenId > 0 {
		if err := tx.Unscoped().Model(&Token{}).Where("id = ? AND enterprise_id = ? AND membership_id = ?", sub.TokenId, sub.EnterpriseId, sub.MemberId).Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
			return receipt, err
		}
	}
	return receipt, nil
}

// FinalizeEnterpriseTask consumes a durable terminal intent. Wallet correction,
// token usage and acknowledgement share one transaction, so retries after a crash
// do not duplicate a refund. The authoritative receipt also survives a deleted token.
func FinalizeEnterpriseTask(taskID int64) (EnterpriseReservation, error) {
	var receipt EnterpriseReservation
	var candidate Task
	if err := DB.First(&candidate, taskID).Error; err != nil {
		return receipt, err
	}
	if candidate.EnterpriseId <= 0 {
		return receipt, ErrEnterpriseSubject
	}
	// Initial charge can be replayed safely: model settlement updates token stats
	// in the same transaction. Keep the task durable before acknowledging the bill.
	if candidate.PrivateData.BillingSource != EnterpriseQuotaSource {
		return receipt, ErrEnterpriseSubject
	}
	sub := EnterpriseSubject{EnterpriseId: candidate.EnterpriseId, MemberId: candidate.EnterpriseMemberId, UserId: candidate.UserId, TokenId: candidate.PrivateData.TokenId}
	var initial EnterpriseReservation
	if err := DB.Where("request_id = ?", candidate.PrivateData.RequestId).First(&initial).Error; err != nil {
		return receipt, err
	}
	if initial.EnterpriseId != sub.EnterpriseId || initial.MemberId != sub.MemberId || initial.UserId != sub.UserId || initial.TokenId != sub.TokenId {
		return receipt, ErrEnterpriseSubject
	}
	if enterpriseReservationOpen(initial.Status) {
		if candidate.EnterpriseInitialQuota < 0 || candidate.EnterpriseInitialQuota > enterpriseMaxAmount {
			return receipt, ErrEnterpriseReservation
		}
		if _, err := AdjustEnterpriseReservation(sub, candidate.PrivateData.RequestId, int(candidate.EnterpriseInitialQuota), EnterpriseReservationSettled, false); err != nil {
			return receipt, err
		}
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Match correction lock order before reading the task to serialize stale workers.
		if _, err := lockEnterpriseMemberTx(tx, candidate.EnterpriseMemberId); err != nil {
			return err
		}
		var task Task
		if err := tx.First(&task, taskID).Error; err != nil {
			return err
		}
		if task.EnterpriseId != candidate.EnterpriseId || task.EnterpriseMemberId != candidate.EnterpriseMemberId || task.PrivateData.BillingSource != EnterpriseQuotaSource {
			return ErrEnterpriseSubject
		}
		if !task.EnterpriseBillingPending {
			return nil
		}
		if !task.EnterpriseInitialRecorded {
			if err := tx.Model(&User{}).Where("id = ?", task.UserId).Update("request_count", gorm.Expr("request_count + 1")).Error; err != nil {
				return err
			}
			if task.ChannelId > 0 {
				if err := tx.Model(&Channel{}).Where("id = ?", task.ChannelId).Update("used_quota", gorm.Expr("used_quota + ?", task.EnterpriseInitialQuota)).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&Task{}).Where("id = ?", task.ID).Update("enterprise_initial_recorded", true).Error; err != nil {
				return err
			}
		}

		if task.Status != TaskStatusSuccess && task.Status != TaskStatusFailure {
			return tx.Model(&Task{}).Where("id = ? AND status = ?", task.ID, task.Status).Update("enterprise_billing_pending", false).Error
		}
		target := task.EnterpriseFinalQuota
		if task.Status == TaskStatusFailure && target != 0 {
			return ErrEnterpriseReservation
		}
		sub := EnterpriseSubject{EnterpriseId: task.EnterpriseId, MemberId: task.EnterpriseMemberId, UserId: task.UserId, TokenId: task.PrivateData.TokenId}
		var err error
		receipt, err = applyEnterpriseAsyncCorrectionTx(tx, sub, task.PrivateData.RequestId, target, fmt.Sprintf("task-final:%x", sha256.Sum256([]byte(task.PrivateData.RequestId))))
		if err != nil {
			return err
		}
		if receipt.Changed && receipt.Quota != 0 && task.ChannelId > 0 {
			if err := tx.Model(&Channel{}).Where("id = ?", task.ChannelId).Update("used_quota", gorm.Expr("used_quota + ?", receipt.Quota)).Error; err != nil {
				return err
			}
		}
		return tx.Model(&Task{}).Where("id = ? AND enterprise_billing_pending = ?", task.ID, true).Updates(map[string]any{"quota": target, "enterprise_billing_pending": false}).Error
	})
	return receipt, err
}

func ReconcileEnterpriseTasks(limit int) (int, error) {
	var tasks []Task
	if err := DB.Where("enterprise_id > 0 AND enterprise_billing_pending = ?", true).Order("id ASC").Limit(limit).Find(&tasks).Error; err != nil {
		return 0, err
	}
	done := 0
	var failures []error
	for _, task := range tasks {
		if _, err := FinalizeEnterpriseTask(task.ID); err != nil {
			failures = append(failures, fmt.Errorf("enterprise task %d: %w", task.ID, err))
		} else {
			done++
		}
	}
	return done, errors.Join(failures...)
}

// MarkEnterpriseTaskUncertain retains a submitted but unconfirmed task's hold.
// An unknown upstream result is not evidence that a paid task failed.
func MarkEnterpriseTaskUncertain(sub EnterpriseSubject, requestID string) error {
	if !sub.valid() || requestID == "" {
		return ErrEnterpriseSubject
	}
	return DB.Model(&EnterpriseReservation{}).Where("request_id = ? AND enterprise_id = ? AND member_id = ? AND user_id = ? AND status = ?", requestID, sub.EnterpriseId, sub.MemberId, sub.UserId, EnterpriseReservationReserved).Update("status", EnterpriseReservationReview).Error
}
