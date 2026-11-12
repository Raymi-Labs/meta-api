package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	EnterpriseRequestPending   = "pending"
	EnterpriseRequestApproved  = "approved"
	EnterpriseRequestRejected  = "rejected"
	EnterpriseRequestCancelled = "cancelled"
	EnterpriseRequestExpired   = "expired"

	EnterpriseRequestTTLSeconds = 7 * 24 * 3600
)

var (
	ErrEnterpriseRequestPending = errors.New("已有一个待审批的申请")
	ErrEnterpriseRequestState   = errors.New("申请已处理或已失效")
)

// EnterpriseQuotaRequest 是成员的额度追加申请。批准只做内部划拨，不产生企业资产。
type EnterpriseQuotaRequest struct {
	Id           int `json:"id" gorm:"primaryKey"`
	EnterpriseId int `json:"enterprise_id" gorm:"not null;index:idx_enterprise_request_ent,priority:1"`
	MemberId     int `json:"member_id" gorm:"not null;index"`
	UserId       int `json:"user_id" gorm:"not null"`
	// PendingMemberId 仅在待审批时等于 MemberId，处理后置空；唯一索引保证同一成员同时只有一个待办。
	PendingMemberId *int   `json:"-" gorm:"uniqueIndex"`
	Amount          int64  `json:"amount" gorm:"not null"`
	ApprovedAmount  int64  `json:"approved_amount" gorm:"not null;default:0"`
	Reason          string `json:"reason" gorm:"type:varchar(255)"`
	Status          string `json:"status" gorm:"type:varchar(16);not null;index:idx_enterprise_request_ent,priority:2"`
	ReviewerId      int    `json:"reviewer_id" gorm:"not null;default:0"`
	ReviewNote      string `json:"review_note" gorm:"type:varchar(255)"`
	ExpiresAt       int64  `json:"expires_at" gorm:"not null"`
	CreatedAt       int64  `json:"created_at" gorm:"not null"`
	ReviewedAt      int64  `json:"reviewed_at" gorm:"not null;default:0"`

	MemberName      string `json:"member_name" gorm:"-"`
	MemberEmail     string `json:"member_email" gorm:"-"`
	ReviewerName    string `json:"reviewer_name" gorm:"-"`
	MemberAvailable int64  `json:"member_available" gorm:"-"`
	MemberReserved  int64  `json:"member_reserved" gorm:"-"`
	MemberConsumed  int64  `json:"member_consumed" gorm:"-"`
}

// expireEnterpriseQuotaRequestsTx 使符合条件的待审批申请失效（企业冻结、成员离开）。
func expireEnterpriseQuotaRequestsTx(tx *gorm.DB, cond string, arg any) error {
	return tx.Model(&EnterpriseQuotaRequest{}).Where(cond, arg).Where("status = ?", EnterpriseRequestPending).
		Updates(map[string]any{"status": EnterpriseRequestExpired, "pending_member_id": nil, "reviewed_at": time.Now().Unix()}).Error
}

// ExpireStaleEnterpriseQuotaRequests 由后台任务调用，使超过有效期的待办失效。
func ExpireStaleEnterpriseQuotaRequests(now int64) (int64, error) {
	res := DB.Model(&EnterpriseQuotaRequest{}).Where("status = ? AND expires_at <= ?", EnterpriseRequestPending, now).
		Updates(map[string]any{"status": EnterpriseRequestExpired, "pending_member_id": nil, "reviewed_at": now})
	return res.RowsAffected, res.Error
}

// CreateEnterpriseQuotaRequest 成员发起追加申请；同一成员同时只能有一个待审批。
func CreateEnterpriseQuotaRequest(userID, enterpriseID int, amount int64, reason string) (*EnterpriseQuotaRequest, error) {
	reason = trimEnterpriseReason(reason)
	if !validEnterpriseAmount(amount) {
		return nil, ErrEnterpriseInvalidInput
	}
	var req *EnterpriseQuotaRequest
	err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		if ent.Status != EnterpriseStatusActive {
			return ErrEnterpriseFrozen
		}
		member, err := enterpriseActorMemberTx(tx, ent.Id, EnterpriseActor{UserId: userID})
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		// 过期的待办先让位，避免占住唯一槽位。
		if err := tx.Model(&EnterpriseQuotaRequest{}).
			Where("member_id = ? AND status = ? AND expires_at <= ?", member.Id, EnterpriseRequestPending, now).
			Updates(map[string]any{"status": EnterpriseRequestExpired, "pending_member_id": nil, "reviewed_at": now}).Error; err != nil {
			return err
		}
		var pending int64
		if err := tx.Model(&EnterpriseQuotaRequest{}).Where("pending_member_id = ?", member.Id).Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return ErrEnterpriseRequestPending
		}
		memberID := member.Id
		req = &EnterpriseQuotaRequest{EnterpriseId: ent.Id, MemberId: member.Id, UserId: userID, PendingMemberId: &memberID,
			Amount: amount, Reason: reason, Status: EnterpriseRequestPending, ExpiresAt: now + EnterpriseRequestTTLSeconds, CreatedAt: now}
		if err := tx.Create(req).Error; err != nil {
			if isEnterpriseDuplicateKeyErr(err) {
				return ErrEnterpriseRequestPending
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return req, nil
}

// CancelEnterpriseQuotaRequest 成员撤销本人待审批的申请。
func CancelEnterpriseQuotaRequest(userID, enterpriseID, requestID int) error {
	now := time.Now().Unix()
	res := DB.Model(&EnterpriseQuotaRequest{}).
		Where("id = ? AND enterprise_id = ? AND user_id = ? AND status = ?", requestID, enterpriseID, userID, EnterpriseRequestPending).
		Updates(map[string]any{"status": EnterpriseRequestCancelled, "pending_member_id": nil, "reviewed_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrEnterpriseRequestState
	}
	return nil
}

// ReviewEnterpriseQuotaRequest 审批申请。批准时在同一事务内核对权限、申请状态、成员有效性与未分配余额，
// 完成状态变更、划拨与审计；批准金额可小于申请金额。余额不足时申请保持待审批并返回明确原因。
// 多人同时审批只有一个成功：企业行锁串行化，状态用条件更新。
func ReviewEnterpriseQuotaRequest(actor EnterpriseActor, enterpriseID, requestID int, approve bool, approvedAmount int64, note string) (*EnterpriseQuotaRequest, error) {
	note = trimEnterpriseReason(note)
	var out *EnterpriseQuotaRequest
	err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		var req EnterpriseQuotaRequest
		if err := tx.Where("id = ? AND enterprise_id = ?", requestID, ent.Id).First(&req).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnterpriseNotFound
			}
			return err
		}
		now := time.Now().Unix()
		if req.Status != EnterpriseRequestPending || req.ExpiresAt <= now {
			return ErrEnterpriseRequestState
		}
		target, err := lockEnterpriseMemberTx(tx, req.MemberId)
		if err != nil || target.EnterpriseId != ent.Id || target.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseRequestState
		}
		// 禁止自批；管理员的申请由所有者处理。
		if !actor.Platform && actorMember.Id == target.Id {
			return ErrEnterpriseSelfOperation
		}
		if err := canManageEnterpriseMember(actor, actorMember, target); err != nil {
			return err
		}
		status := EnterpriseRequestRejected
		if approve {
			if ent.Status != EnterpriseStatusActive {
				return ErrEnterpriseFrozen
			}
			if approvedAmount == 0 {
				approvedAmount = req.Amount
			}
			if approvedAmount <= 0 || approvedAmount > req.Amount {
				return ErrEnterpriseInvalidInput
			}
			if ent.Unallocated < approvedAmount {
				return ErrEnterpriseInsufficientUnallocated
			}
			status = EnterpriseRequestApproved
		} else {
			approvedAmount = 0
		}
		res := tx.Model(&EnterpriseQuotaRequest{}).Where("id = ? AND status = ?", req.Id, EnterpriseRequestPending).
			Updates(map[string]any{"status": status, "pending_member_id": nil, "approved_amount": approvedAmount,
				"reviewer_id": actor.UserId, "review_note": note, "reviewed_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrEnterpriseRequestState
		}
		before := target.Available
		if approve {
			if err := transferEnterpriseQuotaTx(tx, ent, target, approvedAmount, EnterpriseLedgerRequestGrant, actor, actor.auditRole(actorMember), note); err != nil {
				return err
			}
		}
		req.Status, req.ApprovedAmount, req.ReviewerId, req.ReviewNote, req.ReviewedAt, req.PendingMemberId = status, approvedAmount, actor.UserId, note, now, nil
		out = &req
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "request."+status, "request", req.Id,
			map[string]any{"available": before, "amount": req.Amount},
			map[string]any{"available": target.Available, "approved_amount": approvedAmount}, note)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListEnterpriseQuotaRequests 列出申请。userID 大于 0 时只看本人；status 为空表示全部。
func ListEnterpriseQuotaRequests(enterpriseID, userID int, status string, offset, limit int) ([]*EnterpriseQuotaRequest, int64, error) {
	query := DB.Model(&EnterpriseQuotaRequest{}).Where("enterprise_id = ?", enterpriseID)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	now := time.Now().Unix()
	switch status {
	case "":
	case EnterpriseRequestPending:
		query = query.Where("status = ? AND expires_at > ?", status, now)
	default:
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*EnterpriseQuotaRequest
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	userIDs, memberIDs := make([]int, 0, len(items)*2), make([]int, 0, len(items))
	for _, item := range items {
		// 已过有效期但后台任务尚未处理的待办，对外按已过期展示。
		if item.Status == EnterpriseRequestPending && item.ExpiresAt <= now {
			item.Status = EnterpriseRequestExpired
		}
		userIDs = append(userIDs, item.UserId, item.ReviewerId)
		memberIDs = append(memberIDs, item.MemberId)
	}
	briefs := enterpriseUserBriefs(DB, userIDs)
	members := map[int]EnterpriseMember{}
	if len(memberIDs) > 0 {
		var rows []EnterpriseMember
		if err := DB.Where("id IN ?", memberIDs).Find(&rows).Error; err == nil {
			for _, row := range rows {
				members[row.Id] = row
			}
		}
	}
	for _, item := range items {
		item.MemberName, item.MemberEmail = briefs[item.UserId].label(), briefs[item.UserId].Email
		if item.ReviewerId > 0 {
			item.ReviewerName = briefs[item.ReviewerId].label()
		}
		if m, ok := members[item.MemberId]; ok {
			item.MemberAvailable, item.MemberReserved, item.MemberConsumed = m.Available, m.Reserved, m.Consumed
		}
	}
	return items, total, nil
}

// CountEnterprisePendingRequests 审批中心的待办数量。
func CountEnterprisePendingRequests(enterpriseID int) (int64, error) {
	var n int64
	err := DB.Model(&EnterpriseQuotaRequest{}).
		Where("enterprise_id = ? AND status = ? AND expires_at > ?", enterpriseID, EnterpriseRequestPending, time.Now().Unix()).
		Count(&n).Error
	return n, err
}
