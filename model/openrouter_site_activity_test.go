package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenRouterSiteOwnership(t *testing.T) {
	hashes := map[string]bool{cashKeyHash("a"): true, cashKeyHash("b"): true}
	channels := []Channel{{Id: 1, Key: "a"}, {Id: 2, Key: "a\nb"}, {Id: 3, Key: "a\nforeign"}, {Id: 4, Key: ""}, {Id: 5, Key: "foreign"}}
	require.Equal(t, []int{1, 2}, openRouterOwnedChannels(channels, hashes))
}

func TestOpenRouterSiteActivityScopeAndHistoricalModel(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	entries := []Log{
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: start.Unix(), ModelName: "alias", Quota: 20, Other: `{"upstream_model_name":"vendor/model"}`},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: end.Unix() - 1, ModelName: "vendor/model", Quota: 30},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: start.Unix() + 1, ModelName: "local-only", Quota: 0, Other: `broken`},
		{ChannelId: 2, Type: LogTypeConsume, CreatedAt: start.Unix(), ModelName: "foreign", Quota: 500},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: end.Unix(), ModelName: "today", Quota: 500},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: start.Unix() - 1, ModelName: "old", Quota: 500},
		{ChannelId: 1, Type: LogTypeError, CreatedAt: start.Unix(), ModelName: "error", Quota: 500},
	}
	require.NoError(t, LOG_DB.Create(&entries).Error)
	rows, err := queryOpenRouterSiteActivity(context.Background(), []int{1}, start, end)
	require.NoError(t, err)
	require.Equal(t, []OpenRouterSiteActivity{{Date: "2026-11-01", Model: "local-only", Requests: 1}, {Date: "2026-11-01", Model: "vendor/model", Requests: 2, Quota: 50}}, rows)
}
