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
	rows, channels, err := queryOpenRouterSiteActivity(context.Background(), []int{1}, start, end)
	require.NoError(t, err)
	require.Equal(t, []OpenRouterChannelActivity{{Date: "2026-11-01", ChannelID: 1, Quota: 50, Requests: 3}}, channels)
	require.Equal(t, []OpenRouterSiteActivity{{Date: "2026-11-01", Model: "local-only", Requests: 1}, {Date: "2026-11-01", Model: "vendor/model", Requests: 2, Quota: 50}}, rows)
}

func TestOpenRouterSiteAndChannelTotalsAgree(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	entries := []Log{
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: start.Unix(), ModelName: "alias", Quota: 10, Other: `{"upstream_model_name":"m"}`},
		{ChannelId: 2, Type: LogTypeConsume, CreatedAt: start.Unix(), ModelName: "m", Quota: 20},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: start.AddDate(0, 0, 1).Unix(), ModelName: "m", Quota: 30},
	}
	require.NoError(t, LOG_DB.Create(&entries).Error)
	models, channels, err := queryOpenRouterSiteActivity(context.Background(), []int{1, 2}, start, start.AddDate(0, 0, 2))
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Len(t, channels, 3)
	require.Equal(t, int64(30), models[1].Quota)
	var modelQuota, channelQuota, modelRequests, channelRequests int64
	for _, row := range models {
		modelQuota += row.Quota
		modelRequests += row.Requests
	}
	for _, row := range channels {
		channelQuota += row.Quota
		channelRequests += row.Requests
	}
	require.Equal(t, int64(60), modelQuota)
	require.Equal(t, modelQuota, channelQuota)
	require.Equal(t, int64(3), modelRequests)
	require.Equal(t, modelRequests, channelRequests)
}
