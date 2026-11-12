package model

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 企业工作空间：企业持有独立钱包，成员额度是企业内部的预算划拨，不进入个人钱包。
// 账务守恒：NetCredited = Unallocated + Σ(Available + Reserved + Consumed)。
// 锁顺序固定为 企业行 → 成员行；请求路径只锁成员行。

const (
	EnterpriseStatusActive = "active"
	EnterpriseStatusFrozen = "frozen"

	EnterpriseRoleOwner  = "owner"
	EnterpriseRoleAdmin  = "admin"
	EnterpriseRoleMember = "member"

	EnterpriseMemberStatusActive   = "active"
	EnterpriseMemberStatusDisabled = "disabled"
	EnterpriseMemberStatusLeft     = "left"

	EnterpriseModelModeInherit = "inherit"
	EnterpriseModelModeCustom  = "custom"

	EnterpriseInvitationPending  = "pending"
	EnterpriseInvitationAccepted = "accepted"
	EnterpriseInvitationRevoked  = "revoked"

	EnterpriseInvitationTTLSeconds = 7 * 24 * 3600

	// 审计与流水中的操作者身份。
	EnterpriseActorPlatform = "platform"
	EnterpriseActorSystem   = "system"
)

var (
	// 越权与不存在使用同一个错误，避免通过响应差异探测其他企业的对象。
	ErrEnterpriseNotFound       = errors.New("企业不存在或无权访问")
	ErrEnterpriseForbidden      = errors.New("无权执行该企业操作")
	ErrEnterpriseFrozen         = errors.New("企业已冻结")
	ErrEnterpriseAlreadyMember  = errors.New("该账号已加入企业，一个账号只能加入一家企业")
	ErrEnterpriseOwnerRequired  = errors.New("企业必须保留一名所有者，请先转移所有权")
	ErrEnterpriseInvitation     = errors.New("邀请无效、已使用或已过期")
	ErrEnterpriseInvitationMail = errors.New("当前账号邮箱与邀请邮箱不一致，请先绑定被邀请的邮箱")
	ErrEnterpriseInvalidInput   = errors.New("参数无效")
	ErrEnterpriseSelfOperation  = errors.New("不能对自己执行该操作，请联系企业所有者")
	ErrEnterpriseBillingGroup   = errors.New("计费用户组不存在")
	ErrEnterpriseTimezone       = errors.New("时区无效")
	ErrEnterpriseOwnerAccount   = errors.New("所有者账号不存在或已被禁用")
	ErrEnterpriseDisabled       = errors.New("企业功能未开启")
	ErrEnterpriseUnsupported    = errors.New("暂不支持企业额度")
)

type Enterprise struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	Name         string `json:"name" gorm:"type:varchar(128);not null"`
	Status       string `json:"status" gorm:"type:varchar(16);not null;index"`
	OwnerUserId  int    `json:"owner_user_id" gorm:"not null;index"`
	BillingGroup string `json:"billing_group" gorm:"type:varchar(64);not null"`
	Timezone     string `json:"timezone" gorm:"type:varchar(64);not null"`
	// ModelMode=inherit 表示继承平台对计费用户组开放的全部模型，平台新增模型自动可用；
	// custom 表示只允许 ModelList 中的模型，空清单即全部禁止。
	ModelMode     string `json:"model_mode" gorm:"type:varchar(16);not null"`
	ModelList     string `json:"-" gorm:"type:text"`
	PolicyVersion int64  `json:"policy_version" gorm:"not null;default:0"`
	// 钱包：未分配额度与累计净入账。成员侧余额在 EnterpriseMember 上。
	Unallocated          int64 `json:"unallocated" gorm:"not null;default:0"`
	NetCredited          int64 `json:"net_credited" gorm:"not null;default:0"`
	LowBalanceThreshold  int64 `json:"low_balance_threshold" gorm:"not null;default:0"`
	LowBalanceNotifiedAt int64 `json:"-" gorm:"not null;default:0"`
	CreatedAt            int64 `json:"created_at" gorm:"not null"`
	UpdatedAt            int64 `json:"updated_at" gorm:"not null"`

	Models    []string `json:"models" gorm:"-"`
	OwnerName string   `json:"owner_name" gorm:"-"`
}

type EnterpriseMember struct {
	Id           int `json:"id" gorm:"primaryKey"`
	EnterpriseId int `json:"enterprise_id" gorm:"not null;index:idx_enterprise_member_ent,priority:1"`
	UserId       int `json:"user_id" gorm:"not null;index"`
	// ActiveUserId 仅在成员有效时等于 UserId，失效后置空；唯一索引允许多个空值，
	// 由数据库保证一个账号同时只有一个有效成员关系。
	ActiveUserId *int   `json:"-" gorm:"uniqueIndex"`
	Role         string `json:"role" gorm:"type:varchar(16);not null"`
	Status       string `json:"status" gorm:"type:varchar(16);not null;index:idx_enterprise_member_ent,priority:2"`
	// Generation 是同一账号在同一企业的成员代次，重新加入时递增，旧令牌不复活。
	Generation int    `json:"generation" gorm:"not null;default:1"`
	ModelMode  string `json:"model_mode" gorm:"type:varchar(16);not null"`
	ModelList  string `json:"-" gorm:"type:text"`
	// 成员预算：Available 可为负（结算超出预留时），为负或为零时拒绝新请求。
	Available            int64 `json:"available" gorm:"not null;default:0"`
	Reserved             int64 `json:"reserved" gorm:"not null;default:0"`
	Consumed             int64 `json:"consumed" gorm:"not null;default:0"`
	LowBalanceNotifiedAt int64 `json:"-" gorm:"not null;default:0"`
	JoinedAt             int64 `json:"joined_at" gorm:"not null"`
	LeftAt               int64 `json:"left_at" gorm:"not null;default:0"`
	UpdatedAt            int64 `json:"updated_at" gorm:"not null"`

	Models      []string `json:"models" gorm:"-"`
	Username    string   `json:"username" gorm:"-"`
	DisplayName string   `json:"display_name" gorm:"-"`
	Email       string   `json:"email" gorm:"-"`
}

type EnterpriseInvitation struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	Digest       string `json:"-" gorm:"type:varchar(64);not null;uniqueIndex"`
	EnterpriseId int    `json:"enterprise_id" gorm:"not null;index"`
	Email        string `json:"email" gorm:"type:varchar(254);not null"`
	Role         string `json:"role" gorm:"type:varchar(16);not null"`
	InitialQuota int64  `json:"initial_quota" gorm:"not null;default:0"`
	GrantedQuota int64  `json:"granted_quota" gorm:"not null;default:0"`
	InviterId    int    `json:"inviter_id" gorm:"not null"`
	Status       string `json:"status" gorm:"type:varchar(16);not null"`
	ExpiresAt    int64  `json:"expires_at" gorm:"not null"`
	AcceptedBy   int    `json:"accepted_by" gorm:"not null;default:0"`
	AcceptedAt   int64  `json:"accepted_at" gorm:"not null;default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"not null"`
}

// EnterpriseAuditEvent 只追加，不提供修改或删除入口。
type EnterpriseAuditEvent struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	EnterpriseId int    `json:"enterprise_id" gorm:"not null;index:idx_enterprise_audit_ent,priority:1"`
	ActorId      int    `json:"actor_id" gorm:"not null"`
	ActorRole    string `json:"actor_role" gorm:"type:varchar(16);not null"`
	Action       string `json:"action" gorm:"type:varchar(48);not null"`
	TargetType   string `json:"target_type" gorm:"type:varchar(24);not null"`
	TargetId     int    `json:"target_id" gorm:"not null;default:0"`
	Before       string `json:"before" gorm:"type:text"`
	After        string `json:"after" gorm:"type:text"`
	Reason       string `json:"reason" gorm:"type:varchar(255)"`
	CreatedAt    int64  `json:"created_at" gorm:"not null;index:idx_enterprise_audit_ent,priority:2"`

	ActorName string `json:"actor_name" gorm:"-"`
}

func enterpriseModels() []any {
	return []any{
		&Enterprise{}, &EnterpriseMember{}, &EnterpriseInvitation{}, &EnterpriseAuditEvent{},
		&EnterpriseLedger{}, &EnterpriseReservation{}, &EnterpriseTokenBudget{}, &EnterpriseQuotaRequest{},
	}
}

// MigrateEnterpriseTables 建立企业相关的表，供启动迁移之外的调用方（如其他包的测试）使用。
func MigrateEnterpriseTables(db *gorm.DB) error {
	return db.AutoMigrate(enterpriseModels()...)
}

// EnterpriseActor 描述一次操作的发起者。Platform 为真表示平台管理员在平台后台操作，
// 不受企业内角色约束；否则 Member 必须是该企业的有效成员。
type EnterpriseActor struct {
	UserId   int
	Platform bool
}

func (a EnterpriseActor) auditRole(member *EnterpriseMember) string {
	if a.Platform {
		return EnterpriseActorPlatform
	}
	if member != nil {
		return member.Role
	}
	return EnterpriseActorSystem
}

func enterpriseRoleRank(role string) int {
	switch role {
	case EnterpriseRoleOwner:
		return 3
	case EnterpriseRoleAdmin:
		return 2
	case EnterpriseRoleMember:
		return 1
	}
	return 0
}

// EnterpriseRoleAtLeast 判断 role 是否不低于 min。
func EnterpriseRoleAtLeast(role, min string) bool {
	return enterpriseRoleRank(role) >= enterpriseRoleRank(min) && enterpriseRoleRank(role) > 0
}

func isEnterpriseDuplicateKeyErr(err error) bool {
	return isInvoiceDuplicateKeyErr(err)
}

// normalizeEnterpriseModels 去空白、去重并排序，保证存储稳定。
func normalizeEnterpriseModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func encodeEnterpriseModels(models []string) (string, error) {
	data, err := common.Marshal(normalizeEnterpriseModels(models))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeEnterpriseModels(raw string) []string {
	out := []string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := common.UnmarshalJsonStr(raw, &out); err != nil {
		// 清单损坏时按空清单处理：custom 模式下等于全部禁止，不会意外放宽。
		return []string{}
	}
	return out
}

func validEnterpriseModelMode(mode string) bool {
	return mode == EnterpriseModelModeInherit || mode == EnterpriseModelModeCustom
}

func (e *Enterprise) fill() *Enterprise {
	if e != nil {
		e.Models = decodeEnterpriseModels(e.ModelList)
	}
	return e
}

func (m *EnterpriseMember) fill() *EnterpriseMember {
	if m != nil {
		m.Models = decodeEnterpriseModels(m.ModelList)
	}
	return m
}

// lockEnterpriseTx 锁定企业行。所有改动成员关系、角色与企业钱包的事务都先取这把锁。
func lockEnterpriseTx(tx *gorm.DB, enterpriseID int) (*Enterprise, error) {
	if enterpriseID <= 0 {
		return nil, ErrEnterpriseNotFound
	}
	if common.UsingSQLite {
		// SQLite 没有 SELECT FOR UPDATE：先自赋值取得写锁再读。
		if err := tx.Model(&Enterprise{}).Where("id = ?", enterpriseID).
			UpdateColumn("unallocated", gorm.Expr("unallocated")).Error; err != nil {
			return nil, err
		}
	}
	query := tx.Where("id = ?", enterpriseID)
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var ent Enterprise
	if err := query.First(&ent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnterpriseNotFound
		}
		return nil, err
	}
	return ent.fill(), nil
}

// lockEnterpriseMemberTx 锁定成员行；必须在企业行之后获取（若本事务需要企业行）。
func lockEnterpriseMemberTx(tx *gorm.DB, memberID int) (*EnterpriseMember, error) {
	if memberID <= 0 {
		return nil, ErrEnterpriseNotFound
	}
	if common.UsingSQLite {
		if err := tx.Model(&EnterpriseMember{}).Where("id = ?", memberID).
			UpdateColumn("available", gorm.Expr("available")).Error; err != nil {
			return nil, err
		}
	}
	query := tx.Where("id = ?", memberID)
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var member EnterpriseMember
	if err := query.First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnterpriseNotFound
		}
		return nil, err
	}
	return member.fill(), nil
}

// enterpriseTx 在企业行锁内执行 fn。
func enterpriseTx(enterpriseID int, fn func(tx *gorm.DB, ent *Enterprise) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		ent, err := lockEnterpriseTx(tx, enterpriseID)
		if err != nil {
			return err
		}
		return fn(tx, ent)
	})
}

// enterpriseActorMemberTx 在事务内重新读取操作者的有效成员关系，避免使用中间件里的过期角色。
// 平台操作者返回 nil 成员。
func enterpriseActorMemberTx(tx *gorm.DB, enterpriseID int, actor EnterpriseActor) (*EnterpriseMember, error) {
	if actor.Platform {
		return nil, nil
	}
	var member EnterpriseMember
	err := tx.Where("enterprise_id = ? AND user_id = ? AND status = ?", enterpriseID, actor.UserId, EnterpriseMemberStatusActive).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnterpriseNotFound
		}
		return nil, err
	}
	return member.fill(), nil
}

func appendEnterpriseAuditTx(tx *gorm.DB, enterpriseID int, actor EnterpriseActor, actorMember *EnterpriseMember,
	action, targetType string, targetID int, before, after any, reason string) error {
	encode := func(v any) string {
		if v == nil {
			return ""
		}
		data, err := common.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 255 {
		reason = string([]rune(reason)[:255])
	}
	return tx.Create(&EnterpriseAuditEvent{
		EnterpriseId: enterpriseID,
		ActorId:      actor.UserId,
		ActorRole:    actor.auditRole(actorMember),
		Action:       action,
		TargetType:   targetType,
		TargetId:     targetID,
		Before:       encode(before),
		After:        encode(after),
		Reason:       reason,
		CreatedAt:    time.Now().Unix(),
	}).Error
}

func bumpEnterprisePolicyTx(tx *gorm.DB, enterpriseID int) error {
	return tx.Model(&Enterprise{}).Where("id = ?", enterpriseID).Updates(map[string]any{
		"policy_version": gorm.Expr("policy_version + 1"),
		"updated_at":     time.Now().Unix(),
	}).Error
}

// CreateEnterprise 由平台开通企业：指定所有者并配置计费用户组。
func CreateEnterprise(actorID int, name, billingGroup, timezone string, ownerUserID int) (*Enterprise, error) {
	name = strings.TrimSpace(name)
	billingGroup = strings.TrimSpace(billingGroup)
	timezone = strings.TrimSpace(timezone)
	if name == "" || len([]rune(name)) > 64 || ownerUserID <= 0 {
		return nil, ErrEnterpriseInvalidInput
	}
	if billingGroup == "" || billingGroup == "auto" || !ratio_setting.ContainsGroupRatio(billingGroup) {
		return nil, ErrEnterpriseBillingGroup
	}
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, ErrEnterpriseTimezone
	}
	now := time.Now().Unix()
	ent := &Enterprise{Name: name, Status: EnterpriseStatusActive, OwnerUserId: ownerUserID, BillingGroup: billingGroup,
		Timezone: timezone, ModelMode: EnterpriseModelModeInherit, ModelList: "[]", CreatedAt: now, UpdatedAt: now}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := tx.Select("id", "status").Where("id = ?", ownerUserID).First(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnterpriseOwnerAccount
			}
			return err
		}
		if owner.Status != common.UserStatusEnabled {
			return ErrEnterpriseOwnerAccount
		}
		if err := tx.Create(ent).Error; err != nil {
			return err
		}
		member := &EnterpriseMember{EnterpriseId: ent.Id, UserId: ownerUserID, ActiveUserId: &ownerUserID,
			Role: EnterpriseRoleOwner, Status: EnterpriseMemberStatusActive, Generation: 1,
			ModelMode: EnterpriseModelModeInherit, ModelList: "[]", JoinedAt: now, UpdatedAt: now}
		if err := tx.Create(member).Error; err != nil {
			if isEnterpriseDuplicateKeyErr(err) {
				return ErrEnterpriseAlreadyMember
			}
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, EnterpriseActor{UserId: actorID, Platform: true}, nil,
			"enterprise.create", "enterprise", ent.Id, nil,
			map[string]any{"name": name, "billing_group": billingGroup, "owner_user_id": ownerUserID}, "")
	})
	if err != nil {
		return nil, err
	}
	return ent.fill(), nil
}

// SetEnterpriseStatus 冻结或解冻企业。冻结后企业令牌的新请求被拒绝，待审批申请失效。
func SetEnterpriseStatus(actorID, enterpriseID int, status, reason string) error {
	if status != EnterpriseStatusActive && status != EnterpriseStatusFrozen {
		return ErrEnterpriseInvalidInput
	}
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		if ent.Status == status {
			return nil
		}
		now := time.Now().Unix()
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).Updates(map[string]any{
			"status": status, "policy_version": gorm.Expr("policy_version + 1"), "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if status == EnterpriseStatusFrozen {
			if err := expireEnterpriseQuotaRequestsTx(tx, "enterprise_id = ?", ent.Id); err != nil {
				return err
			}
		}
		return appendEnterpriseAuditTx(tx, ent.Id, EnterpriseActor{UserId: actorID, Platform: true}, nil,
			"enterprise.status", "enterprise", ent.Id, map[string]any{"status": ent.Status}, map[string]any{"status": status}, reason)
	})
}

// UpdateEnterpriseByPlatform 修改企业名称与计费用户组（合同价落在该组的专属倍率上）。
func UpdateEnterpriseByPlatform(actorID, enterpriseID int, name, billingGroup string) error {
	name = strings.TrimSpace(name)
	billingGroup = strings.TrimSpace(billingGroup)
	if name == "" || len([]rune(name)) > 64 {
		return ErrEnterpriseInvalidInput
	}
	if billingGroup == "" || billingGroup == "auto" || !ratio_setting.ContainsGroupRatio(billingGroup) {
		return ErrEnterpriseBillingGroup
	}
	return enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).Updates(map[string]any{
			"name": name, "billing_group": billingGroup,
			"policy_version": gorm.Expr("policy_version + 1"), "updated_at": time.Now().Unix(),
		}).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, EnterpriseActor{UserId: actorID, Platform: true}, nil,
			"enterprise.update", "enterprise", ent.Id,
			map[string]any{"name": ent.Name, "billing_group": ent.BillingGroup},
			map[string]any{"name": name, "billing_group": billingGroup}, "")
	})
}

// EnterpriseSettingsInput 是企业所有者可修改的设置。
type EnterpriseSettingsInput struct {
	Name                string
	Timezone            string
	ModelMode           string
	Models              []string
	LowBalanceThreshold int64
}

// UpdateEnterpriseSettings 仅企业所有者可改企业资料与企业级模型范围。
func UpdateEnterpriseSettings(actor EnterpriseActor, enterpriseID int, in EnterpriseSettingsInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Timezone = strings.TrimSpace(in.Timezone)
	if in.Name == "" || len([]rune(in.Name)) > 64 || !validEnterpriseModelMode(in.ModelMode) || in.LowBalanceThreshold < 0 {
		return ErrEnterpriseInvalidInput
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" {
		return ErrEnterpriseTimezone
	}
	models := normalizeEnterpriseModels(in.Models)
	if len(models) > 2000 {
		return ErrEnterpriseInvalidInput
	}
	if in.ModelMode == EnterpriseModelModeInherit {
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
		if !actor.Platform && actorMember.Role != EnterpriseRoleOwner {
			return ErrEnterpriseForbidden
		}
		if err := tx.Model(&Enterprise{}).Where("id = ?", ent.Id).Updates(map[string]any{
			"name": in.Name, "timezone": in.Timezone, "model_mode": in.ModelMode, "model_list": encoded,
			"low_balance_threshold": in.LowBalanceThreshold, "low_balance_notified_at": 0,
			"policy_version": gorm.Expr("policy_version + 1"), "updated_at": time.Now().Unix(),
		}).Error; err != nil {
			return err
		}
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "enterprise.settings", "enterprise", ent.Id,
			map[string]any{"name": ent.Name, "timezone": ent.Timezone, "model_mode": ent.ModelMode, "models": ent.Models, "low_balance_threshold": ent.LowBalanceThreshold},
			map[string]any{"name": in.Name, "timezone": in.Timezone, "model_mode": in.ModelMode, "models": models, "low_balance_threshold": in.LowBalanceThreshold}, "")
	})
}

func GetEnterpriseByID(enterpriseID int) (*Enterprise, error) {
	var ent Enterprise
	if err := DB.Where("id = ?", enterpriseID).First(&ent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnterpriseNotFound
		}
		return nil, err
	}
	return ent.fill(), nil
}

// ListEnterprises 平台后台的企业列表。
func ListEnterprises(keyword string, offset, limit int) ([]*Enterprise, int64, error) {
	query := DB.Model(&Enterprise{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*Enterprise
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	for _, item := range items {
		item.fill()
	}
	FillEnterpriseOwners(items...)
	return items, total, nil
}

// FillEnterpriseOwners 为企业补上所有者的展示名。
func FillEnterpriseOwners(items ...*Enterprise) {
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.OwnerUserId)
	}
	briefs := enterpriseUserBriefs(DB, ids)
	for _, item := range items {
		item.OwnerName = briefs[item.OwnerUserId].label()
	}
}

// ListEnterpriseAuditEvents 企业操作记录，按时间倒序。
func ListEnterpriseAuditEvents(enterpriseID, offset, limit int) ([]*EnterpriseAuditEvent, int64, error) {
	query := DB.Model(&EnterpriseAuditEvent{}).Where("enterprise_id = ?", enterpriseID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*EnterpriseAuditEvent
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ActorId)
	}
	names := enterpriseUserBriefs(DB, ids)
	for _, item := range items {
		if item.ActorRole == EnterpriseActorPlatform {
			item.ActorName = "平台"
			continue
		}
		if brief, ok := names[item.ActorId]; ok {
			item.ActorName = brief.label()
		}
	}
	return items, total, nil
}

type enterpriseUserBrief struct {
	Id          int
	Username    string
	DisplayName string
	Email       string
}

func (b enterpriseUserBrief) label() string {
	if b.DisplayName != "" {
		return b.DisplayName
	}
	return b.Username
}

func enterpriseUserBriefs(tx *gorm.DB, ids []int) map[int]enterpriseUserBrief {
	out := make(map[int]enterpriseUserBrief, len(ids))
	if len(ids) == 0 {
		return out
	}
	var rows []enterpriseUserBrief
	if err := tx.Model(&User{}).Unscoped().Select("id", "username", "display_name", "email").
		Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return out
	}
	for _, row := range rows {
		out[row.Id] = row
	}
	return out
}

// IsEnterpriseUserError 判断错误是否为可以原样展示给用户的企业业务错误；
// 其余错误（数据库故障等）由接口层记录日志并返回通用提示。
func IsEnterpriseUserError(err error) bool {
	for _, known := range []error{
		ErrEnterpriseNotFound, ErrEnterpriseForbidden, ErrEnterpriseFrozen, ErrEnterpriseAlreadyMember,
		ErrEnterpriseOwnerRequired, ErrEnterpriseInvitation, ErrEnterpriseInvitationMail, ErrEnterpriseInvalidInput,
		ErrEnterpriseSelfOperation, ErrEnterpriseBillingGroup, ErrEnterpriseTimezone, ErrEnterpriseOwnerAccount,
		ErrEnterpriseDisabled, ErrEnterpriseInsufficientUnallocated, ErrEnterpriseDuplicateVoucher,
		ErrEnterpriseReclaimExceeds, ErrEnterpriseRequestPending, ErrEnterpriseRequestState,
		ErrEnterpriseMemberInactive, ErrEnterpriseTokenCap, ErrEnterpriseUnsupported,
	} {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}

// ResolveEnterpriseOwnerCandidate 把平台后台填写的所有者标识（用户 ID、用户名或邮箱）解析为用户 ID。
// 邮箱不是唯一列，匹配到多个账号时拒绝，要求改填用户名或 ID。
func ResolveEnterpriseOwnerCandidate(identifier string) (int, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return 0, ErrEnterpriseOwnerAccount
	}
	var ids []int
	if id, err := strconv.Atoi(identifier); err == nil && id > 0 {
		if err := DB.Model(&User{}).Where("id = ?", id).Limit(1).Pluck("id", &ids).Error; err != nil {
			return 0, err
		}
		if len(ids) == 1 {
			return ids[0], nil
		}
	}
	if err := DB.Model(&User{}).Where("username = ?", identifier).Limit(1).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if email := normalizeEnterpriseEmail(identifier); strings.Contains(email, "@") {
		if err := DB.Model(&User{}).Where("email = ?", email).Limit(2).Pluck("id", &ids).Error; err != nil {
			return 0, err
		}
		if len(ids) == 1 {
			return ids[0], nil
		}
	}
	return 0, ErrEnterpriseOwnerAccount
}

// EnterpriseSummary 是企业后台总览的计数。
type EnterpriseSummary struct {
	Members            int64 `json:"members"`
	PendingRequests    int64 `json:"pending_requests"`
	PendingInvitations int64 `json:"pending_invitations"`
}

func GetEnterpriseSummary(enterpriseID int) (*EnterpriseSummary, error) {
	out := &EnterpriseSummary{}
	now := time.Now().Unix()
	if err := DB.Model(&EnterpriseMember{}).Where("enterprise_id = ? AND status = ?", enterpriseID, EnterpriseMemberStatusActive).
		Count(&out.Members).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&EnterpriseQuotaRequest{}).
		Where("enterprise_id = ? AND status = ? AND expires_at > ?", enterpriseID, EnterpriseRequestPending, now).
		Count(&out.PendingRequests).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&EnterpriseInvitation{}).
		Where("enterprise_id = ? AND status = ? AND expires_at > ?", enterpriseID, EnterpriseInvitationPending, now).
		Count(&out.PendingInvitations).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// EnterpriseContact 是通知收件人。
type EnterpriseContact struct {
	UserId int
	Name   string
	Email  string
}

// ListEnterpriseManagerContacts 返回企业所有者与管理员（有邮箱的），用于审批待办和企业余额提醒。
// ownerOnly 为真时只返回所有者：管理员自己的申请只能由所有者处理。
func ListEnterpriseManagerContacts(enterpriseID int, ownerOnly bool) ([]EnterpriseContact, error) {
	roles := []string{EnterpriseRoleOwner, EnterpriseRoleAdmin}
	if ownerOnly {
		roles = roles[:1]
	}
	var userIDs []int
	if err := DB.Model(&EnterpriseMember{}).
		Where("enterprise_id = ? AND status = ? AND role IN ?", enterpriseID, EnterpriseMemberStatusActive, roles).
		Pluck("user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	return enterpriseContacts(userIDs), nil
}

// GetEnterpriseContact 返回单个账号的通知收件信息；没有邮箱时 ok 为 false。
func GetEnterpriseContact(userID int) (EnterpriseContact, bool) {
	contacts := enterpriseContacts([]int{userID})
	if len(contacts) == 0 {
		return EnterpriseContact{}, false
	}
	return contacts[0], true
}

func enterpriseContacts(userIDs []int) []EnterpriseContact {
	briefs := enterpriseUserBriefs(DB, userIDs)
	out := make([]EnterpriseContact, 0, len(userIDs))
	for _, id := range userIDs {
		brief, ok := briefs[id]
		if !ok || brief.Email == "" {
			continue
		}
		out = append(out, EnterpriseContact{UserId: id, Name: brief.label(), Email: brief.Email})
	}
	return out
}
