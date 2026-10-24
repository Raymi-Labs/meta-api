package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx             *gin.Context
	TokenGroup      string
	ModelName       string
	Retry           *int
	resetNextTry    bool
	AllowedChannels []int // token 允许使用的渠道白名单；为空表示不限制
	ExcludeChannels []int // 需要排除的渠道 ID（重试时跳过已失败的渠道）
	// SameChannelFailoverDone 同渠道额度错误快速切换只允许触发一次，避免重试风暴。
	SameChannelFailoverDone bool
	baseURLChannelCache     map[string][]int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func (p *RetryParam) getSelectionGroup() string {
	if p == nil {
		return ""
	}
	if p.Ctx != nil {
		if autoGroup := common.GetContextKeyString(p.Ctx, constant.ContextKeyAutoGroup); autoGroup != "" {
			return autoGroup
		}
		if usingGroup := common.GetContextKeyString(p.Ctx, constant.ContextKeyUsingGroup); usingGroup != "" && usingGroup != "auto" {
			return usingGroup
		}
	}
	if p.TokenGroup != "auto" {
		return p.TokenGroup
	}
	return ""
}

// getCachedBaseURLChannelIDs 查询同 base_url 下启用的渠道 ID，结果在本次请求生命周期内缓存。
func (p *RetryParam) getCachedBaseURLChannelIDs(baseURL string, allowedChannels []int) []int {
	normalizedBaseURL := model.NormalizeChannelBaseURL(baseURL)
	if normalizedBaseURL == "" {
		return nil
	}
	group := p.getSelectionGroup()
	modelName := p.ModelName
	cacheKey := normalizedBaseURL + "|" + group + "|" + modelName
	if p.baseURLChannelCache != nil {
		if ids, ok := p.baseURLChannelCache[cacheKey]; ok {
			return ids
		}
	}
	ids, err := model.GetEnabledChannelIDsByBaseURL(normalizedBaseURL, allowedChannels)
	if err != nil || len(ids) == 0 {
		return nil
	}
	ids = p.filterChannelIDsForSelection(ids)
	if len(ids) == 0 {
		return nil
	}
	if p.baseURLChannelCache == nil {
		p.baseURLChannelCache = make(map[string][]int)
	}
	p.baseURLChannelCache[cacheKey] = ids
	return ids
}

func (p *RetryParam) filterChannelIDsForSelection(ids []int) []int {
	group := p.getSelectionGroup()
	modelName := p.ModelName
	if group == "" || modelName == "" {
		return ids
	}
	filtered := make([]int, 0, len(ids))
	for _, id := range ids {
		if model.IsChannelEnabledForGroupModel(group, modelName, id) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func IsCodexAutoReviewRequestModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if constant.IsHiddenModel(modelName) {
		return true
	}
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return constant.IsHiddenModel(strings.TrimSuffix(modelName, ratio_setting.CompactModelSuffix))
	}
	return false
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	if param == nil {
		return nil, "", errors.New("retry param is nil")
	}
	if UsesPreferredGroupFallback(param.Ctx, param.TokenGroup) {
		candidates, group, err := preferredGroupCandidates(param)
		if err != nil || len(candidates) == 0 {
			return nil, group, err
		}
		// Preserve the existing weighted selection within this priority tier.
		ids := make([]int, 0, len(candidates))
		for _, channel := range candidates {
			ids = append(ids, channel.Id)
		}
		channel, err := model.GetRandomSatisfiedChannel(group, param.ModelName, 0, ids, param.ExcludeChannels, buildChannelSelectionFilters(param)...)
		if err == nil && channel != nil {
			commitPreferredGroupSelection(param, group)
		}
		return channel, group, err
	}
	return cacheGetRandomSatisfiedChannel(param)
}

func cacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	filters := buildChannelSelectionFilters(param)

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroup(userGroup)

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			channel, _ = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, priorityRetry, param.AllowedChannels, param.ExcludeChannels, filters...)
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.AllowedChannels, param.ExcludeChannels, filters...)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

// CacheGetSatisfiedChannelCandidates 返回当前分组与重试优先级下的全部候选渠道，
// 不在这里随机选中单个渠道，供 controller 按实时容量压力排序后执行原子准入。
func CacheGetSatisfiedChannelCandidates(param *RetryParam) ([]*model.Channel, string, error) {
	if param == nil {
		return nil, "", errors.New("retry param is nil")
	}
	if UsesPreferredGroupFallback(param.Ctx, param.TokenGroup) {
		return preferredGroupCandidates(param)
	}

	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	filters := buildChannelSelectionFilters(param)

	if param.TokenGroup != "auto" {
		channels, err := model.GetSatisfiedChannelCandidates(
			param.TokenGroup,
			param.ModelName,
			param.GetRetry(),
			param.AllowedChannels,
			param.ExcludeChannels,
			filters...,
		)
		return channels, selectGroup, err
	}

	if len(setting.GetAutoGroups()) == 0 {
		return nil, selectGroup, errors.New("auto groups is not enabled")
	}
	autoGroups := GetUserAutoGroup(userGroup)
	startGroupIndex := 0
	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			startGroupIndex = idx
		}
	}

	for i := startGroupIndex; i < len(autoGroups); i++ {
		autoGroup := autoGroups[i]
		priorityRetry := param.GetRetry()
		if i > startGroupIndex {
			priorityRetry = 0
		}
		logger.LogDebug(param.Ctx, "Auto selecting capacity candidates: group=%s, priorityRetry=%d", autoGroup, priorityRetry)

		channels, err := model.GetSatisfiedChannelCandidates(
			autoGroup,
			param.ModelName,
			priorityRetry,
			param.AllowedChannels,
			param.ExcludeChannels,
			filters...,
		)
		if err != nil {
			return nil, autoGroup, err
		}
		if len(channels) == 0 {
			continue
		}

		return channels, autoGroup, nil
	}

	return nil, selectGroup, nil
}

// CommitChannelCandidateSelection 在候选渠道完成原子容量申请并成功写入上下文后，
// 提交 auto 分组的选择状态。候选读取阶段不提前修改这些状态，避免所有渠道满载等待时
// 把 auto 分组索引推进到末尾，导致后续轮询无法重新探测容量。
func CommitChannelCandidateSelection(param *RetryParam, selectGroup string) {
	if param != nil && UsesPreferredGroupFallback(param.Ctx, param.TokenGroup) && selectGroup != "" {
		commitPreferredGroupSelection(param, selectGroup)
		return
	}
	if param == nil || param.TokenGroup != "auto" || selectGroup == "" {
		return
	}
	autoGroups := GetUserAutoGroup(common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup))
	selectedIndex := -1
	for i, group := range autoGroups {
		if group == selectGroup {
			selectedIndex = i
			break
		}
	}
	if selectedIndex < 0 {
		return
	}

	startGroupIndex := 0
	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			startGroupIndex = idx
		}
	}
	priorityRetry := param.GetRetry()
	if selectedIndex > startGroupIndex {
		priorityRetry = 0
	}

	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectGroup)
	if common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry) && priorityRetry >= common.RetryTimes {
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, selectedIndex+1)
		param.SetRetry(0)
		param.ResetRetryNextTry()
		return
	}
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, selectedIndex)
}

func buildChannelSelectionFilters(param *RetryParam) []model.ChannelFilter {
	filters := make([]model.ChannelFilter, 0, 3)
	if clientFilter := buildClientFilter(param.Ctx); clientFilter != nil {
		filters = append(filters, clientFilter)
	}
	filters = append(filters, func(ch *model.Channel) bool {
		return !IsChannelUnavailableForRequestContext(param.Ctx, ch) &&
			!IsChannelModelCircuitOpen(ch, param.ModelName)
	})
	if IsCodexAutoReviewRequestModel(param.ModelName) {
		filters = append(filters, func(ch *model.Channel) bool {
			return ch != nil && constant.IsCodexAutoReviewCompatibleChannelType(ch.Type)
		})
	}
	return filters
}

// buildClientFilter creates a ChannelFilter that checks client restriction settings.
// Returns nil if no client ID is detected (meaning no filtering needed).
func buildClientFilter(ctx *gin.Context) model.ChannelFilter {
	if ctx == nil {
		return nil
	}
	clientID := common.GetContextKeyString(ctx, constant.ContextKeyClientID)
	if clientID == "" {
		return nil
	}
	return func(ch *model.Channel) bool {
		return IsChannelAllowedForClient(ch, clientID)
	}
}
