package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPersonalMonthUsageRetainedSemantics(t *testing.T) {
	setupCacheRateScanDB(t)
	location, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	// October includes a DST fallback: 31 days + 1 hour at month end.
	now := time.Date(2026, 10, 31, 23, 59, 59, 0, location)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, location)
	require.Greater(t, now.Unix()-start.Unix(), int64(31*86400))
	rows := []Log{
		{UserId: 1, Type: LogTypeConsume, CreatedAt: start.Unix(), Quota: 100, PromptTokens: 10, CompletionTokens: 2, Other: `{"model_ratio":1,"prompt_tokens_semantic":"non_cache","cache_tokens":20}`},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: now.Unix(), Quota: 50, PromptTokens: 5, Other: `{"model_ratio":1}`},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: now.Unix(), Quota: 30, Other: `{"task_id":"adjustment"}`},
		{UserId: 1, Type: LogTypeRefund, CreatedAt: now.Unix(), Quota: 20},
		{UserId: 1, Type: LogTypeConsume, CreatedAt: start.Unix() - 1, Quota: 999},
		{UserId: 2, Type: LogTypeConsume, CreatedAt: now.Unix(), Quota: 999},
	}
	for _, row := range rows {
		require.NoError(t, LOG_DB.Create(&row).Error)
	}
	result, err := GetPersonalMonthUsage(context.Background(), 1, location, now)
	require.NoError(t, err)
	require.Equal(t, "2026-10", result.Month)
	require.Equal(t, 2, result.ActiveDays)
	require.EqualValues(t, 2, result.Summary.Requests)
	require.EqualValues(t, 160, result.Summary.Quota)
	require.EqualValues(t, 37, result.Summary.Tokens)
	// Compare every summary field against the existing accumulator over the same rows.
	accumulator := newDashboardUsageAccumulator(start.Unix(), now.Unix()+1, false)
	for _, row := range rows[:4] {
		require.NoError(t, accumulator.add(dashboardUsageLog{Type: row.Type, CreatedAt: row.CreatedAt, Quota: row.Quota, PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, Other: row.Other}))
	}
	require.Equal(t, accumulator.finish().Summary, result.Summary)
	empty, err := GetPersonalMonthUsage(context.Background(), 3, location, now)
	require.NoError(t, err)
	require.Zero(t, empty.Summary.Requests)
	require.Zero(t, empty.ActiveDays)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetPersonalMonthUsage(ctx, 1, location, now)
	require.Error(t, err)
	_, err = GetPersonalMonthUsage(context.Background(), 0, location, now)
	require.Error(t, err)
}
func TestPersonalTokensSummaryPrivacyAndFullCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Token{}))
	old := DB
	DB = db
	t.Cleanup(func() { DB = old; conn, _ := db.DB(); _ = conn.Close() })
	now := time.Now().Unix()
	for i := 1; i <= 8; i++ {
		require.NoError(t, db.Create(&Token{Id: i, UserId: 1, Key: fmt.Sprintf("private-key-%d-do-not-expose-1234", i), Name: "fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, CreatedTime: now}).Error)
	}
	require.NoError(t, db.Model(&Token{}).Where("id = ?", 1).Update("expired_time", now-1).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", 2).Update("status", common.TokenStatusDisabled).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", 3).Update("status", common.TokenStatusExhausted).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", 8).Update("key", "short").Error)
	require.NoError(t, db.Create(&Token{Id: 9, UserId: 1, Key: "temporary-secret", SourceType: TokenSourceTypeImagePlayground, Status: 1, ExpiredTime: now + 1000}).Error)
	require.NoError(t, db.Create(&Token{Id: 10, UserId: 2, Key: "other-user-secret", Status: 1, ExpiredTime: -1}).Error)
	result, err := GetPersonalTokenSummary(context.Background(), 1, now)
	require.NoError(t, err)
	require.EqualValues(t, 8, result.Total)
	require.EqualValues(t, 5, result.Enabled)
	require.EqualValues(t, 3, result.Other)
	require.Len(t, result.Recent, 5)
	require.Equal(t, 8, result.Recent[0].ID)
	require.Equal(t, "••••", result.Recent[0].MaskedKey)
	require.Equal(t, "••••1234", result.Recent[1].MaskedKey)
	bytes, err := common.Marshal(result)
	require.NoError(t, err)
	for _, secret := range []string{"private-key", "temporary-secret", "other-user-secret", "short", "\"key\":"} {
		require.NotContains(t, string(bytes), secret)
	}
	empty, err := GetPersonalTokenSummary(context.Background(), 3, now)
	require.NoError(t, err)
	require.Zero(t, empty.Total)
	require.NotNil(t, empty.Recent)
	_, err = GetPersonalTokenSummary(context.Background(), 0, now)
	require.Error(t, err)
}
