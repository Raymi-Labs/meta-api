package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func enterpriseInvitationDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeEnterpriseEmail(email string) string {
	return strings.ToLower(common.NormalizeEmailAddress(email))
}

// GetActiveEnterpriseMemberByUser 返回账号当前的有效成员关系；未加入企业时返回 nil。
func GetActiveEnterpriseMemberByUser(userID int) (*EnterpriseMember, error) {
	if userID <= 0 {
		return nil, nil
	}
	var member EnterpriseMember
	err := DB.Where("active_user_id = ?", userID).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return member.fill(), nil
}

// GetEnterpriseMember 按企业条件读取成员，跨企业访问返回统一的不可访问错误。
func GetEnterpriseMember(enterpriseID, memberID int) (*EnterpriseMember, error) {
	var member EnterpriseMember
	err := DB.Where("id = ? AND enterprise_id = ?", memberID, enterpriseID).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnterpriseNotFound
		}
		return nil, err
	}
	return member.fill(), nil
}

// GetEnterpriseTokenContext 供请求鉴权使用：每次请求直接读主库，不依赖可能过期的缓存。
// 成员关系必须与令牌记录的成员代次一致且仍然有效。
func GetEnterpriseTokenContext(enterpriseID, membershipID, userID int) (*Enterprise, *EnterpriseMember, error) {
	var member EnterpriseMember
	err := DB.Where("id = ? AND enterprise_id = ? AND user_id = ?", membershipID, enterpriseID, userID).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrEnterpriseNotFound
		}
		return nil, nil, err
	}
	ent, err := GetEnterpriseByID(enterpriseID)
	if err != nil {
		return nil, nil, err
	}
	return ent, member.fill(), nil
}

// ValidateEnterpriseToken 校验企业令牌当前是否可用于调用：功能已开启、成员关系有效、企业未冻结。
// 每次请求都读主库，停用成员或冻结企业后立即生效。
func ValidateEnterpriseToken(token *Token) (*Enterprise, *EnterpriseMember, error) {
	if token == nil || token.EnterpriseId <= 0 || token.MembershipId <= 0 {
		return nil, nil, ErrEnterpriseNotFound
	}
	if !common.EnterpriseEnabled {
		return nil, nil, ErrEnterpriseDisabled
	}
	ent, member, err := GetEnterpriseTokenContext(token.EnterpriseId, token.MembershipId, token.UserId)
	if err != nil {
		return nil, nil, err
	}
	if member.Status != EnterpriseMemberStatusActive {
		return nil, nil, ErrEnterpriseMemberInactive
	}
	if ent.Status != EnterpriseStatusActive {
		return nil, nil, ErrEnterpriseFrozen
	}
	return ent, member, nil
}

// GetEnterprisePayer 返回账号当前可用于付款的企业与成员关系，供创建企业令牌等入口使用。
func GetEnterprisePayer(userID int) (*Enterprise, *EnterpriseMember, error) {
	if !common.EnterpriseEnabled {
		return nil, nil, ErrEnterpriseDisabled
	}
	member, err := GetActiveEnterpriseMemberByUser(userID)
	if err != nil {
		return nil, nil, err
	}
	if member == nil {
		return nil, nil, ErrEnterpriseNotFound
	}
	ent, err := GetEnterpriseByID(member.EnterpriseId)
	if err != nil {
		return nil, nil, err
	}
	if ent.Status != EnterpriseStatusActive {
		return nil, nil, ErrEnterpriseFrozen
	}
	return ent, member, nil
}

// CreateEnterpriseToken 在成员行锁内创建企业令牌并写入令牌上限，令牌只会挂在仍然有效的成员代次上。
// 企业令牌不使用令牌表的剩余额度：固定为不限额，实际可用额度由成员预算和企业账本里的令牌上限约束。
func CreateEnterpriseToken(token *Token, limit int64) error {
	if token == nil || token.UserId <= 0 || token.EnterpriseId <= 0 || token.MembershipId <= 0 ||
		limit < 0 || limit > enterpriseMaxAmount {
		return ErrEnterpriseInvalidInput
	}
	token.UnlimitedQuota, token.RemainQuota = true, 0
	return DB.Transaction(func(tx *gorm.DB) error {
		member, err := lockEnterpriseMemberTx(tx, token.MembershipId)
		if err != nil {
			return err
		}
		if member.EnterpriseId != token.EnterpriseId || member.UserId != token.UserId {
			return ErrEnterpriseNotFound
		}
		if member.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseMemberInactive
		}
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		if limit == 0 {
			return nil
		}
		return tx.Create(&EnterpriseTokenBudget{TokenId: token.Id, EnterpriseId: member.EnterpriseId, MemberId: member.Id,
			LimitQuota: limit, UpdatedAt: time.Now().Unix()}).Error
	})
}

func fillEnterpriseMemberUsers(members []*EnterpriseMember) {
	ids := make([]int, 0, len(members))
	for _, m := range members {
		m.fill()
		ids = append(ids, m.UserId)
	}
	briefs := enterpriseUserBriefs(DB, ids)
	for _, m := range members {
		if brief, ok := briefs[m.UserId]; ok {
			m.Username, m.DisplayName, m.Email = brief.Username, brief.DisplayName, brief.Email
		}
	}
}

// ListEnterpriseMembers 列出企业成员。status 为空时只返回有效成员。
func ListEnterpriseMembers(enterpriseID int, status, keyword string, offset, limit int) ([]*EnterpriseMember, int64, error) {
	if status == "" {
		status = EnterpriseMemberStatusActive
	}
	query := DB.Model(&EnterpriseMember{}).Where("enterprise_id = ?", enterpriseID)
	if status != "all" {
		query = query.Where("status = ?", status)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		var userIDs []int
		if err := DB.Model(&User{}).Unscoped().Where("username LIKE ? OR display_name LIKE ? OR email LIKE ?", like, like, like).
			Limit(500).Pluck("id", &userIDs).Error; err != nil {
			return nil, 0, err
		}
		if len(userIDs) == 0 {
			return []*EnterpriseMember{}, 0, nil
		}
		query = query.Where("user_id IN ?", userIDs)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var members []*EnterpriseMember
	if err := query.Order("id ASC").Offset(offset).Limit(limit).Find(&members).Error; err != nil {
		return nil, 0, err
	}
	fillEnterpriseMemberUsers(members)
	return members, total, nil
}

// canManageEnterpriseMember 判断操作者能否管理目标成员的额度、模型或状态：
// 普通成员由管理员或所有者管理；管理员与所有者只能由所有者管理；平台不受限。
// 管理员不能操作自己，所有者对自己的调整作为直接管理操作留痕。
func canManageEnterpriseMember(actor EnterpriseActor, actorMember, target *EnterpriseMember) error {
	if actor.Platform {
		return nil
	}
	if actorMember == nil || !EnterpriseRoleAtLeast(actorMember.Role, EnterpriseRoleAdmin) {
		return ErrEnterpriseForbidden
	}
	if actorMember.Role == EnterpriseRoleOwner {
		return nil
	}
	if actorMember.Id == target.Id {
		return ErrEnterpriseSelfOperation
	}
	if target.Role != EnterpriseRoleMember {
		return ErrEnterpriseForbidden
	}
	return nil
}

// CreateEnterpriseInvitation 创建邀请并返回一次性明文令牌；库内只保存摘要。
// 未接受的邀请不占用预算。
func CreateEnterpriseInvitation(actor EnterpriseActor, enterpriseID int, email, role string, initialQuota int64) (string, *EnterpriseInvitation, error) {
	email = normalizeEnterpriseEmail(email)
	if email == "" || len(email) > 254 || !strings.Contains(email, "@") || initialQuota < 0 || initialQuota > enterpriseMaxAmount {
		return "", nil, ErrEnterpriseInvalidInput
	}
	if role == "" {
		role = EnterpriseRoleMember
	}
	if role != EnterpriseRoleMember && role != EnterpriseRoleAdmin {
		return "", nil, ErrEnterpriseInvalidInput
	}
	token, err := DesktopLoginToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now().Unix()
	inv := &EnterpriseInvitation{Digest: enterpriseInvitationDigest(token), EnterpriseId: enterpriseID, Email: email, Role: role,
		InitialQuota: initialQuota, InviterId: actor.UserId, Status: EnterpriseInvitationPending,
		ExpiresAt: now + EnterpriseInvitationTTLSeconds, CreatedAt: now}
	err = enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		if ent.Status != EnterpriseStatusActive {
			return ErrEnterpriseFrozen
		}
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		// Invitations delegate an enterprise membership, never an inferred platform role.
		if actor.Platform || actorMember == nil || !EnterpriseRoleAtLeast(actorMember.Role, EnterpriseRoleAdmin) {
			return ErrEnterpriseForbidden
		}
		if role == EnterpriseRoleAdmin && actorMember.Role != EnterpriseRoleOwner {
			return ErrEnterpriseForbidden
		}
		var issuer User
		if err := tx.Select("id", "email", "status").Where("id = ?", actor.UserId).First(&issuer).Error; err != nil || issuer.Status != common.UserStatusEnabled {
			return ErrEnterpriseForbidden
		}
		if normalizeEnterpriseEmail(issuer.Email) == email {
			return ErrEnterpriseSelfOperation
		}
		inv.InviterMemberId = actorMember.Id
		// 同一邮箱只保留最新一封待接受邀请。
		if err := tx.Model(&EnterpriseInvitation{}).
			Where("enterprise_id = ? AND email = ? AND status = ?", ent.Id, email, EnterpriseInvitationPending).
			Update("status", EnterpriseInvitationRevoked).Error; err != nil {
			return err
		}
		if err := tx.Create(inv).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "invitation.create", "invitation", inv.Id, nil,
			map[string]any{"email": email, "role": role, "initial_quota": initialQuota}, "")
	})
	if err != nil {
		return "", nil, err
	}
	return token, inv, nil
}

// RevokeEnterpriseInvitation 撤销尚未接受的邀请。
func RevokeEnterpriseInvitation(actor EnterpriseActor, enterpriseID, invitationID int) error {
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		if !actor.Platform && !EnterpriseRoleAtLeast(actorMember.Role, EnterpriseRoleAdmin) {
			return ErrEnterpriseForbidden
		}
		res := tx.Model(&EnterpriseInvitation{}).
			Where("id = ? AND enterprise_id = ? AND status = ?", invitationID, ent.Id, EnterpriseInvitationPending).
			Update("status", EnterpriseInvitationRevoked)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrEnterpriseInvitation
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "invitation.revoke", "invitation", invitationID, nil, nil, "")
	})
}

// ListEnterpriseInvitations 列出企业邀请（最近的在前）。
func ListEnterpriseInvitations(enterpriseID, offset, limit int) ([]*EnterpriseInvitation, int64, error) {
	query := DB.Model(&EnterpriseInvitation{}).Where("enterprise_id = ?", enterpriseID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*EnterpriseInvitation
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// EnterpriseInvitationPreview 是接受页展示的信息，不含任何企业内部数据。
type EnterpriseInvitationPreview struct {
	EnterpriseName string `json:"enterprise_name"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	ExpiresAt      int64  `json:"expires_at"`
}

func PreviewEnterpriseInvitation(token string) (*EnterpriseInvitationPreview, error) {
	if !ValidDesktopLoginProof(token) {
		return nil, ErrEnterpriseInvitation
	}
	var inv EnterpriseInvitation
	if err := DB.Where("digest = ? AND status = ? AND expires_at > ?", enterpriseInvitationDigest(token),
		EnterpriseInvitationPending, time.Now().Unix()).First(&inv).Error; err != nil {
		return nil, ErrEnterpriseInvitation
	}
	ent, err := GetEnterpriseByID(inv.EnterpriseId)
	if err != nil || ent.Status != EnterpriseStatusActive {
		return nil, ErrEnterpriseInvitation
	}
	return &EnterpriseInvitationPreview{EnterpriseName: ent.Name, Email: inv.Email, Role: inv.Role, ExpiresAt: inv.ExpiresAt}, nil
}

// EnterpriseInvitationResult 描述接受邀请的结果。QuotaShortfall 为真表示企业未分配余额不足，
// 成员已加入但初始额度为 0，需要提示邀请人。
type EnterpriseInvitationResult struct {
	Member         *EnterpriseMember
	Invitation     *EnterpriseInvitation
	Enterprise     *Enterprise
	QuotaShortfall bool
}

// AcceptEnterpriseInvitation 以登录用户 ID 为准建立成员关系，邮箱一致只作为校验。
// 邀请一次性消费；初始额度在同一事务内从未分配额度划拨。
func AcceptEnterpriseInvitation(token string, userID int) (*EnterpriseInvitationResult, error) {
	if !ValidDesktopLoginProof(token) || userID <= 0 {
		return nil, ErrEnterpriseInvitation
	}
	digest := enterpriseInvitationDigest(token)
	var probe EnterpriseInvitation
	if err := DB.Select("id", "enterprise_id").Where("digest = ?", digest).First(&probe).Error; err != nil {
		return nil, ErrEnterpriseInvitation
	}
	result := &EnterpriseInvitationResult{}
	err := enterpriseTx(probe.EnterpriseId, func(tx *gorm.DB, ent *Enterprise) error {
		if ent.Status != EnterpriseStatusActive {
			return ErrEnterpriseInvitation
		}
		now := time.Now().Unix()
		var inv EnterpriseInvitation
		if err := tx.Where("id = ?", probe.Id).First(&inv).Error; err != nil {
			return ErrEnterpriseInvitation
		}
		if inv.Status != EnterpriseInvitationPending || inv.ExpiresAt <= now {
			return ErrEnterpriseInvitation
		}
		var user User
		if err := tx.Select("id", "email", "status").Where("id = ?", userID).First(&user).Error; err != nil {
			return ErrEnterpriseInvitation
		}
		if user.Status != common.UserStatusEnabled {
			return ErrEnterpriseInvitation
		}
		if normalizeEnterpriseEmail(user.Email) != inv.Email {
			return ErrEnterpriseInvitationMail
		}
		if inv.InviterId == userID {
			return ErrEnterpriseSelfOperation
		}
		// Old pending invitations without an issuing generation must be reissued.
		if inv.InviterMemberId <= 0 {
			return ErrEnterpriseInvitation
		}
		inviterMember, err := lockEnterpriseMemberTx(tx, inv.InviterMemberId)
		if err != nil || inviterMember.EnterpriseId != ent.Id || inviterMember.UserId != inv.InviterId || inviterMember.Status != EnterpriseMemberStatusActive || !EnterpriseRoleAtLeast(inviterMember.Role, EnterpriseRoleAdmin) {
			return ErrEnterpriseInvitation
		}
		if inv.Role == EnterpriseRoleAdmin && inviterMember.Role != EnterpriseRoleOwner {
			return ErrEnterpriseInvitation
		}
		var issuer User
		if err := tx.Select("id", "status").Where("id = ?", inv.InviterId).First(&issuer).Error; err != nil || issuer.Status != common.UserStatusEnabled {
			return ErrEnterpriseInvitation
		}
		// 条件更新保证多节点并发下只消费一次。
		res := tx.Model(&EnterpriseInvitation{}).
			Where("id = ? AND status = ? AND expires_at > ?", inv.Id, EnterpriseInvitationPending, now).
			Updates(map[string]any{"status": EnterpriseInvitationAccepted, "accepted_by": userID, "accepted_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrEnterpriseInvitation
		}
		var existing int64
		if err := tx.Model(&EnterpriseMember{}).Where("active_user_id = ?", userID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return ErrEnterpriseAlreadyMember
		}
		var previous int64
		if err := tx.Model(&EnterpriseMember{}).Where("enterprise_id = ? AND user_id = ?", ent.Id, userID).Count(&previous).Error; err != nil {
			return err
		}
		member := &EnterpriseMember{EnterpriseId: ent.Id, UserId: userID, ActiveUserId: &userID, Role: inv.Role,
			Status: EnterpriseMemberStatusActive, Generation: int(previous) + 1,
			ModelMode: EnterpriseModelModeInherit, ModelList: "[]", JoinedAt: now, UpdatedAt: now}
		if err := tx.Create(member).Error; err != nil {
			if isEnterpriseDuplicateKeyErr(err) {
				return ErrEnterpriseAlreadyMember
			}
			return err
		}
		actor := EnterpriseActor{UserId: userID}
		if err := appendEnterpriseAuditTx(tx, ent.Id, actor, member, "member.join", "member", member.Id, nil,
			map[string]any{"role": member.Role, "generation": member.Generation, "invitation_id": inv.Id}, ""); err != nil {
			return err
		}
		if inv.InitialQuota > 0 {
			if ent.Unallocated >= inv.InitialQuota {
				inviter, inviterRole := EnterpriseActor{UserId: inv.InviterId}, inviterMember.Role
				if err := transferEnterpriseQuotaTx(tx, ent, member, inv.InitialQuota, EnterpriseLedgerInviteGrant, inviter, inviterRole, "邀请初始额度"); err != nil {
					return err
				}
				inv.GrantedQuota = inv.InitialQuota
				if err := tx.Model(&EnterpriseInvitation{}).Where("id = ?", inv.Id).Update("granted_quota", inv.GrantedQuota).Error; err != nil {
					return err
				}
			} else {
				result.QuotaShortfall = true
			}
		}
		inv.Status, inv.AcceptedBy, inv.AcceptedAt = EnterpriseInvitationAccepted, userID, now
		result.Member, result.Invitation, result.Enterprise = member, &inv, ent
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SetEnterpriseMemberRole 任免管理员，仅所有者或平台可操作；所有者变更走所有权转移。
func SetEnterpriseMemberRole(actor EnterpriseActor, enterpriseID, memberID int, role string) error {
	if role != EnterpriseRoleAdmin && role != EnterpriseRoleMember {
		return ErrEnterpriseInvalidInput
	}
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		if !actor.Platform && actorMember.Role != EnterpriseRoleOwner {
			return ErrEnterpriseForbidden
		}
		target, err := lockEnterpriseMemberTx(tx, memberID)
		if err != nil || target.EnterpriseId != ent.Id || target.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseNotFound
		}
		if target.Role == EnterpriseRoleOwner {
			return ErrEnterpriseOwnerRequired
		}
		if target.Role == role {
			return nil
		}
		if err := revokeInvitationsByMemberTx(tx, ent.Id, target.UserId); err != nil {
			return err
		}
		if err := tx.Model(&EnterpriseMember{}).Where("id = ?", target.Id).
			Updates(map[string]any{"role": role, "updated_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "member.role", "member", target.Id,
			map[string]any{"role": target.Role}, map[string]any{"role": role}, "")
	})
}

// TransferEnterpriseOwnership 把所有权转给另一名有效成员，原所有者降为管理员。
// 企业始终恰有一名有效所有者。
func TransferEnterpriseOwnership(actor EnterpriseActor, enterpriseID, memberID int) error {
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		if !actor.Platform && actorMember.Role != EnterpriseRoleOwner {
			return ErrEnterpriseForbidden
		}
		target, err := lockEnterpriseMemberTx(tx, memberID)
		if err != nil || target.EnterpriseId != ent.Id || target.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseNotFound
		}
		var targetUser User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&targetUser, target.UserId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnterpriseOwnerAccount
			}
			return err
		}
		if targetUser.Status != common.UserStatusEnabled {
			return ErrEnterpriseOwnerAccount
		}
		if target.Role == EnterpriseRoleOwner {
			return nil
		}
		if err := revokeInvitationsByMemberTx(tx, ent.Id, ent.OwnerUserId); err != nil {
			return err
		}
		now := time.Now().Unix()
		if err := tx.Model(&EnterpriseMember{}).
			Where("enterprise_id = ? AND status = ? AND role = ?", ent.Id, EnterpriseMemberStatusActive, EnterpriseRoleOwner).
			Updates(map[string]any{"role": EnterpriseRoleAdmin, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&EnterpriseMember{}).Where("id = ?", target.Id).
			Updates(map[string]any{"role": EnterpriseRoleOwner, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).
			Updates(map[string]any{"owner_user_id": target.UserId, "updated_at": now}).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "enterprise.transfer", "member", target.Id,
			map[string]any{"owner_user_id": ent.OwnerUserId}, map[string]any{"owner_user_id": target.UserId}, "")
	})
}

// SetEnterpriseMemberModels 设置成员级模型范围：inherit 继承企业范围，custom 进一步收窄。
func SetEnterpriseMemberModels(actor EnterpriseActor, enterpriseID, memberID int, mode string, models []string) error {
	if !validEnterpriseModelMode(mode) {
		return ErrEnterpriseInvalidInput
	}
	models = normalizeEnterpriseModels(models)
	if len(models) > 2000 {
		return ErrEnterpriseInvalidInput
	}
	if mode == EnterpriseModelModeInherit {
		models = []string{}
	}
	encoded, err := encodeEnterpriseModels(models)
	if err != nil {
		return err
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
		if err := tx.Model(&EnterpriseMember{}).Where("id = ?", target.Id).
			Updates(map[string]any{"model_mode": mode, "model_list": encoded, "updated_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		if err := bumpEnterprisePolicyTx(tx, ent.Id); err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "member.models", "member", target.Id,
			map[string]any{"model_mode": target.ModelMode, "models": target.Models},
			map[string]any{"model_mode": mode, "models": models}, "")
	})
}

// revokeInvitationsByMemberTx invalidates delegated authority before membership transitions.
func revokeInvitationsByMemberTx(tx *gorm.DB, enterpriseID, userID int) error {
	return tx.Model(&EnterpriseInvitation{}).Where("enterprise_id = ? AND inviter_id = ? AND status = ?", enterpriseID, userID, EnterpriseInvitationPending).Update("status", EnterpriseInvitationRevoked).Error
}

// deactivateEnterpriseMemberTx 使成员关系失效：阻止新请求、令牌立即失效、待审批申请失效，
// 并把剩余额度（无论正负）并入未分配。在途预留保留在成员行上，结算后由 SweepEnterpriseMemberResidue 归并。
func deactivateEnterpriseMemberTx(tx *gorm.DB, ent *Enterprise, target *EnterpriseMember, status string,
	actor EnterpriseActor, actorMember *EnterpriseMember, reason string) ([]string, error) {
	if err := revokeInvitationsByMemberTx(tx, ent.Id, target.UserId); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if err := tx.Model(&EnterpriseMember{}).Where("id = ?", target.Id).Updates(map[string]any{
		"status": status, "active_user_id": nil, "left_at": now, "updated_at": now,
	}).Error; err != nil {
		return nil, err
	}
	var tokens []Token
	if err := tx.Where("enterprise_id = ? AND membership_id = ?", ent.Id, target.Id).Find(&tokens).Error; err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(tokens))
	for _, token := range tokens {
		keys = append(keys, token.Key)
	}
	if err := tx.Model(&Token{}).Where("enterprise_id = ? AND membership_id = ? AND status = ?", ent.Id, target.Id, common.TokenStatusEnabled).
		Update("status", common.TokenStatusDisabled).Error; err != nil {
		return nil, err
	}
	if err := expireEnterpriseQuotaRequestsTx(tx, "member_id = ?", target.Id); err != nil {
		return nil, err
	}
	if err := sweepEnterpriseMemberAvailableTx(tx, ent, target, actor, actor.auditRole(actorMember), reason); err != nil {
		return nil, err
	}
	if err := bumpEnterprisePolicyTx(tx, ent.Id); err != nil {
		return nil, err
	}
	action := "member.disable"
	if status == EnterpriseMemberStatusLeft {
		action = "member.leave"
	}
	if err := appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, action, "member", target.Id,
		map[string]any{"status": target.Status, "role": target.Role}, map[string]any{"status": status}, reason); err != nil {
		return nil, err
	}
	return keys, nil
}

func purgeEnterpriseTokenCache(keys []string) {
	if !common.RedisEnabled {
		return
	}
	for _, key := range keys {
		if err := cacheDeleteToken(key); err != nil {
			common.SysLog("failed to delete enterprise token cache: " + err.Error())
		}
	}
}

// DisableEnterpriseMember 停用成员。所有者不能被停用，操作者不能停用自己。
func DisableEnterpriseMember(actor EnterpriseActor, enterpriseID, memberID int, reason string) error {
	var keys []string
	err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		target, err := lockEnterpriseMemberTx(tx, memberID)
		if err != nil || target.EnterpriseId != ent.Id || target.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseNotFound
		}
		if target.Role == EnterpriseRoleOwner {
			return ErrEnterpriseOwnerRequired
		}
		if !actor.Platform && actorMember.Id == target.Id {
			return ErrEnterpriseSelfOperation
		}
		if err := canManageEnterpriseMember(actor, actorMember, target); err != nil {
			return err
		}
		keys, err = deactivateEnterpriseMemberTx(tx, ent, target, EnterpriseMemberStatusDisabled, actor, actorMember, reason)
		return err
	})
	if err != nil {
		return err
	}
	purgeEnterpriseTokenCache(keys)
	return nil
}

// LeaveEnterprise 成员主动退出。最后一名所有者不能直接退出，必须先转移所有权。
func LeaveEnterprise(userID, enterpriseID int) error {
	var keys []string
	actor := EnterpriseActor{UserId: userID}
	err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		target, err := lockEnterpriseMemberTx(tx, actorMember.Id)
		if err != nil {
			return err
		}
		if target.Status != EnterpriseMemberStatusActive {
			return ErrEnterpriseNotFound
		}
		if target.Role == EnterpriseRoleOwner {
			return ErrEnterpriseOwnerRequired
		}
		keys, err = deactivateEnterpriseMemberTx(tx, ent, target, EnterpriseMemberStatusLeft, actor, target, "")
		return err
	})
	if err != nil {
		return err
	}
	purgeEnterpriseTokenCache(keys)
	return nil
}

// GetEnterpriseMemberDetail 读取成员并带上账号的展示信息。
func GetEnterpriseMemberDetail(enterpriseID, memberID int) (*EnterpriseMember, error) {
	member, err := GetEnterpriseMember(enterpriseID, memberID)
	if err != nil {
		return nil, err
	}
	fillEnterpriseMemberUsers([]*EnterpriseMember{member})
	return member, nil
}

// Lock the account before checking ownership so transfer/create and deletion cannot
// commit an owner pointing at a deleted account. Keep this guard active when the feature is off.
func protectEnterpriseOwnerDeletionTx(tx *gorm.DB, userID int) error {
	var user User
	if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&Enterprise{}).Where("owner_user_id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrEnterpriseOwnerRequired
	}
	return nil
}
