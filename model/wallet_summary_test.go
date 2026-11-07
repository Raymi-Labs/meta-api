package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestWalletUsageScopeSourcesAndBoundaries(t *testing.T) {
	setupCacheRateScanDB(t)
	now := time.Date(2026, 11, 1, 7, 0, 0, 0, time.UTC) // DST changes do not change a rolling 24h window.
	rows := []Log{
		{UserId: 1, Type: LogTypeConsume, Quota: 100, CreatedAt: now.Unix() - 86400, Other: `{"billing_source":"wallet"}`},
		{UserId: 1, Type: LogTypeConsume, Quota: 40, CreatedAt: now.Unix() - 1, Other: `{"billing_source":"wallet","task_id":"adjustment"}`},
		{UserId: 1, Type: LogTypeRefund, Quota: 30, CreatedAt: now.Unix() - 1, Other: `{"billing_source":"wallet"}`},
		{UserId: 1, Type: LogTypeConsume, Quota: 900, CreatedAt: now.Unix() - 1, Other: `{"billing_source":"subscription"}`},
		{UserId: 1, Type: LogTypeRefund, Quota: 200, CreatedAt: now.Unix() - 1, Other: `{"billing_source":"subscription"}`},
		{UserId: 1, Type: LogTypeConsume, Quota: 900, CreatedAt: now.Unix() - 1, Other: `{"billing_source":"token"}`},
		{UserId: 2, Type: LogTypeConsume, Quota: 999, CreatedAt: now.Unix() - 1},
		{UserId: 1, Type: LogTypeConsume, Quota: 999, CreatedAt: now.Unix() - 86401},
		{UserId: 1, Type: LogTypeConsume, Quota: 999, CreatedAt: now.Unix()},
	}
	require.NoError(t, LOG_DB.Create(&rows).Error)
	usage, err := scanWalletUsage(context.Background(), 1, now, 100)
	require.NoError(t, err)
	require.Equal(t, "ok", usage.Status)
	require.EqualValues(t, 1710, *usage.NetQuota)
	require.EqualValues(t, 86400, usage.EndTimestamp-usage.StartTimestamp)
	usage, err = scanWalletUsage(context.Background(), 3, now, 100)
	require.NoError(t, err)
	require.Zero(t, *usage.NetQuota)
	usage, err = scanWalletUsage(context.Background(), 2, now, 100)
	require.NoError(t, err)
	require.Equal(t, "ok", usage.Status)
	require.NotNil(t, usage.NetQuota)
	// A retained refund may exceed consumption in this window; do not invent positive burn.
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Type: LogTypeRefund, Quota: 1800, CreatedAt: now.Unix() - 1, Other: `{"billing_source":"wallet"}`}).Error)
	usage, err = scanWalletUsage(context.Background(), 1, now, 100)
	require.NoError(t, err)
	require.EqualValues(t, -90, *usage.NetQuota)
	usage, err = scanWalletUsage(context.Background(), 1, now, 1)
	require.Error(t, err)
	require.Nil(t, usage.NetQuota)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = scanWalletUsage(ctx, 1, now, 100)
	require.Error(t, err)
	for _, other := range []string{`{"billing_source":"new_source"}`, `{"task_id":"legacy"}`} {
		require.NoError(t, LOG_DB.Create(&Log{UserId: 3, Type: LogTypeConsume, Quota: 10, CreatedAt: now.Unix() - 1, Other: other}).Error)
	}
	usage, err = scanWalletUsage(context.Background(), 3, now, 100)
	require.NoError(t, err)
	require.Equal(t, "ok", usage.Status)
	require.EqualValues(t, 20, *usage.NetQuota)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 4, Type: LogTypeConsume, Quota: 10, CreatedAt: now.Unix() - 1, Other: `bad-json`}).Error)
	usage, err = scanWalletUsage(context.Background(), 4, now, 100)
	require.NoError(t, err)
	require.EqualValues(t, 10, *usage.NetQuota)
	old := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = old })
	usage, err = scanWalletUsage(context.Background(), 1, now, 100)
	require.NoError(t, err)
	require.Equal(t, "not_recorded", usage.Status)
	require.Nil(t, usage.NetQuota)
}
