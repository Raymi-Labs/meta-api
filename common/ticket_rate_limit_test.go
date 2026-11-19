package common

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTicketRedisBurstAndExpiry(t *testing.T) {
	server := miniredis.RunT(t)
	old := RDB
	RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { RDB.Close(); RDB = old }()
	now := time.Now()
	for i := 0; i < 100; i++ {
		allowed, err := runRedisRateLimit(context.Background(), "ticket:test", now, 10, 3600, 2*time.Hour)
		require.NoError(t, err)
		require.Equal(t, i < 10, allowed, "request %d", i)
	}
	allowed, err := runRedisRateLimit(context.Background(), "ticket:test", now.Add(time.Hour), 10, 3600, 2*time.Hour)
	require.NoError(t, err)
	require.True(t, allowed)
}
