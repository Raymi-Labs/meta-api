package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	responsesAutoContinueMaxTailRunes = 3000
	responsesAutoContinuePrompt       = "上一次回答因为上游网络中断没有正常结束。请从下面“已输出内容”的末尾自然继续完成回答，不要重复已经输出的内容，不要解释中断原因。\n\n已输出内容末尾：\n%s"
)

func newResponsesAutoContinueOptions(c *gin.Context, info *relaycommon.RelayInfo, originalReq *dto.OpenAIResponsesRequest, passThrough bool) *openaichannel.ResponsesStreamHandlerOptions {
	if !shouldEnableResponsesAutoContinue(c, info, originalReq, passThrough) {
		return nil
	}
	return &openaichannel.ResponsesStreamHandlerOptions{
		AutoContinue: func(streamCtx openaichannel.ResponsesStreamAutoContinueContext) (*dto.Usage, bool) {
			return continueResponsesStream(c, info, originalReq, streamCtx)
		},
	}
}

func shouldEnableResponsesAutoContinue(c *gin.Context, info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest, passThrough bool) bool {
	if c == nil || info == nil || req == nil {
		return false
	}
	if passThrough || info.RelayMode != relayconstant.RelayModeResponses || !info.IsStream || info.IsChannelTest {
		return false
	}
	if len(req.Tools) > 0 || len(req.ToolChoice) > 0 || req.HasImageGenerationTool() {
		return false
	}
	return !responsesTextConfigLooksStructured(req.Text)
}

func responsesTextConfigLooksStructured(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	lower := strings.ToLower(string(raw))
	return strings.Contains(lower, "json_schema") || strings.Contains(lower, `"type":"json`)
}

func continueResponsesStream(c *gin.Context, info *relaycommon.RelayInfo, originalReq *dto.OpenAIResponsesRequest, streamCtx openaichannel.ResponsesStreamAutoContinueContext) (*dto.Usage, bool) {
	if strings.TrimSpace(streamCtx.OutputText) == "" {
		return nil, false
	}
	continueReq, err := buildResponsesContinuationRequest(originalReq, streamCtx.OutputText)
	if err != nil {
		logger.LogError(c, "responses auto-continue build request failed: "+err.Error())
		return nil, false
	}

	originalChannelID := info.ChannelId
	channel, releaseSlot, ok := selectResponsesAutoContinueChannel(c, info, originalChannelID)
	if !ok {
		logger.LogInfo(c, fmt.Sprintf("responses auto-continue skipped: no fallback channel, original_channel=%d", originalChannelID))
		return nil, false
	}

	common.SetContextKey(c, constant.ContextKeyResponsesAutoContinue, true)
	common.SetContextKey(c, constant.ContextKeyResponsesAutoContinueFromChannel, originalChannelID)
	common.SetContextKey(c, constant.ContextKeyResponsesAutoContinueToChannel, channel.Id)
	common.SetContextKey(c, constant.ContextKeyResponsesAutoContinueEndReason, string(streamCtx.EndReason))
	appendResponsesAutoContinueChannel(c, channel.Id)
	logger.LogInfo(c, fmt.Sprintf("responses auto-continue started: from_channel=%d to_channel=%d end_reason=%s", originalChannelID, channel.Id, streamCtx.EndReason))

	usage, err := func() (*dto.Usage, error) {
		if releaseSlot != nil {
			defer releaseSlot()
		}
		return requestResponsesContinuation(c, info, continueReq)
	}()
	if err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) {
			if cooldown := service.RecordChannelRateLimitCooldown(c, channel, apiErr); cooldown > 0 {
				logger.LogInfo(c, fmt.Sprintf("responses auto-continue rate limit cooldown set: channel=%d ttl=%ds", channel.Id, int((cooldown+time.Second-1)/time.Second)))
			}
		}
		logger.LogError(c, "responses auto-continue failed: "+err.Error())
		return usage, false
	}
	logger.LogInfo(c, fmt.Sprintf("responses auto-continue finished: from_channel=%d to_channel=%d", originalChannelID, channel.Id))
	return usage, true
}

func appendResponsesAutoContinueChannel(c *gin.Context, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelID))
	c.Set("use_channel", useChannel)
}

func appendUniqueChannelID(ids []int, id int) []int {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func selectResponsesAutoContinueChannel(c *gin.Context, info *relaycommon.RelayInfo, failedChannelID int) (*model.Channel, func(), bool) {
	if c == nil || info == nil {
		return nil, nil, false
	}
	excludeSets := [][]int{nil}
	if failedChannelID > 0 {
		excludeSets = [][]int{{failedChannelID}, nil}
	}

	for _, excludeChannels := range excludeSets {
		channel, releaseSlot, ok := selectResponsesAutoContinueChannelWithExclusion(c, info, excludeChannels)
		if ok {
			return channel, releaseSlot, true
		}
	}
	return nil, nil, false
}

func selectResponsesAutoContinueChannelWithExclusion(c *gin.Context, info *relaycommon.RelayInfo, excludeChannels []int) (*model.Channel, func(), bool) {
	retry := 0
	retryParam := &service.RetryParam{
		Ctx:             c,
		TokenGroup:      info.TokenGroup,
		ModelName:       info.OriginModelName,
		AllowedChannels: middleware.GetAllowedTokenChannelIDs(c),
		ExcludeChannels: append([]int(nil), excludeChannels...),
		Retry:           &retry,
	}
	for retryParam.GetRetry() <= common.RetryTimes {
		channels, selectGroup, err := service.CacheGetSatisfiedChannelCandidates(retryParam)
		if err != nil {
			return nil, nil, false
		}
		if len(channels) == 0 {
			retryParam.IncreaseRetry()
			continue
		}

		for _, channel := range middleware.RankChannelsByCapacityPressure(channels) {
			var releaseSlot func()
			if middleware.ChannelConcurrencyEnabled() {
				setting := operation_setting.GetChannelConcurrencySetting()
				admission := middleware.AcquireChannelCapacityWithMetric(
					channel.Id,
					middleware.ResolveChannelMaxConcurrency(channel),
					middleware.ResolveChannelRpmLimit(channel),
					time.Duration(setting.NormalizedRpmWindowSeconds())*time.Second,
					c.GetString(common.RequestIdKey),
				)
				if !admission.Acquired {
					retryParam.ExcludeChannels = appendUniqueChannelID(retryParam.ExcludeChannels, channel.Id)
					continue
				}
				releaseSlot = admission.Release
			}
			if setupErr := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName); setupErr != nil {
				if releaseSlot != nil {
					releaseSlot()
				}
				if !service.ShouldFallbackAfterSetupError(setupErr) {
					logger.LogInfo(c, fmt.Sprintf("responses auto-continue channel setup failed: channel=%d err=%s", channel.Id, setupErr.Error()))
					return nil, nil, false
				}
				retryParam.ExcludeChannels = appendUniqueChannelID(retryParam.ExcludeChannels, channel.Id)
				continue
			}
			service.CommitChannelCandidateSelection(retryParam, selectGroup)
			info.InitChannelMeta(c)
			return channel, releaseSlot, true
		}
		retryParam.IncreaseRetry()
	}
	return nil, nil, false
}

func requestResponsesContinuation(c *gin.Context, info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) (*dto.Usage, error) {
	strippedStreamOptions := false
	for attempt := 0; attempt < 2; attempt++ {
		adaptor := GetAdaptor(info.ApiType)
		if adaptor == nil {
			return nil, fmt.Errorf("invalid api type: %d", info.ApiType)
		}
		adaptor.Init(info)

		requestBody, err := buildResponsesRequestBody(c, info, adaptor, req, strippedStreamOptions)
		if err != nil {
			return nil, err
		}
		if attempt > 0 {
			if capacityErr := acquireAdditionalChannelRequestRpm(c, info); capacityErr != nil {
				return nil, capacityErr
			}
		}
		respAny, err := adaptor.DoRequest(c, info, requestBody)
		if err != nil {
			return nil, err
		}
		if respAny == nil {
			return nil, fmt.Errorf("empty upstream response")
		}
		resp := respAny.(*http.Response)
		if resp.StatusCode == http.StatusOK {
			usage, apiErr := openaichannel.OaiResponsesStreamHandlerWithOptions(c, info, resp, &openaichannel.ResponsesStreamHandlerOptions{
				ContinuationOutputOnly:  true,
				DisableAutoContinuation: true,
			})
			if apiErr != nil {
				return usage, apiErr
			}
			return usage, nil
		}

		issue := classifyUpstreamCompatibilityIssue(resp, info.RelayMode)
		if issue == upstreamCompatIssueStreamOptions && !strippedStreamOptions {
			strippedStreamOptions = true
			logCompatFallback(c, info, string(issue))
			_ = resp.Body.Close()
			continue
		}
		apiErr := service.RelayErrorHandler(c.Request.Context(), resp, false)
		service.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		if apiErr != nil {
			return nil, apiErr
		}
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("responses auto-continue exhausted compatibility fallback")
}

func buildResponsesRequestBody(c *gin.Context, info *relaycommon.RelayInfo, adaptor interface {
	ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error)
}, req *dto.OpenAIResponsesRequest, strippedStreamOptions bool) (io.Reader, error) {
	requestForUpstream, err := common.DeepCopy(req)
	if err != nil {
		return nil, err
	}
	relaycommon.NormalizeResponsesStreamOptions(requestForUpstream, info.SupportsResponsesStreamOptions)
	if strippedStreamOptions {
		requestForUpstream.StreamOptions = nil
	}
	if err := helper.ModelMappedHelper(c, info, requestForUpstream); err != nil {
		return nil, err
	}
	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *requestForUpstream)
	if err != nil {
		return nil, err
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, err
	}
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, err
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, err
		}
	}
	jsonData, err = relaycommon.NormalizeJSONStreamOptions(jsonData)
	if err != nil {
		return nil, err
	}
	if strippedStreamOptions {
		jsonData, err = relaycommon.RemoveJSONStreamOptions(jsonData)
		if err != nil {
			return nil, err
		}
	}
	service.SyncRelayReasoningEffortFromResponsesPayload(info, jsonData)
	service.CaptureFinalReasoningAdaptation(c, info, jsonData)
	return bytes.NewBuffer(jsonData), nil
}

func buildResponsesContinuationRequest(originalReq *dto.OpenAIResponsesRequest, outputText string) (*dto.OpenAIResponsesRequest, error) {
	req, err := common.DeepCopy(originalReq)
	if err != nil {
		return nil, err
	}
	req.Stream = true
	req.PreviousResponseID = ""
	prompt := fmt.Sprintf(responsesAutoContinuePrompt, tailRunes(outputText, responsesAutoContinueMaxTailRunes))
	if err := appendResponsesTextInput(req, prompt); err != nil {
		return nil, err
	}
	return req, nil
}

func appendResponsesTextInput(req *dto.OpenAIResponsesRequest, text string) error {
	if req == nil {
		return fmt.Errorf("nil responses request")
	}
	message := map[string]any{
		"role": "user",
		"content": []map[string]string{
			{
				"type": "input_text",
				"text": text,
			},
		},
	}

	var input any
	if len(req.Input) > 0 {
		if err := common.Unmarshal(req.Input, &input); err != nil {
			return err
		}
	}
	var nextInput []any
	switch v := input.(type) {
	case nil:
		nextInput = []any{message}
	case string:
		nextInput = []any{
			map[string]any{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": v,
					},
				},
			},
			message,
		}
	case []any:
		nextInput = append(v, message)
	default:
		nextInput = []any{v, message}
	}
	raw, err := common.Marshal(nextInput)
	if err != nil {
		return err
	}
	req.Input = raw
	return nil
}

func tailRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[len(runes)-maxRunes:])
}
