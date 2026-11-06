package model

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestUserActivityScopeDatesAndCounts(t *testing.T) {
	setupCacheRateScanDB(t)
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	now := time.Date(2024, 3, 12, 12, 0, 0, 0, location)
	for _, row := range []Log{
		{UserId: 1, CreatedAt: now.AddDate(0, 0, -2).Unix(), Type: LogTypeConsume},
		{UserId: 1, CreatedAt: now.AddDate(0, 0, -1).Unix(), Type: LogTypeConsume},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeConsume},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeConsume, Other: `{"task_id":"adjustment"}`},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeRefund},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeError},
		{UserId: 2, CreatedAt: now.Unix(), Type: LogTypeConsume},
		{UserId: 1, CreatedAt: now.AddDate(0, 0, -365).Unix(), Type: LogTypeConsume},
		{UserId: 1, CreatedAt: now.Add(time.Second).Unix(), Type: LogTypeConsume},
	} {
		require.NoError(t, LOG_DB.Create(&row).Error)
	}
	data, err := scanUserActivity(context.Background(), 1, location, now, 100)
	require.NoError(t, err)
	require.Len(t, data.Days, 365)
	require.Equal(t, "2023-03-14", data.Days[0].Date)
	require.Equal(t, "2024-03-12", data.Days[364].Date)
	require.EqualValues(t, 3, data.Total)
	require.Equal(t, 3, data.LongestStreak)
	require.InDelta(t, 3.0/365, data.DailyAverage, 1e-10)
	require.InDelta(t, 21.0/365, data.WeeklyAverage, 1e-10)
	require.EqualValues(t, 1, data.Days[362].Requests)
	empty, err := scanUserActivity(context.Background(), 3, location, now, 100)
	require.NoError(t, err)
	require.Zero(t, empty.Total)
	_, err = scanUserActivity(context.Background(), 1, location, now, 1)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = scanUserActivity(ctx, 1, location, now, 100)
	require.Error(t, err)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Type: LogTypeConsume, CreatedAt: now.Unix(), Other: `bad-json`}).Error)
	_, err = scanUserActivity(context.Background(), 1, location, now, 100)
	require.Error(t, err)
}
func TestUserActivityCacheIsolation(t *testing.T) {
	setupCacheRateScanDB(t)
	userActivityCache.Lock()
	userActivityCache.entries = make(map[string]userActivityCached)
	userActivityCache.Unlock()
	now := time.Date(2026, 11, 6, 1, 0, 0, 0, time.UTC)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Type: LogTypeConsume, CreatedAt: now.Unix()}).Error)
	a, err := GetUserActivity(context.Background(), 1, "UTC", now)
	require.NoError(t, err)
	require.EqualValues(t, 1, a.Total)
	a.Days[364].Requests = 99
	b, err := GetUserActivity(context.Background(), 1, "UTC", now)
	require.NoError(t, err)
	require.EqualValues(t, 1, b.Days[364].Requests)
	b, err = GetUserActivity(context.Background(), 2, "UTC", now)
	require.NoError(t, err)
	require.Zero(t, b.Total)
	b, err = GetUserActivity(context.Background(), 1, "America/New_York", now)
	require.NoError(t, err)
	require.Equal(t, "2026-11-05", b.Days[364].Date)
	_, err = GetUserActivity(context.Background(), 0, "UTC", now)
	require.Error(t, err)
	_, err = GetUserActivity(context.Background(), 1, "invalid", now)
	require.Error(t, err)
}
