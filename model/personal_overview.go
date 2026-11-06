package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type PersonalMonthUsage struct {
	Month      string                `json:"month"`
	Timezone   string                `json:"timezone"`
	AsOf       int64                 `json:"as_of"`
	ActiveDays int                   `json:"active_days"`
	Summary    DashboardUsageSummary `json:"summary"`
}

// A fixed current calendar month, not a client-selectable unbounded query.
// Reuse the dashboard accumulator to keep refunds, adjustments and token usage
// identical. Active days use actual event timestamps, not rounded hour buckets.
func GetPersonalMonthUsage(ctx context.Context, userID int, location *time.Location, now time.Time) (PersonalMonthUsage, error) {
	if userID <= 0 || location == nil {
		return PersonalMonthUsage{}, errors.New("invalid personal scope")
	}
	select {
	case dashboardUsageSlots <- struct{}{}:
		defer func() { <-dashboardUsageSlots }()
	default:
		return PersonalMonthUsage{}, errors.New("usage query busy")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location).Unix()
	end := now.Unix() + 1
	a := newDashboardUsageAccumulator(start, end, false)
	rows, err := LOG_DB.WithContext(ctx).Table("logs").Select("user_id, username, token_id, token_name, created_at, type, model_name, quota, prompt_tokens, completion_tokens, COALESCE(other, '') AS other").Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at < ?", userID, []int{LogTypeConsume, LogTypeRefund}, start, end).Limit(dashboardUsageRowLimit + 1).Rows()
	if err != nil {
		return PersonalMonthUsage{}, err
	}
	defer rows.Close()
	active := map[string]bool{}
	for n := 0; rows.Next(); n++ {
		if n >= dashboardUsageRowLimit {
			return PersonalMonthUsage{}, errors.New("usage record limit exceeded")
		}
		if err := ctx.Err(); err != nil {
			return PersonalMonthUsage{}, err
		}
		var row dashboardUsageLog
		if err := LOG_DB.ScanRows(rows, &row); err != nil {
			return PersonalMonthUsage{}, err
		}
		before := a.result.Summary.Requests
		if err := a.add(row); err != nil {
			return PersonalMonthUsage{}, err
		}
		if a.result.Summary.Requests > before {
			active[time.Unix(row.CreatedAt, 0).In(location).Format("2006-01-02")] = true
		}
	}
	if err := rows.Err(); err != nil {
		return PersonalMonthUsage{}, err
	}
	if err := ctx.Err(); err != nil {
		return PersonalMonthUsage{}, err
	}
	return PersonalMonthUsage{Month: local.Format("2006-01"), Timezone: location.String(), AsOf: now.Unix(), ActiveDays: len(active), Summary: a.finish().Summary}, nil
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
