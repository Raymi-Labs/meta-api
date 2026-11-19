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

func TestTicketMemoryDailyLimitSurvivesCleanup(t *testing.T) {
	ResetTicketRateLimiter()
	t.Cleanup(ResetTicketRateLimiter)
	const key = "appeal:daily-retention"
	const day = int64(24 * 60 * 60)
	require.NoError(t, checkMemoryRateLimit(key, 1, day))
	now := time.Now().Unix()
	age := func(seconds int64) {
		ticketInMemoryRateLimiter.mutex.Lock()
		(*ticketInMemoryRateLimiter.store[key])[0] = now - seconds
		ticketInMemoryRateLimiter.mutex.Unlock()
	}
	age(3 * 60 * 60)
	ticketInMemoryRateLimiter.clearExpiredItemsAt(now)
	require.Error(t, checkMemoryRateLimit(key, 1, day), "cleanup must not reset a still-active daily limit")
	age(day - 60)
	ticketInMemoryRateLimiter.clearExpiredItemsAt(now)
	require.Error(t, checkMemoryRateLimit(key, 1, day))
	age(day)
	require.NoError(t, checkMemoryRateLimit(key, 1, day), "window expires at exactly 24 hours")
	age(day + 1)
	ticketInMemoryRateLimiter.clearExpiredItemsAt(now)
	ticketInMemoryRateLimiter.mutex.Lock()
	_, retained := ticketInMemoryRateLimiter.store[key]
	ticketInMemoryRateLimiter.mutex.Unlock()
	require.False(t, retained, "expired keys must still be reclaimed")
}
