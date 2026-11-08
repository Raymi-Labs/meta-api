package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
)

type PersonalMonthUsage struct {
	Month      string                `json:"month"`
	Timezone   string                `json:"timezone"`
	AsOf       int64                 `json:"as_of"`
	ActiveDays int                   `json:"active_days"`
	Summary    DashboardUsageSummary `json:"summary"`
}

var errUsageQueryBusy = errors.New("usage query busy")

// usageSlotWait lets a personal query queue briefly behind another one
// instead of failing at once; a full pool for longer is reported as busy.
var usageSlotWait = 3 * time.Second

func acquireUsageSlot(ctx context.Context, slots chan struct{}) (func(), error) {
	timer := time.NewTimer(usageSlotWait)
	defer timer.Stop()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-timer.C:
		return nil, errUsageQueryBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Separate from dashboardUsageSlots: an administrator's site-wide dashboard
// scan must not turn every personal overview into a busy error.
var personalUsageSlots = make(chan struct{}, 4)

// A local day is cached only once it ended this long ago, so a log stamped
// just before midnight but written just after it is still counted.
const personalMonthSettleSeconds = 10 * 60
const personalMonthCacheTTL = 32 * 24 * time.Hour

// personalMonthPrefix holds the counters of the month's completed days up to
// and including Through. Past days never change (refunds count on the day
// they happen), so each day is scanned once instead of on every page view.
type personalMonthPrefix struct {
	Through    string                `json:"through"`
	ActiveDays int                   `json:"active_days"`
	Summary    DashboardUsageSummary `json:"summary"`
}

var personalMonthCacheOnce sync.Once
var personalMonthCache *cachex.HybridCache[personalMonthPrefix]

func getPersonalMonthCache() *cachex.HybridCache[personalMonthPrefix] {
	personalMonthCacheOnce.Do(func() {
		personalMonthCache = cachex.NewHybridCache[personalMonthPrefix](cachex.HybridCacheConfig[personalMonthPrefix]{
			Namespace: cachex.Namespace("personal_month_prefix:v1"),
			Redis:     common.RDB, RedisEnabled: func() bool { return common.RedisEnabled && common.RDB != nil },
			RedisCodec: cachex.JSONCodec[personalMonthPrefix]{},
			Memory: func() *hot.HotCache[string, personalMonthPrefix] {
				return hot.NewHotCache[string, personalMonthPrefix](hot.LRU, 1024).WithTTL(personalMonthCacheTTL).WithJanitor().Build()
			},
		})
	})
	return personalMonthCache
}

// A fixed current calendar month, not a client-selectable unbounded query.
// Reuse the dashboard accumulator to keep refunds, adjustments and token usage
// identical. Each local day is scanned on its own, so active days use actual
// event timestamps; completed days are cached and only today is read live.
func GetPersonalMonthUsage(ctx context.Context, userID int, location *time.Location, now time.Time) (PersonalMonthUsage, error) {
	if userID <= 0 || location == nil {
		return PersonalMonthUsage{}, errors.New("invalid personal scope")
	}
	release, err := acquireUsageSlot(ctx, personalUsageSlots)
	if err != nil {
		return PersonalMonthUsage{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	local := now.In(location)
	monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
	month := local.Format("2006-01")
	// No account name in the key; the zone decides the day boundaries.
	key := fmt.Sprintf("%d:%s:%s", userID, location.String(), month)
	cache := getPersonalMonthCache()
	prefix := personalMonthPrefix{}
	day := monthStart
	if cached, found, err := cache.Get(key); err == nil && found && cached.Through != "" {
		if through, err := time.ParseInLocation("2006-01-02", cached.Through, location); err == nil {
			next := through.AddDate(0, 0, 1)
			if !through.Before(monthStart) && next.Unix()+personalMonthSettleSeconds <= now.Unix() {
				prefix, day = cached, next
			}
		}
	}
	// Extend the cached prefix one completed day at a time, saving after each,
	// so a heavy account's first view converges even if one request times out.
	for {
		next := day.AddDate(0, 0, 1)
		if next.Unix()+personalMonthSettleSeconds > now.Unix() {
			break
		}
		summary, err := scanPersonalUsage(ctx, userID, day.Unix(), next.Unix())
		if err != nil {
			return PersonalMonthUsage{}, err
		}
		if err := addDashboardUsageCounters(&prefix.Summary, summary); err != nil {
			return PersonalMonthUsage{}, err
		}
		if summary.Requests > 0 {
			prefix.ActiveDays++
		}
		prefix.Through = day.Format("2006-01-02")
		_ = cache.SetWithTTL(key, prefix, personalMonthCacheTTL)
		day = next
	}
	// Today, and yesterday during the settle window, are always read live.
	total, activeDays := prefix.Summary, prefix.ActiveDays
	end := now.Unix() + 1
	for day.Unix() < end {
		next := day.AddDate(0, 0, 1)
		summary, err := scanPersonalUsage(ctx, userID, day.Unix(), min(next.Unix(), end))
		if err != nil {
			return PersonalMonthUsage{}, err
		}
		if err := addDashboardUsageCounters(&total, summary); err != nil {
			return PersonalMonthUsage{}, err
		}
		if summary.Requests > 0 {
			activeDays++
		}
		day = next
	}
	total.StartTimestamp, total.EndTimestamp, total.Source = monthStart.Unix(), end, "retained_settlement_logs_v1"
	finishDashboardUsageSummary(&total)
	return PersonalMonthUsage{Month: month, Timezone: location.String(), AsOf: now.Unix(), ActiveDays: activeDays, Summary: total}, nil
}

// scanPersonalUsage returns the additive counters of one user's consume and
// refund logs in [start, end). A request is a consume without task_id, so
// Requests > 0 exactly when the window had activity.
func scanPersonalUsage(ctx context.Context, userID int, start, end int64) (DashboardUsageSummary, error) {
	a := newDashboardUsageAccumulator(start, end, false)
	rows, err := LOG_DB.WithContext(ctx).Table("logs").Select("user_id, username, token_id, token_name, created_at, type, model_name, quota, prompt_tokens, completion_tokens, COALESCE(other, '') AS other").Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at < ?", userID, []int{LogTypeConsume, LogTypeRefund}, start, end).Limit(dashboardUsageRowLimit + 1).Rows()
	if err != nil {
		return DashboardUsageSummary{}, err
	}
	defer rows.Close()
	for n := 0; rows.Next(); n++ {
		if n >= dashboardUsageRowLimit {
			return DashboardUsageSummary{}, errors.New("usage record limit exceeded")
		}
		if err := ctx.Err(); err != nil {
			return DashboardUsageSummary{}, err
		}
		var row dashboardUsageLog
		if err := LOG_DB.ScanRows(rows, &row); err != nil {
			return DashboardUsageSummary{}, err
		}
		if err := a.add(row); err != nil {
			return DashboardUsageSummary{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return DashboardUsageSummary{}, err
	}
	if err := ctx.Err(); err != nil {
		return DashboardUsageSummary{}, err
	}
	return a.result.Summary, nil
}

type PersonalTokenItem struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	MaskedKey   string `json:"masked_key"`
	Status      string `json:"status"`
	CreatedTime int64  `json:"created_time"`
}
type PersonalTokenSummary struct {
	Total   int64               `json:"total"`
	Enabled int64               `json:"enabled"`
	Other   int64               `json:"other"`
	Recent  []PersonalTokenItem `json:"recent"`
	AsOf    int64               `json:"as_of"`
}

func GetPersonalTokenSummary(ctx context.Context, userID int, now int64) (PersonalTokenSummary, error) {
	if userID <= 0 {
		return PersonalTokenSummary{}, errors.New("user identity required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	base := DB.WithContext(ctx).Model(&Token{}).Where("user_id = ? AND (source_type IS NULL OR source_type <> ?)", userID, TokenSourceTypeImagePlayground)
	var aggregate struct {
		Total   int64
		Enabled int64
	}
	if err := base.Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN status = ? AND (expired_time = -1 OR expired_time >= ?) THEN 1 ELSE 0 END), 0) AS enabled", common.TokenStatusEnabled, now).Scan(&aggregate).Error; err != nil {
		return PersonalTokenSummary{}, err
	}
	// Explicit projection; these private server rows are never serialized.
	var rows []struct {
		Id          int
		Name        string
		Key         string
		Status      int
		CreatedTime int64
		ExpiredTime int64
	}
	if err := DB.WithContext(ctx).Model(&Token{}).Where("user_id = ? AND (source_type IS NULL OR source_type <> ?)", userID, TokenSourceTypeImagePlayground).Select("id, name, " + commonKeyCol + ", status, created_time, expired_time").Order("created_time DESC, id DESC").Limit(5).Scan(&rows).Error; err != nil {
		return PersonalTokenSummary{}, err
	}
	result := PersonalTokenSummary{Total: aggregate.Total, Enabled: aggregate.Enabled, Other: aggregate.Total - aggregate.Enabled, AsOf: now, Recent: make([]PersonalTokenItem, 0, len(rows))}
	for _, row := range rows {
		status := "disabled"
		switch {
		case row.Status == common.TokenStatusExpired || row.ExpiredTime != -1 && row.ExpiredTime < now:
			status = "expired"
		case row.Status == common.TokenStatusEnabled:
			status = "enabled"
		case row.Status == common.TokenStatusExhausted:
			status = "exhausted"
		}
		masked := "••••"
		if len(row.Key) > 8 {
			masked = fmt.Sprintf("••••%s", row.Key[len(row.Key)-4:])
		}
		result.Recent = append(result.Recent, PersonalTokenItem{ID: row.Id, Name: strings.TrimSpace(row.Name), MaskedKey: masked, Status: status, CreatedTime: row.CreatedTime})
	}
	return result, nil
}
