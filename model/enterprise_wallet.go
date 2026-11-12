package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	EnterpriseLedgerCredit       = "credit"        // 平台确认到账
	EnterpriseLedgerAdjust       = "adjust"        // 平台调账
	EnterpriseLedgerAllocate     = "allocate"      // 分配给成员
	EnterpriseLedgerReclaim      = "reclaim"       // 从成员回收
	EnterpriseLedgerInviteGrant  = "invite_grant"  // 邀请初始额度
	EnterpriseLedgerRequestGrant = "request_grant" // 审批通过的追加
	EnterpriseLedgerMemberSweep  = "member_sweep"  // 成员失效后余额并入未分配

	enterpriseMaxAmount = int64(1_000_000_000_000_000)
)

var (
	ErrEnterpriseInsufficientUnallocated = errors.New("企业未分配余额不足")
	ErrEnterpriseDuplicateVoucher        = errors.New("该转账凭证号已录入")
	ErrEnterpriseReclaimExceeds          = errors.New("回收金额超过成员可用额度")
)

// EnterpriseLedger 记录企业资产的入账与内部划拨，只追加。
// 模型消费记录在 EnterpriseReservation（按请求），不在这里重复。
type EnterpriseLedger struct {
	Id               int64  `json:"id" gorm:"primaryKey"`
	EnterpriseId     int    `json:"enterprise_id" gorm:"not null;index:idx_enterprise_ledger_ent,priority:1;uniqueIndex:idx_enterprise_ledger_voucher,priority:1"`
	MemberId         int    `json:"member_id" gorm:"not null;default:0;index"`
	Kind             string `json:"kind" gorm:"type:varchar(24);not null"`
	UnallocatedDelta int64  `json:"unallocated_delta" gorm:"not null;default:0"`
	MemberDelta      int64  `json:"member_delta" gorm:"not null;default:0"`
	UnallocatedAfter int64  `json:"unallocated_after" gorm:"not null;default:0"`
	MemberAfter      int64  `json:"member_after" gorm:"not null;default:0"`
	// Voucher 仅入账流水填写；同一企业内唯一，其他流水为空（唯一索引允许多个空值）。
	Voucher   *string `json:"voucher" gorm:"type:varchar(128);uniqueIndex:idx_enterprise_ledger_voucher,priority:2"`
	ActorId   int     `json:"actor_id" gorm:"not null"`
	ActorRole string  `json:"actor_role" gorm:"type:varchar(16);not null"`
	Reason    string  `json:"reason" gorm:"type:varchar(255)"`
	CreatedAt int64   `json:"created_at" gorm:"not null;index:idx_enterprise_ledger_ent,priority:2"`

	MemberName string `json:"member_name" gorm:"-"`
	ActorName  string `json:"actor_name" gorm:"-"`
}

func trimEnterpriseReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if r := []rune(reason); len(r) > 255 {
		reason = string(r[:255])
	}
	return reason
}

func validEnterpriseAmount(amount int64) bool {
	return amount > 0 && amount <= enterpriseMaxAmount
}

// CreditEnterprise 平台核对对公转账到账后入账：未分配额度与累计净入账同时增加。
// 不调用任何邀请返佣逻辑。未分配为负时，入账自动先抵欠额。
func CreditEnterprise(actorID, enterpriseID int, amount int64, voucher, reason string) (*EnterpriseLedger, error) {
	voucher = strings.TrimSpace(voucher)
	if !validEnterpriseAmount(amount) || voucher == "" || len(voucher) > 128 {
		return nil, ErrEnterpriseInvalidInput
	}
	var entry *EnterpriseLedger
	err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		// 先查一次给出明确错误；唯一索引兜住并发重复录入。
		var dup int64
		if err := tx.Model(&EnterpriseLedger{}).Where("enterprise_id = ? AND voucher = ?", ent.Id, voucher).Count(&dup).Error; err != nil {
			return err
		}
		if dup > 0 {
			return ErrEnterpriseDuplicateVoucher
		}
		now := time.Now().Unix()
		entry = &EnterpriseLedger{EnterpriseId: ent.Id, Kind: EnterpriseLedgerCredit, UnallocatedDelta: amount,
			UnallocatedAfter: ent.Unallocated + amount, Voucher: &voucher, ActorId: actorID, ActorRole: EnterpriseActorPlatform,
			Reason: trimEnterpriseReason(reason), CreatedAt: now}
		if err := tx.Create(entry).Error; err != nil {
			if isEnterpriseDuplicateKeyErr(err) {
				return ErrEnterpriseDuplicateVoucher
			}
			return err
		}
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).Updates(map[string]any{
			"unallocated": ent.Unallocated + amount, "net_credited": ent.NetCredited + amount,
			"low_balance_notified_at": 0, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, EnterpriseActor{UserId: actorID, Platform: true}, nil,
			"wallet.credit", "enterprise", ent.Id, map[string]any{"unallocated": ent.Unallocated},
			map[string]any{"unallocated": ent.Unallocated + amount, "amount": amount, "voucher": voucher}, reason)
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// AdjustEnterprise 平台调账（可正可负），必须填写原因。用于纠正录错的入账或受控退款。
func AdjustEnterprise(actorID, enterpriseID int, delta int64, reason string) (*EnterpriseLedger, error) {
	reason = trimEnterpriseReason(reason)
	if delta == 0 || delta > enterpriseMaxAmount || delta < -enterpriseMaxAmount || reason == "" {
		return nil, ErrEnterpriseInvalidInput
	}
	var entry *EnterpriseLedger
	err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		now := time.Now().Unix()
		entry = &EnterpriseLedger{EnterpriseId: ent.Id, Kind: EnterpriseLedgerAdjust, UnallocatedDelta: delta,
			UnallocatedAfter: ent.Unallocated + delta, ActorId: actorID, ActorRole: EnterpriseActorPlatform, Reason: reason, CreatedAt: now}
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).Updates(map[string]any{
			"unallocated": ent.Unallocated + delta, "net_credited": ent.NetCredited + delta, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, EnterpriseActor{UserId: actorID, Platform: true}, nil,
			"wallet.adjust", "enterprise", ent.Id, map[string]any{"unallocated": ent.Unallocated},
			map[string]any{"unallocated": ent.Unallocated + delta, "delta": delta}, reason)
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// transferEnterpriseQuotaTx 把未分配额度划给成员。调用方必须已持有企业行锁（ent 为锁内读取的值）。
// 未分配不足（含为负）时拒绝；成员为负时，划入金额先抵欠额。
func transferEnterpriseQuotaTx(tx *gorm.DB, ent *Enterprise, member *EnterpriseMember, amount int64, kind string,
	actor EnterpriseActor, actorRole, reason string) error {
	if !validEnterpriseAmount(amount) {
		return ErrEnterpriseInvalidInput
	}
	if ent.Unallocated < amount {
		return ErrEnterpriseInsufficientUnallocated
	}
	locked, err := lockEnterpriseMemberTx(tx, member.Id)
	if err != nil {
		return err
	}
	if locked.EnterpriseId != ent.Id || locked.Status != EnterpriseMemberStatusActive {
		return ErrEnterpriseNotFound
	}
	now := time.Now().Unix()
	if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).
		Updates(map[string]any{"unallocated": ent.Unallocated - amount, "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&EnterpriseMember{}).Where("id = ?", locked.Id).Updates(map[string]any{
		"available": locked.Available + amount, "low_balance_notified_at": 0, "updated_at": now,
	}).Error; err != nil {
		return err
	}
	if err := tx.Create(&EnterpriseLedger{EnterpriseId: ent.Id, MemberId: locked.Id, Kind: kind,
		UnallocatedDelta: -amount, MemberDelta: amount, UnallocatedAfter: ent.Unallocated - amount,
		MemberAfter: locked.Available + amount, ActorId: actor.UserId, ActorRole: actorRole,
		Reason: trimEnterpriseReason(reason), CreatedAt: now}).Error; err != nil {
		return err
	}
	ent.Unallocated -= amount
	member.Available = locked.Available + amount
	return nil
}

// sweepEnterpriseMemberAvailableTx 把成员可用额度（无论正负）并入未分配。调用方必须持有企业行锁。
func sweepEnterpriseMemberAvailableTx(tx *gorm.DB, ent *Enterprise, member *EnterpriseMember, actor EnterpriseActor, actorRole, reason string) error {
	locked, err := lockEnterpriseMemberTx(tx, member.Id)
	if err != nil {
		return err
	}
	if locked.EnterpriseId != ent.Id {
		return ErrEnterpriseNotFound
	}
	amount := locked.Available
	if amount == 0 {
		return nil
	}
	now := time.Now().Unix()
	if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).
		Updates(map[string]any{"unallocated": ent.Unallocated + amount, "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&EnterpriseMember{}).Where("id = ?", locked.Id).
		Updates(map[string]any{"available": 0, "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Create(&EnterpriseLedger{EnterpriseId: ent.Id, MemberId: locked.Id, Kind: EnterpriseLedgerMemberSweep,
		UnallocatedDelta: amount, MemberDelta: -amount, UnallocatedAfter: ent.Unallocated + amount, MemberAfter: 0,
		ActorId: actor.UserId, ActorRole: actorRole, Reason: trimEnterpriseReason(reason), CreatedAt: now}).Error; err != nil {
		return err
	}
	ent.Unallocated += amount
	member.Available = 0
	return nil
}

// SweepEnterpriseMemberResidue 成员失效后，在途请求结算可能再次改变其可用额度；
// 每次发现残余（无论正负）就并入未分配并记一笔流水，在途预留之后终结产生的新残余由下一次调用归并。
func SweepEnterpriseMemberResidue(enterpriseID, memberID int) error {
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		member, err := lockEnterpriseMemberTx(tx, memberID)
		if err != nil {
			return err
		}
		if member.EnterpriseId != ent.Id || member.Status == EnterpriseMemberStatusActive || member.Available == 0 {
			return nil
		}
		return sweepEnterpriseMemberAvailableTx(tx, ent, member, EnterpriseActor{}, EnterpriseActorSystem, "成员失效后结算归并")
	})
}

// SweepInactiveEnterpriseResidues 由后台任务调用，归并所有失效成员的残余额度。
func SweepInactiveEnterpriseResidues(limit int) (int, error) {
	var members []EnterpriseMember
	if err := DB.Select("id", "enterprise_id").
		Where("status <> ? AND available <> 0", EnterpriseMemberStatusActive).Limit(limit).Find(&members).Error; err != nil {
		return 0, err
	}
	done := 0
	for _, m := range members {
		if err := SweepEnterpriseMemberResidue(m.EnterpriseId, m.Id); err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}

// AllocateEnterpriseQuota 管理员给成员分配额度。管理员不能给自己分配，管理员的额度由所有者分配。
func AllocateEnterpriseQuota(actor EnterpriseActor, enterpriseID, memberID int, amount int64, reason string) error {
	if !validEnterpriseAmount(amount) {
		return ErrEnterpriseInvalidInput
	}
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		if ent.Status != EnterpriseStatusActive {
			return ErrEnterpriseFrozen
		}
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		target, err := lockEnterpriseMemberTx(tx, memberID)
		if err != nil || target.EnterpriseId != ent.Id || target.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseNotFound
		}
		if err := canManageEnterpriseMember(actor, actorMember, target); err != nil {
			return err
		}
		before := target.Available
		if err := transferEnterpriseQuotaTx(tx, ent, target, amount, EnterpriseLedgerAllocate, actor, actor.auditRole(actorMember), reason); err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "budget.allocate", "member", target.Id,
			map[string]any{"available": before}, map[string]any{"available": target.Available, "amount": amount}, reason)
	})
}

// ReclaimEnterpriseQuota 从成员回收额度，仅限可用额度为正的部分。
func ReclaimEnterpriseQuota(actor EnterpriseActor, enterpriseID, memberID int, amount int64, reason string) error {
	if !validEnterpriseAmount(amount) {
		return ErrEnterpriseInvalidInput
	}
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		target, err := lockEnterpriseMemberTx(tx, memberID)
		if err != nil || target.EnterpriseId != ent.Id || target.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseNotFound
		}
		if err := canManageEnterpriseMember(actor, actorMember, target); err != nil {
			return err
		}
		if target.Available < amount {
			return ErrEnterpriseReclaimExceeds
		}
		now := time.Now().Unix()
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).
			Updates(map[string]any{"unallocated": ent.Unallocated + amount, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&EnterpriseMember{}).Where("id = ?", target.Id).
			Updates(map[string]any{"available": target.Available - amount, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&EnterpriseLedger{EnterpriseId: ent.Id, MemberId: target.Id, Kind: EnterpriseLedgerReclaim,
			UnallocatedDelta: amount, MemberDelta: -amount, UnallocatedAfter: ent.Unallocated + amount,
			MemberAfter: target.Available - amount, ActorId: actor.UserId, ActorRole: actor.auditRole(actorMember),
			Reason: trimEnterpriseReason(reason), CreatedAt: now}).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "budget.reclaim", "member", target.Id,
			map[string]any{"available": target.Available}, map[string]any{"available": target.Available - amount, "amount": amount}, reason)
	})
}

// EnterpriseBalance 是企业账务的汇总快照。Balanced 为真表示守恒式成立：
// NetCredited = Unallocated + MemberAvailable + MemberReserved + Consumed。
type EnterpriseBalance struct {
	NetCredited     int64 `json:"net_credited"`
	Unallocated     int64 `json:"unallocated"`
	MemberAvailable int64 `json:"member_available"`
	MemberReserved  int64 `json:"member_reserved"`
	Consumed        int64 `json:"consumed"`
	Balanced        bool  `json:"balanced"`
}

func GetEnterpriseBalance(enterpriseID int) (*EnterpriseBalance, error) {
	out := &EnterpriseBalance{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		ent, err := lockEnterpriseTx(tx, enterpriseID)
		if err != nil {
			return err
		}
		var sums struct {
			Available int64
			Reserved  int64
			Consumed  int64
		}
		if err := tx.Model(&EnterpriseMember{}).Where("enterprise_id = ?", ent.Id).
			Select("COALESCE(SUM(available),0) AS available, COALESCE(SUM(reserved),0) AS reserved, COALESCE(SUM(consumed),0) AS consumed").
			Scan(&sums).Error; err != nil {
			return err
		}
		out.NetCredited, out.Unallocated = ent.NetCredited, ent.Unallocated
		out.MemberAvailable, out.MemberReserved, out.Consumed = sums.Available, sums.Reserved, sums.Consumed
		out.Balanced = out.NetCredited == out.Unallocated+out.MemberAvailable+out.MemberReserved+out.Consumed
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListEnterpriseLedger 企业资产流水，按时间倒序；memberID 大于 0 时只看该成员。
func ListEnterpriseLedger(enterpriseID, memberID int, kind string, offset, limit int) ([]*EnterpriseLedger, int64, error) {
	query := DB.Model(&EnterpriseLedger{}).Where("enterprise_id = ?", enterpriseID)
	if memberID > 0 {
		query = query.Where("member_id = ?", memberID)
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*EnterpriseLedger
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	memberIDs := make([]int, 0, len(items))
	actorIDs := make([]int, 0, len(items))
	for _, item := range items {
		if item.MemberId > 0 {
			memberIDs = append(memberIDs, item.MemberId)
		}
		actorIDs = append(actorIDs, item.ActorId)
	}
	memberUsers := map[int]int{}
	if len(memberIDs) > 0 {
		var rows []EnterpriseMember
		if err := DB.Select("id", "user_id").Where("id IN ?", memberIDs).Find(&rows).Error; err == nil {
			for _, row := range rows {
				memberUsers[row.Id] = row.UserId
				actorIDs = append(actorIDs, row.UserId)
			}
		}
	}
	briefs := enterpriseUserBriefs(DB, actorIDs)
	for _, item := range items {
		if uid, ok := memberUsers[item.MemberId]; ok {
			item.MemberName = briefs[uid].label()
		}
		switch item.ActorRole {
		case EnterpriseActorPlatform:
			item.ActorName = "平台"
		case EnterpriseActorSystem:
			item.ActorName = "系统"
		default:
			item.ActorName = briefs[item.ActorId].label()
		}
	}
	return items, total, nil
}
