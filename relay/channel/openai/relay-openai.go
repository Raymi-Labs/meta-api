package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"

	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}

	if !forceFormat && !thinkToContent {
		return helper.StringData(c, data)
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		return err
	}
	sendFormatted := func(value any) error {
		raw, err := common.Marshal(value)
		if err != nil {
			return err
		}
		raw = preserveKimiChatUsage(info, []byte(data), raw, false)
		return helper.StringData(c, string(raw))
	}

	if !thinkToContent {
		return sendFormatted(lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}
	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return sendFormatted(response)
		}
	}

	if lastStreamResponse.Choices == nil || len(lastStreamResponse.Choices) == 0 {
		return sendFormatted(lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			if err := sendFormatted(response); err != nil {
				return err
			}
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}

	return sendFormatted(lastStreamResponse)
}

func chatCompletionsStreamEventError(data string) *types.NewAPIError {
	var payload map[string]any
	if common.UnmarshalJsonStr(data, &payload) != nil {
		return nil
	}
	if openAIError := dto.GetOpenAIError(payload["error"]); hasResponsesOpenAIError(openAIError) {
		return types.WithOpenAIError(*openAIError, responsesStreamErrorStatus(*openAIError))
	}
	var streamResponse dto.ChatCompletionsStreamResponse
	if common.UnmarshalJsonStr(data, &streamResponse) != nil {
		return nil
	}
	for _, choice := range streamResponse.Choices {
		if choice.FinishReason != nil && *choice.FinishReason == constant.FinishReasonContentFilter {
			return types.WithOpenAIError(types.OpenAIError{
				Message: "upstream rejected the request with finish_reason=content_filter",
				Type:    "invalid_request", Code: "content_filter",
			}, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	return nil
}

func decorateChatCompletionsPolicyFailureData(data string, apiErr *types.NewAPIError) string {
	if apiErr == nil {
		return data
	}
	openAIError := apiErr.ToOpenAIError()
	openAIError.Message = "本站警告：上游内容安全策略已拒绝并记录本次请求；请勿重复提交类似内容，反复触发将进入冷静期并提交管理员复核。"
	payload := map[string]any{"error": openAIError}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return data
	}
	return string(encoded)
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var streamItems []string // store stream items
	var lastStreamData string
	var lastUsageData string
	var kimiUsageEvidence kimiUsageStreamEvidence
	var secondLastStreamData string // 存储倒数第二个stream data，用于音频模型
	var terminalError *types.NewAPIError
	var completed bool
	var claudeTerminal *dto.ChatCompletionsStreamResponse
	// Only terminal metadata is deferred for Messages; content never waits for
	// another upstream block. Gemini keeps its existing conversion behavior.
	immediate := info.RelayFormat == types.RelayFormatOpenAI || info.RelayFormat == types.RelayFormatClaude
	thinkToContent := info.ChannelSetting.ThinkingToContent
	if request, ok := info.Request.(*dto.GeneralOpenAIRequest); ok && request.ResponseFormat != nil {
		if request.ResponseFormat.Type == "json_object" || request.ResponseFormat.Type == "json_schema" {
			thinkToContent = false // Reasoning must not corrupt structured content.
		}
	}

	// 检查是否为音频模型
	isAudioModel := strings.Contains(strings.ToLower(model), "audio")

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		if !immediate && lastStreamData != "" {
			err := HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
			if err != nil {
				terminalError = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				return false
			}
		}
		if len(data) > 0 {
			data = string(kimiUsageEvidence.normalize(info, []byte(data)))
			if eventError := chatCompletionsStreamEventError(data); eventError != nil {
				terminalError = eventError
				if service.IsContentSafetyPolicyError(eventError) {
					terminalError = service.NormalizeContentSafetyPolicyError(eventError)
					if info.RelayFormat == types.RelayFormatOpenAI {
						_ = sendStreamData(c, info, decorateChatCompletionsPolicyFailureData(data, eventError), false, false)
						common.SetContextKey(c, constant.ContextKeyResponsesStreamErrorWritten, true)
					}
				} else if info.SendResponseCount > 0 && info.RelayFormat == types.RelayFormatOpenAI {
					// Once partial output reached the client, retrying on another channel
					// would splice two unrelated streams together.
					_ = sendStreamData(c, info, data, false, false)
					common.SetContextKey(c, constant.ContextKeyResponsesStreamErrorWritten, true)
					terminalError = types.WithOpenAIError(eventError.ToOpenAIError(), eventError.StatusCode, types.ErrOptionWithSkipRetry())
				}
				return false
			}
			info.ObserveOpenAIStreamOutput(data)
			var event dto.ChatCompletionsStreamResponse
			if common.UnmarshalJsonStr(data, &event) == nil {
				info.ObserveResponseModel(event.Model)
				for _, choice := range event.Choices {
					completed = completed || (choice.FinishReason != nil && *choice.FinishReason != "")
				}
			} else {
				info.StreamStatus.RecordError("invalid OpenAI stream event")
				terminalError = types.NewOpenAIError(fmt.Errorf("invalid OpenAI stream event"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				return false
			}
			if err := validateKimiUsageTotals(info, event.Usage); err != nil {
				info.StreamStatus.RecordError("invalid K3 usage totals")
				terminalError = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				return false
			}
			// Some providers append metadata chunks after the standard usage chunk.
			// Keep the latest valid usage instead of assuming it is the final SSE event.
			updateUsageFromStreamData(data, &usage, &containStreamUsage)
			if usesKimiChatUsage(info) && event.Usage != nil {
				observed := *event.Usage
				normalizeOpenAIUsage(&observed)
				if service.ValidUsage(&observed) {
					lastUsageData = data
				}
			}

			// 对音频模型，保存倒数第二个stream data
			if isAudioModel && lastStreamData != "" {
				secondLastStreamData = lastStreamData
			}

			lastStreamData = data
			streamItems = append(streamItems, data)
			if info.RelayFormat == types.RelayFormatClaude {
				if err := deliverClaudeStreamEvent(c, info, event, &claudeTerminal); err != nil {
					info.StreamStatus.RecordError("failed to deliver Messages stream event")
					terminalError = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
					return false
				}
			} else if immediate {
				out, err := filterChatStreamUsage(data, info.ShouldIncludeUsage)
				if err == nil && out != "" {
					err = HandleStreamFormat(c, info, out, info.ChannelSetting.ForceFormat, thinkToContent)
				}
				if err != nil {
					info.StreamStatus.RecordError("failed to deliver OpenAI stream event")
					return false
				}
			}
		}
		return true
	})
	if terminalError != nil {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeFailed)
		return nil, finishChatStreamError(c, info, terminalError)
	}
	info.StreamStatus.RequireTerminal()
	if completed || (info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone) {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeCompleted)
	}
	if !completed && info.StreamStatus != nil && info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone {
		info.StreamStatus.RecordError("OpenAI stream ended without a completion marker")
	}
	if !info.StreamStatus.IsSuccessful() {
		if c.Request.Context().Err() != nil {
			return nil, types.NewClientCanceledError()
		}
		return nil, finishChatStreamError(c, info, types.NewOpenAIError(
			fmt.Errorf("upstream stream ended unsuccessfully: %s", info.StreamStatus.EndReason),
			types.ErrorCodeBadResponseBody, http.StatusBadGateway))
	}

	// 对音频模型，从倒数第二个stream data中提取usage信息
	if isAudioModel && secondLastStreamData != "" {
		var streamResp struct {
			Usage *dto.Usage `json:"usage"`
		}
		err := common.Unmarshal([]byte(secondLastStreamData), &streamResp)
		if err == nil && streamResp.Usage != nil && service.ValidUsage(streamResp.Usage) {
			usage = streamResp.Usage
			containStreamUsage = true

			if common.DebugEnabled {
				logger.LogDebug(c, fmt.Sprintf("Audio model usage extracted from second last SSE: PromptTokens=%d, CompletionTokens=%d, TotalTokens=%d, InputTokens=%d, OutputTokens=%d",
					usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
					usage.InputTokens, usage.OutputTokens))
			}
		}
	}

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		logger.LogError(c, fmt.Sprintf("error handling last response: %s, lastStreamData: [%s]", err.Error(), lastStreamData))
	}

	// 处理token计算
	if err := processTokens(info.RelayMode, streamItems, &responseTextBuilder, &toolCount); err != nil {
		logger.LogError(c, "error processing tokens: "+err.Error())
	}

	if !containStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
	}

	usageData := lastStreamData
	if usesKimiChatUsage(info) && lastUsageData != "" {
		usageData = lastUsageData
	}
	applyUsagePostProcessing(info, usage, common.StringToByteSlice(usageData))

	if info.RelayFormat == types.RelayFormatClaude {
		if claudeTerminal == nil {
			claudeTerminal = &dto.ChatCompletionsStreamResponse{
				Choices: []dto.ChatCompletionsStreamResponseChoice{{FinishReason: common.GetPointer("stop")}},
			}
		}
		claudeTerminal.Usage = usage
		final, err := common.Marshal(claudeTerminal)
		if err == nil {
			err = HandleStreamFormat(c, info, string(final), false, false)
		}
		if err != nil {
			info.StreamStatus.RecordError("failed to finalize Messages stream")
			return nil, finishChatStreamError(c, info, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway))
		}
	} else {
		HandleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, usage, containStreamUsage)
	}

	return usage, nil
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if common.DebugEnabled {
		println("upstream response body:", openAIDebugResponseBodyForLog(responseBody))
	}
	// Unmarshal to simpleResponse
	if info.ChannelType == constant.ChannelTypeOpenRouter && info.ChannelOtherSettings.IsOpenRouterEnterprise() {
		// 尝试解析为 openrouter enterprise
		var enterpriseResponse openrouter.OpenRouterEnterpriseResponse
		err = common.Unmarshal(responseBody, &enterpriseResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if enterpriseResponse.Success {
			responseBody = enterpriseResponse.Data
		} else {
			logger.LogError(c, fmt.Sprintf("openrouter enterprise response success=false, data: %s", enterpriseResponse.Data))
			return nil, types.NewOpenAIError(fmt.Errorf("openrouter response success=false"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	var kimiUsageEvidence kimiUsageStreamEvidence
	responseBody = kimiUsageEvidence.normalize(info, responseBody)
	reportedBody := responseBody
	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := simpleResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	if err := validateKimiUsageTotals(info, &simpleResponse.Usage); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	info.ObserveResponseModel(simpleResponse.Model)

	for _, choice := range simpleResponse.Choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
			return nil, types.WithOpenAIError(types.OpenAIError{
				Message: "upstream rejected the request with finish_reason=content_filter",
				Type:    "invalid_request", Code: "content_filter",
			}, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	if usesKimiChatUsage(info) {
		// Match the streaming total-token normalization before deciding
		// whether local estimates are needed. Count authority is reapplied
		// below from raw evidence, not from these generic alias copies.
		normalizeOpenAIUsage(&simpleResponse.Usage)
	}
	usageModified := false
	if simpleResponse.Usage.PromptTokens == 0 {
		completionTokens := simpleResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			for _, choice := range simpleResponse.Choices {
				ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.ReasoningContent+choice.Message.Reasoning, info.UpstreamModelName)
				completionTokens += ctkm
			}
		}
		simpleResponse.Usage = dto.Usage{
			InputTokensEstimated: true,
			PromptTokens:         info.GetEstimatePromptTokens(),
			CompletionTokens:     completionTokens,
			TotalTokens:          info.GetEstimatePromptTokens() + completionTokens,
		}
		usageModified = true
	}

	applyUsagePostProcessing(info, &simpleResponse.Usage, responseBody)

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if usageModified {
			var bodyMap map[string]interface{}
			err = common.Unmarshal(responseBody, &bodyMap)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			bodyMap["usage"] = simpleResponse.Usage
			responseBody, _ = common.Marshal(bodyMap)
		}
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		}
		responseBody = preserveKimiChatUsage(info, reportedBody, responseBody, usageModified)
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(&simpleResponse, info)
		claudeRespStr, err := common.Marshal(claudeResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(&simpleResponse, info)
		geminiRespStr, err := common.Marshal(geminiResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = geminiRespStr
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}

func openAIDebugResponseBodyForLog(responseBody []byte) string {
	const maxDebugResponseBodyBytes = 8 * 1024
	if len(responseBody) > maxDebugResponseBodyBytes {
		return fmt.Sprintf("<response omitted: %d bytes>", len(responseBody))
	}
	return string(responseBody)
}

func streamTTSResponse(c *gin.Context, resp *http.Response) {
	c.Writer.WriteHeaderNow()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		logger.LogWarn(c, "streaming not supported")
		_, err := io.Copy(c.Writer, resp.Body)
		if err != nil {
			logger.LogWarn(c, err.Error())
		}
		return
	}

	buffer := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buffer)
		//logger.LogInfo(c, fmt.Sprintf("streamTTSResponse read %d bytes", n))
		if n > 0 {
			if _, writeErr := c.Writer.Write(buffer[:n]); writeErr != nil {
				logger.LogError(c, writeErr.Error())
				break
			}
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				logger.LogError(c, err.Error())
			}
			break
		}
	}
}

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	sendChan := make(chan []byte, 100)
	receiveChan := make(chan []byte, 100)
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate {
					if realtimeEvent.Session != nil {
						if realtimeEvent.Session.Tools != nil {
							info.RealtimeTools = realtimeEvent.Session.Tools
						}
					}
				}

				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
				if err != nil {
					errChan <- fmt.Errorf("error counting text token: %v", err)
					return
				}
				logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
				localUsage.TotalTokens += textToken + audioToken
				localUsage.InputTokens += textToken + audioToken
				localUsage.InputTokenDetails.TextTokens += textToken
				localUsage.InputTokenDetails.AudioTokens += audioToken

				err = helper.WssString(c, targetConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}

				select {
				case sendChan <- message:
				default:
				}
			}
		}
	})

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
					realtimeUsage := realtimeEvent.Response.Usage
					if realtimeUsage != nil {
						usage.TotalTokens += realtimeUsage.TotalTokens
						usage.InputTokens += realtimeUsage.InputTokens
						usage.OutputTokens += realtimeUsage.OutputTokens
						usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
						usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
						usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
						usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
						usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
						err := preConsumeUsage(c, info, usage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						usage = &dto.RealtimeUsage{}

						localUsage = &dto.RealtimeUsage{}
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							errChan <- fmt.Errorf("error counting text token: %v", err)
							return
						}
						logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						info.IsFirstRequest = false
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
						err = preConsumeUsage(c, info, localUsage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						localUsage = &dto.RealtimeUsage{}
						// print now usage
					}
					logger.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", sumUsage))
					logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))
					logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))

				} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
					realtimeSession := realtimeEvent.Session
					if realtimeSession != nil {
						// update audio format
						info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
						info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
					}
				} else {
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						errChan <- fmt.Errorf("error counting text token: %v", err)
						return
					}
					logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					localUsage.TotalTokens += textToken + audioToken
					localUsage.OutputTokens += textToken + audioToken
					localUsage.OutputTokenDetails.TextTokens += textToken
					localUsage.OutputTokenDetails.AudioTokens += audioToken
				}

				err = helper.WssString(c, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				select {
				case receiveChan <- message:
				default:
				}
			}
		}
	})

	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		//return service.OpenAIErrorWrapper(err, "realtime_error", http.StatusInternalServerError), nil
		logger.LogError(c, "realtime error: "+err.Error())
	case <-c.Done():
	}

	if usage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, usage, sumUsage)
	}

	if localUsage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	// check usage total tokens, if 0, use local usage

	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens
	// clear usage
	err := service.PreWssConsumeQuota(ctx, info, usage)
	return err
}

func OpenaiHandlerWithUsage(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") || strings.HasPrefix(strings.TrimSpace(string(responseBody)), "data:") {
		service.IOCopyBytesGracefully(c, resp, responseBody)
		return &dto.Usage{}, nil
	}

	var usageResp dto.SimpleResponse
	err = common.Unmarshal(responseBody, &usageResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := usageResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	// 写入新的 response body
	if info != nil && info.RelayFormat == types.RelayFormatOpenAIImage {
		_, images, present, err := parseOpenAIImagePayloads(responseBody)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		if present {
			count := len(images)
			info.ImageResponseCount = &count
		}
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)

	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)
	return &usageResp.Usage, nil
}

func normalizeOpenAIUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	usage.NormalizeWebSearchUsage()
	if usage.InputTokens != 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.OutputTokens != 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails = usage.InputTokensDetails.Clone()
	}
	if usage.OutputTokensDetails != nil {
		usage.CompletionTokenDetails = *usage.OutputTokensDetails
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
}

func OpenaiImageStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid image stream response")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return OpenaiHandlerWithUsage(c, info, resp)
	}
	if !strings.Contains(contentType, "text/event-stream") {
		return OpenaiImageJSONAsStreamHandler(c, info, resp)
	}

	usage := &dto.Usage{}
	var lastStreamData []byte

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		raw := common.StringToByteSlice(data)
		lastStreamData = raw

		shouldContinue := true
		if isOpenAIImageStreamErrorEvent(raw) {
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.RecordError(extractOpenAIImageStreamErrorMessage(raw))
			}
			shouldContinue = false
		}

		var usageResp dto.SimpleResponse
		if err := common.Unmarshal(raw, &usageResp); err == nil {
			normalizeOpenAIUsage(&usageResp.Usage)
			if service.ValidUsage(&usageResp.Usage) {
				usage = &usageResp.Usage
			}
		}

		if err := writeOpenaiImageStreamChunk(c, raw); err != nil {
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
			}
			return false
		}

		return shouldContinue
	})

	if info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone {
		helper.Done(c)
	}

	applyUsagePostProcessing(info, usage, lastStreamData)
	return usage, nil
}

func writeOpenaiImageStreamChunk(c *gin.Context, data []byte) error {
	var payload struct {
		Type string `json:"type"`
	}
	_ = common.Unmarshal(data, &payload)
	if eventName := strings.TrimSpace(payload.Type); eventName != "" {
		c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", eventName)})
	}
	c.Render(-1, common.CustomEvent{Data: "data: " + string(data)})
	return helper.FlushWriter(c)
}

func isOpenAIImageStreamErrorEvent(data []byte) bool {
	if !json.Valid(data) {
		return false
	}
	var payload struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error"`
	}
	if err := common.Unmarshal(data, &payload); err != nil {
		return false
	}
	payloadType := strings.ToLower(strings.TrimSpace(payload.Type))
	return payloadType == "error" || payloadType == "upstream_error" || len(payload.Error) > 0
}

func extractOpenAIImageStreamErrorMessage(data []byte) string {
	if len(data) == 0 || !json.Valid(data) {
		return "upstream image stream returned error event"
	}
	var payload struct {
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if err := common.Unmarshal(data, &payload); err != nil {
		return "upstream image stream returned error event"
	}
	if msg := strings.TrimSpace(payload.Message); msg != "" {
		return msg
	}
	if len(payload.Error) > 0 {
		var nested struct {
			Message string `json:"message"`
		}
		if err := common.Unmarshal(payload.Error, &nested); err == nil {
			if msg := strings.TrimSpace(nested.Message); msg != "" {
				return msg
			}
		}
		if msg := strings.TrimSpace(common.JsonRawMessageToString(payload.Error)); msg != "" {
			return msg
		}
	}
	return "upstream image stream returned error event"
}

func OpenaiImageJSONAsStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	created, images, present, err := parseOpenAIImagePayloads(responseBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	var usageResp dto.SimpleResponse
	_ = common.Unmarshal(responseBody, &usageResp)
	if oaiError := usageResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)

	if !present {
		return nil, types.NewOpenAIError(fmt.Errorf("missing image data"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if info != nil {
		count := len(images)
		info.ImageResponseCount = &count
	}
	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)

	if created == 0 {
		created = time.Now().Unix()
	}
	if info != nil {
		info.SetFirstResponseTime()
	}
	for _, image := range images {
		payload := map[string]any{
			"type":       "image_generation.completed",
			"created_at": created,
		}
		if image.Url != "" {
			payload["url"] = image.Url
		}
		if image.B64Json != "" {
			payload["b64_json"] = image.B64Json
		}
		if image.RevisedPrompt != "" {
			payload["revised_prompt"] = image.RevisedPrompt
		}
		if service.ValidUsage(&usageResp.Usage) {
			payload["usage"] = usageResp.Usage
		}
		if err := writeOpenaiImageStreamPayload(c, "image_generation.completed", payload); err != nil {
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
			}
			return &usageResp.Usage, nil
		}
	}
	if err := writeOpenaiImageStreamDone(c); err != nil {
		if info != nil && info.StreamStatus != nil {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
		}
		return &usageResp.Usage, nil
	}
	if info != nil {
		info.ReceivedResponseCount += len(images)
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	return &usageResp.Usage, nil
}

func writeOpenaiImageStreamPayload(c *gin.Context, eventName string, payload any) error {
	data, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	if eventName != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
		return err
	}
	return helper.FlushWriter(c)
}

func writeOpenaiImageStreamDone(c *gin.Context) error {
	if _, err := fmt.Fprint(c.Writer, "data: [DONE]\n\n"); err != nil {
		return err
	}
	return helper.FlushWriter(c)
}

func applyUsagePostProcessing(info *relaycommon.RelayInfo, usage *dto.Usage, responseBody []byte) {
	if info == nil || usage == nil || info.ChannelMeta == nil {
		return
	}
	if usesKimiChatUsage(info) {
		root := usageObject(responseBody)
		reported := usageObject(root["usage"])
		if reported != nil {
			choice := singleChoiceUsage(info, root)
			// Generic normalization can have copied a lower-priority alias.
			// Rebuild both internal counts from the same authority rules as
			// the wire normalizer and diagnostics, including unknown/invalid.
			// These zero defaults are internal only, never fabricated on wire.
			usage.PromptTokensDetails.CachedTokens = 0
			usage.CompletionTokenDetails.ReasoningTokens = 0
			if evidence := kimiCountEvidence(reported, choice, kimiUsageFields[0]); evidence.Status == "reported" {
				usage.PromptTokensDetails.CachedTokens = *evidence.Value
			}
			if evidence := kimiCountEvidence(reported, choice, kimiUsageFields[1]); evidence.Status == "reported" {
				usage.CompletionTokenDetails.ReasoningTokens = *evidence.Value
			}
			return
		}
		// No response-level usage: keep the legacy provider-specific fallback.
	}

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek:
		if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
		}
	case constant.ChannelTypeZhipu_v4:
		// 智普的cached_tokens在标准位置: usage.prompt_tokens_details.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeMoonshot:
		// Moonshot的cached_tokens在非标准位置: choices[].usage.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractMoonshotCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeOpenAI:
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if cachedTokens, ok := extractLlamaCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			}
		}
	}
}

func extractCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CachedTokens         *int `json:"cached_tokens"`
			PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Usage.PromptTokensDetails.CachedTokens != nil {
		return *payload.Usage.PromptTokensDetails.CachedTokens, true
	}
	if payload.Usage.CachedTokens != nil {
		return *payload.Usage.CachedTokens, true
	}
	if payload.Usage.PromptCacheHitTokens != nil {
		return *payload.Usage.PromptCacheHitTokens, true
	}
	return 0, false
}

// extractMoonshotCachedTokensFromBody 从Moonshot的非标准位置提取cached_tokens
// Moonshot的流式响应格式: {"choices":[{"usage":{"cached_tokens":111}}]}
func extractMoonshotCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Choices []struct {
			Usage struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"usage"`
		} `json:"choices"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	// 遍历choices查找cached_tokens
	for _, choice := range payload.Choices {
		if choice.Usage.CachedTokens != nil && *choice.Usage.CachedTokens > 0 {
			return *choice.Usage.CachedTokens, true
		}
	}

	return 0, false
}

// extractLlamaCachedTokensFromBody 从llama.cpp的非标准位置提取cache_n。
func extractLlamaCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Timings struct {
			CachedTokens *int `json:"cache_n"`
		} `json:"timings"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}
	if payload.Timings.CachedTokens == nil {
		return 0, false
	}
	return *payload.Timings.CachedTokens, true
}
