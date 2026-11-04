package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const vmCapacityStartKey = "vm_capacity_request_started"
const vmCapacityDeadlineKey = "vm_capacity_request_deadline"

func capacityDeadline(c *gin.Context, ch *model.Channel, fallback time.Duration) time.Time {
	value, ok := c.Get(vmCapacityStartKey)
	if !ok {
		value = time.Now()
		c.Set(vmCapacityStartKey, value)
	}
	start := value.(time.Time)
	if !service.VMCapacityEnabled(ch) {
		if old, ok := c.Get(vmCapacityDeadlineKey); ok {
			return old.(time.Time)
		}
		return time.Now().Add(fallback)
	}
	if service.VMCapacityEnabled(ch) && ch.GetSetting().UpstreamCapacity.PreferWait() {
		fallback = time.Duration(ch.GetSetting().UpstreamCapacity.WaitDurationMs()) * time.Millisecond
	}
	deadline := start.Add(fallback)
	if old, ok := c.Get(vmCapacityDeadlineKey); ok && old.(time.Time).After(deadline) {
		deadline = old.(time.Time)
	}
	if d, ok := c.Request.Context().Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c.Set(vmCapacityDeadlineKey, deadline)
	return deadline
}

func acquireRelayChannelCapacity(c *gin.Context, ch *model.Channel, modelName string) middleware.ChannelAdmissionResult {
	limit := middleware.ResolveChannelMaxConcurrency(ch)
	if service.VMCapacityEnabled(ch) {
		if !middleware.ChannelConcurrencyEnabled() {
			return middleware.ChannelAdmissionResult{Reason: middleware.ChannelAdmissionUpstreamCapacity, RetryAfter: time.Second}
		}
		ctx := c.Request.Context()
		if value, ok := c.Get(vmCapacityDeadlineKey); ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, value.(time.Time))
			defer cancel()
		}
		snapshot, err := service.GetVMChannelCapacity(ctx, ch, modelName)
		if err != nil || snapshot.EffectiveAdmissionLimit <= 0 || snapshot.AvailableNow <= 0 {
			return middleware.ChannelAdmissionResult{Reason: middleware.ChannelAdmissionUpstreamCapacity, RetryAfter: time.Second}
		}
		limit = min(limit, snapshot.EffectiveAdmissionLimit)
	}
	setting := operation_setting.GetChannelConcurrencySetting()
	return middleware.AcquireChannelCapacityWithMetric(ch.Id, limit, middleware.ResolveChannelRpmLimit(ch), time.Duration(setting.NormalizedRpmWindowSeconds())*time.Second, c.GetString(common.RequestIdKey))
}

func rankRelayChannels(c *gin.Context, channels []*model.Channel, modelName string) []*model.Channel {
	limits := map[int]int{}
	var ready, blocked []*model.Channel
	for _, ch := range channels {
		if service.VMCapacityEnabled(ch) {
			s, err := service.GetVMChannelCapacity(c.Request.Context(), ch, modelName)
			if err != nil || s.AvailableNow <= 0 || s.EffectiveAdmissionLimit <= 0 {
				blocked = append(blocked, ch)
				continue
			}
			limits[ch.Id] = min(middleware.ResolveChannelMaxConcurrency(ch), s.EffectiveAdmissionLimit)
		}
		ready = append(ready, ch)
	}
	return append(middleware.RankChannelsByCapacityPressureWithLimits(ready, limits), blocked...)
}

// Wait only in the caller goroutine, without holding a lease or resetting the request deadline.
func waitVMChannelCapacity(c *gin.Context, ch *model.Channel, modelName, group string, deadline time.Time) middleware.ChannelAdmissionResult {
	setting := operation_setting.GetChannelConcurrencySetting()
	entered := false
	var started time.Time
	defer func() {
		if entered {
			middleware.LeaveChannelWaitQueue()
			middleware.RecordChannelWaitDuration(time.Since(started))
		}
	}()
	for {
		if c.Request.Context().Err() != nil {
			return middleware.ChannelAdmissionResult{Reason: middleware.ChannelAdmissionUpstreamCapacity}
		}
		if time.Until(deadline) <= 0 {
			middleware.RecordChannelWaitTimeout()
			return middleware.ChannelAdmissionResult{Reason: middleware.ChannelAdmissionUpstreamCapacity}
		}
		if model.DB != nil {
			current, err := model.CacheGetChannel(ch.Id)
			if group == "" {
				group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
			}
			if err != nil || current == nil || service.IsChannelUnavailableForRequestContext(c, current) || service.IsChannelModelCircuitOpen(current, modelName) ||
				!model.IsChannelEnabledForGroupModel(group, modelName, ch.Id) {
				return middleware.ChannelAdmissionResult{Reason: middleware.ChannelAdmissionUpstreamCapacity}
			}
			ch = current
		}
		admission := acquireRelayChannelCapacity(c, ch, modelName)
		if admission.Acquired || admission.Reason == middleware.ChannelAdmissionBackendUnavailable {
			if entered && admission.Acquired {
				middleware.RecordChannelWaitAcquired()
			}
			return admission
		}
		if time.Until(deadline) <= 0 {
			middleware.RecordChannelWaitTimeout()
			return admission
		}
		if !entered {
			if !middleware.EnterChannelWaitQueue(setting.NormalizedMaxQueueLength()) {
				middleware.RecordChannelQueueReject()
				return admission
			}
			entered = true
			started = time.Now()
			middleware.RecordChannelWaitEnter()
		}
		// A stale health snapshot must not cause a 50ms storm of upstream queries.
		timer := time.NewTimer(min(time.Until(deadline), time.Duration(setting.NormalizedPollIntervalMs())*time.Millisecond))
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			middleware.RecordChannelWaitCancel()
			return admission
		case <-timer.C:
		}
	}
}

func vmCapacityRetrySafe(c *gin.Context, err *types.NewAPIError) bool {
	if err == nil || c.Request.Context().Err() != nil {
		return false
	}
	return helper.CanAdjustClaudeRequestBeforeOutput(c)
}

// Used only for fixed non-Claude paths, which must not bypass an enabled VM gate.
func fixedCapacityModel(c *gin.Context) string {
	return common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
}
