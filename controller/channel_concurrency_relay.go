package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// channel_concurrency_relay.go 是渠道并发控制（含 Layer C 有界过载处理）的编排层。
//
// 它先获取当前优先级下的全部候选渠道，按 inflight/maxConcurrency 的实时压力排序，
// 再依次原子申请名额；若该渠道已满则自动换到其它未满渠道（不消耗外层 RetryTimes）。
// 当所有候选渠道都满时进入
// Layer C——有界等待容量释放：
//   - 客户端断开：立即取消等待（不返回错误响应，客户端已走）；
//   - 等待超时：仅 RPM 满返回 429，并发满或混合满返回 503，同时携带 Retry-After；
//   - 等待队列已满：按当前瓶颈快速失败 429/503，防止过载时等待请求无界堆积；
//   - 等到容量：清除因满而排除的渠道，重新选择并占用。
//
// 关键正确性保证：
//   - 等待逻辑全程在请求自身 goroutine 内 select；Redis Lease 仅为已准入请求创建可由 release 终止的心跳；
//   - 等待时长由 WaitTimeoutMs 上界，等待并发数由 MaxQueueLength 上界（双重有界）；
//   - 返回的 releaseSlot 由调用方在本轮上游调用结束后 defer 释放，覆盖所有退出路径。
//
// 未启用（Enabled=false）时完全短路，直接透传 getChannel，行为与未引入本功能一致。

// selectChannelWithConcurrency 选择一个可用且通过并发 + RPM 原子准入的渠道。
// 返回 (channel, releaseSlot, apiErr, isOverloadControl)：
//   - 成功：channel 非 nil，releaseSlot 非 nil（须在本轮上游调用后释放一次）；
//   - 客户端在等待中断开：apiErr 为可跳过重试的 client-canceled 错误，isOverloadControl=true；
//   - 等待超时/队列满：仅 RPM 满为 429，其它容量不足为 503（均设置 Retry-After），isOverloadControl=true；
//   - 真无可用渠道（非并发原因）：透传 getChannel 的原始错误，isOverloadControl=false。
//
// isOverloadControl=true 表示这是 Layer C 过载控制的终态错误，调用方应直接采用它，
// 不要用历史重试错误（lastRelayError）覆盖——因为“系统整体过载”比“之前某次上游报错”
// 对用户更有指导性。未启用并发控制时 releaseSlot 为 nil、isOverloadControl=false。
func selectChannelWithConcurrency(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	retryParam *service.RetryParam,
) (*model.Channel, func(), *types.NewAPIError, bool) {
	if _, ok := c.Get(service.AudioVoiceContext); ok {
		ch, err := middleware.SelectAudioVoiceChannel(c, retryParam.ModelName, retryParam.TokenGroup, service.DetectClient(c))
		if err != nil {
			return nil, nil, err, false
		}
		info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
		release, capacityErr := acquireFixedChannelCapacityWithWait(c, ch)
		return ch, release, capacityErr, capacityErr != nil
	}
	if !middleware.ChannelConcurrencyEnabled() {
		channel, apiErr := getChannel(c, info, retryParam)
		return channel, nil, apiErr, false
	}

	setting := operation_setting.GetChannelConcurrencySetting()
	waitTimeout := time.Duration(setting.NormalizedWaitTimeoutMs()) * time.Millisecond
	pollInterval := time.Duration(setting.NormalizedPollIntervalMs()) * time.Millisecond
	rpmWindow := time.Duration(setting.NormalizedRpmWindowSeconds()) * time.Second
	maxQueueLen := setting.NormalizedMaxQueueLength()
	deadline := time.Now().Add(waitTimeout)

	// ---- 阶段1：亲和粘性优先（保上游 prompt cache 命中）----
	// 仅在首轮（未发生真实上游失败）尝试。重试轮意味着亲和渠道可能真的故障了，
	// 不该再死粘它，直接进入阶段2 的正常负载均衡以保障请求成功。
	if setting.AffinityStickyEnabled && retryParam.GetRetry() == 0 {
		ch, release, outcome := tryStickToAffinityChannel(c, info, retryParam, setting, deadline)
		switch outcome {
		case affinityStickAcquired:
			return ch, release, nil, false
		case affinityStickClientGone:
			return nil, nil, buildClientCanceledError(), true
		case affinityStickSkip, affinityStickDegrade:
			// 无亲和绑定 / 亲和渠道不可用 / 等待其超时 → 落到阶段2 正常选择（保体验）。
		}
	}

	// ---- 阶段2：正常负载均衡 + 并发控制（有界过载兜底）----
	// 本次请求内因“并发或 RPM 容量不足”而被临时排除的渠道集合。
	blockedChannels := make(map[int]bool)
	var blockSummary channelCapacityBlockSummary
	inWaitQueue := false
	var waitStart time.Time
	defer func() {
		if inWaitQueue {
			middleware.LeaveChannelWaitQueue()
			middleware.RecordChannelWaitDuration(time.Since(waitStart))
		}
	}()

	for {
		candidates, selectGroup, chErr := getChannelCapacityCandidates(c, info, retryParam)

		needWait := false
		switch {
		case chErr != nil:
			if len(blockedChannels) == 0 {
				// 与并发无关的“无可用渠道”（如模型确实无渠道）：透传原始错误。
				return nil, nil, chErr, false
			}
			// 有渠道，但都被容量准入临时排除了 → 进入等待。
			needWait = true
		default:
			// 同优先级候选按 max(并发占比, RPM 占比) 从低到高排列；压力相同时随机打散，
			// 避免所有实例同时命中固定渠道。快照只负责排序，最终由原子准入裁决。
			candidates = middleware.RankChannelsByCapacityPressure(candidates)
			for _, channel := range candidates {
				maxConcurrency := middleware.ResolveChannelMaxConcurrency(channel)
				rpmLimit := middleware.ResolveChannelRpmLimit(channel)
				admission := middleware.AcquireChannelCapacityWithMetric(
					channel.Id, maxConcurrency, rpmLimit, rpmWindow, c.GetString(common.RequestIdKey),
				)
				if !admission.Acquired {
					if admission.Reason == middleware.ChannelAdmissionBackendUnavailable {
						return nil, nil, buildChannelCapacityError(c, http.StatusServiceUnavailable, "channel capacity backend unavailable", admission.RetryAfter), true
					}
					blockSummary.Record(admission)
					blockedChannels[channel.Id] = true
					retryParam.ExcludeChannels = appendUniqueInt(retryParam.ExcludeChannels, channel.Id)
					continue
				}
				release := admission.Release

				// 只有原子申请容量成功后才正式写入渠道上下文，避免“先选死渠道再等待”的旧流程。
				setupErr := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
				if setupErr != nil {
					release()
					if !service.ShouldFallbackAfterSetupError(setupErr) {
						return nil, nil, setupErr, false
					}
					retryParam.ExcludeChannels = appendUniqueInt(retryParam.ExcludeChannels, channel.Id)
					continue
				}
				service.CommitChannelCandidateSelection(retryParam, selectGroup)
				info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

				// 临时容量排除只在本轮选渠期间生效，成功后恢复，避免影响后续真实失败重试。
				if len(blockedChannels) > 0 {
					retryParam.ExcludeChannels = removeChannelsFromExclusion(retryParam.ExcludeChannels, blockedChannels)
				}
				if inWaitQueue {
					middleware.RecordChannelWaitAcquired()
				}
				return channel, release, nil, false
			}

			// 当前优先级候选都容量不足或初始化失败，继续获取下一优先级/下一分组候选。
			continue
		}

		if !needWait {
			continue
		}

		// ---- Layer C 有界等待 ----
		remaining := time.Until(deadline)
		if remaining <= 0 {
			middleware.RecordChannelWaitTimeout()
			return nil, nil, buildBlockedChannelCapacityError(c, setting, blockSummary, "all channels reached capacity limit"), true
		}
		// RPM 名额只能随窗口推进恢复；若最短恢复时间已超过本层等待预算，直接返回精确 429。
		if blockSummary.OnlyRpmLimited() && blockSummary.RetryAfter > remaining {
			middleware.RecordChannelWaitTimeout()
			return nil, nil, buildBlockedChannelCapacityError(c, setting, blockSummary, "all channels reached rpm limit"), true
		}

		if !inWaitQueue {
			if !middleware.EnterChannelWaitQueue(maxQueueLen) {
				// 等待队列已满：快速失败，防止过载时等待请求无界堆积。
				middleware.RecordChannelQueueReject()
				return nil, nil, buildBlockedChannelCapacityError(c, setting, blockSummary, "channel capacity wait queue is full"), true
			}
			inWaitQueue = true
			waitStart = time.Now()
			middleware.RecordChannelWaitEnter()
		}

		wait := pollInterval
		if blockSummary.OnlyRpmLimited() && blockSummary.RetryAfter > 0 {
			wait = blockSummary.RetryAfter
		}
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			middleware.RecordChannelWaitCancel()
			return nil, nil, buildClientCanceledError(), true
		case <-timer.C:
			// 一个轮询周期过去：清除容量临时排除，让渠道重新参与选择。
			retryParam.ExcludeChannels = removeChannelsFromExclusion(retryParam.ExcludeChannels, blockedChannels)
			blockedChannels = make(map[int]bool)
			blockSummary = channelCapacityBlockSummary{}
			continue
		}
	}
}

// acquireFixedChannelCapacityWithWait 为必须锁定渠道的请求申请容量，例如任务 remix。
func acquireFixedChannelCapacityWithWait(c *gin.Context, channel *model.Channel) (func(), *types.NewAPIError) {
	if channel == nil {
		return nil, types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if !middleware.ChannelConcurrencyEnabled() {
		return nil, nil
	}
	if remaining := service.ChannelRateLimitCooldownRemaining(channel.Id); remaining > 0 {
		return nil, buildChannelCapacityError(c, http.StatusTooManyRequests, "channel is cooling down after upstream rate limit", remaining)
	}

	setting := operation_setting.GetChannelConcurrencySetting()
	deadline := time.Now().Add(time.Duration(setting.NormalizedWaitTimeoutMs()) * time.Millisecond)
	pollInterval := time.Duration(setting.NormalizedPollIntervalMs()) * time.Millisecond
	rpmWindow := time.Duration(setting.NormalizedRpmWindowSeconds()) * time.Second
	maxQueueLen := setting.NormalizedMaxQueueLength()
	maxConcurrency := middleware.ResolveChannelMaxConcurrency(channel)
	rpmLimit := middleware.ResolveChannelRpmLimit(channel)
	var summary channelCapacityBlockSummary
	inWaitQueue := false
	var waitStart time.Time
	defer func() {
		if inWaitQueue {
			middleware.LeaveChannelWaitQueue()
			middleware.RecordChannelWaitDuration(time.Since(waitStart))
		}
	}()

	for {
		admission := middleware.AcquireChannelCapacityWithMetric(
			channel.Id, maxConcurrency, rpmLimit, rpmWindow, c.GetString(common.RequestIdKey),
		)
		if admission.Acquired {
			if inWaitQueue {
				middleware.RecordChannelWaitAcquired()
			}
			return admission.Release, nil
		}
		if admission.Reason == middleware.ChannelAdmissionBackendUnavailable {
			return nil, buildChannelCapacityError(c, http.StatusServiceUnavailable, "channel capacity backend unavailable", admission.RetryAfter)
		}
		summary.Record(admission)

		remaining := time.Until(deadline)
		if remaining <= 0 || (summary.OnlyRpmLimited() && summary.RetryAfter > remaining) {
			middleware.RecordChannelWaitTimeout()
			return nil, buildBlockedChannelCapacityError(c, setting, summary, "fixed channel reached capacity limit")
		}
		if !inWaitQueue {
			if !middleware.EnterChannelWaitQueue(maxQueueLen) {
				middleware.RecordChannelQueueReject()
				return nil, buildBlockedChannelCapacityError(c, setting, summary, "channel capacity wait queue is full")
			}
			inWaitQueue = true
			waitStart = time.Now()
			middleware.RecordChannelWaitEnter()
		}

		wait := pollInterval
		if summary.OnlyRpmLimited() && summary.RetryAfter > 0 {
			wait = summary.RetryAfter
		}
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			middleware.RecordChannelWaitCancel()
			return nil, buildClientCanceledError()
		case <-timer.C:
			summary = channelCapacityBlockSummary{}
		}
	}
}

// getChannelCapacityCandidates 获取当前重试层级的全部候选渠道，不提前固定单个渠道。
// 指定渠道在首轮仍优先尝试；若其满载被临时排除，后续可按原有降级语义尝试其它渠道。
func getChannelCapacityCandidates(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	retryParam *service.RetryParam,
) ([]*model.Channel, string, *types.NewAPIError) {
	if info.ChannelMeta == nil && retryParam.GetRetry() == 0 {
		if _, specific := c.Get("specific_channel_id"); specific {
			channelID := c.GetInt("channel_id")
			if !containsChannelID(retryParam.ExcludeChannels, channelID) {
				if selected, err := model.CacheGetChannel(channelID); err == nil && selected != nil &&
					!service.IsChannelRateLimitCoolingDown(selected) &&
					!service.IsChannelModelCircuitOpen(selected, retryParam.ModelName) {
					return []*model.Channel{selected}, "", nil
				}
			}
		}
	}

	channels, selectGroup, err := service.CacheGetSatisfiedChannelCandidates(retryParam)
	if err != nil {
		return nil, selectGroup, types.NewError(
			fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（capacity）: %s", selectGroup, info.OriginModelName, err.Error()),
			types.ErrorCodeGetChannelFailed,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if len(channels) == 0 {
		return nil, selectGroup, types.NewError(
			fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（capacity）", selectGroup, info.OriginModelName),
			types.ErrorCodeGetChannelFailed,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return channels, selectGroup, nil
}

func containsChannelID(ids []int, target int) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

type channelCapacityBlockSummary struct {
	ConcurrencyLimited bool
	RpmLimited         bool
	RetryAfter         time.Duration
}

func (s *channelCapacityBlockSummary) Record(admission middleware.ChannelAdmissionResult) {
	switch admission.Reason {
	case middleware.ChannelAdmissionConcurrencyLimited:
		s.ConcurrencyLimited = true
	case middleware.ChannelAdmissionRpmLimited:
		s.RpmLimited = true
		if admission.RetryAfter > 0 && (s.RetryAfter <= 0 || admission.RetryAfter < s.RetryAfter) {
			s.RetryAfter = admission.RetryAfter
		}
	}
}

func (s channelCapacityBlockSummary) OnlyRpmLimited() bool {
	return s.RpmLimited && !s.ConcurrencyLimited
}

func buildBlockedChannelCapacityError(
	c *gin.Context,
	setting *operation_setting.ChannelConcurrencySetting,
	summary channelCapacityBlockSummary,
	reason string,
) *types.NewAPIError {
	if summary.OnlyRpmLimited() {
		retryAfter := summary.RetryAfter
		if retryAfter <= 0 {
			retryAfter = time.Duration(setting.NormalizedRetryAfterSeconds()) * time.Second
		}
		return buildChannelCapacityError(c, http.StatusTooManyRequests, reason, retryAfter)
	}
	return buildChannelCapacityError(
		c, http.StatusServiceUnavailable, reason,
		time.Duration(setting.NormalizedRetryAfterSeconds())*time.Second,
	)
}

// buildChannelCapacityError 构造容量错误，并设置向上取整的 Retry-After。
func buildChannelCapacityError(c *gin.Context, statusCode int, reason string, retryAfter time.Duration) *types.NewAPIError {
	retryAfterSeconds := int((retryAfter + time.Second - 1) / time.Second)
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	if c != nil {
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	}
	apiErr := types.NewErrorWithStatusCode(
		errors.New(reason+", please retry later"),
		types.ErrorCodeGetChannelFailed,
		statusCode,
		types.ErrOptionWithSkipRetry(),
	)
	apiErr.RetryAfter = time.Duration(retryAfterSeconds) * time.Second
	return apiErr
}

// buildClientCanceledError 构造客户端断开取消的错误：跳过重试、隐藏内部细节，不污染错误率指标。
func buildClientCanceledError() *types.NewAPIError {
	return types.NewClientCanceledError(
		types.ErrOptionWithHideErrMsg("client canceled while waiting for channel capacity"),
	)
}

// affinityStickOutcome 表示"尽力粘住亲和渠道"的结果。
type affinityStickOutcome int

const (
	affinityStickSkip       affinityStickOutcome = iota // 无亲和绑定，跳过粘性（新会话走正常选择，自然建立绑定）
	affinityStickAcquired                               // 成功粘住亲和渠道并占用名额（缓存最优）
	affinityStickClientGone                             // 等待亲和渠道时客户端断开
	affinityStickDegrade                                // 亲和渠道不可用/等待超时/队列满 → 降级到正常负载均衡
)

// tryStickToAffinityChannel 尽力把请求粘在其亲和绑定渠道上，以命中上游 prompt cache。
//
// 缓存与体验的平衡点：
//   - 亲和渠道可用且未满 → 立即占用并返回（缓存最优，无额外延迟）；
//   - 亲和渠道并发满 → 在 AffinityWaitMs 内有界等待它释放，等到即用（保住缓存局部性）；
//   - 亲和渠道熔断/禁用/等待超时/等待队列已满 → 返回 degrade，交由阶段2 正常负载均衡兜底（保体验）；
//   - 客户端断开 → 立即取消（不空等）。
//
// 全程在请求自身 goroutine 内 select，等待时长由 AffinityWaitMs 上界、等待并发由队列上界，无额外 goroutine。
// 返回 acquired 时已通过 SetupContextForSelectedChannel 把渠道写入 context，调用方可直接转发。
func tryStickToAffinityChannel(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	retryParam *service.RetryParam,
	setting *operation_setting.ChannelConcurrencySetting,
	overallDeadline time.Time,
) (*model.Channel, func(), affinityStickOutcome) {
	// 复用 distributor 的完整亲和校验（可用性/熔断、token 与客户端权限、codex 兼容、
	// 分组内模型启用、auto 分组展开），避免自行实现校验遗漏把请求粘到无权/未启用的渠道。
	affinityCh, selectGroup, ok := middleware.ResolveAffinityChannelForRelay(
		c, info.OriginModelName, info.UsingGroup, retryParam.ExcludeChannels,
	)
	if !ok {
		// 无亲和绑定，或绑定渠道不可用/无权限 → 交由阶段2 正常负载均衡（新会话会自然建立绑定）。
		return nil, nil, affinityStickSkip
	}

	maxConcurrency := middleware.ResolveChannelMaxConcurrency(affinityCh)
	rpmLimit := middleware.ResolveChannelRpmLimit(affinityCh)
	rpmWindow := time.Duration(setting.NormalizedRpmWindowSeconds()) * time.Second
	// 亲和等待预算：不超过 AffinityWaitMs，也不超过总 deadline（把剩余时间留给降级阶段的等待）。
	affinityDeadline := time.Now().Add(time.Duration(setting.NormalizedAffinityWaitMs()) * time.Millisecond)
	if affinityDeadline.After(overallDeadline) {
		affinityDeadline = overallDeadline
	}
	pollInterval := time.Duration(setting.NormalizedPollIntervalMs()) * time.Millisecond
	maxQueueLen := setting.NormalizedMaxQueueLength()

	inWaitQueue := false
	var waitStart time.Time
	defer func() {
		if inWaitQueue {
			middleware.LeaveChannelWaitQueue()
			middleware.RecordChannelWaitDuration(time.Since(waitStart))
		}
	}()

	for {
		admission := middleware.AcquireChannelCapacityWithMetric(
			affinityCh.Id, maxConcurrency, rpmLimit, rpmWindow, c.GetString(common.RequestIdKey),
		)
		if admission.Acquired {
			release := admission.Release
			// 阶段1 绕过了 getChannel，需自己把亲和渠道写入 context 供后续转发链路使用。
			if setupErr := middleware.SetupContextForSelectedChannel(c, affinityCh, info.OriginModelName); setupErr != nil {
				release()
				return nil, nil, affinityStickDegrade
			}
			// 与 distributor 路径对齐：标记亲和已用（供 SkipRetryOnFailure 规则与 admin 日志/统计生效），
			// 用 tryAffinityChannel 返回的生效分组，避免 auto 分组下的 group 维度错乱。
			if service.HasTokenRoutingConfig(c) {
				service.CommitChannelCandidateSelection(retryParam, selectGroup)
				info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
			}
			service.MarkChannelAffinityUsed(c, selectGroup, affinityCh.Id)
			if inWaitQueue {
				middleware.RecordChannelWaitAcquired()
			}
			return affinityCh, release, affinityStickAcquired
		}
		if admission.Reason == middleware.ChannelAdmissionBackendUnavailable {
			return nil, nil, affinityStickDegrade
		}

		// 亲和渠道容量不足：在预算内有界等待（并发释放或 RPM 窗口推进）。
		if !inWaitQueue {
			if !middleware.EnterChannelWaitQueue(maxQueueLen) {
				// 等待队列已满 → 降级换渠道，保障体验（不无界堆积）。
				middleware.RecordChannelQueueReject()
				return nil, nil, affinityStickDegrade
			}
			inWaitQueue = true
			waitStart = time.Now()
			middleware.RecordChannelWaitEnter()
		}

		remaining := time.Until(affinityDeadline)
		if remaining <= 0 {
			// 等待亲和渠道超时 → 降级换渠道（用有界延迟换缓存，超时则让位给可用性）。
			middleware.RecordChannelWaitTimeout()
			return nil, nil, affinityStickDegrade
		}
		wait := pollInterval
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			middleware.RecordChannelWaitCancel()
			return nil, nil, affinityStickClientGone
		case <-timer.C:
			// 继续尝试占用亲和渠道（可能已释放容量）。
		}
	}
}

// removeChannelsFromExclusion 返回移除了 remove 集合中所有渠道 ID 的新排除列表。
func removeChannelsFromExclusion(exclude []int, remove map[int]bool) []int {
	if len(remove) == 0 || len(exclude) == 0 {
		return exclude
	}
	result := make([]int, 0, len(exclude))
	for _, id := range exclude {
		if !remove[id] {
			result = append(result, id)
		}
	}
	return result
}
