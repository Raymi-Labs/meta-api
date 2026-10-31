package helper

import (
	"fmt"
	"github.com/tidwall/sjson"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const defaultConservativeCompletionTokens = 4096

func shouldApplyDefaultCompletionReserve(info *relaycommon.RelayInfo, priceData types.PriceData) bool {
	if priceData.UsePrice || priceData.UseAudioDurationPrice || priceData.ImageRatio > 1 {
		return false
	}
	if info == nil {
		return true
	}
	_, isImageRequest := info.Request.(*dto.ImageRequest)
	return !isImageRequest
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = ratio_setting.ClaudeCacheCreation1hMultiplier

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, fmt.Sprintf("final group: %s", autoGroup))
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func estimateConservativeBuiltInToolQuota(info *relaycommon.RelayInfo, priceData types.PriceData) decimal.Decimal {
	if info == nil {
		return decimal.Zero
	}

	groupRatio := decimal.NewFromFloat(priceData.GroupRatioInfo.GroupRatio).
		Mul(decimal.NewFromFloat(priceData.TimeRatioInfo.EffectiveRatio()))
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	toolQuota := decimal.Zero

	if info.ResponsesUsageInfo != nil {
		if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
			searchContextSize := webSearchTool.SearchContextSize
			if searchContextSize == "" {
				searchContextSize = "medium"
			}
			toolQuota = toolQuota.Add(
				decimal.NewFromFloat(operation_setting.GetToolPriceForModel(dto.BuildInToolWebSearchPreview, info.OriginModelName)).
					Div(decimal.NewFromInt(1000)).
					Mul(groupRatio).
					Mul(quotaPerUnit),
			)
		}
		if fileSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool != nil {
			toolQuota = toolQuota.Add(
				decimal.NewFromFloat(operation_setting.GetToolPriceForModel(dto.BuildInToolFileSearch, info.OriginModelName)).
					Div(decimal.NewFromInt(1000)).
					Mul(groupRatio).
					Mul(quotaPerUnit),
			)
		}
	}

	if strings.HasSuffix(info.OriginModelName, "search-preview") {
		toolQuota = toolQuota.Add(
			decimal.NewFromFloat(operation_setting.GetToolPriceForModel(dto.BuildInToolWebSearchPreview, info.OriginModelName)).
				Div(decimal.NewFromInt(1000)).
				Mul(groupRatio).
				Mul(quotaPerUnit),
		)
	}

	if request, ok := info.Request.(*dto.GeneralOpenAIRequest); ok && request.WebSearchOptions != nil && info.RelayFormat == types.RelayFormatClaude {
		toolQuota = toolQuota.Add(
			decimal.NewFromFloat(operation_setting.GetToolPriceForModel(dto.BuildInToolWebSearch, info.OriginModelName)).
				Div(decimal.NewFromInt(1000)).
				Mul(groupRatio).
				Mul(quotaPerUnit),
		)
	}

	return toolQuota
}

func EstimateConservativePreConsumeQuota(info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, priceData types.PriceData) int {
	if meta == nil {
		meta = &types.TokenCountMeta{}
	}

	conservativePromptTokens := common.Max(promptTokens, common.PreConsumedQuota)
	conservativeCompletionTokens := common.Max(meta.MaxTokens, 0)
	if conservativeCompletionTokens == 0 && shouldApplyDefaultCompletionReserve(info, priceData) {
		conservativeCompletionTokens = defaultConservativeCompletionTokens
	}
	groupRatio := decimal.NewFromFloat(priceData.GroupRatioInfo.GroupRatio).
		Mul(decimal.NewFromFloat(priceData.TimeRatioInfo.EffectiveRatio()))
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	quota := decimal.Zero
	if priceData.UseAudioDurationPrice {
		quota = decimal.NewFromInt(int64(CalculateAudioDurationPreConsumeQuota(priceData)))
	} else if priceData.UsePrice {
		quota = decimal.NewFromFloat(priceData.ModelPrice).Mul(quotaPerUnit).Mul(groupRatio)
	} else {
		promptQuota := decimal.NewFromInt(int64(conservativePromptTokens))
		if priceData.ImageRatio > 1 {
			promptQuota = promptQuota.Mul(decimal.NewFromFloat(priceData.ImageRatio))
		}
		completionRatio := priceData.CompletionRatio
		if completionRatio < 1 {
			completionRatio = 1
		}
		completionQuota := decimal.NewFromInt(int64(conservativeCompletionTokens)).Mul(decimal.NewFromFloat(completionRatio))
		quota = promptQuota.
			Add(completionQuota).
			Mul(decimal.NewFromFloat(priceData.ModelRatio)).
			Mul(groupRatio)
	}

	quota = quota.Add(estimateConservativeBuiltInToolQuota(info, priceData))

	for _, otherRatio := range priceData.OtherRatios {
		if otherRatio > 0 {
			quota = quota.Mul(decimal.NewFromFloat(otherRatio))
		}
	}

	if !priceData.UsePrice && priceData.ModelRatio != 0 && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}
	if quota.LessThan(decimal.Zero) {
		return 0
	}
	return int(quota.Round(0).IntPart())
}

func CalculateAudioDurationPreConsumeQuota(priceData types.PriceData) int {
	return common.CalculateAudioDurationQuota(
		priceData.AudioDurationPrice,
		priceData.AudioDurationSeconds,
		priceData.GroupRatioInfo.GroupRatio,
		priceData.TimeRatioInfo.EffectiveRatio(),
	)
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	if meta == nil {
		meta = &types.TokenCountMeta{}
	}
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)
	audioDurationPrice, useAudioDurationPrice := ratio_setting.GetAudioDurationPrice(info.OriginModelName)

	groupRatioInfo := HandleGroupRatio(c, info)
	timeRatioInfo := ratio_setting.ResolveTimeRatio(info.OriginModelName, info.UsingGroup, info.UserGroup, info.StartTime)
	timeRatio := timeRatioInfo.EffectiveRatio()

	// Check if this model uses tiered_expr billing
	billingMode, exprStr := billing_setting.GetBillingConfig(info.OriginModelName)
	if billingMode == billing_setting.BillingModeTieredExpr {
		if exprStr == "" {
			return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
		}
		return modelPriceHelperTiered(c, info, exprStr, promptTokens, meta, groupRatioInfo, timeRatioInfo)
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if useAudioDurationPrice {
		if info.InputAudioDurationSeconds <= 0 {
			return types.PriceData{}, fmt.Errorf("model %s uses audio duration billing but no valid input audio duration was measured", info.OriginModelName)
		}
		preConsumedQuota = common.CalculateAudioDurationQuota(
			audioDurationPrice,
			info.InputAudioDurationSeconds,
			groupRatioInfo.GroupRatio,
			timeRatio,
		)
	} else if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, fmt.Errorf("模型 %s 倍率或价格未配置，请联系管理员设置或开始自用模式；Model %s ratio or price not set, please set or start self-use mode", matchName, matchName)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio * timeRatio
		preConsumedQuota = int(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio * timeRatio)
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if useAudioDurationPrice {
			if audioDurationPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:             freeModel,
		ModelPrice:            modelPrice,
		ModelRatio:            modelRatio,
		CompletionRatio:       completionRatio,
		GroupRatioInfo:        groupRatioInfo,
		UsePrice:              usePrice && !useAudioDurationPrice,
		CacheRatio:            cacheRatio,
		ImageRatio:            imageRatio,
		AudioRatio:            audioRatio,
		AudioCompletionRatio:  audioCompletionRatio,
		AudioDurationPrice:    audioDurationPrice,
		AudioDurationSeconds:  info.InputAudioDurationSeconds,
		UseAudioDurationPrice: useAudioDurationPrice,
		CacheCreationRatio:    cacheCreationRatio,
		CacheCreation5mRatio:  cacheCreationRatio5m,
		CacheCreation1hRatio:  cacheCreationRatio1h,
		QuotaToPreConsume:     preConsumedQuota,
		TimeRatioInfo:         timeRatioInfo,
	}
	priceData.ConservativeQuotaToPreConsume = EstimateConservativePreConsumeQuota(info, promptTokens, meta, priceData)
	if priceData.ConservativeQuotaToPreConsume < priceData.QuotaToPreConsume {
		priceData.ConservativeQuotaToPreConsume = priceData.QuotaToPreConsume
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) types.PriceData {
	groupRatioInfo := HandleGroupRatio(c, info)
	timeRatioInfo := ratio_setting.ResolveTimeRatio(info.OriginModelName, info.UsingGroup, info.UserGroup, info.StartTime)
	timeRatio := timeRatioInfo.EffectiveRatio()

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	// 如果没有配置价格，则使用默认价格
	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if !ok {
			modelPrice = 0.1
		} else {
			modelPrice = defaultPrice
		}
	}
	quota := int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio * timeRatio)

	// 免费模型检测（与 ModelPriceHelper 对齐）
	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
			quota = 0
			freeModel = true
		}
	}

	priceData := types.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
		TimeRatioInfo:  timeRatioInfo,
	}
	info.PriceData = priceData
	return priceData
}

// ModelPriceHelperTask applies expression-based pricing to asynchronous tasks
// when configured. The expression can read request parameters such as
// duration and resolution; other models retain the existing per-call behavior.
func ModelPriceHelperTask(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	if info == nil {
		return types.PriceData{}, fmt.Errorf("relay info is nil")
	}
	info.TaskBillingLogInfo = nil
	exprStr, ok := billing_setting.GetTieredExpr(info.OriginModelName)
	if !ok {
		price := ModelPriceHelperPerCall(c, info)
		_, price.UsePrice = ratio_setting.GetModelPrice(info.OriginModelName, false)
		info.PriceData = price
		return price, nil
	}
	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}
	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered task expr run failed: %w", info.OriginModelName, err)
	}
	groupRatioInfo := HandleGroupRatio(c, info)
	timeRatioInfo := ratio_setting.ResolveTimeRatio(info.OriginModelName, info.UsingGroup, info.UserGroup, info.StartTime)
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	quota, err := billingexpr.QuotaRoundChecked(quotaBeforeGroup * groupRatioInfo.GroupRatio * timeRatioInfo.EffectiveRatio())
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered task quota invalid: %w", info.OriginModelName, err)
	}
	priceData := types.PriceData{
		Quota:                         quota,
		QuotaToPreConsume:             quota,
		ConservativeQuotaToPreConsume: quota,
		GroupRatioInfo:                groupRatioInfo,
		TimeRatioInfo:                 timeRatioInfo,
		OtherRatios:                   map[string]float64{},
	}
	info.BillingRequestInput = &requestInput
	if info.ChannelMeta != nil && common.IsVideoTaskChannel(info.ChannelType) {
		info.TaskBillingLogInfo = map[string]interface{}{
			"kind": "video_expression", "matched_tier": trace.MatchedTier,
			"base_cost": rawCost / 1_000_000, "base_quota": quotaBeforeGroup, "charged_quota": quota,
			"group_ratio": groupRatioInfo.GroupRatio, "time_ratio": timeRatioInfo.EffectiveRatio(),
		}
		var fields map[string]interface{}
		if common.Unmarshal(requestInput.Body, &fields) == nil {
			if duration, ok := fields["duration"].(float64); ok && duration > 0 {
				info.TaskBillingLogInfo["requested_seconds"] = duration
			}
		}
	}
	info.PriceData = priceData
	return priceData, nil
}

func ContainPriceOrRatio(modelName string) bool {
	_, ok := ratio_setting.GetModelPrice(modelName, false)
	if ok {
		return true
	}
	_, ok, _ = ratio_setting.GetModelRatio(modelName)
	if ok {
		return true
	}
	_, ok = billing_setting.GetTieredExpr(modelName)
	return ok
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, exprStr string, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo types.GroupRatioInfo, timeRatioInfo types.TimeRatioInfo) (types.PriceData, error) {
	estimatedPromptTokens := common.Max(promptTokens, common.PreConsumedQuota)
	estimatedCompletionTokens := common.Max(meta.MaxTokens, 0)
	if estimatedCompletionTokens == 0 {
		estimatedCompletionTokens = defaultConservativeCompletionTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	if common.IsImagePriceExpression(exprStr) {
		if info.ChannelType != constant.ChannelTypeAli || (info.RelayMode != relayconstant.RelayModeImagesGenerations && info.RelayMode != relayconstant.RelayModeImagesEdits) {
			return types.PriceData{}, fmt.Errorf("image input/output pricing requires an Ali image endpoint")
		}
		requestInput.Body, err = sjson.DeleteBytes(requestInput.Body, common.ImagePricingField)
		if err != nil {
			return types.PriceData{}, err
		}
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(estimatedPromptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(estimatedPromptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", info.OriginModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, err := billingexpr.QuotaRoundChecked(quotaBeforeGroup * groupRatioInfo.GroupRatio * timeRatioInfo.EffectiveRatio())
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered pre-consume quota invalid: %w", info.OriginModelName, err)
	}

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshotTimeRatio := 0.0
	if timeRatioInfo.Matched() || timeRatioInfo.EffectiveRatio() != 1 {
		snapshotTimeRatio = timeRatioInfo.EffectiveRatio()
	}
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     estimatedPromptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
		TimeRatio:                 snapshotTimeRatio,
		TimeRatioRuleID:           timeRatioInfo.RuleID,
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:                     freeModel,
		GroupRatioInfo:                groupRatioInfo,
		QuotaToPreConsume:             preConsumedQuota,
		ConservativeQuotaToPreConsume: preConsumedQuota,
		TimeRatioInfo:                 timeRatioInfo,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier))
	}

	info.PriceData = priceData
	return priceData, nil
}
