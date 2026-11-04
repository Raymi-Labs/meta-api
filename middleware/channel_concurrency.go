package middleware

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// channel_concurrency.go 实现渠道级并发 Lease、RPM 滑动窗口与过载指标：
//   - Redis 模式使用 Lua 原子清理、检查并写入两个 ZSET，跨实例不超发；
//   - Lease 由心跳续期，实例崩溃后自动过期回收；
//   - Redis 未启用时回退到进程内互斥状态，保证单实例并发 + RPM 原子准入。
//
// 编排逻辑（选渠道 → acquire → 满则换渠道 → 全满进入有界等待）在 controller 层，
// 这里只暴露被其调用的原语，避免 middleware → controller 的反向依赖。

const (
	// Redis key 前缀。每个渠道使用 leases/rpm 两个 ZSET；同一渠道用相同 hash tag，
	// 便于 Redis Cluster 将原子准入脚本涉及的两个 key 放在同一 slot。
	channelCapacityKeyPrefix = "channel:capacity:"
	// Lease 通过心跳续期；实例崩溃或 release 丢失后，过期成员会由后续准入/查询自动清理。
	channelLeaseTTL               = 2 * time.Minute
	channelLeaseHeartbeatInterval = 30 * time.Second
	channelCapacityBackendRetry   = time.Second
)

// channelCapacityAcquireScript 在 Redis 服务端使用同一时钟原子完成：
// 1. 清理过期 Lease 和 RPM 窗口外记录；2. 检查并发；3. 检查 RPM；4. 同时写入。
// 返回值：[是否成功, 拒绝原因(1=并发/2=RPM), retry_after_ms, inflight, rpm_used]。
var channelCapacityAcquireScript = redis.NewScript(`
local lease_key = KEYS[1]
local rpm_key = KEYS[2]
local max_concurrency = tonumber(ARGV[1])
local rpm_limit = tonumber(ARGV[2])
local rpm_window_ms = tonumber(ARGV[3])
local lease_ttl_ms = tonumber(ARGV[4])
local lease_key_ttl_ms = tonumber(ARGV[5])
local rpm_key_ttl_ms = tonumber(ARGV[6])
local member = ARGV[7]

local server_time = redis.call('TIME')
local now_ms = tonumber(server_time[1]) * 1000 + math.floor(tonumber(server_time[2]) / 1000)

redis.call('ZREMRANGEBYSCORE', lease_key, '-inf', now_ms)
if rpm_window_ms > 0 then
    redis.call('ZREMRANGEBYSCORE', rpm_key, '-inf', now_ms - rpm_window_ms)
end

local inflight = redis.call('ZCARD', lease_key)
local rpm_used = redis.call('ZCARD', rpm_key)

if max_concurrency > 0 and inflight >= max_concurrency then
    local oldest = redis.call('ZRANGE', lease_key, 0, 0, 'WITHSCORES')
    local retry_after_ms = 1
    if #oldest >= 2 then
        retry_after_ms = math.max(1, math.floor(tonumber(oldest[2]) - now_ms))
    end
    redis.call('PEXPIRE', lease_key, lease_key_ttl_ms)
    return {0, 1, retry_after_ms, inflight, rpm_used}
end

if rpm_limit > 0 and rpm_used >= rpm_limit then
    local oldest = redis.call('ZRANGE', rpm_key, 0, 0, 'WITHSCORES')
    local retry_after_ms = 1
    if #oldest >= 2 then
        retry_after_ms = math.max(1, math.floor(tonumber(oldest[2]) + rpm_window_ms - now_ms))
    end
    redis.call('PEXPIRE', rpm_key, rpm_key_ttl_ms)
    return {0, 2, retry_after_ms, inflight, rpm_used}
end

if max_concurrency > 0 then
    redis.call('ZADD', lease_key, now_ms + lease_ttl_ms, member)
    redis.call('PEXPIRE', lease_key, lease_key_ttl_ms)
    inflight = inflight + 1
end
if rpm_limit > 0 then
    redis.call('ZADD', rpm_key, now_ms, member)
    redis.call('PEXPIRE', rpm_key, rpm_key_ttl_ms)
    rpm_used = rpm_used + 1
end
return {1, 0, 0, inflight, rpm_used}
`)

var channelLeaseHeartbeatScript = redis.NewScript(`
local lease_key = KEYS[1]
local member = ARGV[1]
local lease_ttl_ms = tonumber(ARGV[2])
local lease_key_ttl_ms = tonumber(ARGV[3])
local server_time = redis.call('TIME')
local now_ms = tonumber(server_time[1]) * 1000 + math.floor(tonumber(server_time[2]) / 1000)
local score = redis.call('ZSCORE', lease_key, member)
if not score then
    return 0
end
if tonumber(score) <= now_ms then
    redis.call('ZREM', lease_key, member)
    return 0
end
redis.call('ZADD', lease_key, now_ms + lease_ttl_ms, member)
redis.call('PEXPIRE', lease_key, lease_key_ttl_ms)
return 1
`)

var channelLeaseReleaseScript = redis.NewScript(`
local removed = redis.call('ZREM', KEYS[1], ARGV[1])
if redis.call('ZCARD', KEYS[1]) == 0 then
    redis.call('DEL', KEYS[1])
end
return removed
`)

// 查询脚本批量清理并返回每个渠道的 [inflight, rpm_used]，使用一次 EVAL 避免逐渠道查询。
var channelCapacityQueryScript = redis.NewScript(`
local rpm_window_ms = tonumber(ARGV[1])
local server_time = redis.call('TIME')
local now_ms = tonumber(server_time[1]) * 1000 + math.floor(tonumber(server_time[2]) / 1000)
local result = {}
for i = 1, #KEYS, 2 do
    local lease_key = KEYS[i]
    local rpm_key = KEYS[i + 1]
    redis.call('ZREMRANGEBYSCORE', lease_key, '-inf', now_ms)
    if rpm_window_ms > 0 then
        redis.call('ZREMRANGEBYSCORE', rpm_key, '-inf', now_ms - rpm_window_ms)
    end
    table.insert(result, redis.call('ZCARD', lease_key))
    table.insert(result, redis.call('ZCARD', rpm_key))
end
return result
`)

// ---- 内存态准入（Redis 未启用时的回退） ----

var channelCapacityStates sync.Map // key(string) -> *channelCapacityState

type channelCapacityState struct {
	mu         sync.Mutex
	inflight   int64
	rpmEntries []int64
	// idleRounds 仅由 cleanup goroutine 在持锁状态下访问：连续为空的清理轮数。
	idleRounds int
}

func getChannelCapacityState(key string) *channelCapacityState {
	if state, ok := channelCapacityStates.Load(key); ok {
		return state.(*channelCapacityState)
	}
	state := &channelCapacityState{}
	actual, _ := channelCapacityStates.LoadOrStore(key, state)
	return actual.(*channelCapacityState)
}

func init() {
	go cleanupIdleChannelCapacityStates()
}

// 定期清理空闲的内存渠道状态，防止 sync.Map 随历史渠道数无界增长。
func cleanupIdleChannelCapacityStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rpmWindow := time.Duration(operation_setting.GetChannelConcurrencySetting().NormalizedRpmWindowSeconds()) * time.Second
		cutoff := time.Now().Add(-rpmWindow).UnixNano()
		channelCapacityStates.Range(func(key, value any) bool {
			state := value.(*channelCapacityState)
			state.mu.Lock()
			state.pruneRpmEntriesLocked(cutoff)
			if state.inflight == 0 && len(state.rpmEntries) == 0 {
				state.idleRounds++
				if state.idleRounds >= 2 {
					channelCapacityStates.Delete(key)
				}
			} else {
				state.idleRounds = 0
			}
			state.mu.Unlock()
			return true
		})
	}
}

func channelLeaseKey(channelID int) string {
	return fmt.Sprintf("%s{%d}:leases", channelCapacityKeyPrefix, channelID)
}

func channelRpmKey(channelID int) string {
	return fmt.Sprintf("%s{%d}:rpm", channelCapacityKeyPrefix, channelID)
}

func channelMemoryCapacityKey(channelID int) string {
	return fmt.Sprintf("%s%d", channelCapacityKeyPrefix, channelID)
}

// ---- 过载指标（进程内 atomic，重启清零；仅用于观测，不作硬依据） ----

var channelConcurrencyMetrics struct {
	acquireTotal      atomic.Int64 // 成功占用名额的总次数
	waitEnterTotal    atomic.Int64 // 进入 Layer C 有界等待的次数（所有渠道满）
	waitAcquiredTotal atomic.Int64 // 等待后成功拿到容量的次数
	waitTimeoutTotal  atomic.Int64 // 等待超时返回 503 的次数
	waitCancelTotal   atomic.Int64 // 等待中客户端断开取消的次数
	queueRejectTotal  atomic.Int64 // 等待队列已满、快速失败 503 的次数
	waitTotalMs       atomic.Int64 // 等待累计毫秒（用于算平均等待时长）
	waitDurationCount atomic.Int64 // 参与等待时长统计的样本数
	currentWaiting    atomic.Int64 // 当前处于 Layer C 等待中的请求数（gauge，即队列占用）
}

// ChannelConcurrencyMetricsSnapshot 是指标的只读快照，供 admin stats 端点返回。
type ChannelConcurrencyMetricsSnapshot struct {
	AcquireTotal      int64 `json:"acquire_total"`
	WaitEnterTotal    int64 `json:"wait_enter_total"`
	WaitAcquiredTotal int64 `json:"wait_acquired_total"`
	WaitTimeoutTotal  int64 `json:"wait_timeout_total"`
	WaitCancelTotal   int64 `json:"wait_cancel_total"`
	QueueRejectTotal  int64 `json:"queue_reject_total"`
	CurrentWaiting    int64 `json:"current_waiting"`
	AvgWaitMs         int64 `json:"avg_wait_ms"`
}

// GetChannelConcurrencyMetrics 返回当前过载指标快照。
func GetChannelConcurrencyMetrics() ChannelConcurrencyMetricsSnapshot {
	count := channelConcurrencyMetrics.waitDurationCount.Load()
	avg := int64(0)
	if count > 0 {
		avg = channelConcurrencyMetrics.waitTotalMs.Load() / count
	}
	return ChannelConcurrencyMetricsSnapshot{
		AcquireTotal:      channelConcurrencyMetrics.acquireTotal.Load(),
		WaitEnterTotal:    channelConcurrencyMetrics.waitEnterTotal.Load(),
		WaitAcquiredTotal: channelConcurrencyMetrics.waitAcquiredTotal.Load(),
		WaitTimeoutTotal:  channelConcurrencyMetrics.waitTimeoutTotal.Load(),
		WaitCancelTotal:   channelConcurrencyMetrics.waitCancelTotal.Load(),
		QueueRejectTotal:  channelConcurrencyMetrics.queueRejectTotal.Load(),
		CurrentWaiting:    channelConcurrencyMetrics.currentWaiting.Load(),
		AvgWaitMs:         avg,
	}
}

// 指标记录器（供 controller 编排层调用）。
func recordChannelAcquire()      { channelConcurrencyMetrics.acquireTotal.Add(1) }
func recordChannelWaitEnter()    { channelConcurrencyMetrics.waitEnterTotal.Add(1) }
func recordChannelWaitAcquired() { channelConcurrencyMetrics.waitAcquiredTotal.Add(1) }
func recordChannelWaitTimeout()  { channelConcurrencyMetrics.waitTimeoutTotal.Add(1) }
func recordChannelWaitCancel()   { channelConcurrencyMetrics.waitCancelTotal.Add(1) }
func recordChannelQueueReject()  { channelConcurrencyMetrics.queueRejectTotal.Add(1) }
func recordChannelWaitDuration(d time.Duration) {
	channelConcurrencyMetrics.waitTotalMs.Add(d.Milliseconds())
	channelConcurrencyMetrics.waitDurationCount.Add(1)
}

// Exported wrappers（controller 包调用）。
func RecordChannelWaitEnter()                   { recordChannelWaitEnter() }
func RecordChannelWaitAcquired()                { recordChannelWaitAcquired() }
func RecordChannelWaitTimeout()                 { recordChannelWaitTimeout() }
func RecordChannelWaitCancel()                  { recordChannelWaitCancel() }
func RecordChannelQueueReject()                 { recordChannelQueueReject() }
func RecordChannelWaitDuration(d time.Duration) { recordChannelWaitDuration(d) }

// ---- 有界等待队列准入（防止过载时等待请求无界堆积） ----

// EnterChannelWaitQueue 尝试进入 Layer C 等待队列。返回 false 表示队列已满，调用方应快速失败（503）。
// 用原子自增 + 越界回退实现无锁准入闸门。maxLen<=0 视为不限。
func EnterChannelWaitQueue(maxLen int) bool {
	n := channelConcurrencyMetrics.currentWaiting.Add(1)
	if maxLen > 0 && n > int64(maxLen) {
		channelConcurrencyMetrics.currentWaiting.Add(-1)
		return false
	}
	return true
}

// LeaveChannelWaitQueue 离开等待队列，必须与 EnterChannelWaitQueue 成功调用配对。
func LeaveChannelWaitQueue() {
	channelConcurrencyMetrics.currentWaiting.Add(-1)
}

// ---- 渠道容量准入 ----

// ChannelConcurrencyEnabled 全局开关，关闭时所有渠道容量控制逻辑短路。
func ChannelConcurrencyEnabled() bool {
	return operation_setting.GetChannelConcurrencySetting().Enabled
}

// ResolveChannelMaxConcurrency 计算某渠道的在途并发上限：渠道级 max_concurrency>0 时覆盖全局默认。
func ResolveChannelMaxConcurrency(channel *model.Channel) int {
	setting := operation_setting.GetChannelConcurrencySetting()
	if channel != nil {
		if override := channel.GetSetting().MaxConcurrency; override > 0 {
			return override
		}
	}
	return setting.NormalizedDefaultMaxConcurrency()
}

// ResolveChannelRpmLimit 计算某渠道的 RPM 上限：渠道级 rpm_limit>0 时覆盖全局默认；
// 渠道与全局均为 0 时表示不限制请求频率。
func ResolveChannelRpmLimit(channel *model.Channel) int {
	setting := operation_setting.GetChannelConcurrencySetting()
	if channel != nil {
		if override := channel.GetSetting().RpmLimit; override > 0 {
			return override
		}
	}
	return setting.NormalizedDefaultRpmLimit()
}

// AcquireChannelCapacityForChannel 使用渠道级配置申请一次完整容量（并发 + RPM）。
// 关闭容量控制时返回成功的空操作，便于固定渠道、后台轮询等调用方统一接入。
func AcquireChannelCapacityForChannel(channel *model.Channel, requestID string) ChannelAdmissionResult {
	if !ChannelConcurrencyEnabled() {
		return ChannelAdmissionResult{Release: func() {}, Acquired: true}
	}
	if channel == nil || channel.Id <= 0 {
		return ChannelAdmissionResult{Reason: ChannelAdmissionBackendUnavailable, RetryAfter: channelCapacityBackendRetry}
	}
	setting := operation_setting.GetChannelConcurrencySetting()
	return AcquireChannelCapacityWithMetric(
		channel.Id,
		ResolveChannelMaxConcurrency(channel),
		ResolveChannelRpmLimit(channel),
		time.Duration(setting.NormalizedRpmWindowSeconds())*time.Second,
		requestID,
	)
}

// AcquireChannelRpmForChannel 只消费一次 RPM，不额外占用并发 Lease。
// 用于同一上游调用生命周期内的兼容重试或第二次 HTTP 请求。
func AcquireChannelRpmForChannel(channel *model.Channel, requestID string) ChannelAdmissionResult {
	if !ChannelConcurrencyEnabled() {
		return ChannelAdmissionResult{Release: func() {}, Acquired: true}
	}
	if channel == nil || channel.Id <= 0 {
		return ChannelAdmissionResult{Reason: ChannelAdmissionBackendUnavailable, RetryAfter: channelCapacityBackendRetry}
	}
	setting := operation_setting.GetChannelConcurrencySetting()
	return AcquireChannelCapacityWithMetric(
		channel.Id,
		0,
		ResolveChannelRpmLimit(channel),
		time.Duration(setting.NormalizedRpmWindowSeconds())*time.Second,
		requestID,
	)
}

// ChannelAdmissionRejectReason 表示渠道容量准入失败原因。
type ChannelAdmissionRejectReason string

const (
	ChannelAdmissionAllowed            ChannelAdmissionRejectReason = ""
	ChannelAdmissionConcurrencyLimited ChannelAdmissionRejectReason = "concurrency"
	ChannelAdmissionRpmLimited         ChannelAdmissionRejectReason = "rpm"
	ChannelAdmissionBackendUnavailable ChannelAdmissionRejectReason = "backend_unavailable"
	ChannelAdmissionUpstreamCapacity   ChannelAdmissionRejectReason = "upstream_capacity"
)

// ChannelAdmissionResult 是一次并发 + RPM 原子准入结果。
type ChannelAdmissionResult struct {
	Release    func()
	Acquired   bool
	Reason     ChannelAdmissionRejectReason
	RetryAfter time.Duration
}

// ChannelCapacityUsage 是渠道当前容量使用快照。
type ChannelCapacityUsage struct {
	Inflight int64
	RpmUsed  int64
}

type channelCapacityCandidate struct {
	channel        *model.Channel
	inflight       int64
	maxConcurrency int
	rpmUsed        int64
	rpmLimit       int
}

// RankChannelsByCapacityPressure 按 max(并发使用率, RPM 使用率) 升序排列渠道。
// 查询失败时退化为随机顺序，最终仍由原子准入保证不会超发。
func RankChannelsByCapacityPressure(channels []*model.Channel) []*model.Channel {
	return RankChannelsByCapacityPressureWithLimits(channels, nil)
}

// Limits are advisory for ordering; the caller must recheck before atomic admission.
func RankChannelsByCapacityPressureWithLimits(channels []*model.Channel, limits map[int]int) []*model.Channel {
	if len(channels) <= 1 {
		return channels
	}

	ranked := append([]*model.Channel(nil), channels...)
	rand.Shuffle(len(ranked), func(i, j int) {
		ranked[i], ranked[j] = ranked[j], ranked[i]
	})

	channelIDs := make([]int, 0, len(ranked))
	for _, channel := range ranked {
		channelIDs = append(channelIDs, channel.Id)
	}
	rpmWindow := time.Duration(operation_setting.GetChannelConcurrencySetting().NormalizedRpmWindowSeconds()) * time.Second
	usageByChannel, err := QueryChannelCapacityUsages(channelIDs, rpmWindow)
	if err != nil {
		return ranked
	}

	candidates := make([]channelCapacityCandidate, 0, len(ranked))
	for _, channel := range ranked {
		usage := usageByChannel[channel.Id]
		limit := ResolveChannelMaxConcurrency(channel)
		if dynamic, ok := limits[channel.Id]; ok {
			limit = dynamic
		}
		candidates = append(candidates, channelCapacityCandidate{
			channel:        channel,
			inflight:       usage.Inflight,
			maxConcurrency: limit,
			rpmUsed:        usage.RpmUsed,
			rpmLimit:       ResolveChannelRpmLimit(channel),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return capacityPressureLess(candidates[i], candidates[j])
	})
	for i, candidate := range candidates {
		ranked[i] = candidate.channel
	}
	return ranked
}

func capacityPressureLess(left, right channelCapacityCandidate) bool {
	leftNumerator, leftDenominator := dominantCapacityPressure(left)
	rightNumerator, rightDenominator := dominantCapacityPressure(right)
	return leftNumerator*rightDenominator < rightNumerator*leftDenominator
}

func dominantCapacityPressure(candidate channelCapacityCandidate) (int64, int64) {
	numerator := int64(0)
	denominator := int64(1)
	if candidate.maxConcurrency > 0 {
		numerator = candidate.inflight
		denominator = int64(candidate.maxConcurrency)
	}
	if candidate.rpmLimit > 0 && candidate.rpmUsed*denominator > numerator*int64(candidate.rpmLimit) {
		numerator = candidate.rpmUsed
		denominator = int64(candidate.rpmLimit)
	}
	return numerator, denominator
}

// TryAcquireChannelCapacity 原子申请渠道容量。RPM 名额在成功准入时消费，Release 只释放
// 在途 Lease，不回滚 RPM；这与上游已接收请求后即应计入频率窗口的语义一致。
func TryAcquireChannelCapacity(
	channelID int,
	maxConcurrency int,
	rpmLimit int,
	rpmWindow time.Duration,
	requestID string,
) ChannelAdmissionResult {
	if maxConcurrency <= 0 && rpmLimit <= 0 {
		return ChannelAdmissionResult{Release: func() {}, Acquired: true}
	}
	if rpmWindow <= 0 {
		rpmWindow = time.Minute
	}

	if common.RedisEnabled {
		return tryAcquireRedisChannelCapacity(channelID, maxConcurrency, rpmLimit, rpmWindow, requestID)
	}
	return tryAcquireMemoryChannelCapacityAt(channelID, maxConcurrency, rpmLimit, rpmWindow, time.Time{})
}

func tryAcquireRedisChannelCapacity(
	channelID int,
	maxConcurrency int,
	rpmLimit int,
	rpmWindow time.Duration,
	requestID string,
) ChannelAdmissionResult {
	if common.RDB == nil {
		return ChannelAdmissionResult{Reason: ChannelAdmissionBackendUnavailable, RetryAfter: channelCapacityBackendRetry}
	}

	member := uuid.NewString()
	if requestID = strings.TrimSpace(requestID); requestID != "" {
		member = requestID + ":" + member
	}
	leaseKey := channelLeaseKey(channelID)
	rpmKey := channelRpmKey(channelID)
	leaseKeyTTL := channelLeaseTTL * 2
	rpmKeyTTL := rpmWindow * 2
	if rpmKeyTTL < time.Minute {
		rpmKeyTTL = time.Minute
	}

	values, err := channelCapacityAcquireScript.Run(
		context.Background(),
		common.RDB,
		[]string{leaseKey, rpmKey},
		maxConcurrency,
		rpmLimit,
		rpmWindow.Milliseconds(),
		channelLeaseTTL.Milliseconds(),
		leaseKeyTTL.Milliseconds(),
		rpmKeyTTL.Milliseconds(),
		member,
	).Int64Slice()
	if err != nil || len(values) < 3 {
		return ChannelAdmissionResult{Reason: ChannelAdmissionBackendUnavailable, RetryAfter: channelCapacityBackendRetry}
	}
	if values[0] != 1 {
		reason := ChannelAdmissionConcurrencyLimited
		if values[1] == 2 {
			reason = ChannelAdmissionRpmLimited
		}
		return ChannelAdmissionResult{
			Reason:     reason,
			RetryAfter: time.Duration(values[2]) * time.Millisecond,
		}
	}

	if maxConcurrency <= 0 {
		return ChannelAdmissionResult{Release: func() {}, Acquired: true}
	}
	return ChannelAdmissionResult{
		Release:  redisLeaseReleaseOnce(leaseKey, member),
		Acquired: true,
	}
}

func redisLeaseReleaseOnce(leaseKey, member string) func() {
	var once sync.Once
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(channelLeaseHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if common.RDB == nil {
					continue
				}
				_, _ = channelLeaseHeartbeatScript.Run(
					context.Background(),
					common.RDB,
					[]string{leaseKey},
					member,
					channelLeaseTTL.Milliseconds(),
					(channelLeaseTTL * 2).Milliseconds(),
				).Result()
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			if common.RDB != nil {
				_, _ = channelLeaseReleaseScript.Run(
					context.Background(), common.RDB, []string{leaseKey}, member,
				).Result()
			}
		})
	}
}

func tryAcquireMemoryChannelCapacityAt(
	channelID int,
	maxConcurrency int,
	rpmLimit int,
	rpmWindow time.Duration,
	now time.Time,
) ChannelAdmissionResult {
	state := getChannelCapacityState(channelMemoryCapacityKey(channelID))
	state.mu.Lock()
	if now.IsZero() {
		now = time.Now()
	}
	nowNanos := now.UnixNano()
	cutoff := now.Add(-rpmWindow).UnixNano()
	state.pruneRpmEntriesLocked(cutoff)
	if maxConcurrency > 0 && state.inflight >= int64(maxConcurrency) {
		state.mu.Unlock()
		return ChannelAdmissionResult{Reason: ChannelAdmissionConcurrencyLimited}
	}
	if rpmLimit > 0 && len(state.rpmEntries) >= rpmLimit {
		retryAfter := time.Duration(state.rpmEntries[0]+rpmWindow.Nanoseconds()-nowNanos) * time.Nanosecond
		if retryAfter < time.Millisecond {
			retryAfter = time.Millisecond
		}
		state.mu.Unlock()
		return ChannelAdmissionResult{Reason: ChannelAdmissionRpmLimited, RetryAfter: retryAfter}
	}
	if maxConcurrency > 0 {
		state.inflight++
	}
	if rpmLimit > 0 {
		state.rpmEntries = append(state.rpmEntries, nowNanos)
	}
	state.idleRounds = 0
	state.mu.Unlock()

	if maxConcurrency <= 0 {
		return ChannelAdmissionResult{Release: func() {}, Acquired: true}
	}
	return ChannelAdmissionResult{Release: memoryCapacityReleaseOnce(state), Acquired: true}
}

func (state *channelCapacityState) pruneRpmEntriesLocked(cutoff int64) {
	firstActive := 0
	for firstActive < len(state.rpmEntries) && state.rpmEntries[firstActive] <= cutoff {
		firstActive++
	}
	if firstActive == 0 {
		return
	}
	if firstActive == len(state.rpmEntries) {
		state.rpmEntries = nil
		return
	}
	state.rpmEntries = append(state.rpmEntries[:0], state.rpmEntries[firstActive:]...)
}

func memoryCapacityReleaseOnce(state *channelCapacityState) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			state.mu.Lock()
			if state.inflight > 0 {
				state.inflight--
			}
			state.mu.Unlock()
		})
	}
}

// TryAcquireChannelSlot 保留原并发原语接口，内部统一走新准入器且不启用 RPM。
func TryAcquireChannelSlot(channelID int, maxConcurrency int) (func(), bool) {
	result := TryAcquireChannelCapacity(channelID, maxConcurrency, 0, time.Minute, "")
	return result.Release, result.Acquired
}

// AcquireChannelCapacityWithMetric 在原子准入成功后记录指标，供 controller 编排层使用。
func AcquireChannelCapacityWithMetric(
	channelID int,
	maxConcurrency int,
	rpmLimit int,
	rpmWindow time.Duration,
	requestID string,
) ChannelAdmissionResult {
	result := TryAcquireChannelCapacity(channelID, maxConcurrency, rpmLimit, rpmWindow, requestID)
	if result.Acquired {
		recordChannelAcquire()
	}
	return result
}

// AcquireChannelSlotWithMetric 保留原接口，供尚未迁移的调用方使用。
func AcquireChannelSlotWithMetric(channelID int, maxConcurrency int) (func(), bool) {
	result := AcquireChannelCapacityWithMetric(channelID, maxConcurrency, 0, time.Minute, "")
	return result.Release, result.Acquired
}

// QueryChannelCapacityUsages 批量返回渠道当前在途并发和 RPM 窗口使用量。
func QueryChannelCapacityUsages(channelIDs []int, rpmWindow time.Duration) (map[int]ChannelCapacityUsage, error) {
	result := make(map[int]ChannelCapacityUsage, len(channelIDs))
	if len(channelIDs) == 0 {
		return result, nil
	}
	if rpmWindow <= 0 {
		rpmWindow = time.Minute
	}

	uniqueIDs := make([]int, 0, len(channelIDs))
	seen := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		uniqueIDs = append(uniqueIDs, channelID)
		result[channelID] = ChannelCapacityUsage{}
	}

	if common.RedisEnabled {
		if common.RDB == nil {
			return nil, fmt.Errorf("redis client is nil")
		}
		keys := make([]string, 0, len(uniqueIDs)*2)
		for _, channelID := range uniqueIDs {
			keys = append(keys, channelLeaseKey(channelID), channelRpmKey(channelID))
		}
		values, err := channelCapacityQueryScript.Run(
			context.Background(), common.RDB, keys, rpmWindow.Milliseconds(),
		).Int64Slice()
		if err != nil {
			return nil, err
		}
		if len(values) != len(uniqueIDs)*2 {
			return nil, fmt.Errorf("unexpected channel capacity query result length: %d", len(values))
		}
		for i, channelID := range uniqueIDs {
			result[channelID] = ChannelCapacityUsage{
				Inflight: values[i*2],
				RpmUsed:  values[i*2+1],
			}
		}
		return result, nil
	}

	now := time.Now()
	cutoff := now.Add(-rpmWindow).UnixNano()
	for _, channelID := range uniqueIDs {
		if value, ok := channelCapacityStates.Load(channelMemoryCapacityKey(channelID)); ok {
			state := value.(*channelCapacityState)
			state.mu.Lock()
			state.pruneRpmEntriesLocked(cutoff)
			result[channelID] = ChannelCapacityUsage{
				Inflight: state.inflight,
				RpmUsed:  int64(len(state.rpmEntries)),
			}
			state.mu.Unlock()
		}
	}
	return result, nil
}

// QueryChannelConcurrency 只读返回某渠道当前有效 Lease 数。
func QueryChannelConcurrency(channelID int) (int64, error) {
	usages, err := QueryChannelCapacityUsages(
		[]int{channelID},
		time.Duration(operation_setting.GetChannelConcurrencySetting().NormalizedRpmWindowSeconds())*time.Second,
	)
	if err != nil {
		return 0, err
	}
	return usages[channelID].Inflight, nil
}

// QueryChannelConcurrencies 批量返回渠道当前有效 Lease 数。
func QueryChannelConcurrencies(channelIDs []int) (map[int]int64, error) {
	usages, err := QueryChannelCapacityUsages(
		channelIDs,
		time.Duration(operation_setting.GetChannelConcurrencySetting().NormalizedRpmWindowSeconds())*time.Second,
	)
	if err != nil {
		return nil, err
	}
	result := make(map[int]int64, len(usages))
	for channelID, usage := range usages {
		result[channelID] = usage.Inflight
	}
	return result, nil
}
