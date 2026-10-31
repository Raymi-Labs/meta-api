package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat.IsWebSocket() {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			helper.StopStreamKeepAlive(c)
			newAPIError = service.NormalizeClientCancellation(c, newAPIError)
			service.MarkRequestFailure(c, newAPIError)
			if newAPIError.RetryAfter > 0 {
				retryAfterSeconds := int((newAPIError.RetryAfter + time.Second - 1) / time.Second)
				if retryAfterSeconds < 1 {
					retryAfterSeconds = 1
				}
				c.Header("Retry-After", fmt.Sprintf("%d", retryAfterSeconds))
			}
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			if !shouldWriteRelayErrorResponse(c) {
				return
			}
			if !relayFormat.IsWebSocket() && c.Writer.Written() &&
				strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "text/event-stream") {
				writeRelayStreamError(c, relayFormat, newAPIError)
				return
			}
			if !relayFormat.IsWebSocket() {
				c.Header("Content-Type", "application/json; charset=utf-8")
			}
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime, types.RelayFormatAliTTS:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = newRequestBodyTooLargeRelayError(c, 0)
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	var healthRequest *grouphealth.Request
	var healthWriter *groupHealthWriter
	if grouphealth.SupportsRequest(c.Request.Method, c.Request.URL.Path) {
		healthRequest = grouphealth.NewRequest(relayInfo.OriginModelName, relayInfo.IsStream)
		healthWriter = &groupHealthWriter{ResponseWriter: c.Writer}
		c.Writer = healthWriter
		defer func() {
			healthRequest.ObserveUnrecordedFailure(relayInfo.UsingGroup, service.NormalizeClientCancellation(c, newAPIError))
			healthRequest.Finish()
			c.Writer = healthWriter.ResponseWriter
		}()
	}

	c.Set(service.RequestLogStreamKey, relayInfo.IsStream)
	defer func() {
		success := newAPIError == nil && (c.Request.Context().Err() == nil || relayInfo.StreamStatus.HasDeliveredCompletion()) && c.Writer.Status() < 400 && relayInfo.StreamStatus.IsSuccessful()
		c.Set(service.RequestOutcomeKey, success)
		if success && relayInfo.StreamStatus.HasDeliveredCompletion() {
			c.Set(service.RequestDeliveredCompletionKey, true)
		}
		// Record synchronously so a retry/continuation cannot mutate the sampled
		// relay state and partial billing cannot create a second success sample.
		perfmetrics.RecordRelaySample(relayInfo, success, relayInfo.PerformanceOutputTokens)
	}()

	startRequestKeepAlive := func() {
		if relayFormat != types.RelayFormatClaude || !relayInfo.IsStream {
			return
		}
		if enabled, interval := helper.GetStreamPingConfig(relayInfo); enabled {
			helper.StartStreamKeepAlive(c, interval, relayFormat)
		}
	}
	startRequestKeepAlive()
	defer helper.StopStreamKeepAlive(c)

	// Enforce a user-scoped cooling-off period before token counting, billing,
	// channel selection, or any upstream request. Local rejections never create
	// additional safety events and cannot extend the cooling-off period.
	if !service.IsContentSafetyRecordOnlyGroup(relayInfo.UsingGroup) {
		nowUnix := time.Now().Unix()
		cooldownUntil, cooldownErr := model.GetActiveContentSafetyCooldown(relayInfo.UserId, nowUnix)
		if cooldownErr != nil {
			newAPIError = types.NewErrorWithStatusCode(
				fmt.Errorf("content safety state unavailable: %w", cooldownErr),
				types.ErrorCode("content_safety_state_unavailable"),
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			)
			return
		}
		if cooldownUntil > nowUnix {
			retryAfter := cooldownUntil - nowUnix
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			newAPIError = types.WithOpenAIError(types.OpenAIError{
				Message: fmt.Sprintf("内容安全冷静期仍在生效，请在 %d 秒后重试。该本地拒绝不会新增风控记录。", retryAfter),
				Type:    "content_safety_cooldown",
				Code:    "content_safety_cooldown",
			}, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			healthRequest.ExcludeUserError(relayInfo.UsingGroup)
			return
		}
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	needAudioDuration := ratio_setting.ContainsAudioDurationPrice(relayInfo.OriginModelName)
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken || needAudioDuration {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			healthRequest.ExcludeUserError(relayInfo.UsingGroup)
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	_, isImageRequest := request.(*dto.ImageRequest)
	if isImageRequest {
		// ImageHelper reserves the validated outbound quantity after conversion
		// and overrides, before each attempt. The refund defer below still owns it.
	} else if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeContentSafetyPolicyError(newAPIError)
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:             c,
		TokenGroup:      relayInfo.TokenGroup,
		ModelName:       relayInfo.OriginModelName,
		AllowedChannels: middleware.GetAllowedTokenChannelIDs(c),
		Retry:           common.GetPointer(0),
	}

	// lastRelayError 保留最近一次上游错误，避免重试找不到新渠道时用 (retry) 错误覆盖原始错误。
	var lastRelayError *types.NewAPIError

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		if c.Request.Context().Err() != nil {
			newAPIError = service.NormalizeClientCancellation(c, types.NewError(c.Request.Context().Err(), types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry()))
			break
		}
		// Header access during preparation joins the previous heartbeat. Resume
		// it for selection/retry gaps; business output still forbids replay.
		startRequestKeepAlive()
		channel, releaseSlot, channelErr, overloadControl := selectChannelWithConcurrency(c, relayInfo, retryParam)
		if channelErr != nil {
			healthRequest.ObserveFailure(relayInfo.UsingGroup, channelErr)
			logger.LogError(c, channelErr.Error())
			if overloadControl {
				// Layer C 过载控制的终态错误（429/503 / 客户端断开）：直接采用，
				// 不用历史重试错误覆盖——系统整体过载对用户更有指导性。
				newAPIError = channelErr
			} else if lastRelayError != nil {
				// 保留原始上游错误，让用户能看到真正的失败原因
				newAPIError = lastRelayError
			} else {
				newAPIError = channelErr
			}
			break
		}

		// 用闭包包裹本轮上游调用的完整生命周期，确保渠道并发名额 releaseSlot 无论走哪条
		// 退出路径都释放一次，且在每轮迭代结束即释放（避免 for 循环内 defer 堆积到函数返回）。
		// addUsedChannel 也置于 defer releaseSlot 之后，保证 acquire 成功到 defer 注册之间
		// 没有任何可 panic 的语句，杜绝 panic 绕过释放导致的名额泄漏。
		breakLoop := false
		func() {
			if releaseSlot != nil {
				defer releaseSlot()
			}
			addUsedChannel(c, channel.Id)
			bodyStorage, bodyErr := common.GetBodyStorage(c)
			if bodyErr != nil {
				// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
				if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
					newAPIError = newRequestBodyTooLargeRelayError(c, 0)
				} else {
					newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				}
				breakLoop = true
				return
			}
			c.Request.Body = io.NopCloser(bodyStorage)
			if healthRequest != nil {
				grouphealth.ResetAttempt(relayInfo)
				healthWriter.beginAttempt()
			}

			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				newAPIError = relay.WssHelper(c, relayInfo)
			case types.RelayFormatAliTTS:
				newAPIError = relay.AliTTSHelper(c, relayInfo, nil)
			case types.RelayFormatClaude:
				newAPIError = relay.ClaudeHelper(c, relayInfo)
			case types.RelayFormatGemini:
				newAPIError = geminiRelayHandler(c, relayInfo)
			default:
				newAPIError = relayHandler(c, relayInfo)
			}
		}()
		// Health sampling also inspects the underlying writer's status. Join
		// before that observation; a subsequent attempt resumes keepalive.
		helper.StopStreamKeepAlive(c)
		if breakLoop {
			break
		}
		newAPIError = service.NormalizeClientCancellation(c, newAPIError)
		healthErr := newAPIError
		if healthErr == nil && !healthWriter.delivered() && errors.Is(c.Request.Context().Err(), context.Canceled) {
			// A disconnected response with no successful write is still unknown,
			// not proof of an intentional user stop. Only health attribution changes.
			healthErr = types.NewClientCanceledError()
		}
		if helper.IsDownstreamWriteError(healthErr) {
			// A proven local delivery failure is neither upstream unavailability
			// nor proof of an intentional user stop. Keep it in the denominator
			// as unknown without penalizing the upstream channel.
			healthRequest.ObserveResult(relayInfo, nil, false, time.Now())
		} else {
			healthRequest.ObserveResult(relayInfo, healthErr, healthWriter.delivered(), time.Now())
		}

		if newAPIError == nil {
			service.ClearUserScopedCircuit(c, channel)
			// 调用成功：异步清掉该 Key 的连续失败计数与整池耗尽通知标记，
			// 避免历史失败抬高健康 Key 下次冷却的起点。
			if common.QuotaStabilityEnabled && channel.ChannelInfo.IsMultiKey {
				if keyIndex, ok := common.GetContextKeyType[int](c, constant.ContextKeyChannelMultiKeyIndex); ok && keyIndex >= 0 {
					channelId := channel.Id
					gopool.Go(func() {
						_ = model.ResetMultiKeyCooldownBackoff(channelId, keyIndex)
						_ = model.ClearChannelPoolExhaustedNotified(channelId)
					})
				}
			}
			return
		}

		if types.ClassifyFailure(newAPIError) == types.FailureClientCanceled {
			service.AppendRequestFailureAttempt(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan()), newAPIError)
			logger.LogInfo(c, "client request canceled; stopping relay without channel failover")
			break
		}
		if helper.IsDownstreamWriteError(newAPIError) {
			service.AppendRequestFailureAttempt(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan()), newAPIError)
			logger.LogInfo(c, "downstream stream write failed; stopping relay without channel failover")
			break
		}

		newAPIError = service.NormalizeContentSafetyPolicyError(newAPIError)
		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		newAPIError = service.NormalizeResponsesConversationStateError(newAPIError)
		newAPIError = service.GuardUpstreamCapabilityRetry(c, relayInfo.Request, newAPIError)
		if cooldown := service.RecordChannelRateLimitCooldown(c, channel, newAPIError); cooldown > 0 {
			logger.LogInfo(c, fmt.Sprintf("upstream rate limit cooldown set: channel=%d ttl=%ds", channel.Id, int((cooldown+time.Second-1)/time.Second)))
		}
		if service.IsContentSafetyPolicyError(newAPIError) {
			result, auditErr := service.RecordContentSafetyPolicyViolation(c, relayInfo, newAPIError)
			if auditErr != nil {
				logger.LogError(c, fmt.Sprintf("content safety audit failed: user_id=%d request_id=%s error_code=%s err=%v",
					relayInfo.UserId, relayInfo.RequestId, newAPIError.GetErrorCode(), auditErr))
			} else if result != nil && !result.Duplicate && result.Violation != nil {
				logger.LogWarn(c, fmt.Sprintf("content safety event recorded: user_id=%d token_id=%d channel_id=%d request_id=%s error_code=%s window_count=%d action=%s",
					relayInfo.UserId, relayInfo.TokenId, relayInfo.ChannelId, relayInfo.RequestId,
					result.Violation.ErrorCode, result.Violation.WindowCount, result.Violation.Action))
			}
			if result != nil && result.Violation != nil {
				newAPIError = service.EnrichContentSafetyClientError(newAPIError, result)
			}
		}

		lastRelayError = newAPIError
		// Duplex task input cannot be reconstructed, and HTTP speech may already
		// have delivered audio. Never enter model/key failover for these requests.
		if relayFormat == types.RelayFormatAliTTS || (channel.Type == constant.ChannelTypeAli && relayInfo.RelayMode == relayconstant.RelayModeAudioSpeech) {
			service.AppendRequestFailureAttempt(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan()), newAPIError)
			break
		}
		if service.IsResponsesConversationStateError(newAPIError) {
			attachUpstreamDiagnosticsFromContext(c, newAPIError)
			message := fmt.Sprintf(
				"responses conversation state unavailable: classification=%s channel=%d model=%s",
				types.ErrorCodeConversationStateNotFound,
				channel.Id,
				relayInfo.OriginModelName,
			)
			if diagnostics := newAPIError.UpstreamDiagnosticsLogString(); diagnostics != "" {
				message = fmt.Sprintf("%s upstream={%s}", message, diagnostics)
			}
			logger.LogInfo(c, message)
			break
		}

		if service.IsChannelModelMismatchError(newAPIError) {
			if _, ok := c.Get("specific_channel_id"); ok {
				break
			}
			attachUpstreamDiagnosticsFromContext(c, newAPIError)
			service.ApplyChannelFailureRetryExclusion(retryParam, channel, newAPIError)
			retryParam.ResetRetryNextTry()
			errorMessage := newAPIError.Error()
			if diagnostics := newAPIError.UpstreamDiagnosticsLogString(); diagnostics != "" {
				errorMessage = fmt.Sprintf("%s, upstream={%s}", errorMessage, diagnostics)
			}
			logger.LogInfo(c, fmt.Sprintf("跳过不兼容渠道并继续重试：channel=%d model=%s err=%s", channel.Id, relayInfo.OriginModelName, errorMessage))
			continue
		}

		logCRSMemoryPressureShortCircuit(c, channel, newAPIError)
		service.RecordChannelModelCircuitFailure(channel, relayInfo.OriginModelName, newAPIError)
		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
		service.RecordUserScopedCircuitFailure(c, channel, newAPIError)

		// 同渠道多 Key 快速切换：配额耗尽时先在当前渠道内换一个 Key 重试。
		// 此处故意不调用 ApplyChannelFailureRetryExclusion，因为目的是换 Key 而非换渠道，
		// 不应将当前渠道排除出重试列表。
		allowSameChannelFailover := common.QuotaStabilityEnabled &&
			common.MultiKeySameChannelFailoverOnce &&
			channel.ChannelInfo.IsMultiKey &&
			service.IsQuotaRelatedError(newAPIError) &&
			!retryParam.SameChannelFailoverDone

		if allowSameChannelFailover {
			retryParam.SameChannelFailoverDone = true
			retryParam.ResetRetryNextTry()
			logger.LogInfo(c, fmt.Sprintf("同渠道多 Key 快速切换重试：channel=%d", channel.Id))
			continue
		}

		service.ApplyChannelFailureRetryExclusion(retryParam, channel, newAPIError)

		if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
}

func shouldWriteRelayErrorResponse(c *gin.Context) bool {
	// A streamed audio response cannot contain a trailing JSON error. The
	// failure is still logged; clients detect a truncated stream/absent SSE done.
	if c.Request.URL.Path == "/v1/audio/speech" && c.Writer.Written() && strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "audio/") {
		return false
	}
	return !common.GetContextKeyBool(c, constant.ContextKeyResponsesStreamErrorWritten)
}

func writeRelayStreamError(c *gin.Context, format types.RelayFormat, apiError *types.NewAPIError) {
	// HTTP headers may have been committed by a preparation heartbeat. The
	// failure remains a failure; return it within the protocol already opened.
	if format == types.RelayFormatClaude {
		_ = helper.ClaudeData(c, dto.ClaudeResponse{Type: "error", Error: apiError.ToClaudeError()})
		return
	}
	_ = helper.ObjectData(c, gin.H{"error": apiError.ToOpenAIError()})
	helper.Done(c)
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func newRequestBodyTooLargeRelayError(c *gin.Context, actualBytes int64) *types.NewAPIError {
	limitBytes := common.GetRequestBodyLimitBytes(c)
	if actualBytes <= 0 && c != nil && c.Request != nil && c.Request.ContentLength > limitBytes {
		actualBytes = c.Request.ContentLength
	}
	return types.WithOpenAIError(types.OpenAIError{
		Message: common.FormatRequestBodyTooLargeMessage(actualBytes, limitBytes),
		Type:    "invalid_request_error",
		Code:    "request_body_too_large",
	}, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
}

func appendUniqueInt(ids []int, id int) []int {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		meta.MaxTokens = int(r.GetMaxTokens())
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(r.MaxOutputTokens)
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(r.MaxTokens)
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		channelID := c.GetInt("channel_id")
		if containsChannelID(retryParam.ExcludeChannels, channelID) {
			// 首轮预选渠道可能因容量满载被临时排除；此时必须进入重新选渠路径，
			// 不能继续反复返回 Context 中同一个渠道。
			info.ChannelMeta = &relaycommon.ChannelMeta{}
		} else if selected, err := model.CacheGetChannel(channelID); err == nil && selected != nil {
			if service.IsChannelModelCircuitOpen(selected, retryParam.ModelName) {
				retryParam.ExcludeChannels = appendUniqueInt(retryParam.ExcludeChannels, channelID)
				info.ChannelMeta = &relaycommon.ChannelMeta{}
			} else {
				return selected, nil
			}
		}
	}
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	for {
		channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

		info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

		if err != nil {
			return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		if channel == nil {
			return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}

		newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
		if newAPIError == nil {
			return channel, nil
		}
		if !service.ShouldFallbackAfterSetupError(newAPIError) {
			return nil, newAPIError
		}
		retryParam.ExcludeChannels = appendUniqueInt(retryParam.ExcludeChannels, channel.Id)
	}
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return service.ShouldRetryChannelError(c, openaiErr, retryTimes)
}

func logCRSMemoryPressureShortCircuit(c *gin.Context, channel *model.Channel, err *types.NewAPIError) {
	trips := service.ApplyCRSMemoryPressureShortCircuit(channel, err)
	if len(trips) == 0 {
		return
	}
	parts := make([]string, 0, len(trips))
	for _, trip := range trips {
		state := "existing"
		if trip.Opened {
			state = "opened"
		}
		parts = append(parts, fmt.Sprintf("%s=%s ttl=%ds %s", trip.Scope, trip.Key, trip.TTLSeconds, state))
	}
	logger.LogWarn(c, fmt.Sprintf("CRS memory pressure short-circuit: channel=%d %s", channel.Id, strings.Join(parts, "; ")))
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	attachUpstreamDiagnosticsFromContext(c, err)
	errorMessage := err.Error()
	if diagnostics := err.UpstreamDiagnosticsLogString(); diagnostics != "" {
		errorMessage = fmt.Sprintf("%s, upstream={%s}", errorMessage, diagnostics)
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, errorMessage))
	isQuotaRelated := service.IsQuotaRelatedError(err)
	// 多 Key 且额度类错误：先给当前 key 打冷却，避免下一次又选到同一个 key。
	if common.QuotaStabilityEnabled && channelError.IsMultiKey && isQuotaRelated {
		keyIndex, hasKeyIndex := common.GetContextKeyType[int](c, constant.ContextKeyChannelMultiKeyIndex)
		if hasKeyIndex && keyIndex >= 0 && common.MultiKeyCooldownSeconds > 0 {
			reason := string(err.GetErrorCode())
			if reason == "" {
				reason = "quota_related_error"
			}
			// 指数退避：连续额度失败的 Key 冷却时长翻倍递增（封顶 2h），
			// 避免余额耗尽的死 Key 每个基础周期就复活拿用户请求当探针。
			if appliedSeconds, cooldownErr := model.SetMultiKeyCooldownWithBackoff(channelError.ChannelId, keyIndex, reason); cooldownErr != nil {
				common.SysError(fmt.Sprintf("set multi-key cooldown failed: channel=%d key_index=%d err=%v", channelError.ChannelId, keyIndex, cooldownErr))
			} else {
				logger.LogInfo(c, fmt.Sprintf("multi-key cooldown set: channel=%d key_index=%d ttl=%ds", channelError.ChannelId, keyIndex, appliedSeconds))
			}
			// 整池耗尽检测：异步核对该渠道是否已无可用 Key，是则通知管理员（6h 去重）。
			cooldownChannelId := channelError.ChannelId
			gopool.Go(func() {
				service.NotifyChannelPoolExhausted(cooldownChannelId)
			})
		}
	}
	// 不要在异步协程里从 context 重新读取渠道信息，避免不一致。
	// 多 Key 的额度类错误只做短时冷却，不做永久禁用。
	shouldDisable := service.ShouldDisableChannel(channelError.ChannelType, err) && channelError.AutoBan
	if shouldDisable && !(common.QuotaStabilityEnabled && channelError.IsMultiKey && isQuotaRelated) {
		if scheduled, scheduleErr := service.SchedulePreDisableWait(channelError, err.ErrorWithStatusCode()); scheduleErr != nil {
			common.SysError(fmt.Sprintf("schedule channel pre-disable wait failed: channel=%d err=%v", channelError.ChannelId, scheduleErr))
		} else if scheduled {
			logger.LogInfo(c, fmt.Sprintf("channel pre-disable wait scheduled: channel=%d", channelError.ChannelId))
			shouldDisable = false
		}
	}
	if shouldDisable && !(common.QuotaStabilityEnabled && channelError.IsMultiKey && isQuotaRelated) {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	service.AppendRequestFailureAttempt(c, channelError, err)

}

func attachUpstreamDiagnosticsFromContext(c *gin.Context, err *types.NewAPIError) {
	if c == nil || err == nil || err.Upstream == nil {
		return
	}
	if err.Upstream.ConfiguredBaseURL == "" {
		err.Upstream.ConfiguredBaseURL = common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
	}
	if err.Upstream.RequestLength <= 0 && c.Request != nil && c.Request.ContentLength > 0 {
		err.Upstream.RequestLength = c.Request.ContentLength
	}
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var releaseSlot func()
	if middleware.ChannelConcurrencyEnabled() && midjourneyModeUsesUpstream(relayInfo.RelayMode) {
		var capacityErr *types.NewAPIError
		var fixedChannelHandled bool
		releaseSlot, capacityErr, fixedChannelHandled = acquireMidjourneyBoundChannelCapacity(c, relayInfo)
		if !fixedChannelHandled {
			retryParam := &service.RetryParam{
				Ctx:             c,
				TokenGroup:      relayInfo.TokenGroup,
				ModelName:       relayInfo.OriginModelName,
				AllowedChannels: middleware.GetAllowedTokenChannelIDs(c),
				Retry:           common.GetPointer(0),
			}
			_, releaseSlot, capacityErr, _ = selectChannelWithConcurrency(c, relayInfo, retryParam)
		}
		if capacityErr != nil {
			respondMidjourneyCapacityError(c, capacityErr)
			return
		}
		if releaseSlot != nil {
			defer releaseSlot()
		}
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func midjourneyModeUsesUpstream(relayMode int) bool {
	switch relayMode {
	case relayconstant.RelayModeMidjourneyNotify,
		relayconstant.RelayModeMidjourneyTaskFetch,
		relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		return false
	default:
		return true
	}
}

func acquireMidjourneyBoundChannelCapacity(c *gin.Context, info *relaycommon.RelayInfo) (func(), *types.NewAPIError, bool) {
	if info == nil {
		return nil, nil, false
	}
	if info.RelayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
		release, apiErr := acquireMidjourneyTaskChannelCapacity(c, info)
		return release, apiErr, true
	}

	switch info.RelayMode {
	case relayconstant.RelayModeMidjourneyAction,
		relayconstant.RelayModeMidjourneyChange,
		relayconstant.RelayModeMidjourneySimpleChange,
		relayconstant.RelayModeMidjourneyModal,
		relayconstant.RelayModeMidjourneyVideo:
	default:
		return nil, nil, false
	}

	var request dto.MidjourneyRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		// relay 层会按原有协议返回参数错误；此时不会访问上游，不应消费 RPM。
		return nil, nil, true
	}
	taskID := strings.TrimSpace(request.TaskId)
	if taskID == "" && info.RelayMode == relayconstant.RelayModeMidjourneySimpleChange {
		if params := service.ConvertSimpleChangeParams(request.Content); params != nil {
			taskID = strings.TrimSpace(params.TaskId)
		}
	}
	if taskID == "" {
		// action 等请求也可能完全由上游 customId 处理，此时仍使用正常选渠。
		return nil, nil, false
	}

	originTask := model.GetByMJId(info.UserId, taskID)
	if originTask == nil {
		// relay 层会返回 task_not_found；没有实际上游请求。
		return nil, nil, true
	}
	channel, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
		// relay 层保留原有错误格式；没有实际上游请求。
		return nil, nil, true
	}
	if setupErr := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName); setupErr != nil {
		return nil, setupErr, true
	}
	release, apiErr := acquireFixedChannelCapacityWithWait(c, channel)
	return release, apiErr, true
}

func acquireMidjourneyTaskChannelCapacity(c *gin.Context, info *relaycommon.RelayInfo) (func(), *types.NewAPIError) {
	originTask := model.GetByMJId(c.GetInt("id"), c.Param("id"))
	if originTask == nil {
		return nil, nil
	}
	channel, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
		return nil, nil
	}
	if setupErr := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName); setupErr != nil {
		return nil, setupErr
	}
	return acquireFixedChannelCapacityWithWait(c, channel)
}

func respondMidjourneyCapacityError(c *gin.Context, apiErr *types.NewAPIError) {
	statusCode := http.StatusServiceUnavailable
	if apiErr != nil && apiErr.StatusCode > 0 {
		statusCode = apiErr.StatusCode
	}
	description := "channel capacity unavailable"
	if apiErr != nil {
		description = apiErr.Error()
	}
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        30,
	})
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:             c,
		TokenGroup:      relayInfo.TokenGroup,
		ModelName:       relayInfo.OriginModelName,
		AllowedChannels: middleware.GetAllowedTokenChannelIDs(c),
		Retry:           common.GetPointer(0),
	}

	// lastTaskRelayErr 保留最近一次上游任务错误，避免重试找不到新渠道时覆盖原始错误。
	var lastTaskRelayErr *dto.TaskError
	var lastTaskChannel *model.Channel

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		var channel *model.Channel
		var releaseSlot func()
		var channelErr *types.NewAPIError
		overloadControl := false

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
				channelErr = setupErr
			} else {
				releaseSlot, channelErr = acquireFixedChannelCapacityWithWait(c, channel)
				overloadControl = channelErr != nil
			}
		} else {
			channel, releaseSlot, channelErr, overloadControl = selectChannelWithConcurrency(c, relayInfo, retryParam)
		}
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			if overloadControl || lastTaskRelayErr == nil {
				taskErr = service.TaskErrorFromAPIError(channelErr)
			} else {
				taskErr = lastTaskRelayErr
			}
			break
		}
		lastTaskChannel = channel

		breakLoop := false
		func() {
			if releaseSlot != nil {
				defer releaseSlot()
			}
			addUsedChannel(c, channel.Id)
			bodyStorage, bodyErr := common.GetBodyStorage(c)
			if bodyErr != nil {
				if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
					taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
				} else {
					taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
				}
				breakLoop = true
				return
			}
			c.Request.Body = io.NopCloser(bodyStorage)
			result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		}()
		if breakLoop {
			break
		}
		if taskErr == nil {
			break
		}

		lastTaskRelayErr = taskErr
		if !taskErr.LocalError {
			apiErr := types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode)
			apiErr.UpstreamStatusCode = taskErr.UpstreamStatusCode
			apiErr.RetryAfter = taskErr.RetryAfter
			if cooldown := service.RecordChannelRateLimitCooldown(c, channel, apiErr); cooldown > 0 {
				taskErr.RetryAfter = apiErr.RetryAfter
				logger.LogInfo(c, fmt.Sprintf("task upstream rate limit cooldown set: channel=%d ttl=%ds", channel.Id, int((cooldown+time.Second-1)/time.Second)))
			}
			logCRSMemoryPressureShortCircuit(c, channel, apiErr)
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				apiErr)
			service.RecordUserScopedCircuitFailure(c, channel, apiErr)
		}

		retryParam.ExcludeChannels = appendUniqueInt(retryParam.ExcludeChannels, channel.Id)
		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		service.ClearUserScopedCircuit(c, lastTaskChannel)
		if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}
		service.LogTaskConsumption(c, relayInfo)

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		taskTimeRatio := 0.0
		if relayInfo.PriceData.TimeRatioInfo.Matched() || relayInfo.PriceData.TimeRatioInfo.EffectiveRatio() != 1 {
			taskTimeRatio = relayInfo.PriceData.TimeRatioInfo.EffectiveRatio()
		}
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:         relayInfo.PriceData.ModelPrice,
			GroupRatio:         relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:         relayInfo.PriceData.ModelRatio,
			TimeRatio:          taskTimeRatio,
			TimeRatioRuleID:    relayInfo.PriceData.TimeRatioInfo.RuleID,
			TimeRatioTimezone:  relayInfo.PriceData.TimeRatioInfo.Timezone,
			TimeRatioMatchedAt: relayInfo.PriceData.TimeRatioInfo.MatchedAt,
			OtherRatios:        relayInfo.PriceData.OtherRatios,
			OriginModelName:    relayInfo.OriginModelName,
			PerCallBilling:     relayInfo.PriceData.UsePrice || common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || billing_setting.GetBillingMode(relayInfo.OriginModelName) == billing_setting.BillingModeTieredExpr,
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	logErr := types.NewErrorWithStatusCode(fmt.Errorf("%s", taskErr.Message), types.ErrorCode(taskErr.Code), taskErr.StatusCode)
	logErr.UpstreamStatusCode = taskErr.UpstreamStatusCode
	logErr.RetryAfter = taskErr.RetryAfter
	service.MarkRequestFailure(c, logErr)
	if taskErr.RetryAfter > 0 {
		retryAfterSeconds := int((taskErr.RetryAfter + time.Second - 1) / time.Second)
		if retryAfterSeconds < 1 {
			retryAfterSeconds = 1
		}
		c.Header("Retry-After", fmt.Sprintf("%d", retryAfterSeconds))
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if c.GetBool("video_submission_started") {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
