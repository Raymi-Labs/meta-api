package relay

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func applySystemPromptIfNeeded(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil {
		return
	}
	if info.ChannelSetting.SystemPrompt == "" {
		return
	}

	systemRole := request.GetSystemRoleName()

	containSystemPrompt := false
	for _, message := range request.Messages {
		if message.Role == systemRole && len(message.Tools) == 0 {
			containSystemPrompt = true
			break
		}
	}
	if !containSystemPrompt {
		systemMessage := dto.Message{
			Role:    systemRole,
			Content: info.ChannelSetting.SystemPrompt,
		}
		request.Messages = append([]dto.Message{systemMessage}, request.Messages...)
		return
	}

	if !info.ChannelSetting.SystemPromptOverride {
		return
	}

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
	for i, message := range request.Messages {
		if message.Role != systemRole || len(message.Tools) > 0 {
			continue
		}
		if message.IsStringContent() {
			request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
			return
		}
		contents := message.ParseContent()
		contents = append([]dto.MediaContent{
			{
				Type: dto.ContentTypeText,
				Text: info.ChannelSetting.SystemPrompt,
			},
		}, contents...)
		request.Messages[i].Content = contents
		return
	}
}

func chatCompletionsViaResponses(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.GeneralOpenAIRequest) (*dto.Usage, *types.NewAPIError) {
	// Keep the original Chat request intact if this bridge falls back.
	requestCopy := *request
	request = &requestCopy
	service.AdaptChatReasoningForResponses(info, request)
	chatJSON, err := common.Marshal(request)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	chatJSON, err = relaycommon.RemoveDisabledFields(chatJSON, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	if len(info.ParamOverride) > 0 {
		chatJSON, err = relaycommon.ApplyParamOverrideWithRelayInfo(chatJSON, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}

	var overriddenChatReq dto.GeneralOpenAIRequest
	if err := common.Unmarshal(chatJSON, &overriddenChatReq); err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
	}
	toolProtocol, err := service.NormalizeChatToolProtocol(&overriddenChatReq)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	info.ChatToolProtocol = toolProtocol
	info.ChatToolCount = len(overriddenChatReq.Tools)

	requestForResponses := &overriddenChatReq
	var fallbackChatReq *dto.GeneralOpenAIRequest
	var previousResponseID string
	if sessionMatch, err := service.ApplyResponsesSessionBridge(info, &overriddenChatReq); err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	} else if sessionMatch != nil {
		requestForResponses = sessionMatch.Trimmed
		fallbackChatReq = &overriddenChatReq
		previousResponseID = sessionMatch.ResponseID
	}

	responsesReq, err := service.ChatCompletionsRequestToResponsesRequest(requestForResponses)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if previousResponseID != "" {
		responsesReq.PreviousResponseID = previousResponseID
	}
	if fallbackChatReq != nil {
		if err := restoreResponsesInstructionsFromOriginalChat(responsesReq, fallbackChatReq); err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	var fallbackResponsesReq *dto.OpenAIResponsesRequest
	if fallbackChatReq != nil {
		fallbackResponsesReq, err = service.ChatCompletionsRequestToResponsesRequest(fallbackChatReq)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	info.AppendRequestConversion(types.RelayFormatOpenAIResponses)

	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	defer func() {
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
	}()

	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestURLPath = "/v1/responses"

	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *responsesReq)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	strippedStreamOptions := false
	retriedWithoutPreviousResponseID := false
	// clientWantsStream 记录客户端的原始 stream 意图。codex 上游被 newapi 恒定强制流式后,
	// 绝不能再用「上游返回 SSE」去污染 info.IsStream(否则非流式客户端会拿到无法解析的 SSE);
	// 响应分派严格按「客户端意图 + 上游实际类型」决定,info.IsStream 全程保持客户端原始意图。
	clientWantsStream := info.IsStream
	var httpResp *http.Response
	var upstreamIsStream bool
	requestAttempt := 0
	for {
		requestTemplate := responsesReq
		if retriedWithoutPreviousResponseID && fallbackResponsesReq != nil {
			requestTemplate = fallbackResponsesReq
		}

		responsesReqForUpstream, err := common.DeepCopy(requestTemplate)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		if strippedStreamOptions {
			responsesReqForUpstream.StreamOptions = nil
		}
		if retriedWithoutPreviousResponseID {
			responsesReqForUpstream.PreviousResponseID = ""
		}

		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *responsesReqForUpstream)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err = relaycommon.NormalizeJSONStreamOptions(jsonData)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		if strippedStreamOptions {
			jsonData, err = relaycommon.RemoveJSONStreamOptions(jsonData)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
		}

		if requestAttempt > 0 {
			if capacityErr := acquireAdditionalChannelRequestRpm(c, info); capacityErr != nil {
				return nil, capacityErr
			}
		}
		requestAttempt++
		service.SyncRelayReasoningEffortFromResponsesPayload(info, jsonData)
		service.CaptureFinalReasoningAdaptation(c, info, jsonData)
		resp, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		if resp == nil {
			return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}

		httpResp = resp.(*http.Response)
		upstreamIsStream = strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode == http.StatusOK {
			break
		}

		issue := classifyUpstreamCompatibilityIssue(httpResp, info.RelayMode)
		if issue == upstreamCompatIssueStreamOptions && !strippedStreamOptions {
			strippedStreamOptions = true
			logCompatFallback(c, info, string(issue))
			_ = httpResp.Body.Close()
			continue
		}
		if issue == upstreamCompatIssuePreviousResponseID && !retriedWithoutPreviousResponseID {
			retriedWithoutPreviousResponseID = true
			service.MarkResponsesPreviousResponseIDUnsupported(info)
			logCompatFallback(c, info, string(issue))
			_ = httpResp.Body.Close()
			continue
		}

		newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}

	if clientWantsStream {
		// 客户端要流式 → 转成 chat/claude SSE 转发(上游此时必为 SSE)。
		usage, newApiErr := openaichannel.OaiResponsesToChatStreamHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}

	if upstreamIsStream {
		// 客户端要非流式,但 codex 上游被强制流式返回 SSE → 聚合成非流式 JSON 返回,对客户端透明。
		usage, newApiErr := openaichannel.OaiResponsesToChatAggregateHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}

	usage, newApiErr := openaichannel.OaiResponsesToChatHandler(c, info, httpResp)
	if newApiErr != nil {
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}
	return usage, nil
}

func restoreResponsesInstructionsFromOriginalChat(responsesReq *dto.OpenAIResponsesRequest, originalChatReq *dto.GeneralOpenAIRequest) error {
	if responsesReq == nil || originalChatReq == nil || len(responsesReq.Instructions) > 0 {
		return nil
	}
	var parts []string
	for _, msg := range originalChatReq.Messages {
		if msg.Role != "system" && msg.Role != "developer" {
			continue
		}
		if msg.IsStringContent() {
			if text := strings.TrimSpace(msg.StringContent()); text != "" {
				parts = append(parts, text)
			}
			continue
		}
		for _, item := range msg.ParseContent() {
			if item.Type == dto.ContentTypeText && strings.TrimSpace(item.Text) != "" {
				parts = append(parts, strings.TrimSpace(item.Text))
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	instructions, err := common.Marshal(strings.Join(parts, "\n\n"))
	if err != nil {
		return err
	}
	responsesReq.Instructions = instructions
	return nil
}
