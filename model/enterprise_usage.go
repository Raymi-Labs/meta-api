package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// 企业用量只查主库的预留/结算表：账单以结算记录为准，日志库可能在另一台服务器、
// 写入也可能遗漏，所以这里不读日志表，也不联表。

const (
	// EnterpriseUsageStatusAll 包含已退回（未产生费用）的记录；默认不含。
	EnterpriseUsageStatusAll = "all"

	enterpriseUsageGroupLimit = 500
	enterpriseUsageBatch      = 250
)

var ErrEnterpriseUsageTooLarge = errors.New("导出超过 10000 条，请缩小筛选范围")

// EnterpriseUsageFilter 是列表、汇总、导出共用的筛选条件，保证三者口径一致。
// EnterpriseId 必填，由调用方从已鉴权的上下文取得，不接受请求参数。
type EnterpriseUsageFilter struct {
	EnterpriseId int
	MemberId     int
	TokenId      int
	ModelName    string
	Kind         string
	Status       string
	Start        int64
	End          int64
}

func (f EnterpriseUsageFilter) apply(tx *gorm.DB) (*gorm.DB, error) {
	if f.EnterpriseId <= 0 {
		return nil, ErrEnterpriseNotFound
	}
	if f.MemberId < 0 || f.TokenId < 0 || f.Start < 0 || f.End < 0 || (f.End > 0 && f.End < f.Start) {
		return nil, ErrEnterpriseInvalidInput
	}
	tx = tx.Where("enterprise_id = ?", f.EnterpriseId)
	if f.MemberId > 0 {
		tx = tx.Where("member_id = ?", f.MemberId)
	}
	if f.TokenId > 0 {
		tx = tx.Where("token_id = ?", f.TokenId)
	}
	if f.ModelName != "" {
		tx = tx.Where("model_name = ?", f.ModelName)
	}
	switch f.Kind {
	case "":
	case EnterpriseChargeRequest, EnterpriseChargeViolationFee, EnterpriseChargeTaskAdjust, EnterpriseChargeManual:
		tx = tx.Where("kind = ?", f.Kind)
	default:
		return nil, ErrEnterpriseInvalidInput
	}
	switch f.Status {
	case "":
		tx = tx.Where("status <> ?", EnterpriseReservationRefunded)
	case EnterpriseUsageStatusAll:
	case EnterpriseReservationReserved, EnterpriseReservationSettled, EnterpriseReservationRefunded, EnterpriseReservationReview:
		tx = tx.Where("status = ?", f.Status)
	default:
		return nil, ErrEnterpriseInvalidInput
	}
	if f.Start > 0 {
		tx = tx.Where("created_at >= ?", f.Start)
	}
	if f.End > 0 {
		tx = tx.Where("created_at <= ?", f.End)
	}
	return tx, nil
}

func fillEnterpriseUsageNames(items []*EnterpriseReservation) {
	userIDs := make([]int, 0, len(items))
	for _, item := range items {
		userIDs = append(userIDs, item.UserId)
	}
	briefs := enterpriseUserBriefs(DB, userIDs)
	for _, item := range items {
		item.MemberName = briefs[item.UserId].label()
	}
}

// ListEnterpriseUsage 企业用量明细，按受理时间倒序。只含金额、模型、时间、令牌名称，
// 不含密钥与请求内容。
func ListEnterpriseUsage(filter EnterpriseUsageFilter, offset, limit int) ([]*EnterpriseReservation, int64, error) {
	query, err := filter.apply(DB.Model(&EnterpriseReservation{}))
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []*EnterpriseReservation{}
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	fillEnterpriseUsageNames(items)
	return items, total, nil
}

type EnterpriseUsageGroup struct {
	MemberId    int    `json:"member_id,omitempty"`
	UserId      int    `json:"-"`
	Name        string `json:"name"`
	NetConsumed int64  `json:"net_consumed"`
	Requests    int64  `json:"requests"`
	Pending     int64  `json:"pending"`
}

// EnterpriseUsageSummary 与列表同条件的汇总。净消费 = 已结算记录之和（退款、事后调整为负数行，
// 已在其中抵减）；在途与待对账的占用单列，不计入净消费。分配不是消费，不在这里出现。
type EnterpriseUsageSummary struct {
	NetConsumed  int64                  `json:"net_consumed"`
	Requests     int64                  `json:"requests"`
	Pending      int64                  `json:"pending"`
	PendingCount int64                  `json:"pending_count"`
	AsOf         int64                  `json:"as_of"`
	Timezone     string                 `json:"timezone"`
	ByMember     []EnterpriseUsageGroup `json:"by_member"`
	ByModel      []EnterpriseUsageGroup `json:"by_model"`
}

// 常量拼接，不含外部输入。
const enterpriseUsageAggregates = "COALESCE(SUM(CASE WHEN status = '" + EnterpriseReservationSettled + "' THEN quota ELSE 0 END), 0) AS net_consumed, " +
	"COALESCE(SUM(CASE WHEN status = '" + EnterpriseReservationSettled + "' AND kind = '" + EnterpriseChargeRequest + "' THEN 1 ELSE 0 END), 0) AS requests, " +
	"COALESCE(SUM(CASE WHEN status IN ('" + EnterpriseReservationReserved + "', '" + EnterpriseReservationReview + "') THEN quota ELSE 0 END), 0) AS pending, " +
	"COALESCE(SUM(CASE WHEN status IN ('" + EnterpriseReservationReserved + "', '" + EnterpriseReservationReview + "') THEN 1 ELSE 0 END), 0) AS pending_count"

func GetEnterpriseUsageSummary(filter EnterpriseUsageFilter) (*EnterpriseUsageSummary, error) {
	// 汇总自己按状态分列，筛选里的状态不参与；但非法取值仍与列表一样拒绝。
	if _, err := filter.apply(DB.Model(&EnterpriseReservation{})); err != nil {
		return nil, err
	}
	filter.Status = EnterpriseUsageStatusAll
	base := func() (*gorm.DB, error) { return filter.apply(DB.Model(&EnterpriseReservation{})) }

	out := &EnterpriseUsageSummary{AsOf: time.Now().Unix(), ByMember: []EnterpriseUsageGroup{}, ByModel: []EnterpriseUsageGroup{}}
	query, err := base()
	if err != nil {
		return nil, err
	}
	var totals struct {
		NetConsumed  int64
		Requests     int64
		Pending      int64
		PendingCount int64
	}
	if err := query.Select(enterpriseUsageAggregates).Scan(&totals).Error; err != nil {
		return nil, err
	}
	out.NetConsumed, out.Requests, out.Pending, out.PendingCount = totals.NetConsumed, totals.Requests, totals.Pending, totals.PendingCount

	if query, err = base(); err != nil {
		return nil, err
	}
	if err := query.Select("member_id, MAX(user_id) AS user_id, " + enterpriseUsageAggregates).
		Group("member_id").Order("net_consumed DESC").Order("member_id ASC").
		Limit(enterpriseUsageGroupLimit).Scan(&out.ByMember).Error; err != nil {
		return nil, err
	}
	userIDs := make([]int, 0, len(out.ByMember))
	for _, row := range out.ByMember {
		userIDs = append(userIDs, row.UserId)
	}
	briefs := enterpriseUserBriefs(DB, userIDs)
	for i := range out.ByMember {
		out.ByMember[i].Name = briefs[out.ByMember[i].UserId].label()
	}

	if query, err = base(); err != nil {
		return nil, err
	}
	if err := query.Select("model_name AS name, " + enterpriseUsageAggregates).
		Group("model_name").Order("net_consumed DESC").Order("model_name ASC").
		Limit(enterpriseUsageGroupLimit).Scan(&out.ByModel).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// WalkEnterpriseUsage 按与列表相同的条件和顺序遍历，供导出使用。ID 游标避免新记录造成翻页漂移。
func WalkEnterpriseUsage(ctx context.Context, filter EnterpriseUsageFilter, write func(*EnterpriseReservation) error) error {
	var cursor int64
	count := 0
	for {
		query, err := filter.apply(DB.WithContext(ctx).Model(&EnterpriseReservation{}))
		if err != nil {
			return err
		}
		if cursor > 0 {
			query = query.Where("id < ?", cursor)
		}
		var rows []*EnterpriseReservation
		if err := query.Order("id DESC").Limit(enterpriseUsageBatch).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		cursor = rows[len(rows)-1].Id
		if count+len(rows) > LogExportMaxRows {
			return ErrEnterpriseUsageTooLarge
		}
		fillEnterpriseUsageNames(rows)
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := write(row); err != nil {
				return err
			}
		}
		count += len(rows)
		if len(rows) < enterpriseUsageBatch {
			return nil
		}
	}
}

type EnterpriseMemberSpend struct {
	MemberId  int    `json:"member_id" gorm:"column:id"`
	UserId    int    `json:"-"`
	Name      string `json:"name" gorm:"-"`
	Status    string `json:"status"`
	Consumed  int64  `json:"consumed"`
	Available int64  `json:"available"`
}

// ListEnterpriseTopSpenders 总览页的成员消费排行，按累计已消费降序，含已停用成员。
func ListEnterpriseTopSpenders(enterpriseID, limit int) ([]EnterpriseMemberSpend, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows := []EnterpriseMemberSpend{}
	if err := DB.Model(&EnterpriseMember{}).Select("id", "user_id", "status", "consumed", "available").
		Where("enterprise_id = ? AND consumed <> 0", enterpriseID).
		Order("consumed DESC").Order("id ASC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	userIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserId)
	}
	briefs := enterpriseUserBriefs(DB, userIDs)
	for i := range rows {
		rows[i].Name = briefs[rows[i].UserId].label()
	}
	return rows, nil
}

// EnterpriseTokenView 企业管理员看到的成员令牌：名称与掩码，永远不含密钥明文。
type EnterpriseTokenView struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	MaskedKey    string `json:"masked_key"`
	MemberId     int    `json:"member_id"`
	MemberName   string `json:"member_name"`
	Status       int    `json:"status"`
	CreatedTime  int64  `json:"created_time"`
	AccessedTime int64  `json:"accessed_time"`
	ExpiredTime  int64  `json:"expired_time"`
	LimitQuota   int64  `json:"limit_quota"`
	UsedQuota    int64  `json:"used_quota"`
}

// ListEnterpriseTokens 列出企业付款的令牌（不含已删除），可按成员筛选。
func ListEnterpriseTokens(enterpriseID, memberID, offset, limit int) ([]*EnterpriseTokenView, int64, error) {
	if enterpriseID <= 0 {
		return nil, 0, ErrEnterpriseNotFound
	}
	query := DB.Model(&Token{}).Where("enterprise_id = ?", enterpriseID)
	if memberID > 0 {
		query = query.Where("membership_id = ?", memberID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tokens []Token
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&tokens).Error; err != nil {
		return nil, 0, err
	}
	tokenIDs, userIDs := make([]int, 0, len(tokens)), make([]int, 0, len(tokens))
	for _, token := range tokens {
		tokenIDs = append(tokenIDs, token.Id)
		userIDs = append(userIDs, token.UserId)
	}
	budgets, err := GetEnterpriseTokenBudgets(tokenIDs)
	if err != nil {
		return nil, 0, err
	}
	briefs := enterpriseUserBriefs(DB, userIDs)
	items := make([]*EnterpriseTokenView, 0, len(tokens))
	for _, token := range tokens {
		budget := budgets[token.Id]
		items = append(items, &EnterpriseTokenView{
			Id: token.Id, Name: token.Name, MaskedKey: MaskTokenKey(token.Key), MemberId: token.MembershipId,
			MemberName: briefs[token.UserId].label(), Status: token.Status, CreatedTime: token.CreatedTime,
			AccessedTime: token.AccessedTime, ExpiredTime: token.ExpiredTime,
			LimitQuota: budget.LimitQuota, UsedQuota: budget.UsedQuota,
		})
	}
	return items, total, nil
}

// RevokeEnterpriseToken 企业管理员撤销成员的企业令牌。撤销即删除：成员不能自行恢复，
// 历史账单保留（账单里有令牌名称快照）。在途请求仍按真实费用结算。
func RevokeEnterpriseToken(actor EnterpriseActor, enterpriseID, tokenID int, reason string) error {
	if tokenID <= 0 {
		return ErrEnterpriseNotFound
	}
	var key string
	err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
		actorMember, err := enterpriseActorMemberTx(tx, ent.Id, actor)
		if err != nil {
			return err
		}
		var token Token
		if err := tx.Where("id = ? AND enterprise_id = ?", tokenID, ent.Id).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnterpriseNotFound
			}
			return err
		}
		target, err := lockEnterpriseMemberTx(tx, token.MembershipId)
		if err != nil || target.EnterpriseId != ent.Id {
			return ErrEnterpriseNotFound
		}
		// 自己的令牌本来就能在控制台删除；别人的令牌按成员管理权限判定。
		if !actor.Platform && actorMember.Id != target.Id {
			if err := canManageEnterpriseMember(actor, actorMember, target); err != nil {
				return err
			}
		}
		if err := tx.Delete(&token).Error; err != nil {
			return err
		}
		key = token.Key
		return appendEnterpriseAuditTx(tx, ent.Id, actor, actorMember, "token.revoke", "token", token.Id,
			map[string]any{"name": token.Name, "member_id": target.Id, "status": token.Status}, nil, reason)
	})
	if err != nil {
		return err
	}
	purgeEnterpriseTokenCache([]string{key})
	return nil
}

// GetUserEnterpriseTokenIDs 返回该账号名下由企业付款的全部令牌（含已删除），
// 用于在日志库上按付款方筛选：日志库可能与主库分离，只能先取 ID 再过滤。
func GetUserEnterpriseTokenIDs(userID int) ([]int, error) {
	ids := []int{}
	if userID <= 0 {
		return ids, nil
	}
	err := DB.Unscoped().Model(&Token{}).Where("user_id = ? AND enterprise_id > 0", userID).Pluck("id", &ids).Error
	return ids, err
}

// fillLogPayers 给本人日志逐行标出付款方。按令牌归属判断，而不是看日志里的附加信息：
// 失败请求的日志没有计费信息，但令牌归属始终可查。
func fillLogPayers(logs []*Log) {
	if !common.EnterpriseEnabled {
		return
	}
	seen := map[int]struct{}{}
	tokenIDs := make([]int, 0, len(logs))
	for _, log := range logs {
		if log.TokenId <= 0 {
			continue
		}
		if _, ok := seen[log.TokenId]; !ok {
			seen[log.TokenId] = struct{}{}
			tokenIDs = append(tokenIDs, log.TokenId)
		}
	}
	if len(tokenIDs) == 0 {
		return
	}
	var tokens []Token
	if err := DB.Unscoped().Select("id", "enterprise_id").Where("id IN ? AND enterprise_id > 0", tokenIDs).Find(&tokens).Error; err != nil {
		return
	}
	tokenEnterprise := make(map[int]int, len(tokens))
	enterpriseIDs := make([]int, 0, len(tokens))
	for _, token := range tokens {
		tokenEnterprise[token.Id] = token.EnterpriseId
		enterpriseIDs = append(enterpriseIDs, token.EnterpriseId)
	}
	names := map[int]string{}
	if len(enterpriseIDs) > 0 {
		var ents []Enterprise
		if err := DB.Select("id", "name").Where("id IN ?", enterpriseIDs).Find(&ents).Error; err == nil {
			for _, ent := range ents {
				names[ent.Id] = ent.Name
			}
		}
	}
	for _, log := range logs {
		if log.TokenId <= 0 {
			continue
		}
		if enterpriseID, ok := tokenEnterprise[log.TokenId]; ok {
			log.Payer, log.EnterpriseName = TokenPayerEnterprise, names[enterpriseID]
		} else {
			log.Payer = TokenPayerPersonal
		}
	}
}
