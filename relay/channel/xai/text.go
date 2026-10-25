package xai

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func streamResponseXAI2OpenAI(xAIResp *dto.ChatCompletionsStreamResponse) *dto.ChatCompletionsStreamResponse {
	if xAIResp == nil {
		return nil
	}
	normalizeXAIUsage(xAIResp.Usage)
	openAIResp := &dto.ChatCompletionsStreamResponse{
		Id:                xAIResp.Id,
		Object:            xAIResp.Object,
		Created:           xAIResp.Created,
		Model:             xAIResp.Model,
		SystemFingerprint: xAIResp.SystemFingerprint,
		Choices:           xAIResp.Choices,
		Usage:             xAIResp.Usage,
	}

	return openAIResp
}

func xAIStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewError(errors.New("invalid xAI stream response"), types.ErrorCodeBadResponseBody)
	}
	ensureXAIClaudeConvertInfo(info)

	usage := &dto.Usage{}
	var responseTextBuilder strings.Builder
	var toolCount int
	var containStreamUsage bool
	var lastStreamData string
	var handlerErr error
	var upstreamAPIError *types.NewAPIError

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		var xAIResp *struct {
			dto.ChatCompletionsStreamResponse
			Error any `json:"error,omitempty"`
		}
		if err := common.UnmarshalJsonStr(data, &xAIResp); err != nil {
			handlerErr = fmt.Errorf("decode xAI stream chunk: %w", err)
			return false
		}
		if xAIResp == nil {
			handlerErr = errors.New("xAI stream chunk is null")
			return false
		}
		if openAIError := dto.GetOpenAIError(xAIResp.Error); openAIError != nil {
			upstreamAPIError = types.WithOpenAIError(*openAIError, xAIErrorStatusCode(resp))
			return false
		}
		openAIResponse := streamResponseXAI2OpenAI(&xAIResp.ChatCompletionsStreamResponse)

		if service.ValidUsage(openAIResponse.Usage) {
			containStreamUsage = true
			*usage = *openAIResponse.Usage
			if info != nil && info.ClaudeConvertInfo != nil {
				info.ClaudeConvertInfo.Usage = usage
			}
		}

		_ = openai.ProcessStreamResponse(*openAIResponse, &responseTextBuilder, &toolCount)
		openAIResponseData, err := common.Marshal(openAIResponse)
		if err != nil {
			handlerErr = fmt.Errorf("encode normalized xAI stream chunk: %w", err)
			return false
		}
		if lastStreamData != "" {
			if err := openai.HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
				if requestContextDone(c) {
					return false
				}
				handlerErr = fmt.Errorf("convert xAI stream response: %w", err)
				return false
			}
		}
		lastStreamData = string(openAIResponseData)
		return true
	})

	if requestContextDone(c) {
		return usage, nil
	}
	if upstreamAPIError != nil {
		return nil, upstreamAPIError
	}
	if handlerErr != nil {
		return nil, types.NewError(handlerErr, types.ErrorCodeBadResponseBody)
	}
	if lastStreamData == "" {
		if requestContextDone(c) {
			return usage, nil
		}
		return nil, types.NewError(errors.New("xAI stream ended without response data"), types.ErrorCodeBadResponseBody)
	}

	usage = ensureXAIUsage(c, usage, responseTextBuilder.String(), toolCount, info.UpstreamModelName, info.GetEstimatePromptTokens())
	if info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if err := finalizeXAIClaudeStream(c, info, lastStreamData, usage); err != nil && !requestContextDone(c) {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatOpenAI:
		if err := openai.HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil && !requestContextDone(c) {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		helper.Done(c)
	default:
		openai.HandleFinalResponse(c, info, lastStreamData, "", 0, info.UpstreamModelName, "", usage, containStreamUsage)
	}

	return usage, nil
}

func xAIHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewError(errors.New("invalid xAI response"), types.ErrorCodeBadResponseBody)
	}
	defer service.CloseResponseBodyGracefully(resp)
	ensureXAIClaudeConvertInfo(info)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	var xaiResponse ChatCompletionResponse
	err = common.Unmarshal(responseBody, &xaiResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if openAIError := dto.GetOpenAIError(xaiResponse.Error); openAIError != nil {
		return nil, types.WithOpenAIError(*openAIError, xAIErrorStatusCode(resp))
	}
	normalizeXAIUsage(xaiResponse.Usage)
	responseText, toolCount := xAIResponseText(&xaiResponse)
	usage := ensureXAIUsage(c, xaiResponse.Usage, responseText, toolCount, info.UpstreamModelName, info.GetEstimatePromptTokens())
	xaiResponse.Usage = usage

	openAIResponse := &dto.OpenAITextResponse{
		Id:      xaiResponse.Id,
		Object:  xaiResponse.Object,
		Created: xaiResponse.Created,
		Model:   xaiResponse.Model,
		Choices: xaiResponse.Choices,
		Usage:   *usage,
	}
	var responseObject any = &xaiResponse
	if info.RelayFormat == types.RelayFormatClaude {
		responseObject = service.ResponseOpenAI2Claude(openAIResponse, info)
	}

	encodeJson, err := common.Marshal(responseObject)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	service.IOCopyBytesGracefully(c, resp, encodeJson)

	return usage, nil
}

func normalizeXAIUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.PromptTokens < 0 {
		usage.PromptTokens = 0
	}
	if usage.CompletionTokens < 0 {
		usage.CompletionTokens = 0
	}
	if usage.TotalTokens < 0 {
		usage.TotalTokens = 0
	}
	if usage.CompletionTokenDetails.ReasoningTokens < 0 {
		usage.CompletionTokenDetails.ReasoningTokens = 0
	}
	if usage.TotalTokens >= usage.PromptTokens && usage.TotalTokens > 0 {
		usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens
	} else if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	textTokens := usage.CompletionTokens - usage.CompletionTokenDetails.ReasoningTokens
	if textTokens < 0 {
		textTokens = 0
	}
	usage.CompletionTokenDetails.TextTokens = textTokens
}

func ensureXAIUsage(c *gin.Context, usage *dto.Usage, responseText string, toolCount int, model string, estimatedPromptTokens int) *dto.Usage {
	if usage == nil {
		usage = &dto.Usage{}
	}
	normalizeXAIUsage(usage)
	if usage.PromptTokens == 0 {
		usage.PromptTokens = estimatedPromptTokens
		usage.InputTokensEstimated = true
	}
	if usage.CompletionTokens == 0 && (responseText != "" || toolCount > 0) {
		estimated := service.ResponseText2Usage(c, responseText, model, usage.PromptTokens)
		usage.CompletionTokens = estimated.CompletionTokens + toolCount*7
		usage.InputTokensEstimated = true
	}
	if usage.TotalTokens == 0 || usage.TotalTokens < usage.PromptTokens+usage.CompletionTokens {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.CompletionTokenDetails.TextTokens == 0 && usage.CompletionTokens > usage.CompletionTokenDetails.ReasoningTokens {
		usage.CompletionTokenDetails.TextTokens = usage.CompletionTokens - usage.CompletionTokenDetails.ReasoningTokens
	}
	return usage
}

func xAIResponseText(response *ChatCompletionResponse) (string, int) {
	if response == nil {
		return "", 0
	}
	var builder strings.Builder
	toolCount := 0
	for _, choice := range response.Choices {
		builder.WriteString(choice.Message.StringContent())
		builder.WriteString(choice.Message.ReasoningContent)
		builder.WriteString(choice.Message.Reasoning)
		toolCalls := choice.Message.ParseToolCalls()
		toolCount += len(toolCalls)
		for _, toolCall := range toolCalls {
			builder.WriteString(toolCall.Function.Name)
			builder.WriteString(toolCall.Function.Arguments)
		}
	}
	return builder.String(), toolCount
}

func finalizeXAIClaudeStream(c *gin.Context, info *relaycommon.RelayInfo, lastStreamData string, usage *dto.Usage) error {
	var lastResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(lastStreamData, &lastResponse); err != nil {
		return fmt.Errorf("decode final xAI stream chunk: %w", err)
	}
	if info.ClaudeConvertInfo.Done {
		return nil
	}
	if lastResponse.IsFinished() {
		if info.SendResponseCount == 0 {
			return openai.HandleStreamFormat(c, info, lastStreamData, false, false)
		}
		openai.HandleFinalResponse(c, info, lastStreamData, "", 0, info.UpstreamModelName, "", usage, true)
		return nil
	}
	if len(lastResponse.Choices) > 0 || info.SendResponseCount == 0 {
		if err := openai.HandleStreamFormat(c, info, lastStreamData, false, false); err != nil {
			return err
		}
	}
	if info.ClaudeConvertInfo.Done {
		return nil
	}
	finishReason := "stop"
	terminalResponse := &dto.ChatCompletionsStreamResponse{
		Id:      lastResponse.Id,
		Object:  lastResponse.Object,
		Created: lastResponse.Created,
		Model:   lastResponse.Model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index:        0,
				FinishReason: &finishReason,
			},
		},
		Usage: usage,
	}
	terminalData, err := common.Marshal(terminalResponse)
	if err != nil {
		return fmt.Errorf("encode terminal xAI stream chunk: %w", err)
	}
	return openai.HandleStreamFormat(c, info, string(terminalData), false, false)
}

func ensureXAIClaudeConvertInfo(info *relaycommon.RelayInfo) {
	if info != nil && info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}
}

func requestContextDone(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.Context().Err() != nil
}

func xAIErrorStatusCode(resp *http.Response) int {
	if resp == nil || resp.StatusCode < http.StatusBadRequest {
		return http.StatusBadGateway
	}
	return resp.StatusCode
}
