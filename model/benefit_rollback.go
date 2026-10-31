package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func RollbackBenefitsBySource(businessType string, businessId int, sourceType string, sourceRef string) (int, string, error) {
	return rollbackBenefitsBySource(businessType, businessId, sourceType, sourceRef, "")
}

func rollbackBenefitsBySource(businessType string, businessId int, sourceType string, sourceRef string, reason string) (int, string, error) {
	if strings.TrimSpace(sourceType) == "" || strings.TrimSpace(sourceRef) == "" {
		return 0, "", errors.New("invalid benefit rollback source")
	}

	var appliedQuotaDelta int
	var summary string

	err := DB.Transaction(func(tx *gorm.DB) error {
		op, err := ensureBenefitRollbackOperationTx(tx, businessType, businessId, sourceType, sourceRef)
		if err != nil {
			return err
		}
		if op.Status == BenefitRollbackStatusSucceeded {
			appliedQuotaDelta = 0
			summary = strings.TrimSpace(op.ResultSummary)
			return nil
		}
		if sourceType == BenefitSourceTopUpOrder || sourceType == BenefitSourceSubscriptionOrder {
			if err := lockPaymentSourceForRollbackTx(tx, sourceType, sourceRef); err != nil {
				return err
			}
		}

		grants, err := findBenefitGrantRecordsBySourceTx(tx, sourceType, sourceRef)
		if err != nil {
			return err
		}

		summaries := make([]string, 0, 2)
		quotaDelta, quotaSummary, err := rollbackQuotaBenefitsTx(tx, op, sourceType, sourceRef, grants, reason)
		if err != nil {
			_ = tx.Model(op).Updates(map[string]any{
				"status":        BenefitRollbackStatusFailed,
				"error_message": err.Error(),
			}).Error
			return err
		}
		if strings.TrimSpace(quotaSummary) != "" {
			summaries = append(summaries, quotaSummary)
		}

		subSummary, err := rollbackSubscriptionBenefitsTx(tx, op, sourceType, sourceRef, grants)
		if err != nil {
			_ = tx.Model(op).Updates(map[string]any{
				"status":        BenefitRollbackStatusFailed,
				"error_message": err.Error(),
			}).Error
			return err
		}
		if strings.TrimSpace(subSummary) != "" {
			summaries = append(summaries, subSummary)
		}

		if sourceType == BenefitSourceTopUpOrder || sourceType == BenefitSourceSubscriptionOrder {
			reversedCommissionQuota, reverseErr := reverseInviteCommissionsByTradeNoTx(tx, sourceRef)
			if reverseErr != nil {
				return reverseErr
			}
			if reversedCommissionQuota > 0 {
				summaries = append(summaries, fmt.Sprintf("回退邀请返佣 %s", logger.FormatQuota(reversedCommissionQuota)))
			}
			if reverseErr := markReversedPaymentSourceTx(tx, sourceType, sourceRef); reverseErr != nil {
				return reverseErr
			}
		}
		if len(summaries) == 0 {
			err = errors.New("no benefit grants found to rollback")
			_ = tx.Model(op).Updates(map[string]any{
				"status":        BenefitRollbackStatusFailed,
				"error_message": err.Error(),
			}).Error
			return err
		}

		appliedQuotaDelta = quotaDelta
		summary = strings.Join(summaries, "；")
		return tx.Model(op).Updates(map[string]any{
			"status":         BenefitRollbackStatusSucceeded,
			"result_summary": summary,
			"error_message":  "",
		}).Error
	})
	if err != nil {
		return 0, "", err
	}
	if cacheErr := invalidateBenefitRollbackUserCaches(sourceType, sourceRef); cacheErr != nil {
		common.SysLog("failed to invalidate user quota cache after benefit rollback: " + cacheErr.Error())
	}
	return appliedQuotaDelta, summary, nil
}

func lockPaymentSourceForRollbackTx(tx *gorm.DB, sourceType string, sourceRef string) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return errors.New("payment source is empty")
	}
	lock := func(query *gorm.DB) *gorm.DB {
		if common.UsingSQLite {
			return query
		}
		return query.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	switch sourceType {
	case BenefitSourceTopUpOrder:
		var topUp TopUp
		return lock(tx.Select("id", "status").Where("trade_no = ?", sourceRef)).First(&topUp).Error
	case BenefitSourceSubscriptionOrder:
		var order SubscriptionOrder
		if err := lock(tx.Select("id", "status").Where("trade_no = ?", sourceRef)).First(&order).Error; err != nil {
			return err
		}
		// Settlement locks the payment mirror before touching inviter funds.
		// Acquire it before reading pending ledgers so a concurrent payout is reclaimed.
		var topUp TopUp
		// Older subscriptions may have no payment mirror. Their order lock still
		// serializes completion; no valid commission can settle without a mirror.
		return lock(tx.Select("id", "status").Where("trade_no = ?", sourceRef)).Limit(1).Find(&topUp).Error
	default:
		return nil
	}
}

func invalidateBenefitRollbackUserCaches(sourceType string, sourceRef string) error {
	if !common.RedisEnabled {
		return nil
	}
	userIDs := make(map[int]struct{})
	switch sourceType {
	case BenefitSourceTopUpOrder:
		var topUp TopUp
		if err := DB.Select("user_id").Where("trade_no = ?", sourceRef).First(&topUp).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else if err == nil && topUp.UserId > 0 {
			userIDs[topUp.UserId] = struct{}{}
			var recipientIDs []int
			if err := DB.Model(&Redemption{}).
				Where("user_id = ? AND funding_source = ? AND used_user_id > ?", topUp.UserId, RedemptionFundingSourceWallet, 0).
				Distinct("used_user_id").
				Pluck("used_user_id", &recipientIDs).Error; err != nil {
				return err
			}
			for _, recipientID := range recipientIDs {
				if recipientID > 0 {
					userIDs[recipientID] = struct{}{}
				}
			}
		}
	case BenefitSourceSubscriptionOrder:
		var order SubscriptionOrder
		if err := DB.Select("user_id").Where("trade_no = ?", sourceRef).First(&order).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else if err == nil && order.UserId > 0 {
			userIDs[order.UserId] = struct{}{}
		}
	}

	var inviterIDs []int
	if err := DB.Model(&InviteCommissionLedger{}).
		Where("topup_trade_no = ?", sourceRef).
		Distinct("inviter_user_id").
		Pluck("inviter_user_id", &inviterIDs).Error; err != nil {
		return err
	}
	for _, userID := range inviterIDs {
		if userID > 0 {
			userIDs[userID] = struct{}{}
		}
	}

	var firstErr error
	for userID := range userIDs {
		if err := invalidateUserCache(userID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func markReversedPaymentSourceTx(tx *gorm.DB, sourceType string, sourceRef string) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return nil
	}

	switch sourceType {
	case BenefitSourceTopUpOrder:
		return tx.Model(&TopUp{}).
			Where("trade_no = ? AND status = ?", sourceRef, common.TopUpStatusSuccess).
			Update("status", common.TopUpStatusFailed).Error
	case BenefitSourceSubscriptionOrder:
		if err := tx.Model(&SubscriptionOrder{}).
			Where("trade_no = ? AND status = ?", sourceRef, common.TopUpStatusSuccess).
			Update("status", common.TopUpStatusFailed).Error; err != nil {
			return err
		}
		return tx.Model(&TopUp{}).
			Where("trade_no = ? AND status = ?", sourceRef, common.TopUpStatusSuccess).
			Update("status", common.TopUpStatusFailed).Error
	default:
		return nil
	}
}

func rollbackContext(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "rollback"
	}
	return "rollback:" + reason
}

func rollbackQuotaBenefitsTx(tx *gorm.DB, op *BenefitRollbackOperation, sourceType string, sourceRef string, grants []*BenefitChangeRecord, reason string) (int, string, error) {
	if tx == nil || op == nil {
		return 0, "", errors.New("invalid quota rollback args")
	}

	totalQuota := 0
	for _, grant := range grants {
		if grant == nil || grant.BenefitType != BenefitTypeQuota {
			continue
		}
		rolledBack, err := hasBenefitRollbackRecordForOriginTx(tx, grant.Id)
		if err != nil {
			return 0, "", err
		}
		if rolledBack {
			continue
		}
		detail, err := unmarshalQuotaBenefitDetail(grant.Detail)
		if err != nil {
			return 0, "", err
		}
		if detail.QuotaDelta <= 0 {
			continue
		}
		var revokeErr error
		if sourceType == BenefitSourcePulseReward {
			revokeErr = revokePulseQuotaGrantTx(tx, grant.UserId, detail.QuotaDelta)
		} else {
			revokeErr = RevokeTransferableQuotaGrantTx(tx, grant.UserId, detail.QuotaDelta)
		}
		if revokeErr != nil {
			return 0, "", revokeErr
		}
		totalQuota += detail.QuotaDelta
		if err := createBenefitChangeRecordTx(tx, &BenefitChangeRecord{
			BenefitType:         BenefitTypeQuota,
			Action:              BenefitActionRollback,
			SourceType:          sourceType,
			SourceRef:           sourceRef,
			UserId:              grant.UserId,
			TargetType:          grant.TargetType,
			TargetId:            grant.TargetId,
			OriginRecordId:      grant.Id,
			RollbackOperationId: op.Id,
			Detail: marshalBenefitDetail(&QuotaBenefitDetail{
				QuotaDelta:    -detail.QuotaDelta,
				PaymentMethod: detail.PaymentMethod,
				Context:       rollbackContext(reason),
			}),
		}); err != nil {
			return 0, "", err
		}
	}

	if totalQuota == 0 && strings.TrimSpace(sourceType) == BenefitSourceTopUpOrder {
		topUp := &TopUp{}
		if err := tx.Where("trade_no = ?", sourceRef).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, "", nil
			}
			return 0, "", err
		}
		if topUp.Status != common.TopUpStatusSuccess {
			return 0, "", errors.New("only successful top-up orders can be reversed")
		}
		fallbackQuota := int(topUp.Amount)
		if !strings.EqualFold(topUp.PaymentMethod, "creem") {
			var err error
			fallbackQuota, err = CalculateGrantedQuotaForTopUp(topUp)
			if err != nil {
				return 0, "", err
			}
		}
		if fallbackQuota <= 0 {
			return 0, "", nil
		}
		if err := RevokeTransferableQuotaGrantTx(tx, topUp.UserId, fallbackQuota); err != nil {
			return 0, "", err
		}
		totalQuota = fallbackQuota
		if err := createBenefitChangeRecordTx(tx, &BenefitChangeRecord{
			BenefitType:         BenefitTypeQuota,
			Action:              BenefitActionRollback,
			SourceType:          sourceType,
			SourceRef:           sourceRef,
			UserId:              topUp.UserId,
			TargetType:          BenefitTargetUserQuota,
			TargetId:            topUp.UserId,
			RollbackOperationId: op.Id,
			Detail: marshalBenefitDetail(&QuotaBenefitDetail{
				QuotaDelta:    -fallbackQuota,
				PaymentMethod: topUp.PaymentMethod,
				Context:       "fallback",
			}),
		}); err != nil {
			return 0, "", err
		}
	}

	if totalQuota <= 0 {
		return 0, "", nil
	}

	return -totalQuota, fmt.Sprintf("回退额度 %s", logger.FormatQuota(totalQuota)), nil
}

func rollbackSubscriptionBenefitsTx(tx *gorm.DB, op *BenefitRollbackOperation, sourceType string, sourceRef string, grants []*BenefitChangeRecord) (string, error) {
	if tx == nil || op == nil {
		return "", errors.New("invalid subscription rollback args")
	}

	summaries := make([]string, 0, 4)
	pendingSummary, err := rollbackPendingSubscriptionIssuancesTx(tx, op, sourceType, sourceRef)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(pendingSummary) != "" {
		summaries = append(summaries, pendingSummary)
	}

	reversedCount := 0
	for _, grant := range grants {
		if grant == nil || grant.BenefitType != BenefitTypeSubscription || grant.TargetType != BenefitTargetUserSubscription {
			continue
		}
		rolledBack, err := hasBenefitRollbackRecordForOriginTx(tx, grant.Id)
		if err != nil {
			return "", err
		}
		if rolledBack {
			continue
		}
		detail, err := unmarshalSubscriptionBenefitDetail(grant.Detail)
		if err != nil {
			return "", err
		}
		if detail == nil {
			continue
		}
		switch detail.Operation {
		case SubscriptionBenefitOperationCreate:
			if err := rollbackCreatedSubscriptionTx(tx, grant, detail); err != nil {
				return "", err
			}
		case SubscriptionBenefitOperationRenew:
			if err := rollbackRenewedSubscriptionTx(tx, grant, detail); err != nil {
				return "", err
			}
		default:
			continue
		}
		reversedCount++
		if err := createBenefitChangeRecordTx(tx, &BenefitChangeRecord{
			BenefitType:         BenefitTypeSubscription,
			Action:              BenefitActionRollback,
			SourceType:          sourceType,
			SourceRef:           sourceRef,
			UserId:              grant.UserId,
			TargetType:          grant.TargetType,
			TargetId:            grant.TargetId,
			OriginRecordId:      grant.Id,
			RollbackOperationId: op.Id,
			Detail:              grant.Detail,
		}); err != nil {
			return "", err
		}
	}

	if reversedCount > 0 {
		summaries = append(summaries, fmt.Sprintf("回退套餐权益 %d 条", reversedCount))
	}
	if len(summaries) == 0 {
		return "", nil
	}
	return strings.Join(summaries, "；"), nil
}

func rollbackPendingSubscriptionIssuancesTx(tx *gorm.DB, op *BenefitRollbackOperation, sourceType string, sourceRef string) (string, error) {
	issuanceSourceType := subscriptionIssuanceSourceFromBenefitSource(sourceType)
	if issuanceSourceType == "" {
		return "", nil
	}
	var issuances []SubscriptionIssuance
	if err := tx.Where("source_type = ? AND source_ref = ? AND status = ?",
		issuanceSourceType, strings.TrimSpace(sourceRef), SubscriptionIssuanceStatusPending).
		Find(&issuances).Error; err != nil {
		return "", err
	}
	if len(issuances) == 0 {
		return "", nil
	}

	cancelled := 0
	for i := range issuances {
		issuance := issuances[i]
		if err := tx.Model(&issuance).Updates(map[string]any{
			"status": SubscriptionIssuanceStatusCancelled,
		}).Error; err != nil {
			return "", err
		}
		cancelled++
		if err := createBenefitChangeRecordTx(tx, &BenefitChangeRecord{
			BenefitType:         BenefitTypeSubscription,
			Action:              BenefitActionRollback,
			SourceType:          sourceType,
			SourceRef:           sourceRef,
			UserId:              issuance.UserId,
			TargetType:          BenefitTargetSubscriptionIssuance,
			TargetId:            issuance.Id,
			RollbackOperationId: op.Id,
			Detail: marshalBenefitDetail(&SubscriptionBenefitDetail{
				Operation:    SubscriptionBenefitOperationCancelPending,
				PurchaseMode: issuance.PurchaseMode,
				IssuanceId:   issuance.Id,
				PlanId:       issuance.PlanId,
				StatusBefore: SubscriptionIssuanceStatusPending,
				StatusAfter:  SubscriptionIssuanceStatusCancelled,
			}),
		}); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("取消待发放套餐 %d 条", cancelled), nil
}

func rollbackCreatedSubscriptionTx(tx *gorm.DB, grant *BenefitChangeRecord, detail *SubscriptionBenefitDetail) error {
	now := common.GetTimestamp()
	var sub UserSubscription
	if err := tx.Where("id = ?", grant.TargetId).First(&sub).Error; err != nil {
		return err
	}
	if sub.Status == "cancelled" && sub.EndTime > 0 && sub.EndTime <= now {
		return nil
	}
	sub.Status = "cancelled"
	sub.EndTime = now
	if err := tx.Save(&sub).Error; err != nil {
		return err
	}
	_, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
	return err
}

// rollbackRenewedSubscriptionTx 回退一次续订发放。
//
// 当订阅当前状态与 grant 发放后快照完全一致（exactRestore）时，可以精确还原到
// 发放前的状态；否则走 delta 分支，只减去本次续订贡献的时长/金额增量。
//
// 注意：delta 分支是“尽力而为”的近似回退——如果这条订阅在本次 grant 之后又被
// 其它订单续订过，减掉 delta 不会让订阅回到 grant 发放前的精确原貌，只会抵消
// 本次贡献。后续链路中另一笔不相关的续订不会因此失效。
func rollbackRenewedSubscriptionTx(tx *gorm.DB, grant *BenefitChangeRecord, detail *SubscriptionBenefitDetail) error {
	now := common.GetTimestamp()
	var sub UserSubscription
	if err := tx.Where("id = ?", grant.TargetId).First(&sub).Error; err != nil {
		return err
	}

	exactRestore := sub.EndTime == detail.EndTimeAfter &&
		sub.AmountTotal == detail.AmountTotalAfter &&
		(strings.TrimSpace(detail.StatusAfter) == "" || sub.Status == detail.StatusAfter)

	if exactRestore {
		sub.EndTime = detail.EndTimeBefore
		sub.AmountTotal = detail.AmountTotalBefore
		if strings.TrimSpace(detail.StatusBefore) != "" {
			sub.Status = detail.StatusBefore
		}
	} else {
		if detail.EndTimeDelta > 0 {
			sub.EndTime -= detail.EndTimeDelta
		}
		if detail.AmountTotalDelta > 0 && sub.AmountTotal > 0 {
			sub.AmountTotal -= detail.AmountTotalDelta
			if sub.AmountTotal < 0 {
				sub.AmountTotal = 0
			}
		}
		if sub.EndTime <= now {
			sub.EndTime = now
			sub.Status = "cancelled"
		}
	}
	if sub.NextResetTime > 0 && sub.EndTime > 0 && sub.NextResetTime > sub.EndTime {
		sub.NextResetTime = 0
	}
	if err := tx.Save(&sub).Error; err != nil {
		return err
	}
	_, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
	return err
}
