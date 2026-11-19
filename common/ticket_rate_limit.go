package common

import (
	"context"
	"fmt"
	"time"
)

var ticketInMemoryRateLimiter InMemoryRateLimiter

func init() {
	ticketInMemoryRateLimiter.Init(time.Hour * 2)
}

// ResetTicketRateLimiter 重置工单限流器（仅供测试使用）
func ResetTicketRateLimiter() {
	ticketInMemoryRateLimiter.mutex.Lock()
	defer ticketInMemoryRateLimiter.mutex.Unlock()
	ticketInMemoryRateLimiter.store = make(map[string]*[]int64)
}

// CheckRateLimit 检查限流（工单系统专用）
// 返回 error 表示触发限流
func CheckRateLimit(ctx context.Context, key string, maxRequestNum int, duration int64) error {
	if RedisEnabled {
		return checkRedisRateLimit(ctx, key, maxRequestNum, duration)
	}
	return checkMemoryRateLimit(key, maxRequestNum, duration)
}

func checkRedisRateLimit(ctx context.Context, key string, maxRequestNum int, duration int64) error {
	redisKey := "rate_limit:" + key
	now := time.Now()
	expiration := time.Duration(duration*2) * time.Second

	allowed, err := runRedisRateLimit(ctx, redisKey, now, maxRequestNum, duration, expiration)
	if err != nil {
		return fmt.Errorf("Redis限流检查失败: %w", err)
	}
	if !allowed {
		return fmt.Errorf("操作过于频繁，请在 %d 秒后重试", duration)
	}
	return nil
}

func checkMemoryRateLimit(key string, maxRequestNum int, duration int64) error {
	// Retention must cover every requested window, including daily appeals.
	// Init only increases retention and is safe when requests run concurrently.
	ticketInMemoryRateLimiter.Init(time.Duration(duration) * time.Second)
	if !ticketInMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		return fmt.Errorf("操作过于频繁，请在 %d 秒后重试", duration)
	}
	return nil
}

// runRedisRateLimit 使用滑动窗口 Redis Lua 脚本执行限流
var runRedisRateLimit = func(ctx context.Context, key string, now time.Time, maxRequestNum int, duration int64, expiration time.Duration) (bool, error) {
	nowTs := now.Unix()
	windowStart := nowTs - duration

	// Lua 脚本：滑动窗口限流
	script := `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window_start = tonumber(ARGV[2])
local max_requests = tonumber(ARGV[3])
local expiration = tonumber(ARGV[4])

redis.call('ZREMRANGEBYSCORE', key, 0, window_start)
local current = redis.call('ZCARD', key)
if current < max_requests then
    redis.call('ZADD', key, now, ARGV[5])
    redis.call('EXPIRE', key, expiration)
    return 1
else
    return 0
end
`

	result, err := RDB.Eval(ctx, script, []string{key}, nowTs, windowStart, maxRequestNum, int(expiration.Seconds()), GetUUID()).Result()
	if err != nil {
		return false, err
	}

	allowed, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected Redis response type")
	}

	return allowed == 1, nil
}
