package model

import (
	"context"
	"time"
)

func PulseYesterdayUsage(ctx context.Context, userID int, now time.Time) (string, *int64, error) {
	loc := time.FixedZone("UTC+8", 8*3600)
	day := now.In(loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	start := end.AddDate(0, 0, -1)
	var sum *int64
	err := LOG_DB.WithContext(ctx).Model(&Log{}).Select("SUM(quota)").Where("user_id = ? AND type = ? AND created_at >= ? AND created_at < ?", userID, LogTypeConsume, start.Unix(), end.Unix()).Row().Scan(&sum)
	return start.Format("2006-01-02"), sum, err
}
