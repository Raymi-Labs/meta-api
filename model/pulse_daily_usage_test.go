package model

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPulseYesterdayRecordedUsage(t *testing.T) {
	db := setupLogQueryTestDB(t)
	end := time.Date(2026, 11, 19, 16, 0, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour).Unix()
	rows := []Log{{UserId: 1, Type: LogTypeConsume, CreatedAt: start, Quota: 7}, {UserId: 1, Type: LogTypeConsume, CreatedAt: end.Unix() - 1, Quota: 8}, {UserId: 1, Type: LogTypeConsume, CreatedAt: start - 1, Quota: 20}, {UserId: 1, Type: LogTypeConsume, CreatedAt: end.Unix(), Quota: 30}, {UserId: 2, Type: LogTypeConsume, CreatedAt: start, Quota: 40}, {UserId: 1, Type: LogTypeRefund, CreatedAt: start, Quota: 4}}
	require.NoError(t, db.Create(&rows).Error)
	d, v, e := PulseYesterdayUsage(t.Context(), 1, end)
	require.NoError(t, e)
	require.Equal(t, "2026-11-19", d)
	require.Equal(t, int64(15), *v)
	_, v, e = PulseYesterdayUsage(t.Context(), 3, end)
	require.NoError(t, e)
	require.Nil(t, v)
}
