package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const userActivityDays = 365
const userActivityRowLimit = 250000

var userActivitySlots = make(chan struct{}, 2)

type UserActivityDay struct {
	Date     string `json:"date"`
	Requests int64  `json:"requests"`
}
type UserActivity struct {
	Source        string            `json:"source"`
	Timezone      string            `json:"timezone"`
	AsOf          int64             `json:"as_of"`
	Days          []UserActivityDay `json:"days"`
	Total         int64             `json:"total"`
	LongestStreak int               `json:"longest_streak"`
	DailyAverage  float64           `json:"daily_average"`
	WeeklyAverage float64           `json:"weekly_average"`
}
type userActivityCached struct {
	value   UserActivity
	expires time.Time
}

var userActivityCache = struct {
	sync.Mutex
	entries map[string]userActivityCached
}{entries: make(map[string]userActivityCached)}

func GetUserActivity(ctx context.Context, userID int, zone string, now time.Time) (UserActivity, error) {
	if userID <= 0 {
		return UserActivity{}, errors.New("user identity required")
	}
	if len(zone) > 80 {
		return UserActivity{}, errors.New("invalid timezone")
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return UserActivity{}, errors.New("invalid timezone")
	}
	local := now.In(location)
	key := fmt.Sprintf("%d:%s:%s", userID, zone, local.Format("2006-01-02"))
	userActivityCache.Lock()
	cached, ok := userActivityCache.entries[key]
	userActivityCache.Unlock()
	if ok && now.Before(cached.expires) {
		return cloneUserActivity(cached.value), nil
	}
	result, err := scanUserActivity(ctx, userID, location, now, userActivityRowLimit)
	if err != nil {
		return UserActivity{}, err
	}
	userActivityCache.Lock()
	for k, v := range userActivityCache.entries {
		if !now.Before(v.expires) {
			delete(userActivityCache.entries, k)
		}
	}
	if len(userActivityCache.entries) >= 64 {
		var oldest string
		var expiry time.Time
		for k, v := range userActivityCache.entries {
			if oldest == "" || v.expires.Before(expiry) {
				oldest, expiry = k, v.expires
			}
		}
		delete(userActivityCache.entries, oldest)
	}
	userActivityCache.entries[key] = userActivityCached{cloneUserActivity(result), now.Add(5 * time.Minute)}
	userActivityCache.Unlock()
	return result, nil
}
func cloneUserActivity(value UserActivity) UserActivity {
	value.Days = append([]UserActivityDay(nil), value.Days...)
	return value
}
func scanUserActivity(ctx context.Context, userID int, location *time.Location, now time.Time, limit int) (UserActivity, error) {
	if userID <= 0 || location == nil || limit <= 0 {
		return UserActivity{}, errors.New("invalid activity scope")
	}
	select {
	case userActivitySlots <- struct{}{}:
		defer func() { <-userActivitySlots }()
	default:
		return UserActivity{}, errors.New("activity query busy")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	local := now.In(location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	start := today.AddDate(0, 0, -(userActivityDays - 1))
	result := UserActivity{Source: "retained_settlement_logs_v1", Timezone: location.String(), AsOf: now.Unix(), Days: make([]UserActivityDay, userActivityDays)}
	indices := make(map[string]int, userActivityDays)
	for i := range result.Days {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		result.Days[i].Date = date
		indices[date] = i
	}
	rows, err := LOG_DB.WithContext(ctx).Table("logs").Select("created_at, COALESCE(other, '') AS other").Where("user_id = ? AND type = ? AND created_at >= ? AND created_at <= ?", userID, LogTypeConsume, start.Unix(), now.Unix()).Limit(limit + 1).Rows()
	if err != nil {
		return UserActivity{}, err
	}
	defer rows.Close()
	for n := 0; rows.Next(); n++ {
		if n >= limit {
			return UserActivity{}, errors.New("activity records exceed scan limit")
		}
		if err := ctx.Err(); err != nil {
			return UserActivity{}, err
		}
		var row struct {
			CreatedAt int64
			Other     string
		}
		if err := LOG_DB.ScanRows(rows, &row); err != nil {
			return UserActivity{}, err
		}
		var metadata struct {
			TaskID string `json:"task_id"`
		}
		if len(row.Other) > 1024*1024 {
			return UserActivity{}, errors.New("activity metadata exceeds size limit")
		}
		if row.Other != "" {
			if err := common.UnmarshalJsonStr(row.Other, &metadata); err != nil {
				return UserActivity{}, err
			}
		}
		if metadata.TaskID != "" {
			continue
		}
		date := time.Unix(row.CreatedAt, 0).In(location).Format("2006-01-02")
		if index, ok := indices[date]; ok {
			result.Days[index].Requests++
			result.Total++
		}
	}
	if err := rows.Err(); err != nil {
		return UserActivity{}, err
	}
	if err := ctx.Err(); err != nil {
		return UserActivity{}, err
	}
	streak := 0
	for _, day := range result.Days {
		if day.Requests > 0 {
			streak++
			if streak > result.LongestStreak {
				result.LongestStreak = streak
			}
		} else {
			streak = 0
		}
	}
	result.DailyAverage = float64(result.Total) / userActivityDays
	result.WeeklyAverage = result.DailyAverage * 7
	return result, nil
}
