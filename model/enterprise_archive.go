package model

import (
	"math/big"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EnterpriseArchivePreview is a locked snapshot, not permission to skip the
// checks on submission. Counts include zero-cost and unknown asynchronous work.
type EnterpriseArchivePreview struct {
	Enterprise         *Enterprise       `json:"enterprise"`
	Balance            EnterpriseBalance `json:"balance"`
	Remaining          int64             `json:"remaining"`
	Members            int64             `json:"members"`
	Tokens             int64             `json:"tokens"`
	PendingInvitations int64             `json:"pending_invitations"`
	PendingRequests    int64             `json:"pending_requests"`
	OpenReservations   int64             `json:"open_reservations"`
	PendingTasks       int64             `json:"pending_tasks"`
	PendingMidjourney  int64             `json:"pending_midjourney"`
	CanArchive         bool              `json:"can_archive"`
	Blockers           []string          `json:"blockers"`
}

// Lock the enterprise before calling this helper. Billing locks only members;
// taking every generation seals late callbacks without serializing normal calls.
func enterpriseArchiveMembersTx(tx *gorm.DB, enterpriseID int) ([]*EnterpriseMember, error) {
	var members []*EnterpriseMember
	query := tx.Where("enterprise_id = ?", enterpriseID).Order("id ASC")
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.Find(&members).Error
	return members, err
}

// Use exact arithmetic so a corrupt/oversized aggregate cannot wrap to zero.
func enterpriseArchiveSum(values ...int64) (int64, error) {
	sum := new(big.Int)
	for _, value := range values {
		sum.Add(sum, big.NewInt(value))
	}
	if !sum.IsInt64() {
		return 0, ErrEnterpriseArchiveBlocked
	}
	return sum.Int64(), nil
}

func enterpriseArchivePreviewTx(tx *gorm.DB, ent *Enterprise, members []*EnterpriseMember) (*EnterpriseArchivePreview, error) {
	out := &EnterpriseArchivePreview{Enterprise: ent, Blockers: []string{}}
	out.Balance.NetCredited, out.Balance.Unallocated = ent.NetCredited, ent.Unallocated
	available, reserved, consumed := []int64{}, []int64{}, []int64{}
	memberReserved := false
	for _, member := range members {
		available = append(available, member.Available)
		reserved = append(reserved, member.Reserved)
		consumed = append(consumed, member.Consumed)
		memberReserved = memberReserved || member.Reserved != 0
		if member.ActiveUserId != nil {
			out.Members++
		}
	}
	var err error
	if out.Balance.MemberAvailable, err = enterpriseArchiveSum(available...); err != nil {
		return nil, err
	}
	if out.Balance.MemberReserved, err = enterpriseArchiveSum(reserved...); err != nil {
		return nil, err
	}
	if out.Balance.Consumed, err = enterpriseArchiveSum(consumed...); err != nil {
		return nil, err
	}
	if out.Remaining, err = enterpriseArchiveSum(ent.Unallocated, out.Balance.MemberAvailable, out.Balance.MemberReserved); err != nil {
		return nil, err
	}
	total, err := enterpriseArchiveSum(out.Remaining, out.Balance.Consumed)
	if err != nil {
		return nil, err
	}
	out.Balance.Balanced = total == ent.NetCredited

	counts := []struct {
		query *gorm.DB
		count *int64
	}{
		{tx.Model(&Token{}).Where("enterprise_id = ? AND status <> ?", ent.Id, common.TokenStatusDisabled), &out.Tokens},
		{tx.Model(&EnterpriseInvitation{}).Where("enterprise_id = ? AND status = ?", ent.Id, EnterpriseInvitationPending), &out.PendingInvitations},
		{tx.Model(&EnterpriseQuotaRequest{}).Where("enterprise_id = ? AND status = ?", ent.Id, EnterpriseRequestPending), &out.PendingRequests},
		{tx.Model(&EnterpriseReservation{}).Where("enterprise_id = ? AND status NOT IN ?", ent.Id, []string{EnterpriseReservationSettled, EnterpriseReservationRefunded}), &out.OpenReservations},
		{tx.Model(&Task{}).Where("enterprise_id = ?", ent.Id).Where("status IS NULL OR status NOT IN ? OR enterprise_billing_pending = ? OR enterprise_initial_recorded = ?", []string{TaskStatusSuccess, TaskStatusFailure}, true, false), &out.PendingTasks},
		{tx.Model(&Midjourney{}).Where("enterprise_id = ?", ent.Id).Where("status IS NULL OR status NOT IN ? OR enterprise_billing_state NOT IN ? OR enterprise_billing_settled = ? OR enterprise_billing_finalized = ?", []string{"SUCCESS", "FAILURE"}, []string{MJEnterpriseAccepted, MJEnterpriseRejected}, false, false), &out.PendingMidjourney},
	}
	for _, count := range counts {
		// Missing schema must fail closed, not silently hide an entire billing path.
		if err := count.query.Count(count.count).Error; err != nil {
			return nil, err
		}
	}
	if ent.Status == EnterpriseStatusArchived {
		out.Blockers = append(out.Blockers, "archived")
	} else if ent.Status != EnterpriseStatusFrozen {
		out.Blockers = append(out.Blockers, "not_frozen")
	}
	if !out.Balance.Balanced {
		out.Blockers = append(out.Blockers, "unbalanced")
	}
	if out.Remaining != 0 {
		out.Blockers = append(out.Blockers, "remaining_balance")
	}
	if memberReserved || out.OpenReservations != 0 {
		out.Blockers = append(out.Blockers, "open_reservations")
	}
	if out.PendingTasks != 0 {
		out.Blockers = append(out.Blockers, "pending_tasks")
	}
	if out.PendingMidjourney != 0 {
		out.Blockers = append(out.Blockers, "pending_midjourney")
	}
	out.CanArchive = len(out.Blockers) == 0
	return out, nil
}

func PreviewEnterpriseArchive(enterpriseID int) (*EnterpriseArchivePreview, error) {
	var out *EnterpriseArchivePreview
	err := DB.Transaction(func(tx *gorm.DB) error {
		ent, err := lockEnterpriseTx(tx, enterpriseID)
		if err != nil {
			return err
		}
		members, err := enterpriseArchiveMembersTx(tx, enterpriseID)
		if err != nil {
			return err
		}
		out, err = enterpriseArchivePreviewTx(tx, ent, members)
		return err
	})
	return out, err
}

// ArchiveEnterprise is a terminal soft archive. RootAuth is enforced by the
// platform route, just like credit/adjust. No money is forgiven or refunded here.
func ArchiveEnterprise(actorID, enterpriseID int, name, reason string) (*Enterprise, error) {
	reason = strings.TrimSpace(reason)
	if actorID <= 0 || reason == "" || len([]rune(reason)) > 255 {
		return nil, ErrEnterpriseInvalidInput
	}
	var out *Enterprise
	var keys []string
	err := DB.Transaction(func(tx *gorm.DB) error {
		ent, err := lockEnterpriseTx(tx, enterpriseID)
		if err != nil {
			return err
		}
		if name != ent.Name {
			return ErrEnterpriseArchiveName
		}
		out = ent
		if ent.Status == EnterpriseStatusArchived {
			return nil
		}
		members, err := enterpriseArchiveMembersTx(tx, ent.Id)
		if err != nil {
			return err
		}
		preview, err := enterpriseArchivePreviewTx(tx, ent, members)
		if err != nil {
			return err
		}
		if !preview.CanArchive {
			return ErrEnterpriseArchiveBlocked
		}
		actor := EnterpriseActor{UserId: actorID, Platform: true}
		for _, member := range members {
			if _, err := enterpriseArchiveSum(ent.Unallocated, member.Available); err != nil {
				return err
			}
			if err := sweepEnterpriseMemberAvailableTx(tx, ent, member, actor, EnterpriseActorPlatform, reason); err != nil {
				return err
			}
		}
		if ent.Unallocated != 0 {
			return ErrEnterpriseArchiveBlocked
		}
		if err := tx.Model(&Token{}).Where("enterprise_id = ?", ent.Id).Pluck("key", &keys).Error; err != nil {
			return err
		}
		if err := tx.Model(&Token{}).Where("enterprise_id = ?", ent.Id).Update("status", common.TokenStatusDisabled).Error; err != nil {
			return err
		}
		if err := tx.Model(&EnterpriseInvitation{}).Where("enterprise_id = ? AND status = ?", ent.Id, EnterpriseInvitationPending).Update("status", EnterpriseInvitationRevoked).Error; err != nil {
			return err
		}
		if err := expireEnterpriseQuotaRequestsTx(tx, "enterprise_id = ?", ent.Id); err != nil {
			return err
		}
		now := time.Now().Unix()
		// Preserve earlier left/disabled states and dates. Seal all generations,
		// not only currently active members, so late refunds cannot reopen funds.
		if err := tx.Model(&EnterpriseMember{}).Where("enterprise_id = ? AND status = ?", ent.Id, EnterpriseMemberStatusActive).Updates(map[string]any{"status": EnterpriseMemberStatusLeft, "left_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&EnterpriseMember{}).Where("enterprise_id = ?", ent.Id).Updates(map[string]any{"active_user_id": nil, "archived_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).Updates(map[string]any{"status": EnterpriseStatusArchived, "archived_at": now, "archived_by": actorID, "archive_reason": reason, "policy_version": gorm.Expr("policy_version + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		if err := appendEnterpriseAuditTx(tx, ent.Id, actor, nil, "enterprise.archive", "enterprise", ent.Id,
			map[string]any{"status": ent.Status, "balance": preview.Balance},
			map[string]any{"status": EnterpriseStatusArchived, "members": preview.Members, "tokens": preview.Tokens, "invitations": preview.PendingInvitations, "requests": preview.PendingRequests}, reason); err != nil {
			return err
		}
		ent.Status, ent.ArchivedAt, ent.ArchivedBy, ent.ArchiveReason, ent.UpdatedAt = EnterpriseStatusArchived, now, actorID, reason, now
		ent.PolicyVersion++
		return nil
	})
	if err != nil {
		return nil, err
	}
	purgeEnterpriseTokenCache(keys)
	return out, nil
}
