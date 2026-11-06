package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

const (
	RoutePlanStatusEnabled  = 1
	RoutePlanStatusDisabled = 2
	routePlanCacheTTL       = 30 * time.Second
	maxRoutePlanSaleRatio   = 1000
)

var ErrRoutePlanNotFound = errors.New("路由方案不存在")

// RoutePlan is an administrator-managed smart routing contract: the line pool
// (ordered groups), who may use it, and its own sale multipliers. Groups only
// decide where traffic may go; the plan ratio, not the group ratio, is billed.
type RoutePlan struct {
	Id          int     `json:"id"`
	Name        string  `json:"name" gorm:"size:64;not null"`
	Description string  `json:"description" gorm:"type:text"`
	Status      int     `json:"status" gorm:"default:1"`
	Groups      string  `json:"groups" gorm:"type:text"`       // JSON []string, ordered fallback pool
	SaleRatio   float64 `json:"sale_ratio" gorm:"default:1"`   // default contract multiplier
	ModelRatios string  `json:"model_ratios" gorm:"type:text"` // JSON map[model]multiplier overrides
	UserIds     string  `json:"user_ids" gorm:"type:text"`     // JSON []int authorized accounts
	UserGroups  string  `json:"user_groups" gorm:"type:text"`  // JSON []string authorized user groups
	Version     int64   `json:"version" gorm:"default:1"`
	CreatedTime int64   `json:"created_time" gorm:"bigint"`
	UpdatedTime int64   `json:"updated_time" gorm:"bigint"`
}

type RoutePlanFields struct {
	Groups      []string
	ModelRatios map[string]float64
	UserIds     []int
	UserGroups  []string
}

func (p *RoutePlan) Fields() (RoutePlanFields, error) {
	fields := RoutePlanFields{Groups: []string{}, ModelRatios: map[string]float64{}, UserIds: []int{}, UserGroups: []string{}}
	decode := func(raw string, target any) error {
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		return common.UnmarshalJsonStr(raw, target)
	}
	if err := decode(p.Groups, &fields.Groups); err != nil {
		return fields, fmt.Errorf("方案分组格式错误: %w", err)
	}
	if err := decode(p.ModelRatios, &fields.ModelRatios); err != nil {
		return fields, fmt.Errorf("方案模型价格格式错误: %w", err)
	}
	if err := decode(p.UserIds, &fields.UserIds); err != nil {
		return fields, fmt.Errorf("方案授权用户格式错误: %w", err)
	}
	if err := decode(p.UserGroups, &fields.UserGroups); err != nil {
		return fields, fmt.Errorf("方案授权用户组格式错误: %w", err)
	}
	return fields, nil
}

func validRatio(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= maxRoutePlanSaleRatio
}

// Normalize validates and re-encodes the JSON fields. groupExists rejects a
// pool group that has no configured group ratio (a retired or mistyped group).
func (p *RoutePlan) Normalize(groupExists func(string) bool) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len([]rune(p.Name)) > 64 {
		return errors.New("方案名称不能为空且不超过 64 个字符")
	}
	if p.Status != RoutePlanStatusEnabled && p.Status != RoutePlanStatusDisabled {
		p.Status = RoutePlanStatusEnabled
	}
	if !validRatio(p.SaleRatio) {
		return fmt.Errorf("方案默认倍率必须在 0 到 %d 之间", maxRoutePlanSaleRatio)
	}
	fields, err := p.Fields()
	if err != nil {
		return err
	}
	groups := make([]string, 0, len(fields.Groups))
	seen := map[string]bool{}
	for _, group := range fields.Groups {
		group = strings.TrimSpace(group)
		if group == "" || group == "auto" || seen[group] {
			continue
		}
		if len(group) > 64 || (groupExists != nil && !groupExists(group)) {
			return fmt.Errorf("分组 %s 不存在或已被弃用", group)
		}
		seen[group] = true
		groups = append(groups, group)
	}
	if len(groups) == 0 {
		return errors.New("方案至少需要一个线路分组")
	}
	for name, ratio := range fields.ModelRatios {
		if strings.TrimSpace(name) == "" || !validRatio(ratio) {
			return fmt.Errorf("模型 %s 的方案倍率无效", name)
		}
	}
	userIDs := make([]int, 0, len(fields.UserIds))
	for _, id := range fields.UserIds {
		if id > 0 {
			userIDs = append(userIDs, id)
		}
	}
	userGroups := make([]string, 0, len(fields.UserGroups))
	for _, group := range fields.UserGroups {
		if group = strings.TrimSpace(group); group != "" {
			userGroups = append(userGroups, group)
		}
	}
	encode := func(value any) string {
		data, _ := common.Marshal(value)
		return string(data)
	}
	p.Groups, p.ModelRatios, p.UserIds, p.UserGroups = encode(groups), encode(fields.ModelRatios), encode(userIDs), encode(userGroups)
	return nil
}

// AuthorizedFor is the only plan permission check: an explicit account or user
// group grant. A plan with no grants is usable by nobody.
func (p *RoutePlan) AuthorizedFor(userID int, userGroup string) bool {
	if p == nil || p.Status != RoutePlanStatusEnabled {
		return false
	}
	fields, err := p.Fields()
	if err != nil {
		return false
	}
	for _, id := range fields.UserIds {
		if id == userID {
			return true
		}
	}
	for _, group := range fields.UserGroups {
		if group == userGroup {
			return true
		}
	}
	return false
}

func (p *RoutePlan) Snapshot() (*types.RoutePlanSnapshot, error) {
	fields, err := p.Fields()
	if err != nil {
		return nil, err
	}
	if len(fields.Groups) == 0 {
		return nil, errors.New("路由方案没有可用线路分组")
	}
	return &types.RoutePlanSnapshot{ID: p.Id, Version: p.Version, Name: p.Name, Groups: fields.Groups,
		SaleRatio: p.SaleRatio, ModelRatios: fields.ModelRatios}, nil
}

func ListRoutePlans() ([]RoutePlan, error) {
	plans := make([]RoutePlan, 0)
	err := DB.Order("id asc").Find(&plans).Error
	return plans, err
}

func GetRoutePlanByID(id int) (*RoutePlan, error) {
	if id <= 0 {
		return nil, ErrRoutePlanNotFound
	}
	plan := &RoutePlan{}
	err := DB.First(plan, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRoutePlanNotFound
	}
	return plan, err
}

func CreateRoutePlan(plan *RoutePlan) error {
	now := common.GetTimestamp()
	plan.Id, plan.Version, plan.CreatedTime, plan.UpdatedTime = 0, 1, now, now
	err := DB.Create(plan).Error
	if err == nil {
		InvalidateRoutePlanCache()
	}
	return err
}

// UpdateRoutePlan bumps the version so logs can tell which contract applied.
// Requests already authenticated keep their earlier snapshot.
func UpdateRoutePlan(plan *RoutePlan) error {
	current, err := GetRoutePlanByID(plan.Id)
	if err != nil {
		return err
	}
	plan.Version = current.Version + 1
	plan.CreatedTime = current.CreatedTime
	plan.UpdatedTime = common.GetTimestamp()
	err = DB.Model(&RoutePlan{}).Where("id = ?", plan.Id).Select("name", "description", "status", "groups", "sale_ratio",
		"model_ratios", "user_ids", "user_groups", "version", "updated_time").Updates(plan).Error
	if err == nil {
		InvalidateRoutePlanCache()
	}
	return err
}

// DeleteRoutePlan refuses while tokens still reference the plan, so a delete
// can never silently move customers back to manual group pricing.
func DeleteRoutePlan(id int) error {
	var count int64
	if err := DB.Model(&Token{}).Where("route_plan_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("仍有 %d 个令牌使用该方案，请先停用方案或修改令牌", count)
	}
	err := DB.Delete(&RoutePlan{}, "id = ?", id).Error
	if err == nil {
		InvalidateRoutePlanCache()
	}
	return err
}

func CountRoutePlanTokens() (map[int]int64, error) {
	type row struct {
		RoutePlanId int
		Count       int64
	}
	rows := make([]row, 0)
	err := DB.Model(&Token{}).Select("route_plan_id, COUNT(*) AS count").Where("route_plan_id > 0").
		Group("route_plan_id").Scan(&rows).Error
	result := map[int]int64{}
	for _, r := range rows {
		result[r.RoutePlanId] = r.Count
	}
	return result, err
}

var routePlanCache = struct {
	sync.RWMutex
	plans    map[int]RoutePlan
	loadedAt time.Time
}{plans: map[int]RoutePlan{}}

// InvalidateRoutePlanCache forces the next lookup on this instance to reload.
// Other instances pick up edits within routePlanCacheTTL.
func InvalidateRoutePlanCache() {
	routePlanCache.Lock()
	routePlanCache.loadedAt = time.Time{}
	routePlanCache.Unlock()
}

// GetRoutePlanCached serves the auth path from a short-lived copy of all
// plans; the table is small and a full reload keeps cache logic trivial.
func GetRoutePlanCached(id int) (*RoutePlan, error) {
	routePlanCache.RLock()
	fresh := time.Since(routePlanCache.loadedAt) < routePlanCacheTTL
	plan, ok := routePlanCache.plans[id]
	routePlanCache.RUnlock()
	if !fresh {
		plans, err := ListRoutePlans()
		if err != nil {
			if ok {
				// Keep serving the last known contract during a DB blip.
				return &plan, nil
			}
			return nil, err
		}
		next := make(map[int]RoutePlan, len(plans))
		for _, p := range plans {
			next[p.Id] = p
		}
		routePlanCache.Lock()
		routePlanCache.plans, routePlanCache.loadedAt = next, time.Now()
		routePlanCache.Unlock()
		plan, ok = next[id]
	}
	if !ok {
		return nil, ErrRoutePlanNotFound
	}
	return &plan, nil
}
