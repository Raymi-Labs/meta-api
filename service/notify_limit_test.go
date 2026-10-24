package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func setupNotificationLimitTest(t *testing.T, count, minutes int) {
	t.Helper()
	oldCount, oldMinutes := constant.NotifyLimitCount, constant.NotificationLimitDurationMinute
	constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = count, minutes
	notifyLimitMu.Lock()
	oldStore := notifyLimitStore
	notifyLimitStore = make(map[string]limitCount)
	notifyLimitMu.Unlock()
	t.Cleanup(func() {
		constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = oldCount, oldMinutes
		notifyLimitMu.Lock()
		notifyLimitStore = oldStore
		notifyLimitMu.Unlock()
	})
}

func notificationRedisTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	old := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = old; _ = client.Close() })
	return srv
}

func TestNotificationLimitFixedWindow(t *testing.T) {
	setupNotificationLimitTest(t, 2, 120)
	srv := notificationRedisTest(t)
	start := time.Date(2026, 10, 24, 10, 59, 0, 0, time.UTC)
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			now := start
			allow := func(user int, kind string) bool {
				if backend == "memory" {
					return checkMemoryLimitAt(user, kind, now)
				}
				allowed, err := checkRedisLimit(user, kind)
				require.NoError(t, err)
				return allowed
			}
			advance := func(d time.Duration) { now = now.Add(d); srv.FastForward(d) }
			require.True(t, allow(101, backend))
			advance(61 * time.Minute)
			require.True(t, allow(101, backend))
			require.False(t, allow(101, backend))
			require.True(t, allow(102, backend), "users have separate budgets")
			require.True(t, allow(101, backend+"-other"), "notification kinds have separate budgets")
			if backend == "redis" {
				require.Equal(t, 59*time.Minute, srv.TTL("notify_limit:101:redis"))
			}
			advance(59 * time.Minute)
			require.True(t, allow(101, backend), "window expires from the first notification")
		})
	}
}

func TestNotificationLimitConcurrentAdmission(t *testing.T) {
	setupNotificationLimitTest(t, 3, 10)
	notificationRedisTest(t)
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var admitted atomic.Int32
			var wg sync.WaitGroup
			for i := 0; i < 64; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					allowed := false
					if backend == "memory" {
						allowed = checkMemoryLimitAt(1, backend, time.Now())
					} else {
						var err error
						allowed, err = checkRedisLimit(1, backend)
						if err != nil {
							t.Error(err)
						}
					}
					if allowed {
						admitted.Add(1)
					}
				}()
			}
			wg.Wait()
			require.EqualValues(t, 3, admitted.Load())
		})
	}
}

func TestNotificationLimitConfigurationAndCleanup(t *testing.T) {
	setupNotificationLimitTest(t, 1, 10)
	srv := notificationRedisTest(t)
	for _, minutes := range []int{0, -1, int((1<<63-1)/time.Minute) + 1} {
		constant.NotificationLimitDurationMinute = minutes
		require.Equal(t, 10*time.Minute, getDuration())
		allowed, err := checkRedisLimit(minutes, "invalid-window")
		require.NoError(t, err)
		require.True(t, allowed)
		require.Equal(t, 10*time.Minute, srv.TTL(fmt.Sprintf("notify_limit:%d:invalid-window", minutes)))
	}
	constant.NotificationLimitDurationMinute = 1
	start := time.Now()
	require.True(t, checkMemoryLimitAt(1, "cleanup", start))
	cleanupNotificationLimits(start.Add(time.Minute))
	require.Empty(t, notifyLimitStore)
	require.True(t, checkMemoryLimitAt(1, "cleanup", start.Add(time.Minute)))
	for _, count := range []int{0, -1} {
		constant.NotifyLimitCount = count
		require.False(t, checkMemoryLimitAt(2, "disabled", start))
		allowed, err := checkRedisLimit(2, "disabled")
		require.NoError(t, err)
		require.False(t, allowed)
		require.False(t, srv.Exists("notify_limit:2:disabled"))
	}
}

func TestNotificationLimitRedisErrorsDoNotAllow(t *testing.T) {
	setupNotificationLimitTest(t, 2, 10)
	notificationRedisTest(t)
	require.NoError(t, common.RDB.Set(context.Background(), "notify_limit:1:invalid", "corrupt", time.Minute).Err())
	allowed, err := checkRedisLimit(1, "invalid")
	require.Error(t, err)
	require.False(t, allowed)
	common.RDB = nil
	allowed, err = checkRedisLimit(1, "missing")
	require.Error(t, err)
	require.False(t, allowed)
}
