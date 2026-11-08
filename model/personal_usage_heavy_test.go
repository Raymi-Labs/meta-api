package model

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setActivityDataExport(t *testing.T, enabled bool, interval int) {
	t.Helper()
	previousEnabled, previousInterval := common.DataExportEnabled, common.DataExportInterval
	common.DataExportEnabled, common.DataExportInterval = enabled, interval
	t.Cleanup(func() { common.DataExportEnabled, common.DataExportInterval = previousEnabled, previousInterval })
}

func resetPersonalMonthCache(t *testing.T) {
	t.Helper()
	require.NoError(t, getPersonalMonthCache().Purge())
	t.Cleanup(func() { _ = getPersonalMonthCache().Purge() })
}

func TestUserActivityHourlySplit(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	require.NoError(t, err)
	day := func(now time.Time) int64 {
		local := now.In(now.Location())
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, now.Location()).Unix()
	}
	noon := time.Date(2026, 11, 8, 12, 0, 0, 0, shanghai)
	require.Equal(t, day(noon), userActivityHourlySplit(day(noon), noon.Unix(), 5), "midday: today from logs, history from hours")
	early := time.Date(2026, 11, 8, 0, 3, 0, 0, shanghai)
	require.Equal(t, time.Date(2026, 11, 7, 23, 0, 0, 0, shanghai).Unix(), userActivityHourlySplit(day(early), early.Unix(), 5),
		"just after midnight the last hour of yesterday may be unflushed")
	require.Equal(t, day(early), userActivityHourlySplit(day(early), early.Unix(), 0), "an interval below one minute counts as one: 23:00 is flushed by 00:02")
	late := time.Date(2026, 11, 8, 1, 0, 0, 0, shanghai)
	require.Equal(t, time.Date(2026, 11, 7, 22, 0, 0, 0, shanghai).Unix(), userActivityHourlySplit(day(late), late.Unix(), 120), "a long flush interval widens the raw tail")
	india := time.Date(2026, 11, 8, 12, 0, 0, 0, kolkata)
	require.Equal(t, day(india)-1800, userActivityHourlySplit(day(india), india.Unix(), 5), "the hour containing a half-hour midnight is read from logs")
}

func TestUserActivityHourlyHistoryWithLiveTodayAcrossDatabases(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(":memory:")
			if dialect == "mysql" {
				if os.Getenv("TEST_MYSQL_DSN") == "" {
					t.Skip("no disposable MySQL")
				}
				driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
			}
			if dialect == "postgres" {
				if os.Getenv("TEST_POSTGRES_DSN") == "" {
					t.Skip("no disposable PostgreSQL")
				}
				driver = postgres.Open(os.Getenv("TEST_POSTGRES_DSN"))
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			previousDB, previousLogDB := DB, LOG_DB
			DB, LOG_DB = db, db
			t.Cleanup(func() { DB, LOG_DB = previousDB, previousLogDB })
			require.NoError(t, db.Migrator().DropTable(&Log{}, &QuotaData{}))
			require.NoError(t, db.AutoMigrate(&Log{}, &QuotaData{}))
			setActivityDataExport(t, true, 5)

			location, err := time.LoadLocation("Asia/Shanghai")
			require.NoError(t, err)
			at := func(month time.Month, day, hour, minute int) int64 {
				return time.Date(2026, month, day, hour, minute, 0, 0, location).Unix()
			}
			now := time.Date(2026, 11, 8, 0, 3, 0, 0, location)
			for _, row := range []QuotaData{
				{UserID: 1, Username: "u", ModelName: "a", CreatedAt: at(11, 5, 10, 0), Count: 3},
				{UserID: 1, Username: "u", ModelName: "b", CreatedAt: at(11, 5, 10, 0), Count: 2},
				{UserID: 1, Username: "renamed", ModelName: "a", CreatedAt: at(11, 6, 15, 0), Count: 1},
				{UserID: 1, Username: "u", ModelName: "a", CreatedAt: at(11, 7, 23, 0), Count: 7},                                       // unflushed hour: raw logs instead
				{UserID: 1, Username: "u", ModelName: "a", CreatedAt: at(11, 8, 0, 0), Count: 9},                                        // today: raw logs instead
				{UserID: 1, Username: "u", ModelName: "a", CreatedAt: time.Date(2025, 11, 8, 23, 0, 0, 0, location).Unix(), Count: 100}, // before the window
				{UserID: 2, Username: "v", ModelName: "a", CreatedAt: at(11, 5, 10, 0), Count: 50},
			} {
				require.NoError(t, db.Create(&row).Error)
			}
			for _, row := range []Log{
				{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 5, 10, 30)}, // history comes from hours, never twice
				{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 5, 10, 31)},
				{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 5, 10, 32)},
				{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 7, 23, 30)},
				{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 8, 0, 1)},
				{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 8, 0, 2), Other: `{"task_id":"adjustment"}`},
				{UserId: 1, Type: LogTypeRefund, CreatedAt: at(11, 8, 0, 2)},
			} {
				require.NoError(t, db.Create(&row).Error)
			}

			data, err := scanUserActivity(context.Background(), 1, location, now, 3)
			require.NoError(t, err, "the raw scan limit only covers today and the unflushed tail")
			require.Equal(t, userActivitySourceHourly, data.Source)
			byDate := map[string]int64{}
			for _, day := range data.Days {
				byDate[day.Date] = day.Requests
			}
			require.EqualValues(t, 5, byDate["2026-11-05"], "models and hour rows are summed")
			require.EqualValues(t, 1, byDate["2026-11-06"], "a renamed account still counts by user id")
			require.EqualValues(t, 1, byDate["2026-11-07"], "the unflushed hour is read from logs")
			require.EqualValues(t, 1, byDate["2026-11-08"], "today is read from logs without adjustments or refunds")
			require.EqualValues(t, 8, data.Total)
			require.Equal(t, 4, data.LongestStreak)
			require.Equal(t, "2025-11-09", data.Days[0].Date)

			setActivityDataExport(t, false, 5)
			raw, err := scanUserActivity(context.Background(), 1, location, now, 10)
			require.NoError(t, err)
			require.Equal(t, userActivitySourceLogs, raw.Source, "without hourly collection the calendar keeps the raw scan")
			require.EqualValues(t, 5, raw.Total)
			_, err = scanUserActivity(context.Background(), 1, location, now, 3)
			require.Error(t, err, "the raw-only scan still fails closed past its limit")
		})
	}
}

func TestPersonalMonthUsageCachesCompletedDays(t *testing.T) {
	setupCacheRateScanDB(t)
	resetPersonalMonthCache(t)
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at := func(month time.Month, day, hour, minute, second int) int64 {
		return time.Date(2026, month, day, hour, minute, second, 0, location).Unix()
	}
	rows := []Log{
		{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 1, 0, 0, 0), Quota: 100, PromptTokens: 10, CompletionTokens: 2, Other: `{"model_ratio":1,"prompt_tokens_semantic":"non_cache","cache_tokens":20}`},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 3, 9, 0, 0), Quota: 50, PromptTokens: 5, Other: `{"model_ratio":1}`},
		{UserId: 1, Type: LogTypeRefund, CreatedAt: at(11, 3, 10, 0, 0), Quota: 20},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 4, 12, 0, 0), Quota: 30, Other: `{"task_id":"adjustment"}`},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 7, 23, 59, 50), Quota: 7},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: at(11, 8, 8, 0, 0), Quota: 5},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: at(10, 31, 23, 59, 59), Quota: 999},
		{UserId: 2, Type: LogTypeConsume, CreatedAt: at(11, 3, 9, 0, 0), Quota: 999},
	}
	for _, row := range rows {
		require.NoError(t, LOG_DB.Create(&row).Error)
	}
	insert := func(createdAt int64, quota int) {
		require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Type: LogTypeConsume, CreatedAt: createdAt, Quota: quota}).Error)
	}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, location)
	first, err := GetPersonalMonthUsage(context.Background(), 1, location, now)
	require.NoError(t, err)
	accumulator := newDashboardUsageAccumulator(at(11, 1, 0, 0, 0), now.Unix()+1, false)
	for _, row := range rows[:6] {
		require.NoError(t, accumulator.add(dashboardUsageLog{Type: row.Type, CreatedAt: row.CreatedAt, Quota: row.Quota, PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, Other: row.Other}))
	}
	require.Equal(t, accumulator.finish().Summary, first.Summary, "per-day totals equal one scan of the month")
	require.Equal(t, 4, first.ActiveDays, "an adjustment-only day is not active")

	// Completed days come from the cache; today stays live.
	insert(at(11, 3, 11, 0, 0), 1000)
	insert(at(11, 8, 11, 0, 0), 3)
	second, err := GetPersonalMonthUsage(context.Background(), 1, location, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, first.Summary.Quota+3, second.Summary.Quota)

	// Yesterday is still read live during the settle window, then cached.
	afterMidnight := time.Date(2026, 11, 9, 0, 5, 0, 0, location)
	_, err = GetPersonalMonthUsage(context.Background(), 1, location, afterMidnight)
	require.NoError(t, err)
	insert(at(11, 8, 23, 59, 58), 4)
	settled, err := GetPersonalMonthUsage(context.Background(), 1, location, afterMidnight.Add(15*time.Minute))
	require.NoError(t, err)
	require.Equal(t, second.Summary.Quota+4, settled.Summary.Quota, "a late write stamped before midnight is still counted")
	insert(at(11, 8, 20, 0, 0), 2000)
	cached, err := GetPersonalMonthUsage(context.Background(), 1, location, afterMidnight.Add(25*time.Minute))
	require.NoError(t, err)
	require.Equal(t, settled.Summary.Quota, cached.Summary.Quota)

	// Day boundaries depend on the zone, so each zone has its own prefix.
	utc, err := GetPersonalMonthUsage(context.Background(), 1, time.UTC, afterMidnight.Add(25*time.Minute))
	require.NoError(t, err)
	require.Greater(t, utc.Summary.Quota, cached.Summary.Quota+2000)
}

func TestPersonalUsageSlotsAreSeparateAndWait(t *testing.T) {
	setupCacheRateScanDB(t)
	resetPersonalMonthCache(t)
	setActivityDataExport(t, false, 5)
	previousWait := usageSlotWait
	usageSlotWait = 20 * time.Millisecond
	t.Cleanup(func() { usageSlotWait = previousWait })
	fill := func(slots chan struct{}) func() {
		for i := 0; i < cap(slots); i++ {
			slots <- struct{}{}
		}
		return func() {
			for i := 0; i < cap(slots); i++ {
				<-slots
			}
		}
	}
	now := time.Now()
	drain := fill(dashboardUsageSlots)
	_, err := GetPersonalMonthUsage(context.Background(), 1, time.UTC, now)
	require.NoError(t, err, "an administrator's dashboard scans do not block personal usage")
	drain()

	drain = fill(personalUsageSlots)
	_, err = GetPersonalMonthUsage(context.Background(), 1, time.UTC, now)
	require.True(t, errors.Is(err, errUsageQueryBusy))
	usageSlotWait = 2 * time.Second
	go func() { time.Sleep(50 * time.Millisecond); <-personalUsageSlots }()
	_, err = GetPersonalMonthUsage(context.Background(), 1, time.UTC, now)
	require.NoError(t, err, "a slot freed within the wait is taken")
	personalUsageSlots <- struct{}{}
	drain()

	usageSlotWait = 20 * time.Millisecond
	drain = fill(userActivitySlots)
	_, err = scanUserActivity(context.Background(), 1, time.UTC, now, 10)
	require.True(t, errors.Is(err, errUsageQueryBusy))
	drain()
}

func TestAddDashboardUsageCountersCoversEveryCounter(t *testing.T) {
	var src DashboardUsageSummary
	value := reflect.ValueOf(&src).Elem()
	counters := 0
	for i := 0; i < value.NumField(); i++ {
		field := value.Type().Field(i)
		if field.Type.Kind() == reflect.Int64 && field.Name != "StartTimestamp" && field.Name != "EndTimestamp" {
			value.Field(i).SetInt(int64(i + 1))
			counters++
		}
	}
	require.Equal(t, 13, counters)
	dst := DashboardUsageSummary{StartTimestamp: 7, EndTimestamp: 9, Source: "kept"}
	require.NoError(t, addDashboardUsageCounters(&dst, src))
	require.NoError(t, addDashboardUsageCounters(&dst, src))
	result := reflect.ValueOf(dst)
	for i := 0; i < result.NumField(); i++ {
		field := result.Type().Field(i)
		if field.Type.Kind() == reflect.Int64 && field.Name != "StartTimestamp" && field.Name != "EndTimestamp" {
			require.EqualValues(t, 2*(i+1), result.Field(i).Int(), field.Name)
		}
	}
	require.EqualValues(t, 7, dst.StartTimestamp)
	require.EqualValues(t, 9, dst.EndTimestamp)
	require.Equal(t, "kept", dst.Source)
	require.Error(t, addDashboardUsageCounters(&DashboardUsageSummary{Quota: 9007199254740991}, DashboardUsageSummary{Quota: 1}))
}
