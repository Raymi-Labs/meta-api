package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/bytedance/gopkg/util/gopool"
)

// notifyLimitStore is used for in-memory rate limiting when Redis is disabled
var (
	notifyLimitStore = make(map[string]limitCount)
	notifyLimitMu    sync.Mutex
	cleanupOnce      sync.Once
)

type limitCount struct {
	Count     int
	Timestamp time.Time
}

func getDuration() time.Duration {
	minute := constant.NotificationLimitDurationMinute
	if minute <= 0 || int64(minute) > int64((1<<63-1)/time.Minute) {
		minute = 10
	}
	return time.Duration(minute) * time.Minute
}

// startCleanupTask starts a background task to clean up expired entries
func startCleanupTask() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Hour)
			cleanupNotificationLimits(time.Now())
		}
	})
}

func cleanupNotificationLimits(now time.Time) {
	notifyLimitMu.Lock()
	defer notifyLimitMu.Unlock()
	for key, limit := range notifyLimitStore {
		if now.Sub(limit.Timestamp) >= getDuration() {
			delete(notifyLimitStore, key)
		}
	}
}

// CheckNotificationLimit checks if the user has exceeded their notification limit
// Returns true if the user can send notification, false if limit exceeded
func CheckNotificationLimit(userId int, notifyType string) (bool, error) {
	if common.RedisEnabled {
		return checkRedisLimit(userId, notifyType)
	}
	return checkMemoryLimit(userId, notifyType)
}

func checkRedisLimit(userId int, notifyType string) (bool, error) {
	if constant.NotifyLimitCount <= 0 {
		return false, nil
	}
	if common.RDB == nil {
		return false, fmt.Errorf("notification Redis client is unavailable")
	}
	ctx := context.Background()
	key := fmt.Sprintf("notify_limit:%d:%s", userId, notifyType)
	pipe := common.RDB.TxPipeline()
	pipe.SetNX(ctx, key, 0, getDuration())
	count := pipe.Incr(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("failed to increment notification count: %w", err)
	}
	return count.Val() <= int64(constant.NotifyLimitCount), nil
}

func checkMemoryLimit(userId int, notifyType string) (bool, error) {
	// Ensure cleanup task is started
	cleanupOnce.Do(startCleanupTask)
	return checkMemoryLimitAt(userId, notifyType, time.Now()), nil
}

func checkMemoryLimitAt(userId int, notifyType string, now time.Time) bool {
	if constant.NotifyLimitCount <= 0 {
		return false
	}
	notifyLimitMu.Lock()
	defer notifyLimitMu.Unlock()
	key := fmt.Sprintf("%d:%s", userId, notifyType)
	currentLimit, ok := notifyLimitStore[key]
	if !ok || now.Sub(currentLimit.Timestamp) >= getDuration() {
		currentLimit = limitCount{Count: 0, Timestamp: now}
	}
	if currentLimit.Count >= constant.NotifyLimitCount {
		return false
	}
	currentLimit.Count++
	notifyLimitStore[key] = currentLimit
	return true
}
