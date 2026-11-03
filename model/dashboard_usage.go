package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
)

// These limits fail closed: never turn an incomplete scan into a successful total.
const dashboardUsageRowLimit = 1000000
const dashboardUsageBucketLimit = 25000

var dashboardUsageSlots = make(chan struct{}, 2)

var dashboardUsageCacheOnce sync.Once
var dashboardUsageCache *cachex.HybridCache[DashboardUsage]

func GetCachedDashboardUsage(ctx context.Context, start, end int64, userID int, username string, byUser bool) (DashboardUsage, error) {
	dashboardUsageCacheOnce.Do(func() {
		dashboardUsageCache = cachex.NewHybridCache[DashboardUsage](cachex.HybridCacheConfig[DashboardUsage]{
			Namespace: cachex.Namespace("dashboard_settlement:v2"),
			Redis:     common.RDB, RedisEnabled: func() bool { return common.RedisEnabled && common.RDB != nil },
			RedisCodec: cachex.JSONCodec[DashboardUsage]{},
			Memory: func() *hot.HotCache[string, DashboardUsage] {
				return hot.NewHotCache[string, DashboardUsage](hot.LRU, 16).WithTTL(15 * time.Second).WithJanitor().Build()
			},
		})
	})
	// Exact seconds and authenticated scope are part of the key. Never reuse
	// the old hour-rounded key or expose account names in Redis keys.
	key := fmt.Sprintf("%d:%d:%d:%t:%x", start, end, userID, byUser, sha256.Sum256([]byte(username)))
	if result, found, err := dashboardUsageCache.Get(key); err == nil && found {
		return result, nil
	}
	result, err := GetDashboardUsage(ctx, start, end, userID, username, byUser)
	if err == nil {
		_ = dashboardUsageCache.SetWithTTL(key, result, 15*time.Second)
	}
	return result, err
}

type DashboardUsageSummary struct {
	Quota                int64    `json:"quota"`
	Requests             int64    `json:"requests"`
	Tokens               int64    `json:"tokens"`
	TokenQuota           int64    `json:"token_quota"`
	PricedTokens         int64    `json:"priced_tokens"`
	OtherQuota           int64    `json:"other_quota"`
	CacheReadTokens      int64    `json:"cache_read_tokens"`
	InputTokens          int64    `json:"input_tokens"`
	UnclassifiedRecords  int64    `json:"unclassified_records"`
	CacheIncludedRecords int64    `json:"cache_included_records"`
	CacheExcludedRecords int64    `json:"cache_excluded_records"`
	PriceIncludedRecords int64    `json:"price_included_records"`
	PriceExcludedRecords int64    `json:"price_excluded_records"`
	CacheRate            *float64 `json:"cache_rate"`
	UnitPrice            *float64 `json:"unit_price"`
	StartTimestamp       int64    `json:"start_timestamp"`
	EndTimestamp         int64    `json:"end_timestamp"`
	Source               string   `json:"source"`
}

type DashboardUsage struct {
	Rows    []*QuotaData          `json:"rows"`
	Summary DashboardUsageSummary `json:"summary"`
}

type dashboardUsageLog struct {
	UserId, Type, PromptTokens, CompletionTokens, Quota int
	CreatedAt                                           int64
	Username, ModelName, Other                          string
}

type dashboardUsageOther struct {
	tokenUsageOtherInfo
	BillingUnit string   `json:"billing_unit"`
	ModelPrice  *float64 `json:"model_price"`
	ModelRatio  *float64 `json:"model_ratio"`
	RequestPath string   `json:"request_path"`
	TaskID      string   `json:"task_id"`
	TaskBilling any      `json:"task_billing"`
}

// Never classify by model name or today's price. Old tasks may have a zero model
// price; their event metadata/path is stronger evidence than that default.
func dashboardTokenPriced(o dashboardUsageOther) bool {
	if o.TaskID != "" || o.TaskBilling != nil {
		return false
	}
	for _, path := range []string{"/images", "/videos", "/video", "/audio", "/mj/", "/suno/", "/rerank"} {
		if strings.Contains(o.RequestPath, path) {
			return false
		}
	}
	if o.BillingUnit != "" {
		return o.BillingUnit == "token"
	}
	if o.ModelPrice != nil && *o.ModelPrice > 0 {
		return false
	}
	return o.ModelRatio != nil || o.BillingMode == "tiered_expr"
}

type dashboardUsageKey struct {
	UserID int
	Model  string
	Hour   int64
}

type dashboardUsageAccumulator struct {
	result  DashboardUsage
	buckets map[dashboardUsageKey]*QuotaData
	byUser  bool
}

func newDashboardUsageAccumulator(start, end int64, byUser bool) *dashboardUsageAccumulator {
	return &dashboardUsageAccumulator{
		result: DashboardUsage{Rows: []*QuotaData{}, Summary: DashboardUsageSummary{
			StartTimestamp: start, EndTimestamp: end, Source: "retained_settlement_logs_v1",
		}},
		buckets: make(map[dashboardUsageKey]*QuotaData), byUser: byUser,
	}
}

func (a *dashboardUsageAccumulator) add(row dashboardUsageLog) error {
	if row.Quota < 0 || row.PromptTokens < 0 || row.CompletionTokens < 0 {
		return errors.New("统计日志含无效的金额或用量")
	}
	var other dashboardUsageOther
	if row.Other != "" {
		if err := common.UnmarshalJsonStr(row.Other, &other); err != nil {
			return errors.New("统计日志用量明细无法解析")
		}
	}
	// Bound individual counters before addition (including legacy JSON fields),
	// then bound aggregate counters to exact JavaScript integer precision.
	for _, value := range []int{row.Quota, row.PromptTokens, row.CompletionTokens, other.CacheTokens, other.CacheCreationTokens, other.CacheCreationTokens5m, other.CacheCreationTokens1h} {
		if value < 0 || int64(value) > 1<<50 {
			return errors.New("统计日志数值超出安全范围")
		}
	}
	quota := int64(row.Quota)
	count := 0
	input, tokens, read := 0, 0, 0
	unknown := false
	if row.Type == LogTypeRefund {
		quota = -quota
	} else if row.Type == LogTypeConsume {
		// Task adjustment records represent settlement deltas, not new requests.
		if other.TaskID == "" {
			count = 1
			read = other.CacheTokens
			write := sumCacheCreationTokens(other.tokenUsageOtherInfo)
			if read < 0 || write < 0 || other.CacheCreationTokens < 0 || other.CacheCreationTokens5m < 0 || other.CacheCreationTokens1h < 0 {
				return errors.New("统计日志含无效的缓存用量")
			}
			if other.CacheCreationTokens > 0 && other.CacheCreationTokens5m+other.CacheCreationTokens1h > other.CacheCreationTokens {
				return errors.New("统计日志缓存分项超过总量")
			}
			input = getDisplayedInputTokens(row.PromptTokens, read, write, other.tokenUsageOtherInfo)
			// Missing semantics with caches cannot prove inclusivity. Keep the
			// retained log's display fallback but suppress the blended price.
			unknown = (read+write > 0 && other.PromptTokensSemantic == "" && !other.Claude && other.UsageSemantic == "") ||
				(other.PromptTokensSemantic != "" && other.PromptTokensSemantic != "inclusive" && other.PromptTokensSemantic != "non_cache") || read+write > input
			tokens = input + row.CompletionTokens
		}
	} else {
		return errors.New("不支持的统计日志类型")
	}
	s := &a.result.Summary
	s.Quota += quota
	s.Requests += int64(count)
	s.Tokens += int64(tokens)
	priced := dashboardTokenPriced(other)
	if unknown || (priced && tokens == 0 && row.Type == LogTypeConsume && row.Quota > 0) {
		s.UnclassifiedRecords++
	}
	priceValid := priced && !unknown && (tokens > 0 || row.Type == LogTypeRefund || row.Quota == 0)
	if priced && count > 0 {
		if !unknown && input > 0 {
			s.CacheIncludedRecords++
			s.InputTokens += int64(input)
			s.CacheReadTokens += int64(read)
		} else {
			s.CacheExcludedRecords++
		}
	}
	if priceValid {
		s.PriceIncludedRecords++
		s.TokenQuota += quota
		s.PricedTokens += int64(tokens)
	} else {
		if priced {
			s.PriceExcludedRecords++
		}
		s.OtherQuota += quota
	}
	for _, value := range []int64{s.Quota, s.TokenQuota, s.OtherQuota, s.Tokens, s.PricedTokens, s.InputTokens, s.CacheReadTokens} {
		if value > 9007199254740991 || value < -9007199254740991 {
			return errors.New("统计总量超出安全范围，请缩短时间范围")
		}
	}
	key := dashboardUsageKey{Model: row.ModelName, Hour: row.CreatedAt - row.CreatedAt%3600}
	if a.byUser {
		key.UserID = row.UserId
		key.Model = ""
	}
	bucket := a.buckets[key]
	if bucket == nil {
		if len(a.buckets) >= dashboardUsageBucketLimit {
			return errors.New("统计分组过多，请缩短时间范围或指定用户")
		}
		bucket = &QuotaData{ModelName: key.Model, CreatedAt: key.Hour}
		if a.byUser {
			bucket.UserID, bucket.Username = row.UserId, row.Username
		}
		a.buckets[key] = bucket
	}
	bucket.Quota += int(quota)
	bucket.Count += count
	bucket.TokenUsed += tokens
	return nil
}

func (a *dashboardUsageAccumulator) finish() DashboardUsage {
	s := &a.result.Summary
	if s.InputTokens > 0 {
		rate := float64(s.CacheReadTokens) / float64(s.InputTokens)
		s.CacheRate = &rate
	}
	if s.PricedTokens > 0 {
		price := float64(s.TokenQuota) / float64(s.PricedTokens) * 1000000
		s.UnitPrice = &price
	}
	for _, bucket := range a.buckets {
		a.result.Rows = append(a.result.Rows, bucket)
	}
	sort.Slice(a.result.Rows, func(i, j int) bool {
		x, y := a.result.Rows[i], a.result.Rows[j]
		if x.CreatedAt != y.CreatedAt {
			return x.CreatedAt < y.CreatedAt
		}
		if x.UserID != y.UserID {
			return x.UserID < y.UserID
		}
		return x.ModelName < y.ModelName
	})
	return a.result
}

// GetDashboardUsage reads one database result set: filters precede bucketing,
// all metrics share the same retained events, and refunds use event time. No
// pricing lookup, balance mutation, replay, or asynchronous hourly flush occurs.
func GetDashboardUsage(ctx context.Context, start, end int64, userID int, username string, byUser bool) (DashboardUsage, error) {
	return scanDashboardUsage(ctx, start, end, userID, username, byUser, dashboardUsageRowLimit)
}

func scanDashboardUsage(ctx context.Context, start, end int64, userID int, username string, byUser bool, limit int) (DashboardUsage, error) {
	if start <= 0 || end <= start || end-start > 31*86400 || userID < 0 || (userID > 0 && (username != "" || byUser)) {
		return DashboardUsage{}, errors.New("请选择有效且不超过31天的时间范围")
	}
	select {
	case dashboardUsageSlots <- struct{}{}:
		defer func() { <-dashboardUsageSlots }()
	default:
		return DashboardUsage{}, errors.New("统计查询繁忙，请稍后重试")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	query := LOG_DB.WithContext(ctx).Table("logs").Select("user_id, username, created_at, type, model_name, quota, prompt_tokens, completion_tokens, COALESCE(other, '') AS other").
		Where("type IN ? AND created_at >= ? AND created_at < ?", []int{LogTypeConsume, LogTypeRefund}, start, end)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if username != "" {
		query = query.Where("username = ?", username)
	}
	rows, err := query.Limit(limit + 1).Rows()
	if err != nil {
		return DashboardUsage{}, err
	}
	defer rows.Close()
	a := newDashboardUsageAccumulator(start, end, byUser)
	for n := 0; rows.Next(); n++ {
		if n >= limit {
			return DashboardUsage{}, errors.New("统计记录过多，请缩短时间范围或指定用户")
		}
		if err := ctx.Err(); err != nil {
			return DashboardUsage{}, err
		}
		var row dashboardUsageLog
		if err := LOG_DB.ScanRows(rows, &row); err != nil {
			return DashboardUsage{}, err
		}
		if err := a.add(row); err != nil {
			return DashboardUsage{}, fmt.Errorf("统计失败：%w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return DashboardUsage{}, err
	}
	if err := ctx.Err(); err != nil {
		return DashboardUsage{}, err
	}
	return a.finish(), nil
}
