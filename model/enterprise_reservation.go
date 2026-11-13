package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/types"

	"gorm.io/gorm"
)

const (
	EnterpriseReservationReserved = "reserved"
	EnterpriseReservationSettled  = "settled"
	EnterpriseReservationRefunded = "refunded"
	// review：超时未终结，转待对账。额度仍计在成员在途里，不自动退款；
	// 迟到的真实结算或平台人工处理都可以把它终结。
	EnterpriseReservationReview = "review"

	EnterpriseChargeRequest      = "request"
	EnterpriseChargeViolationFee = "violation_fee"
	EnterpriseChargeTaskAdjust   = "task_adjust"
	EnterpriseChargeManual       = "manual"

	EnterpriseQuotaSource = "enterprise"
	// 令牌上限的拒绝单独成一类：它的主体是令牌而不是成员，处理办法也不同（调上限而非申请额度）。
	EnterpriseTokenQuotaSource = "enterprise_token"
)

var (
	ErrEnterpriseMemberInactive = errors.New("企业成员已停用")
	ErrEnterpriseTokenCap       = errors.New("企业令牌已达到上限")
	ErrEnterpriseReservation    = errors.New("invalid enterprise funding reservation")
	ErrEnterpriseFinalized      = errors.New("enterprise funding already finalized")
	ErrEnterpriseSubject        = errors.New("enterprise funding subject conflict")
)

// EnterpriseReservation 是企业请求的持久预留与结算记录，按 request_id 唯一。
// 开放状态（reserved/review）下 Quota 是在途占用；settled 时是最终费用；refunded 为 0。
// 事后调整（任务退款、违规费等）用独立事件键另记一行，Quota 可为负。
type EnterpriseReservation struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	RequestId    string `json:"request_id" gorm:"type:varchar(96);not null;uniqueIndex"`
	EnterpriseId int    `json:"enterprise_id" gorm:"not null;index:idx_enterprise_resv_ent,priority:1;index:idx_enterprise_resv_time,priority:1"`
	MemberId     int    `json:"member_id" gorm:"not null;index"`
	UserId       int    `json:"user_id" gorm:"not null"`
	TokenId      int    `json:"token_id" gorm:"not null;default:0"`
	Kind         string `json:"kind" gorm:"type:varchar(16);not null"`
	Quota        int64  `json:"quota" gorm:"not null;default:0"`
	Status       string `json:"status" gorm:"type:varchar(16);not null;index:idx_enterprise_resv_ent,priority:2;index:idx_enterprise_resv_open,priority:1"`
	// 受理时的模型与令牌名称快照。企业账单只查主库的这张表：日志库可能在另一台服务器，
	// 不能联表，日志写入也可能遗漏；令牌改名或删除后账单仍显示当时的名称。
	ModelName string `json:"model_name" gorm:"type:varchar(191);not null;default:''"`
	TokenName string `json:"token_name" gorm:"type:varchar(191);not null;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"not null;index:idx_enterprise_resv_time,priority:2"`
	UpdatedAt int64  `json:"updated_at" gorm:"not null;index:idx_enterprise_resv_open,priority:2"`

	// 仅回传给调用方，不落库。
	AvailableBefore int64 `json:"-" gorm:"-"`
	AvailableAfter  int64 `json:"-" gorm:"-"`
	Changed         bool  `json:"-" gorm:"-"`

	EnterpriseName string `json:"enterprise_name" gorm:"-"`
	MemberName     string `json:"member_name" gorm:"-"`
}

// EnterpriseTokenBudget 是企业令牌的安全上限与已用量，记在企业账本，
// 与成员预算在同一事务内检查。企业令牌不使用令牌表的剩余额度字段。
type EnterpriseTokenBudget struct {
	TokenId      int   `json:"token_id" gorm:"primaryKey;autoIncrement:false"`
	EnterpriseId int   `json:"enterprise_id" gorm:"not null;index"`
	MemberId     int   `json:"member_id" gorm:"not null;index"`
	LimitQuota   int64 `json:"limit_quota" gorm:"not null;default:0"` // 0 表示不另设上限
	UsedQuota    int64 `json:"used_quota" gorm:"not null;default:0"`  // 已结算与在途之和
	UpdatedAt    int64 `json:"updated_at" gorm:"not null"`
}

// EnterpriseSubject 标识一次企业请求的付款主体。
type EnterpriseSubject struct {
	EnterpriseId int
	MemberId     int
	UserId       int
	TokenId      int
	// 仅在新建记录时写入账单快照，不参与主体校验。
	ModelName string
	TokenName string
}

// enterpriseSnapshotText 把快照文本截到列宽以内，避免超长名称让预留写入失败。
func enterpriseSnapshotText(value string) string {
	if runes := []rune(value); len(runes) > 191 {
		return string(runes[:191])
	}
	return value
}

func (s EnterpriseSubject) valid() bool {
	return s.EnterpriseId > 0 && s.MemberId > 0 && s.UserId > 0
}

func enterpriseReservationOpen(status string) bool {
	return status == EnterpriseReservationReserved || status == EnterpriseReservationReview
}

// applyEnterpriseTokenBudgetTx 调整令牌已用量并在需要时检查上限。调用方已持有成员行锁，
// 同一成员的令牌因此串行，无需再单独加锁。
func applyEnterpriseTokenBudgetTx(tx *gorm.DB, sub EnterpriseSubject, delta int64, enforce bool) error {
	if sub.TokenId <= 0 || delta == 0 {
		return nil
	}
	var budget EnterpriseTokenBudget
	err := tx.Where("token_id = ?", sub.TokenId).First(&budget).Error
	now := time.Now().Unix()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		budget = EnterpriseTokenBudget{TokenId: sub.TokenId, EnterpriseId: sub.EnterpriseId, MemberId: sub.MemberId, UpdatedAt: now}
		if err := tx.Create(&budget).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if budget.MemberId != sub.MemberId {
		return ErrEnterpriseSubject
	}
	if enforce && delta > 0 && budget.LimitQuota > 0 && budget.UsedQuota+delta > budget.LimitQuota {
		return &types.QuotaRejection{Source: EnterpriseTokenQuotaSource, ResourceID: sub.TokenId,
			Available: budget.LimitQuota - budget.UsedQuota, Required: delta, Known: true, Cause: ErrEnterpriseTokenCap}
	}
	return tx.Model(&EnterpriseTokenBudget{}).Where("token_id = ?", sub.TokenId).
		Updates(map[string]any{"used_quota": budget.UsedQuota + delta, "updated_at": now}).Error
}

// adjustEnterpriseReservation 是企业预留状态机，语义与个人钱包预留一致：
// target 是绝对目标值，终态幂等，重复的终结不会二次扣费或退款。
//   - reserved：建立或调整在途占用；requireAvailable 时成员可用额度必须为正且够本次增量。
//   - settled：按真实费用终结，允许成员可用额度为负。
//   - refunded：释放全部占用。
func adjustEnterpriseReservation(sub EnterpriseSubject, requestID, kind string, target int64, state string,
	requireAvailable bool, chat *chatWalletAdmission) (EnterpriseReservation, error) {
	var result EnterpriseReservation
	if !sub.valid() || requestID == "" || len(requestID) > 96 || target < 0 || target > enterpriseMaxAmount ||
		(state != EnterpriseReservationReserved && state != EnterpriseReservationSettled && state != EnterpriseReservationRefunded) ||
		(state == EnterpriseReservationRefunded && target != 0) {
		return result, ErrEnterpriseReservation
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		member, err := lockEnterpriseMemberTx(tx, sub.MemberId)
		if err != nil {
			return err
		}
		if member.EnterpriseId != sub.EnterpriseId || member.UserId != sub.UserId {
			return ErrEnterpriseSubject
		}
		now := time.Now().Unix()
		created := false
		err = tx.Where("request_id = ?", requestID).First(&result).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = EnterpriseReservation{RequestId: requestID, EnterpriseId: sub.EnterpriseId, MemberId: sub.MemberId,
				UserId: sub.UserId, TokenId: sub.TokenId, Kind: kind, Status: EnterpriseReservationReserved,
				ModelName: enterpriseSnapshotText(sub.ModelName), TokenName: enterpriseSnapshotText(sub.TokenName), CreatedAt: now, UpdatedAt: now}
			created = true
		} else if err != nil {
			return err
		}
		if result.MemberId != sub.MemberId || result.EnterpriseId != sub.EnterpriseId {
			return ErrEnterpriseSubject
		}
		result.AvailableBefore, result.AvailableAfter = member.Available, member.Available
		if !enterpriseReservationOpen(result.Status) {
			if result.Status == state && result.Quota == target {
				return nil
			}
			return ErrEnterpriseFinalized
		}
		// 新受理的请求必须来自有效成员；已受理请求的结算与退款不受停用影响。
		if created && member.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseMemberInactive
		}
		if chat != nil {
			if result.Quota > 0 {
				// 重复准入沿用原占用。
				target = result.Quota
			} else if member.Available > int64(chat.trust) {
				target = int64(chat.initial)
			}
		}
		delta := target - result.Quota
		// 成员可用额度小于等于 0 时拒绝新请求（含免费请求）；已受理请求的扩容只看增量是否够。
		if requireAvailable && ((delta > 0 && member.Available < delta) || (created && member.Available <= 0)) {
			rejection := &types.QuotaRejection{Source: EnterpriseQuotaSource, ResourceID: sub.MemberId,
				Available: member.Available, Required: delta, Known: true}
			if member.Available > 0 && member.Reserved > 0 {
				rejection.Reason = types.QuotaReasonInFlight
			}
			return rejection
		}
		// 令牌上限使用令牌记录的归属，避免调用方传入不同令牌改写他人用量。
		tokenSub := sub
		tokenSub.TokenId = result.TokenId
		if err := applyEnterpriseTokenBudgetTx(tx, tokenSub, delta, requireAvailable && state == EnterpriseReservationReserved); err != nil {
			return err
		}
		newHold, consumed := int64(0), int64(0)
		switch state {
		case EnterpriseReservationReserved:
			newHold = target
		case EnterpriseReservationSettled:
			consumed = target
		}
		updates := map[string]any{
			"available":  member.Available - delta,
			"reserved":   member.Reserved + newHold - result.Quota,
			"consumed":   member.Consumed + consumed,
			"updated_at": now,
		}
		if delta != 0 || consumed != 0 || newHold != result.Quota {
			if err := tx.Model(&EnterpriseMember{}).Where("id = ?", member.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		// Token statistics and enterprise settlement must commit together. Async
		// recovery may replay this transaction after the HTTP process has exited.
		if state == EnterpriseReservationSettled && result.TokenId > 0 && target != 0 {
			if err := tx.Unscoped().Model(&Token{}).Where("id = ? AND enterprise_id = ? AND membership_id = ?", result.TokenId, sub.EnterpriseId, sub.MemberId).Update("used_quota", gorm.Expr("used_quota + ?", target)).Error; err != nil {
				return err
			}
		}
		result.Quota = target
		// 待对账的预留被再次调整在途占用时保持待对账，只有终结才改变状态。
		if state != EnterpriseReservationReserved || created {
			result.Status = state
		}
		result.UpdatedAt = now
		result.AvailableAfter = member.Available - delta
		result.Changed = true
		if created {
			return tx.Create(&result).Error
		}
		return tx.Model(&EnterpriseReservation{}).Where("id = ?", result.Id).
			Updates(map[string]any{"quota": result.Quota, "status": result.Status, "updated_at": now}).Error
	})
	return result, err
}

// AdjustEnterpriseReservation 调整企业请求的预留到绝对目标值。
func AdjustEnterpriseReservation(sub EnterpriseSubject, requestID string, target int, state string, requireAvailable bool) (EnterpriseReservation, error) {
	return adjustEnterpriseReservation(sub, requestID, EnterpriseChargeRequest, int64(target), state, requireAvailable, nil)
}

// ReserveEnterpriseChatQuota 同步聊天的有界首笔预留：成员可用额度高于信任线时只占用 initial，
// 否则占用完整预估。准入与扣减使用同一把成员行锁。
func ReserveEnterpriseChatQuota(sub EnterpriseSubject, requestID string, full, initial, trust int) (EnterpriseReservation, error) {
	if initial <= 0 || initial > full || trust <= 0 || initial > trust {
		return EnterpriseReservation{}, errors.New("invalid enterprise chat admission")
	}
	return adjustEnterpriseReservation(sub, requestID, EnterpriseChargeRequest, int64(full), EnterpriseReservationReserved, true, &chatWalletAdmission{initial, trust})
}

// ApplyEnterpriseCharge 记一笔已确定金额的事后调整（违规费、任务差额与退款等）。
// delta 为正表示补扣、为负表示退还；eventKey 唯一，重复提交不会重复生效。
// 补扣允许成员可用额度为负，不扣个人钱包。
func ApplyEnterpriseCharge(sub EnterpriseSubject, eventKey, kind string, delta int64) (EnterpriseReservation, error) {
	var result EnterpriseReservation
	if !sub.valid() || eventKey == "" || len(eventKey) > 96 || delta == 0 || delta > enterpriseMaxAmount || delta < -enterpriseMaxAmount {
		return result, ErrEnterpriseReservation
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		member, err := lockEnterpriseMemberTx(tx, sub.MemberId)
		if err != nil {
			return err
		}
		if member.EnterpriseId != sub.EnterpriseId || member.UserId != sub.UserId {
			return ErrEnterpriseSubject
		}
		err = tx.Where("request_id = ?", eventKey).First(&result).Error
		if err == nil {
			if result.MemberId != sub.MemberId || result.Quota != delta || result.Status != EnterpriseReservationSettled {
				return ErrEnterpriseFinalized
			}
			result.AvailableBefore, result.AvailableAfter = member.Available, member.Available
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().Unix()
		if err := applyEnterpriseTokenBudgetTx(tx, sub, delta, false); err != nil {
			return err
		}
		if err := tx.Model(&EnterpriseMember{}).Where("id = ?", member.Id).Updates(map[string]any{
			"available": member.Available - delta, "consumed": member.Consumed + delta, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		result = EnterpriseReservation{RequestId: eventKey, EnterpriseId: sub.EnterpriseId, MemberId: sub.MemberId, UserId: sub.UserId,
			TokenId: sub.TokenId, Kind: kind, Quota: delta, Status: EnterpriseReservationSettled,
			ModelName: enterpriseSnapshotText(sub.ModelName), TokenName: enterpriseSnapshotText(sub.TokenName), CreatedAt: now, UpdatedAt: now,
			AvailableBefore: member.Available, AvailableAfter: member.Available - delta, Changed: true}
		return tx.Create(&result).Error
	})
	return result, err
}

// GetEnterpriseReservationSubject 按请求标识找回付款主体，供异步任务等没有请求上下文的路径使用。
func GetEnterpriseReservationSubject(requestID string) (EnterpriseSubject, bool, error) {
	var r EnterpriseReservation
	err := DB.Where("request_id = ?", requestID).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return EnterpriseSubject{}, false, nil
	}
	if err != nil {
		return EnterpriseSubject{}, false, err
	}
	return EnterpriseSubject{EnterpriseId: r.EnterpriseId, MemberId: r.MemberId, UserId: r.UserId, TokenId: r.TokenId}, true, nil
}

// MarkStaleEnterpriseReservations 把超过 cutoff 仍未终结的预留转为待对账，返回转入数量。
// 不自动退款：结果不明时不能仅凭超时退款。
func MarkStaleEnterpriseReservations(cutoff int64, limit int) (int64, error) {
	var ids []int64
	if err := DB.Model(&EnterpriseReservation{}).
		Where("status = ? AND updated_at < ?", EnterpriseReservationReserved, cutoff).
		Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	// 条件更新：期间已被正常终结的预留不会被改回待对账。updated_at 保持不变，记录原始受理时间。
	res := DB.Model(&EnterpriseReservation{}).
		Where("id IN ? AND status = ? AND updated_at < ?", ids, EnterpriseReservationReserved, cutoff).
		Update("status", EnterpriseReservationReview)
	return res.RowsAffected, res.Error
}

// ListEnterpriseReviewReservations 平台后台的待对账预留；enterpriseID 为 0 时列出全部企业。
func ListEnterpriseReviewReservations(enterpriseID, offset, limit int) ([]*EnterpriseReservation, int64, error) {
	query := DB.Model(&EnterpriseReservation{}).Where("status = ?", EnterpriseReservationReview)
	if enterpriseID > 0 {
		query = query.Where("enterprise_id = ?", enterpriseID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*EnterpriseReservation
	if err := query.Order("id ASC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	entIDs, userIDs := make([]int, 0, len(items)), make([]int, 0, len(items))
	for _, item := range items {
		entIDs = append(entIDs, item.EnterpriseId)
		userIDs = append(userIDs, item.UserId)
	}
	names := map[int]string{}
	if len(entIDs) > 0 {
		var ents []Enterprise
		if err := DB.Select("id", "name").Where("id IN ?", entIDs).Find(&ents).Error; err == nil {
			for _, e := range ents {
				names[e.Id] = e.Name
			}
		}
	}
	briefs := enterpriseUserBriefs(DB, userIDs)
	for _, item := range items {
		item.EnterpriseName = names[item.EnterpriseId]
		item.MemberName = briefs[item.UserId].label()
	}
	return items, total, nil
}

// ResolveEnterpriseReservation 平台人工处理待对账预留：按核实的真实费用结算，或全额释放。
func ResolveEnterpriseReservation(actorID int, reservationID int64, settle bool, amount int64, reason string) error {
	reason = trimEnterpriseReason(reason)
	if reason == "" || amount < 0 {
		return ErrEnterpriseInvalidInput
	}
	var r EnterpriseReservation
	if err := DB.Where("id = ?", reservationID).First(&r).Error; err != nil {
		return ErrEnterpriseNotFound
	}
	if r.Status != EnterpriseReservationReview {
		return ErrEnterpriseFinalized
	}
	state, target := EnterpriseReservationRefunded, int64(0)
	if settle {
		state, target = EnterpriseReservationSettled, amount
	}
	sub := EnterpriseSubject{EnterpriseId: r.EnterpriseId, MemberId: r.MemberId, UserId: r.UserId, TokenId: r.TokenId}
	if _, err := adjustEnterpriseReservation(sub, r.RequestId, r.Kind, target, state, false, nil); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return appendEnterpriseAuditTx(tx, r.EnterpriseId, EnterpriseActor{UserId: actorID, Platform: true}, nil,
			"reservation.resolve", "reservation", int(r.Id), map[string]any{"status": r.Status, "quota": r.Quota},
			map[string]any{"status": state, "quota": target, "request_id": r.RequestId}, reason)
	})
}

// SetEnterpriseTokenLimit 设置企业令牌的安全上限（0 表示不另设）。
func SetEnterpriseTokenLimit(sub EnterpriseSubject, limit int64) error {
	if !sub.valid() || sub.TokenId <= 0 || limit < 0 || limit > enterpriseMaxAmount {
		return ErrEnterpriseInvalidInput
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockEnterpriseMemberTx(tx, sub.MemberId); err != nil {
			return err
		}
		now := time.Now().Unix()
		var budget EnterpriseTokenBudget
		err := tx.Where("token_id = ?", sub.TokenId).First(&budget).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&EnterpriseTokenBudget{TokenId: sub.TokenId, EnterpriseId: sub.EnterpriseId, MemberId: sub.MemberId,
				LimitQuota: limit, UpdatedAt: now}).Error
		}
		if err != nil {
			return err
		}
		if budget.MemberId != sub.MemberId || budget.EnterpriseId != sub.EnterpriseId {
			return ErrEnterpriseSubject
		}
		return tx.Model(&EnterpriseTokenBudget{}).Where("token_id = ?", sub.TokenId).
			Updates(map[string]any{"limit_quota": limit, "updated_at": now}).Error
	})
}

// GetEnterpriseTokenBudgets 批量读取令牌上限与已用量。
func GetEnterpriseTokenBudgets(tokenIDs []int) (map[int]EnterpriseTokenBudget, error) {
	out := make(map[int]EnterpriseTokenBudget, len(tokenIDs))
	if len(tokenIDs) == 0 {
		return out, nil
	}
	var rows []EnterpriseTokenBudget
	if err := DB.Where("token_id IN ?", tokenIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.TokenId] = row
	}
	return out, nil
}
