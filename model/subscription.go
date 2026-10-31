package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/hot"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 订阅时长单位
const (
	SubscriptionDurationYear   = "year"
	SubscriptionDurationMonth  = "month"
	SubscriptionDurationDay    = "day"
	SubscriptionDurationHour   = "hour"
	SubscriptionDurationCustom = "custom"
)

// 订阅购买模式
const (
	SubscriptionPurchaseModeStack       = "stack"
	SubscriptionPurchaseModeRenew       = "renew"
	SubscriptionPurchaseModeRenewExtend = "renew_extend"
)

// 管理端订阅动作
const (
	SubscriptionManageActionEnable  = "enable"
	SubscriptionManageActionDisable = "disable"
	SubscriptionManageActionDelete  = "delete"
)

// 订阅额度重置周期
const (
	SubscriptionResetNever   = "never"
	SubscriptionResetDaily   = "daily"
	SubscriptionResetWeekly  = "weekly"
	SubscriptionResetMonthly = "monthly"
	SubscriptionResetCustom  = "custom"
)

var (
	ErrSubscriptionOrderNotFound      = errors.New("subscription order not found")
	ErrSubscriptionOrderStatusInvalid = errors.New("subscription order status invalid")
)

func NormalizeSubscriptionPurchaseMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case SubscriptionPurchaseModeRenew:
		return SubscriptionPurchaseModeRenew
	case SubscriptionPurchaseModeRenewExtend:
		return SubscriptionPurchaseModeRenewExtend
	default:
		return SubscriptionPurchaseModeStack
	}
}

const (
	subscriptionPlanCacheNamespace     = "new-api:subscription_plan:v1"
	subscriptionPlanInfoCacheNamespace = "new-api:subscription_plan_info:v1"
)

var (
	subscriptionPlanCacheOnce     sync.Once
	subscriptionPlanInfoCacheOnce sync.Once

	subscriptionPlanCache     *cachex.HybridCache[SubscriptionPlan]
	subscriptionPlanInfoCache *cachex.HybridCache[SubscriptionPlanInfo]
)

func subscriptionPlanCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_TTL", 300)
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanInfoCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_TTL", 120)
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_CAP", 5000)
	if capacity <= 0 {
		capacity = 5000
	}
	return capacity
}

func subscriptionPlanInfoCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_CAP", 10000)
	if capacity <= 0 {
		capacity = 10000
	}
	return capacity
}

func getSubscriptionPlanCache() *cachex.HybridCache[SubscriptionPlan] {
	subscriptionPlanCacheOnce.Do(func() {
		ttl := subscriptionPlanCacheTTL()
		subscriptionPlanCache = cachex.NewHybridCache[SubscriptionPlan](cachex.HybridCacheConfig[SubscriptionPlan]{
			Namespace: cachex.Namespace(subscriptionPlanCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlan]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlan] {
				return hot.NewHotCache[string, SubscriptionPlan](hot.LRU, subscriptionPlanCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanCache
}

func getSubscriptionPlanInfoCache() *cachex.HybridCache[SubscriptionPlanInfo] {
	subscriptionPlanInfoCacheOnce.Do(func() {
		ttl := subscriptionPlanInfoCacheTTL()
		subscriptionPlanInfoCache = cachex.NewHybridCache[SubscriptionPlanInfo](cachex.HybridCacheConfig[SubscriptionPlanInfo]{
			Namespace: cachex.Namespace(subscriptionPlanInfoCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlanInfo]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlanInfo] {
				return hot.NewHotCache[string, SubscriptionPlanInfo](hot.LRU, subscriptionPlanInfoCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanInfoCache
}

func subscriptionPlanCacheKey(id int) string {
	if id <= 0 {
		return ""
	}
	return strconv.Itoa(id)
}

func InvalidateSubscriptionPlanCache(planId int) {
	if planId <= 0 {
		return
	}
	cache := getSubscriptionPlanCache()
	_, _ = cache.DeleteMany([]string{subscriptionPlanCacheKey(planId)})
	infoCache := getSubscriptionPlanInfoCache()
	_ = infoCache.Purge()
}

// 订阅套餐定义
type SubscriptionPlan struct {
	Id int `json:"id"`

	Title    string `json:"title" gorm:"type:varchar(128);not null"`
	Subtitle string `json:"subtitle" gorm:"type:varchar(255);default:''"`

	// 展示用金额（沿用现有代码风格，金额使用 float64）
	PriceAmount float64 `json:"price_amount" gorm:"type:decimal(10,6);not null;default:0"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);not null;default:'USD'"`

	DurationUnit  string `json:"duration_unit" gorm:"type:varchar(16);not null;default:'month'"`
	DurationValue int    `json:"duration_value" gorm:"type:int;not null;default:1"`
	CustomSeconds int64  `json:"custom_seconds" gorm:"type:bigint;not null;default:0"`

	Enabled   bool `json:"enabled" gorm:"default:true"`
	SortOrder int  `json:"sort_order" gorm:"type:int;default:0"`

	StripePriceId  string `json:"stripe_price_id" gorm:"type:varchar(128);default:''"`
	CreemProductId string `json:"creem_product_id" gorm:"type:varchar(128);default:''"`

	// 每个用户最大购买次数（0 表示不限制）
	MaxPurchasePerUser int `json:"max_purchase_per_user" gorm:"type:int;default:0"`
	// 每个用户最大叠加条数（0 表示不限制），仅限制“叠加新购”，不限制续费。
	MaxStackPerUser int `json:"max_stack_per_user" gorm:"type:int;default:0"`
	// PurchaseQuantityMin/Max 控制该套餐单次购买数量范围。
	// 例如 1~12 表示一次可买 1 到 12 份。
	PurchaseQuantityMin int `json:"purchase_quantity_min" gorm:"type:int;default:1"`
	PurchaseQuantityMax int `json:"purchase_quantity_max" gorm:"type:int;default:12"`

	// 购买后升级用户分组（为空表示不变）
	UpgradeGroup string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`

	// 总额度（以配额单位计，0 表示不限制）
	TotalAmount int64 `json:"total_amount" gorm:"type:bigint;not null;default:0"`

	// 套餐的额度重置周期
	QuotaResetPeriod        string `json:"quota_reset_period" gorm:"type:varchar(16);default:'never'"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds" gorm:"type:bigint;default:0"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (p *SubscriptionPlan) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (p *SubscriptionPlan) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = common.GetTimestamp()
	return nil
}

// 订阅订单（支付 -> webhook 回调 -> 创建 UserSubscription）
type SubscriptionOrder struct {
	Id     int     `json:"id"`
	UserId int     `json:"user_id" gorm:"index"`
	PlanId int     `json:"plan_id" gorm:"index"`
	Money  float64 `json:"money"`
	// PurchaseQuantity 表示本次购买份数，默认 1。
	PurchaseQuantity int `json:"purchase_quantity" gorm:"type:int;not null;default:1"`

	TradeNo         string `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	PurchaseMode    string `json:"purchase_mode" gorm:"type:varchar(16);not null;default:'stack'"`
	// RenewTargetSubscriptionId 仅在 purchase_mode=renew 时生效。
	RenewTargetSubscriptionId int    `json:"renew_target_subscription_id" gorm:"index;default:0"`
	Status                    string `json:"status"`
	CreateTime                int64  `json:"create_time"`
	CompleteTime              int64  `json:"complete_time"`

	ProviderPayload string `json:"provider_payload" gorm:"type:text"`
}

func (o *SubscriptionOrder) Insert() error {
	if o.CreateTime == 0 {
		o.CreateTime = common.GetTimestamp()
	}
	if o.PurchaseQuantity <= 0 {
		o.PurchaseQuantity = 1
	}
	return DB.Create(o).Error
}

func (o *SubscriptionOrder) Update() error {
	return DB.Save(o).Error
}

func GetSubscriptionOrderByTradeNo(tradeNo string) *SubscriptionOrder {
	order, err := GetSubscriptionOrderByTradeNoWithError(tradeNo)
	if err != nil {
		return nil
	}
	return order
}

func GetSubscriptionOrderByTradeNoWithError(tradeNo string) (*SubscriptionOrder, error) {
	if strings.TrimSpace(tradeNo) == "" {
		return nil, ErrSubscriptionOrderNotFound
	}
	order := &SubscriptionOrder{}
	if err := DB.Where("trade_no = ?", tradeNo).First(order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSubscriptionOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

// 用户订阅实例
type UserSubscription struct {
	Id     int `json:"id"`
	UserId int `json:"user_id" gorm:"index;index:idx_user_sub_active,priority:1"`
	PlanId int `json:"plan_id" gorm:"index"`

	AmountTotal int64 `json:"amount_total" gorm:"type:bigint;not null;default:0"`
	AmountUsed  int64 `json:"amount_used" gorm:"type:bigint;not null;default:0"`

	StartTime int64  `json:"start_time" gorm:"bigint"`
	EndTime   int64  `json:"end_time" gorm:"bigint;index;index:idx_user_sub_active,priority:3"`
	Status    string `json:"status" gorm:"type:varchar(32);index;index:idx_user_sub_active,priority:2"` // active/expired/cancelled

	Source string `json:"source" gorm:"type:varchar(32);default:'order'"` // order/admin

	LastResetTime int64 `json:"last_reset_time" gorm:"type:bigint;default:0"`
	NextResetTime int64 `json:"next_reset_time" gorm:"type:bigint;default:0;index"`

	UpgradeGroup  string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`
	PrevUserGroup string `json:"prev_user_group" gorm:"type:varchar(64);default:''"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (s *UserSubscription) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (s *UserSubscription) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = common.GetTimestamp()
	return nil
}

type SubscriptionSummary struct {
	Subscription *UserSubscription `json:"subscription"`
}

func calcPlanEndTime(start time.Time, plan *SubscriptionPlan) (int64, error) {
	if plan == nil {
		return 0, errors.New("plan is nil")
	}
	if plan.DurationValue <= 0 && plan.DurationUnit != SubscriptionDurationCustom {
		return 0, errors.New("duration_value must be > 0")
	}
	switch plan.DurationUnit {
	case SubscriptionDurationYear:
		return start.AddDate(plan.DurationValue, 0, 0).Unix(), nil
	case SubscriptionDurationMonth:
		return start.AddDate(0, plan.DurationValue, 0).Unix(), nil
	case SubscriptionDurationDay:
		return start.Add(time.Duration(plan.DurationValue) * 24 * time.Hour).Unix(), nil
	case SubscriptionDurationHour:
		return start.Add(time.Duration(plan.DurationValue) * time.Hour).Unix(), nil
	case SubscriptionDurationCustom:
		if plan.CustomSeconds <= 0 {
			return 0, errors.New("custom_seconds must be > 0")
		}
		return start.Add(time.Duration(plan.CustomSeconds) * time.Second).Unix(), nil
	default:
		return 0, fmt.Errorf("invalid duration_unit: %s", plan.DurationUnit)
	}
}

func NormalizeResetPeriod(period string) string {
	switch strings.TrimSpace(period) {
	case SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly, SubscriptionResetCustom:
		return strings.TrimSpace(period)
	default:
		return SubscriptionResetNever
	}
}

func calcNextResetTime(base time.Time, plan *SubscriptionPlan, endUnix int64) int64 {
	if plan == nil {
		return 0
	}
	period := NormalizeResetPeriod(plan.QuotaResetPeriod)
	if period == SubscriptionResetNever {
		return 0
	}
	var next time.Time
	switch period {
	case SubscriptionResetDaily:
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, 1)
	case SubscriptionResetWeekly:
		// 对齐到下周一 00:00
		weekday := int(base.Weekday()) // Sunday=0
		// 转换为 Monday=1..Sunday=7
		if weekday == 0 {
			weekday = 7
		}
		daysUntil := 8 - weekday
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, daysUntil)
	case SubscriptionResetMonthly:
		// 对齐到下月第一天 00:00
		next = time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, base.Location()).
			AddDate(0, 1, 0)
	case SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds <= 0 {
			return 0
		}
		next = base.Add(time.Duration(plan.QuotaResetCustomSeconds) * time.Second)
	default:
		return 0
	}
	if endUnix > 0 && next.Unix() > endUnix {
		return 0
	}
	return next.Unix()
}

func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	return getSubscriptionPlanByIdTx(nil, id)
}

func getSubscriptionPlanByIdTx(tx *gorm.DB, id int) (*SubscriptionPlan, error) {
	if id <= 0 {
		return nil, errors.New("invalid plan id")
	}
	key := subscriptionPlanCacheKey(id)
	if key != "" {
		if cached, found, err := getSubscriptionPlanCache().Get(key); err == nil && found {
			return &cached, nil
		}
	}
	var plan SubscriptionPlan
	query := DB
	if tx != nil {
		query = tx
	}
	if err := query.Where("id = ?", id).First(&plan).Error; err != nil {
		return nil, err
	}
	_ = getSubscriptionPlanCache().SetWithTTL(key, plan, subscriptionPlanCacheTTL())
	return &plan, nil
}

func CountUserSubscriptionsByPlan(userId int, planId int) (int64, error) {
	if userId <= 0 || planId <= 0 {
		return 0, errors.New("invalid userId or planId")
	}
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, planId).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func countUserActiveSubscriptionsByPlanTx(tx *gorm.DB, userId int, planId int) (int64, error) {
	if userId <= 0 || planId <= 0 {
		return 0, errors.New("invalid userId or planId")
	}
	if tx == nil {
		tx = DB
	}
	now := common.GetTimestamp()
	var count int64
	if err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, planId, "active", now).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func CountUserActiveSubscriptionsByPlan(userId int, planId int) (int64, error) {
	return countUserActiveSubscriptionsByPlanTx(nil, userId, planId)
}

// fixedPlanDurationSeconds 返回固定时长套餐的周期秒数（day/hour/custom）。
// month/year 不返回固定秒数，交给按边界迭代计算。
func fixedPlanDurationSeconds(plan *SubscriptionPlan) int64 {
	if plan == nil {
		return 0
	}
	switch plan.DurationUnit {
	case SubscriptionDurationDay:
		if plan.DurationValue > 0 {
			return int64(plan.DurationValue) * 24 * 3600
		}
	case SubscriptionDurationHour:
		if plan.DurationValue > 0 {
			return int64(plan.DurationValue) * 3600
		}
	case SubscriptionDurationCustom:
		if plan.CustomSeconds > 0 {
			return plan.CustomSeconds
		}
	}
	return 0
}

// countRemainingSubscriptionQuantity 计算单条生效订阅的“剩余份数”。
// 说明：
// - 固定秒数周期（day/hour/custom）使用向上取整；
// - month/year 使用周期边界迭代，保证与续费 AddDate 行为一致；
// - 对历史异常数据保守兜底为 1，避免出现“明明生效却计算为 0”。
func countRemainingSubscriptionQuantity(sub *UserSubscription, plan *SubscriptionPlan, nowUnix int64) int {
	if sub == nil || plan == nil {
		return 0
	}
	if sub.Status != "active" || sub.EndTime <= nowUnix {
		return 0
	}

	if durationSeconds := fixedPlanDurationSeconds(plan); durationSeconds > 0 {
		remainSeconds := sub.EndTime - nowUnix
		if remainSeconds <= 0 {
			return 0
		}
		// 向上取整，部分周期按 1 份计入。
		return int((remainSeconds + durationSeconds - 1) / durationSeconds)
	}

	startUnix := sub.StartTime
	if startUnix <= 0 || startUnix >= sub.EndTime {
		return 1
	}

	const maxCycleIterations = 5000
	remaining := 0
	cursor := time.Unix(startUnix, 0)
	for i := 0; i < maxCycleIterations; i++ {
		nextUnix, err := calcPlanEndTime(cursor, plan)
		if err != nil || nextUnix <= cursor.Unix() {
			return 1
		}
		if nextUnix > nowUnix {
			remaining++
		}
		if nextUnix >= sub.EndTime {
			break
		}
		cursor = time.Unix(nextUnix, 0)
	}
	if remaining <= 0 {
		return 1
	}
	return remaining
}

// CountRemainingSubscriptionQuantity 对外暴露单条订阅“剩余份数”计算，供控制层复用。
func CountRemainingSubscriptionQuantity(sub *UserSubscription, plan *SubscriptionPlan, nowUnix int64) int {
	return countRemainingSubscriptionQuantity(sub, plan, nowUnix)
}

// CountUserActiveSubscriptionQuantityByPlan 统计用户在某套餐下当前“未过期份数”总和。
// 该值用于限制“最大购买数量”的动态可买上限。
func CountUserActiveSubscriptionQuantityByPlan(userId int, plan *SubscriptionPlan) (int, error) {
	if userId <= 0 || plan == nil || plan.Id <= 0 {
		return 0, errors.New("invalid userId or plan")
	}
	nowUnix := common.GetTimestamp()
	var subs []UserSubscription
	if err := DB.Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?",
		userId, plan.Id, "active", nowUnix).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	total := 0
	for i := range subs {
		total += countRemainingSubscriptionQuantity(&subs[i], plan, nowUnix)
	}
	return total, nil
}

func GetEarliestActiveUserSubscriptionByPlan(userId int, planId int) (*UserSubscription, error) {
	if userId <= 0 || planId <= 0 {
		return nil, errors.New("invalid userId or planId")
	}
	now := common.GetTimestamp()
	var sub UserSubscription
	query := DB.Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?",
		userId, planId, "active", now).
		Order("end_time asc, id asc").
		Limit(1).
		Find(&sub)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, nil
	}
	return &sub, nil
}

// GetActiveUserSubscriptionsByPlan 返回用户在某套餐下所有生效订阅，按最早到期优先排序。
func GetActiveUserSubscriptionsByPlan(userId int, planId int) ([]UserSubscription, error) {
	if userId <= 0 || planId <= 0 {
		return nil, errors.New("invalid userId or planId")
	}
	now := common.GetTimestamp()
	subs := make([]UserSubscription, 0)
	if err := DB.Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?",
		userId, planId, "active", now).
		Order("end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

// getActiveUserSubscriptionsByPlanTx 在事务内读取用户同套餐生效订阅，并按最早到期优先排序。
// targetSubscriptionId>0 时仅返回指定目标（若目标无效则返回空切片）。
func getActiveUserSubscriptionsByPlanTx(tx *gorm.DB, userId int, planId int, targetSubscriptionId int) ([]UserSubscription, error) {
	if userId <= 0 || planId <= 0 {
		return nil, errors.New("invalid userId or planId")
	}
	now := GetDBTimestampTx(newSubscriptionMutationSessionTx(tx))
	session := newSubscriptionMutationSessionTx(tx)
	query := session.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?",
		userId, planId, "active", now).
		Order("end_time asc, id asc")
	if targetSubscriptionId > 0 {
		query = query.Where("id = ?", targetSubscriptionId)
	}
	if !common.UsingSQLite {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}
	subs := make([]UserSubscription, 0)
	if err := query.Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

// newSubscriptionMutationSessionTx 在同一事务里生成一个干净 session，
// 避免上一条查询残留的 where/model 污染后续套餐发放 SQL。
func newSubscriptionMutationSessionTx(tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	return propagateBenefitContextTx(tx, tx.Session(&gorm.Session{NewDB: true}))
}

// lockUserForSubscriptionMutationTx 锁定用户行，避免同用户并发完成订单时出现超卖。
func lockUserForSubscriptionMutationTx(tx *gorm.DB, userId int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if userId <= 0 {
		return errors.New("invalid user id")
	}
	query := newSubscriptionMutationSessionTx(tx).Model(&User{}).Select("id").Where("id = ?", userId)
	if !common.UsingSQLite {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}
	var user User
	return query.First(&user).Error
}

func getUserGroupByIdTx(tx *gorm.DB, userId int) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid userId")
	}
	if tx == nil {
		tx = DB
	}
	var group string
	if err := newSubscriptionMutationSessionTx(tx).
		Model(&User{}).
		Where("id = ?", userId).
		Select(commonGroupCol).
		Find(&group).Error; err != nil {
		return "", err
	}
	return group, nil
}

func downgradeUserGroupForSubscriptionTx(tx *gorm.DB, sub *UserSubscription, now int64) (string, error) {
	if tx == nil || sub == nil {
		return "", errors.New("invalid downgrade args")
	}
	upgradeGroup := strings.TrimSpace(sub.UpgradeGroup)
	if upgradeGroup == "" {
		return "", nil
	}
	currentGroup, err := getUserGroupByIdTx(tx, sub.UserId)
	if err != nil {
		return "", err
	}
	if currentGroup != upgradeGroup {
		return "", nil
	}
	var activeSub UserSubscription
	activeQuery := newSubscriptionMutationSessionTx(tx).Where("user_id = ? AND status = ? AND end_time > ? AND id <> ? AND upgrade_group <> ''",
		sub.UserId, "active", now, sub.Id).
		Order("end_time desc, id desc").
		Limit(1).
		Find(&activeSub)
	if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
		return "", nil
	}
	prevGroup := strings.TrimSpace(sub.PrevUserGroup)
	if prevGroup == "" || prevGroup == currentGroup {
		return "", nil
	}
	if err := newSubscriptionMutationSessionTx(tx).Model(&User{}).Where("id = ?", sub.UserId).
		Update("group", prevGroup).Error; err != nil {
		return "", err
	}
	return prevGroup, nil
}

func createUserSubscriptionFromPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, source string, enforcePurchaseLimit bool) (*UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	session := newSubscriptionMutationSessionTx(tx)
	if plan == nil || plan.Id == 0 {
		return nil, errors.New("invalid plan")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if enforcePurchaseLimit && plan.MaxPurchasePerUser > 0 {
		var count int64
		if err := newSubscriptionMutationSessionTx(tx).Model(&UserSubscription{}).
			Where("user_id = ? AND plan_id = ?", userId, plan.Id).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			return nil, errors.New("已达到该套餐购买上限")
		}
	}
	nowUnix := GetDBTimestampTx(newSubscriptionMutationSessionTx(tx))
	now := time.Unix(nowUnix, 0)
	endUnix, err := calcPlanEndTime(now, plan)
	if err != nil {
		return nil, err
	}
	resetBase := now
	nextReset := calcNextResetTime(resetBase, plan, endUnix)
	lastReset := int64(0)
	if nextReset > 0 {
		lastReset = now.Unix()
	}
	upgradeGroup := strings.TrimSpace(plan.UpgradeGroup)
	prevGroup := ""
	if upgradeGroup != "" {
		currentGroup, err := getUserGroupByIdTx(tx, userId)
		if err != nil {
			return nil, err
		}
		if currentGroup != upgradeGroup {
			prevGroup = currentGroup
			if err := newSubscriptionMutationSessionTx(tx).Model(&User{}).Where("id = ?", userId).
				Update("group", upgradeGroup).Error; err != nil {
				return nil, err
			}
		}
	}
	sub := &UserSubscription{
		UserId:        userId,
		PlanId:        plan.Id,
		AmountTotal:   plan.TotalAmount,
		AmountUsed:    0,
		StartTime:     now.Unix(),
		EndTime:       endUnix,
		Status:        "active",
		Source:        source,
		LastResetTime: lastReset,
		NextResetTime: nextReset,
		UpgradeGroup:  upgradeGroup,
		PrevUserGroup: prevGroup,
		CreatedAt:     common.GetTimestamp(),
		UpdatedAt:     common.GetTimestamp(),
	}
	if err := session.Model(&UserSubscription{}).Create(sub).Error; err != nil {
		return nil, err
	}
	if benefitSourceType, benefitSourceRef, purchaseMode, issuanceId, ok := getBenefitContextTx(tx); ok {
		if err := createBenefitChangeRecordTx(tx, &BenefitChangeRecord{
			BenefitType: BenefitTypeSubscription,
			Action:      BenefitActionGrant,
			SourceType:  benefitSourceType,
			SourceRef:   benefitSourceRef,
			UserId:      userId,
			TargetType:  BenefitTargetUserSubscription,
			TargetId:    sub.Id,
			Detail: marshalBenefitDetail(&SubscriptionBenefitDetail{
				Operation:        SubscriptionBenefitOperationCreate,
				PurchaseMode:     purchaseMode,
				IssuanceId:       issuanceId,
				PlanId:           plan.Id,
				EndTimeAfter:     sub.EndTime,
				AmountTotalAfter: sub.AmountTotal,
				StatusAfter:      sub.Status,
			}),
		}); err != nil {
			return nil, err
		}
	}
	return sub, nil
}

func CreateUserSubscriptionFromPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, source string) (*UserSubscription, error) {
	return createUserSubscriptionFromPlanTx(tx, userId, plan, source, true)
}

// renewUserSubscriptionByPlanTx 对用户同 plan 的生效订阅做续费：
// 1) 优先命中指定目标订阅（targetSubscriptionId）
// 2) 若未指定或目标不可用，则按最早到期优先
func renewUserSubscriptionByPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, targetSubscriptionId int) (*UserSubscription, bool, error) {
	if tx == nil {
		return nil, false, errors.New("tx is nil")
	}
	if plan == nil || plan.Id == 0 {
		return nil, false, errors.New("invalid plan")
	}
	if userId <= 0 {
		return nil, false, errors.New("invalid user id")
	}
	now := GetDBTimestampTx(newSubscriptionMutationSessionTx(tx))
	query := newSubscriptionMutationSessionTx(tx).Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?",
			userId, plan.Id, "active", now)
	if !common.UsingSQLite {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}
	if targetSubscriptionId > 0 {
		query = query.Where("id = ?", targetSubscriptionId)
	}
	var sub UserSubscription
	result := query.Order("end_time asc, id asc").Limit(1).Find(&sub)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	prevEndTime := sub.EndTime
	prevAmountTotal := sub.AmountTotal
	prevStatus := sub.Status

	baseEnd := sub.EndTime
	if baseEnd <= 0 {
		baseEnd = now
	}
	newEnd, err := calcPlanEndTime(time.Unix(baseEnd, 0), plan)
	if err != nil {
		return nil, false, err
	}
	sub.EndTime = newEnd

	// 对“不重置”套餐，续费时累计总额度；
	// 对“会重置”套餐，仅延长有效期，避免每日/月度额度越续越膨胀。
	if NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		if plan.TotalAmount <= 0 || sub.AmountTotal <= 0 {
			sub.AmountTotal = 0
		} else {
			sub.AmountTotal += plan.TotalAmount
		}
	}

	if err := newSubscriptionMutationSessionTx(tx).Save(&sub).Error; err != nil {
		return nil, false, err
	}
	if NormalizeResetPeriod(plan.QuotaResetPeriod) != SubscriptionResetNever {
		if err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
			return nil, false, err
		}
	}
	if benefitSourceType, benefitSourceRef, purchaseMode, issuanceId, ok := getBenefitContextTx(tx); ok {
		endTimeDelta := sub.EndTime - prevEndTime
		amountTotalDelta := sub.AmountTotal - prevAmountTotal
		if endTimeDelta < 0 {
			endTimeDelta = 0
		}
		if amountTotalDelta < 0 {
			amountTotalDelta = 0
		}
		if err := createBenefitChangeRecordTx(tx, &BenefitChangeRecord{
			BenefitType: BenefitTypeSubscription,
			Action:      BenefitActionGrant,
			SourceType:  benefitSourceType,
			SourceRef:   benefitSourceRef,
			UserId:      userId,
			TargetType:  BenefitTargetUserSubscription,
			TargetId:    sub.Id,
			Detail: marshalBenefitDetail(&SubscriptionBenefitDetail{
				Operation:         SubscriptionBenefitOperationRenew,
				PurchaseMode:      purchaseMode,
				IssuanceId:        issuanceId,
				PlanId:            plan.Id,
				EndTimeBefore:     prevEndTime,
				EndTimeAfter:      sub.EndTime,
				EndTimeDelta:      endTimeDelta,
				AmountTotalBefore: prevAmountTotal,
				AmountTotalAfter:  sub.AmountTotal,
				AmountTotalDelta:  amountTotalDelta,
				StatusBefore:      prevStatus,
				StatusAfter:       sub.Status,
			}),
		}); err != nil {
			return nil, false, err
		}
	}
	return &sub, true, nil
}

// CompleteSubscriptionOrder 完成在线支付订阅订单（幂等）：
// - 先稳定落库订单成功态、支付记录镜像和待发放记录
// - 返佣待办同事务保存；返佣结算及套餐自动发放在提交后执行
// - stack: 新建订阅
// - renew: 仅续费同 plan 生效订阅；无可续费目标时不回退到叠加
// - renew_extend: 续费式购买；无生效订阅时先创建 1 条，再按同条顺延
func CompleteSubscriptionOrder(tradeNo string, providerPayload string) error {
	return CompleteSubscriptionOrderWithPayment(tradeNo, providerPayload, "", "")
}

func CompleteSubscriptionOrderWithPayment(tradeNo string, providerPayload string, actualPaymentMethod string, paymentProvider string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}

	result := subscriptionOrderCompletionResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		coreResult, err := completeSubscriptionOrderCoreTx(tx, tradeNo, providerPayload, actualPaymentMethod, paymentProvider)
		if err != nil {
			return err
		}
		result = coreResult
		return nil
	})
	if err != nil {
		return err
	}
	NotifyInviteCommissionSettlement()
	if result.AlreadyCompleted {
		return nil
	}

	result.AutoIssued, result.IssueSummary = autoIssuePaidSubscriptionOrder(result.TradeNo, result.IssuanceId, result.UserId)

	if result.UserId > 0 {
		if result.PurchaseQuantity <= 0 {
			result.PurchaseQuantity = 1
		}
		msg := fmt.Sprintf("订阅支付成功，套餐: %s，份数: %d，支付金额: %.2f，支付方式: %s，发放记录: %d", result.PlanTitle, result.PurchaseQuantity, result.Money, result.PaymentMethod, result.IssuanceId)
		if result.AutoIssued {
			if result.IssueSummary != "" {
				msg = fmt.Sprintf("%s，已自动发放：%s", msg, result.IssueSummary)
			} else {
				msg = fmt.Sprintf("%s，已自动发放", msg)
			}
		} else {
			if result.IssueSummary != "" {
				msg = fmt.Sprintf("%s，待用户确认发放：%s", msg, result.IssueSummary)
			} else {
				msg = fmt.Sprintf("%s，待用户确认发放", msg)
			}
		}
		RecordTopupLog(result.UserId, msg, "", result.PaymentMethod, "subscription_grant",
			map[string]interface{}{"issuance_id": result.IssuanceId, "trade_no": result.TradeNo})
	}
	return nil
}

type subscriptionOrderCompletionResult struct {
	OrderId          int
	TradeNo          string
	UserId           int
	PlanTitle        string
	Money            float64
	PaymentMethod    string
	PurchaseQuantity int
	IssuanceId       int
	AlreadyCompleted bool
	AutoIssued       bool
	IssueSummary     string
}

func completeSubscriptionOrderCoreTx(tx *gorm.DB, tradeNo string, providerPayload string, actualPaymentMethod string, paymentProvider string) (subscriptionOrderCompletionResult, error) {
	if tx == nil {
		return subscriptionOrderCompletionResult{}, errors.New("tx is nil")
	}
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	var order SubscriptionOrder
	query := tx.Where(refCol+" = ?", tradeNo)
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&order).Error; err != nil {
		return subscriptionOrderCompletionResult{}, ErrSubscriptionOrderNotFound
	}
	result := subscriptionOrderCompletionResult{
		OrderId:          order.Id,
		TradeNo:          order.TradeNo,
		UserId:           order.UserId,
		Money:            order.Money,
		PaymentMethod:    order.PaymentMethod,
		PurchaseQuantity: order.PurchaseQuantity,
	}
	if order.Status == common.TopUpStatusSuccess {
		result.AlreadyCompleted = true
		return result, nil
	}
	if order.Status != common.TopUpStatusPending {
		return subscriptionOrderCompletionResult{}, ErrSubscriptionOrderStatusInvalid
	}

	actualPaymentMethod = NormalizePaymentMethod(actualPaymentMethod)
	paymentProvider = InferPaymentProvider(paymentProvider, actualPaymentMethod)
	if paymentProvider != "" && !PaymentProviderMatches(order.PaymentProvider, order.PaymentMethod, paymentProvider, actualPaymentMethod) {
		return subscriptionOrderCompletionResult{}, ErrPaymentMethodMismatch
	}
	if order.PaymentProvider == "" {
		order.PaymentProvider = InferPaymentProvider(paymentProvider, order.PaymentMethod)
	}
	if order.PaymentMethod == "" && actualPaymentMethod != "" {
		order.PaymentMethod = actualPaymentMethod
	}
	result.PaymentMethod = order.PaymentMethod

	plan, err := getSubscriptionPlanByIdTx(tx, order.PlanId)
	if err != nil {
		return subscriptionOrderCompletionResult{}, err
	}
	result.PlanTitle = plan.Title

	if order.PurchaseQuantity <= 0 {
		order.PurchaseQuantity = 1
		result.PurchaseQuantity = 1
	}
	issuance := &SubscriptionIssuance{
		UserId:                    order.UserId,
		PlanId:                    plan.Id,
		PlanTitle:                 plan.Title,
		SourceType:                SubscriptionIssuanceSourceOrder,
		SourceRef:                 order.TradeNo,
		PurchaseMode:              normalizeSubscriptionIssuancePurchaseMode(order.PurchaseMode),
		PurchaseQuantity:          order.PurchaseQuantity,
		RenewTargetSubscriptionId: order.RenewTargetSubscriptionId,
	}
	if err := CreateSubscriptionIssuanceTx(tx, issuance); err != nil {
		return subscriptionOrderCompletionResult{}, err
	}
	if err := upsertSubscriptionTopUpTx(tx, &order); err != nil {
		return subscriptionOrderCompletionResult{}, err
	}
	order.Status = common.TopUpStatusSuccess
	order.CompleteTime = common.GetTimestamp()
	if providerPayload != "" {
		order.ProviderPayload = providerPayload
	}
	if err := tx.Save(&order).Error; err != nil {
		return subscriptionOrderCompletionResult{}, err
	}
	if err := EnqueueInviteCommissionFromSubscriptionOrderTx(tx, &order); err != nil {
		return subscriptionOrderCompletionResult{}, err
	}
	result.IssuanceId = issuance.Id
	return result, nil
}

func autoIssuePaidSubscriptionOrder(tradeNo string, issuanceId int, userId int) (bool, string) {
	if issuanceId <= 0 || userId <= 0 {
		return false, ""
	}
	var summary string
	err := DB.Transaction(func(tx *gorm.DB) error {
		_, confirmSummary, err := ConfirmSubscriptionIssuanceTx(tx, issuanceId, userId, "", 0)
		if err != nil {
			summary = err.Error()
			return err
		}
		summary = confirmSummary
		return nil
	})
	if err == nil {
		return true, summary
	}

	if summary == "" {
		summary = err.Error()
	}
	if updateErr := updatePendingSubscriptionIssuanceIssueSummary(issuanceId, summary); updateErr != nil {
		common.SysError(fmt.Sprintf("update pending subscription issuance summary failed: trade_no=%s, issuance_id=%d, err=%s", tradeNo, issuanceId, updateErr.Error()))
	}
	common.SysError(fmt.Sprintf("auto issue paid subscription order failed: trade_no=%s, issuance_id=%d, err=%s", tradeNo, issuanceId, err.Error()))
	return false, summary
}

func updatePendingSubscriptionIssuanceIssueSummary(issuanceId int, summary string) error {
	if issuanceId <= 0 {
		return nil
	}
	summary = normalizeSubscriptionIssuanceIssueSummary(summary)
	return DB.Model(&SubscriptionIssuance{}).
		Where("id = ? AND status = ?", issuanceId, SubscriptionIssuanceStatusPending).
		Update("issue_summary", summary).Error
}

func normalizeSubscriptionIssuanceIssueSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	runes := []rune(summary)
	if len(runes) > 255 {
		return string(runes[:255])
	}
	return summary
}

func upsertSubscriptionTopUpTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if tx == nil || order == nil {
		return errors.New("invalid subscription order")
	}
	session := tx.Session(&gorm.Session{NewDB: true})
	now := common.GetTimestamp()
	var topup TopUp
	result := session.Where("trade_no = ?", order.TradeNo).Limit(1).Find(&topup)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		topup = TopUp{
			UserId:          order.UserId,
			Amount:          0,
			Money:           order.Money,
			PaidMoney:       order.Money,
			TradeNo:         order.TradeNo,
			PaymentMethod:   order.PaymentMethod,
			PaymentProvider: order.PaymentProvider,
			CreateTime:      order.CreateTime,
			CompleteTime:    now,
			Status:          common.TopUpStatusSuccess,
		}
		return session.Create(&topup).Error
	}
	topup.Money = order.Money
	topup.PaidMoney = order.Money
	if topup.PaymentMethod == "" {
		topup.PaymentMethod = order.PaymentMethod
	}
	if topup.PaymentProvider == "" {
		topup.PaymentProvider = order.PaymentProvider
	}
	if topup.CreateTime == 0 {
		topup.CreateTime = order.CreateTime
	}
	topup.CompleteTime = now
	topup.Status = common.TopUpStatusSuccess
	return session.Save(&topup).Error
}

func ExpireSubscriptionOrder(tradeNo string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		query := tx.Where(refCol+" = ?", tradeNo)
		if !common.UsingSQLite {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if order.Status != common.TopUpStatusPending {
			return nil
		}
		order.Status = common.TopUpStatusExpired
		order.CompleteTime = common.GetTimestamp()
		return tx.Save(&order).Error
	})
}

// 管理端绑定（无需支付）：按套餐创建一条 UserSubscription。
func AdminBindSubscription(userId int, planId int, sourceNote string) (string, error) {
	return AdminBindSubscriptionWithOptions(
		userId,
		planId,
		SubscriptionPurchaseModeStack,
		1,
		0,
		sourceNote,
	)
}

// AdminBindSubscriptionWithOptions 支持管理端在无支付场景下执行 stack/renew 操作。
func AdminBindSubscriptionWithOptions(
	userId int,
	planId int,
	purchaseMode string,
	purchaseQuantity int,
	renewTargetSubscriptionId int,
	sourceNote string,
) (string, error) {
	if userId <= 0 || planId <= 0 {
		return "", errors.New("invalid userId or planId")
	}
	plan, err := GetSubscriptionPlanById(planId)
	if err != nil {
		return "", err
	}
	mode := NormalizeSubscriptionPurchaseMode(purchaseMode)
	if purchaseQuantity <= 0 {
		purchaseQuantity = 1
	}
	now := common.GetTimestamp()

	renewTargetForExtend := 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockUserForSubscriptionMutationTx(tx, userId); err != nil {
			return err
		}
		if mode == SubscriptionPurchaseModeRenewExtend {
			activeSubs, err := getActiveUserSubscriptionsByPlanTx(tx, userId, plan.Id, 0)
			if err != nil {
				return err
			}
			if len(activeSubs) > 0 {
				renewTargetForExtend = activeSubs[0].Id
			}
		}

		for i := 0; i < purchaseQuantity; i++ {
			switch mode {
			case SubscriptionPurchaseModeRenew:
				_, renewed, err := renewUserSubscriptionByPlanTx(tx, userId, plan, renewTargetSubscriptionId)
				if err != nil {
					return err
				}
				if !renewed {
					if renewTargetSubscriptionId > 0 {
						return errors.New("续费目标订阅不存在或已失效")
					}
					return errors.New("无可续费的同规格订阅")
				}
			case SubscriptionPurchaseModeRenewExtend:
				if renewTargetForExtend > 0 {
					_, renewed, renewErr := renewUserSubscriptionByPlanTx(tx, userId, plan, renewTargetForExtend)
					if renewErr != nil {
						return renewErr
					}
					if !renewed {
						return errors.New("续费式顺延失败")
					}
					continue
				}
				createdSub, createErr := createUserSubscriptionFromPlanTx(tx, userId, plan, "admin", true)
				if createErr != nil {
					return createErr
				}
				renewTargetForExtend = createdSub.Id
			default:
				if _, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "admin"); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	modeLabel := "叠加"
	if mode == SubscriptionPurchaseModeRenew {
		modeLabel = "续费"
	} else if mode == SubscriptionPurchaseModeRenewExtend {
		modeLabel = "续费式购买"
	}
	resultMessage := fmt.Sprintf("已按%s方式添加 %d 份套餐", modeLabel, purchaseQuantity)
	if strings.TrimSpace(plan.UpgradeGroup) != "" {
		_ = UpdateUserGroupCache(userId, plan.UpgradeGroup)
		return fmt.Sprintf("%s，用户分组将升级到 %s", resultMessage, plan.UpgradeGroup), nil
	}
	_ = sourceNote
	_ = now
	return resultMessage, nil
}

// GetAllActiveUserSubscriptions 返回用户全部生效订阅。
func GetAllActiveUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var subs []UserSubscription
	err := DB.Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

// HasActiveUserSubscription 返回用户是否存在任意生效订阅。
// 这是轻量级存在性判断，避免进入较重的预扣事务。
func HasActiveUserSubscription(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetAllUserSubscriptions 返回用户全部订阅（含生效与过期）。
func GetAllUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	var subs []UserSubscription
	err := DB.Where("user_id = ?", userId).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

func buildSubscriptionSummaries(subs []UserSubscription) []SubscriptionSummary {
	if len(subs) == 0 {
		return []SubscriptionSummary{}
	}
	result := make([]SubscriptionSummary, 0, len(subs))
	for _, sub := range subs {
		subCopy := sub
		result = append(result, SubscriptionSummary{
			Subscription: &subCopy,
		})
	}
	return result
}

func normalizeSubscriptionManageAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case SubscriptionManageActionEnable:
		return SubscriptionManageActionEnable
	case SubscriptionManageActionDisable:
		return SubscriptionManageActionDisable
	case SubscriptionManageActionDelete:
		return SubscriptionManageActionDelete
	default:
		return ""
	}
}

// AdminManageUserSubscription 执行管理端订阅动作（启用/禁用/删除）。
//
// 规则说明：
// 1. disable：仅把状态改为 cancelled，保持 end_time 不变；
// 2. enable：仅允许 cancelled 且仍未过期（end_time > now）；
// 3. delete：硬删除记录。
func AdminManageUserSubscription(userId int, userSubscriptionId int, action string) (string, error) {
	if userId <= 0 || userSubscriptionId <= 0 {
		return "", errors.New("invalid userId or userSubscriptionId")
	}
	normalizedAction := normalizeSubscriptionManageAction(action)
	if normalizedAction == "" {
		return "", errors.New("不支持的操作类型")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	resultMessage := ""

	err := DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ? AND user_id = ?", userSubscriptionId, userId)
		if !common.UsingSQLite {
			query = query.Set("gorm:query_option", "FOR UPDATE")
		}
		var sub UserSubscription
		if err := query.First(&sub).Error; err != nil {
			return err
		}

		switch normalizedAction {
		case SubscriptionManageActionDisable:
			if sub.Status == "cancelled" {
				return errors.New("订阅已禁用")
			}
			if sub.EndTime > 0 && sub.EndTime <= now {
				return errors.New("订阅已过期，无法禁用")
			}
			if err := tx.Model(&sub).Updates(map[string]interface{}{
				"status":     "cancelled",
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
			targetGroup, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
			if err != nil {
				return err
			}
			if targetGroup != "" {
				cacheGroup = targetGroup
				resultMessage = fmt.Sprintf("已禁用，用户分组将回退到 %s", targetGroup)
			} else {
				resultMessage = "已禁用"
			}
		case SubscriptionManageActionEnable:
			if sub.Status == "active" && (sub.EndTime == 0 || sub.EndTime > now) {
				return errors.New("订阅已启用")
			}
			if sub.Status != "cancelled" {
				return errors.New("仅已禁用订阅可启用")
			}
			if sub.EndTime > 0 && sub.EndTime <= now {
				return errors.New("订阅已过期，无法启用")
			}
			if err := tx.Model(&sub).Updates(map[string]interface{}{
				"status":     "active",
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
			upgradeGroup := strings.TrimSpace(sub.UpgradeGroup)
			if upgradeGroup != "" {
				currentGroup, err := getUserGroupByIdTx(tx, userId)
				if err != nil {
					return err
				}
				if currentGroup != upgradeGroup {
					if strings.TrimSpace(sub.PrevUserGroup) == "" {
						// 首次补齐 prev_user_group，避免后续禁用/删除无法正确回退分组。
						if err := tx.Model(&sub).Update("prev_user_group", currentGroup).Error; err != nil {
							return err
						}
					}
					if err := newSubscriptionMutationSessionTx(tx).Model(&User{}).Where("id = ?", userId).Update("group", upgradeGroup).Error; err != nil {
						return err
					}
				}
				cacheGroup = upgradeGroup
				resultMessage = fmt.Sprintf("已启用，用户分组将升级到 %s", upgradeGroup)
			} else {
				resultMessage = "已启用"
			}
		case SubscriptionManageActionDelete:
			targetGroup, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
			if err != nil {
				return err
			}
			if err := tx.Where("id = ? AND user_id = ?", userSubscriptionId, userId).Delete(&UserSubscription{}).Error; err != nil {
				return err
			}
			if targetGroup != "" {
				cacheGroup = targetGroup
				resultMessage = fmt.Sprintf("已删除，用户分组将回退到 %s", targetGroup)
			} else {
				resultMessage = "已删除"
			}
		default:
			return errors.New("不支持的操作类型")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if cacheGroup != "" {
		_ = UpdateUserGroupCache(userId, cacheGroup)
	}
	return resultMessage, nil
}

// AdminInvalidateUserSubscription 将用户订阅标记为 cancelled 并立即结束。
func AdminInvalidateUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		if err := tx.Model(&sub).Updates(map[string]interface{}{
			"status":     "cancelled",
			"end_time":   now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if cacheGroup != "" && userId > 0 {
		_ = UpdateUserGroupCache(userId, cacheGroup)
	}
	if downgradeGroup != "" {
		return fmt.Sprintf("用户分组将回退到 %s", downgradeGroup), nil
	}
	return "", nil
}

// AdminDeleteUserSubscription 硬删除一条用户订阅。
func AdminDeleteUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		if err := tx.Where("id = ?", userSubscriptionId).Delete(&UserSubscription{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if cacheGroup != "" && userId > 0 {
		_ = UpdateUserGroupCache(userId, cacheGroup)
	}
	if downgradeGroup != "" {
		return fmt.Sprintf("用户分组将回退到 %s", downgradeGroup), nil
	}
	return "", nil
}

type SubscriptionPreConsumeResult struct {
	UserSubscriptionId int
	PreConsumed        int64
	AmountTotal        int64
	AmountUsedBefore   int64
	AmountUsedAfter    int64
}

// ExpireDueSubscriptions 处理到期订阅并执行分组回退。
func ExpireDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("status = ? AND end_time > 0 AND end_time <= ?", "active", now).
		Order("end_time asc, id asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	expiredCount := 0
	userIds := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if sub.UserId > 0 {
			userIds[sub.UserId] = struct{}{}
		}
	}
	for userId := range userIds {
		cacheGroup := ""
		err := DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&UserSubscription{}).
				Where("user_id = ? AND status = ? AND end_time > 0 AND end_time <= ?", userId, "active", now).
				Updates(map[string]interface{}{
					"status":     "expired",
					"updated_at": common.GetTimestamp(),
				})
			if res.Error != nil {
				return res.Error
			}
			expiredCount += int(res.RowsAffected)

			// 若仍有生效的升级订阅，则保持当前分组。
			var activeSub UserSubscription
			activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND upgrade_group <> ''",
				userId, "active", now).
				Order("end_time desc, id desc").
				Limit(1).
				Find(&activeSub)
			if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
				return nil
			}

			// 若没有生效的升级订阅，则按需回退到历史分组。
			var lastExpired UserSubscription
			expiredQuery := tx.Where("user_id = ? AND status = ? AND upgrade_group <> ''",
				userId, "expired").
				Order("end_time desc, id desc").
				Limit(1).
				Find(&lastExpired)
			if expiredQuery.Error != nil || expiredQuery.RowsAffected == 0 {
				return nil
			}
			upgradeGroup := strings.TrimSpace(lastExpired.UpgradeGroup)
			prevGroup := strings.TrimSpace(lastExpired.PrevUserGroup)
			if upgradeGroup == "" || prevGroup == "" {
				return nil
			}
			currentGroup, err := getUserGroupByIdTx(tx, userId)
			if err != nil {
				return err
			}
			if currentGroup != upgradeGroup || currentGroup == prevGroup {
				return nil
			}
			if err := newSubscriptionMutationSessionTx(tx).Model(&User{}).Where("id = ?", userId).
				Update("group", prevGroup).Error; err != nil {
				return err
			}
			cacheGroup = prevGroup
			return nil
		})
		if err != nil {
			return expiredCount, err
		}
		if cacheGroup != "" {
			_ = UpdateUserGroupCache(userId, cacheGroup)
		}
	}
	return expiredCount, nil
}

// SubscriptionPreConsumeRecord 记录每次请求的幂等预扣流水。
type SubscriptionPreConsumeRecord struct {
	Id                 int    `json:"id"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId             int    `json:"user_id" gorm:"index"`
	UserSubscriptionId int    `json:"user_subscription_id" gorm:"index"`
	PreConsumed        int64  `json:"pre_consumed" gorm:"type:bigint;not null;default:0"`
	Status             string `json:"status" gorm:"type:varchar(32);index"` // consumed/refunded
	CreatedAt          int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
}

func (r *SubscriptionPreConsumeRecord) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	r.CreatedAt = now
	r.UpdatedAt = now
	return nil
}

func (r *SubscriptionPreConsumeRecord) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

func maybeResetUserSubscriptionWithPlanTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64) error {
	if tx == nil || sub == nil || plan == nil {
		return errors.New("invalid reset args")
	}
	if sub.NextResetTime > 0 && sub.NextResetTime > now {
		return nil
	}
	if NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return nil
	}
	baseUnix := sub.LastResetTime
	if baseUnix <= 0 {
		baseUnix = sub.StartTime
	}
	base := time.Unix(baseUnix, 0)
	next := calcNextResetTime(base, plan, sub.EndTime)
	advanced := false
	for next > 0 && next <= now {
		advanced = true
		base = time.Unix(next, 0)
		next = calcNextResetTime(base, plan, sub.EndTime)
	}
	if !advanced {
		if sub.NextResetTime == 0 && next > 0 {
			sub.NextResetTime = next
			sub.LastResetTime = base.Unix()
			return newSubscriptionMutationSessionTx(tx).Save(sub).Error
		}
		return nil
	}
	sub.AmountUsed = 0
	sub.LastResetTime = base.Unix()
	sub.NextResetTime = next
	return newSubscriptionMutationSessionTx(tx).Save(sub).Error
}

// PreConsumeUserSubscription 从任意生效订阅总额度中执行预扣。
func PreConsumeUserSubscription(requestId string, userId int, modelName string, quotaType int, amount int64) (*SubscriptionPreConsumeResult, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if strings.TrimSpace(requestId) == "" {
		return nil, errors.New("requestId is empty")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be > 0")
	}
	now := GetDBTimestamp()

	returnValue := &SubscriptionPreConsumeResult{}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing SubscriptionPreConsumeRecord
		query := tx.Where("request_id = ?", requestId).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if existing.Status == "refunded" {
				return errors.New("subscription pre-consume already refunded")
			}
			var sub UserSubscription
			if err := tx.Where("id = ?", existing.UserSubscriptionId).First(&sub).Error; err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = existing.PreConsumed
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = sub.AmountUsed
			returnValue.AmountUsedAfter = sub.AmountUsed
			return nil
		}

		var subs []UserSubscription
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
			Order("end_time asc, id asc").
			Find(&subs).Error; err != nil {
			return errors.New("no active subscription")
		}
		if len(subs) == 0 {
			return &types.QuotaRejection{Source: "subscription", Required: amount, Message: "no active subscription"}
		}
		for _, candidate := range subs {
			sub := candidate
			plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
			if err != nil {
				return err
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
				return err
			}
			usedBefore := sub.AmountUsed
			if sub.AmountTotal > 0 {
				remain := sub.AmountTotal - usedBefore
				if remain < amount {
					continue
				}
			}
			record := &SubscriptionPreConsumeRecord{
				RequestId:          requestId,
				UserId:             userId,
				UserSubscriptionId: sub.Id,
				PreConsumed:        amount,
				Status:             "consumed",
			}
			if err := tx.Create(record).Error; err != nil {
				var dup SubscriptionPreConsumeRecord
				if err2 := tx.Where("request_id = ?", requestId).First(&dup).Error; err2 == nil {
					if dup.Status == "refunded" {
						return errors.New("subscription pre-consume already refunded")
					}
					returnValue.UserSubscriptionId = sub.Id
					returnValue.PreConsumed = dup.PreConsumed
					returnValue.AmountTotal = sub.AmountTotal
					returnValue.AmountUsedBefore = sub.AmountUsed
					returnValue.AmountUsedAfter = sub.AmountUsed
					return nil
				}
				return err
			}
			sub.AmountUsed += amount
			if err := tx.Save(&sub).Error; err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = amount
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = usedBefore
			returnValue.AmountUsedAfter = sub.AmountUsed
			return nil
		}
		return &types.QuotaRejection{Source: "subscription", Required: amount, Message: fmt.Sprintf("subscription quota insufficient, need=%d", amount)}
	})
	if err != nil {
		return nil, err
	}
	return returnValue, nil
}

// RefundSubscriptionPreConsume 是幂等操作，按 requestId 退回已预扣的订阅额度。
func RefundSubscriptionPreConsume(requestId string) error {
	if strings.TrimSpace(requestId) == "" {
		return errors.New("requestId is empty")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record SubscriptionPreConsumeRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("request_id = ?", requestId).First(&record).Error; err != nil {
			return err
		}
		if record.Status == "refunded" {
			return nil
		}
		if record.PreConsumed <= 0 {
			record.Status = "refunded"
			return tx.Save(&record).Error
		}
		if err := postConsumeUserSubscriptionDeltaTx(tx, record.UserSubscriptionId, -record.PreConsumed); err != nil {
			return err
		}
		record.Status = "refunded"
		return tx.Save(&record).Error
	})
}

// ResetDueSubscriptions 重置 next_reset_time 已到期的订阅。
func ResetDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("next_reset_time > 0 AND next_reset_time <= ? AND status = ?", now, "active").
		Order("next_reset_time asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	resetCount := 0
	for _, sub := range subs {
		subCopy := sub
		plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
		if err != nil || plan == nil {
			continue
		}
		err = DB.Transaction(func(tx *gorm.DB) error {
			var locked UserSubscription
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				Where("id = ? AND next_reset_time > 0 AND next_reset_time <= ?", subCopy.Id, now).
				First(&locked).Error; err != nil {
				return nil
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &locked, plan, now); err != nil {
				return err
			}
			resetCount++
			return nil
		})
		if err != nil {
			return resetCount, err
		}
	}
	return resetCount, nil
}

// CleanupSubscriptionPreConsumeRecords 清理旧幂等记录，控制表体积。
func CleanupSubscriptionPreConsumeRecords(olderThanSeconds int64) (int64, error) {
	if olderThanSeconds <= 0 {
		olderThanSeconds = 7 * 24 * 3600
	}
	cutoff := GetDBTimestamp() - olderThanSeconds
	res := DB.Where("updated_at < ?", cutoff).Delete(&SubscriptionPreConsumeRecord{})
	return res.RowsAffected, res.Error
}

type SubscriptionPlanInfo struct {
	PlanId    int
	PlanTitle string
}

func GetSubscriptionPlanInfoByUserSubscriptionId(userSubscriptionId int) (*SubscriptionPlanInfo, error) {
	if userSubscriptionId <= 0 {
		return nil, errors.New("invalid userSubscriptionId")
	}
	cacheKey := fmt.Sprintf("sub:%d", userSubscriptionId)
	if cached, found, err := getSubscriptionPlanInfoCache().Get(cacheKey); err == nil && found {
		return &cached, nil
	}
	var sub UserSubscription
	if err := DB.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
	if err != nil {
		return nil, err
	}
	info := &SubscriptionPlanInfo{
		PlanId:    sub.PlanId,
		PlanTitle: plan.Title,
	}
	_ = getSubscriptionPlanInfoCache().SetWithTTL(cacheKey, *info, subscriptionPlanInfoCacheTTL())
	return info, nil
}

// 按 delta 更新订阅已用额度（正数为继续消耗，负数为退款回滚）。
func PostConsumeUserSubscriptionDelta(userSubscriptionId int, delta int64) error {
	if userSubscriptionId <= 0 {
		return errors.New("invalid userSubscriptionId")
	}
	if delta == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return postConsumeUserSubscriptionDeltaTx(tx, userSubscriptionId, delta)
	})
}

// Reuse the caller's transaction so quota and the refund marker commit (or
// roll back) together, including on SQLite with a single database connection.
func postConsumeUserSubscriptionDeltaTx(tx *gorm.DB, userSubscriptionId int, delta int64) error {
	var sub UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", userSubscriptionId).
		First(&sub).Error; err != nil {
		return err
	}
	newUsed := max(sub.AmountUsed+delta, 0)
	if sub.AmountTotal > 0 && newUsed > sub.AmountTotal {
		return &types.QuotaRejection{Source: "subscription", ResourceID: sub.Id, Available: sub.AmountTotal - sub.AmountUsed, Required: delta, Known: true, Message: fmt.Sprintf("subscription used exceeds total, used=%d total=%d", newUsed, sub.AmountTotal)}
	}
	sub.AmountUsed = newUsed
	return tx.Save(&sub).Error
}

// bindSubscriptionWithOptionsTx 在指定事务内复用套餐绑定逻辑。
// 该方法供非支付场景使用，例如管理员直绑、兑换码兑换等。
func bindSubscriptionWithOptionsTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, mode string, purchaseQuantity int, renewTargetSubscriptionId int, source string) (string, error) {
	if tx == nil {
		return "", errors.New("tx is nil")
	}
	if userId <= 0 || plan == nil || plan.Id <= 0 {
		return "", errors.New("invalid userId or plan")
	}
	if purchaseQuantity <= 0 {
		purchaseQuantity = 1
	}
	if strings.TrimSpace(source) == "" {
		source = "order"
	}

	// renew_extend 模式下需要记住第一次创建/命中的订阅，后续份数都顺延到同一条订阅上。
	renewTargetForExtend := 0
	// 所有非支付发放入口都先锁用户，避免和支付回调、管理员操作并发改订阅。
	if err := lockUserForSubscriptionMutationTx(tx, userId); err != nil {
		return "", err
	}
	if mode == SubscriptionPurchaseModeRenewExtend {
		activeSubs, err := getActiveUserSubscriptionsByPlanTx(tx, userId, plan.Id, 0)
		if err != nil {
			return "", err
		}
		if len(activeSubs) > 0 {
			renewTargetForExtend = activeSubs[0].Id
		}
	}

	for i := 0; i < purchaseQuantity; i++ {
		switch mode {
		case SubscriptionPurchaseModeRenew:
			// renew 只允许续到现有生效订阅，不允许“没有目标时自动新建”。
			_, renewed, err := renewUserSubscriptionByPlanTx(tx, userId, plan, renewTargetSubscriptionId)
			if err != nil {
				return "", err
			}
			if !renewed {
				if renewTargetSubscriptionId > 0 {
					return "", errors.New("续费目标订阅不存在或已失效")
				}
				return "", errors.New("无可续费的同规格订阅")
			}
		case SubscriptionPurchaseModeRenewExtend:
			if renewTargetForExtend > 0 {
				// renew_extend 的核心语义是“始终顺到同一条订阅上”。
				_, renewed, renewErr := renewUserSubscriptionByPlanTx(tx, userId, plan, renewTargetForExtend)
				if renewErr != nil {
					return "", renewErr
				}
				if !renewed {
					return "", errors.New("续费式顺延失败")
				}
				continue
			}
			createdSub, createErr := createUserSubscriptionFromPlanTx(tx, userId, plan, source, true)
			if createErr != nil {
				return "", createErr
			}
			// 第一次没有可顺延订阅时，先创建一条，再把后续份数都续到这条上。
			renewTargetForExtend = createdSub.Id
		default:
			// stack 语义最直接：每份都新增一条订阅实例。
			if _, err := createUserSubscriptionFromPlanTx(tx, userId, plan, source, true); err != nil {
				return "", err
			}
		}
	}

	modeLabel := "叠加"
	if mode == SubscriptionPurchaseModeRenew {
		modeLabel = "续费"
	} else if mode == SubscriptionPurchaseModeRenewExtend {
		modeLabel = "续费式购买"
	}
	resultMessage := fmt.Sprintf("已按%s方式添加 %d 份套餐", modeLabel, purchaseQuantity)
	if strings.TrimSpace(plan.UpgradeGroup) != "" {
		_ = UpdateUserGroupCache(userId, plan.UpgradeGroup)
		return fmt.Sprintf("%s，用户分组将升级到 %s", resultMessage, plan.UpgradeGroup), nil
	}
	return resultMessage, nil
}
