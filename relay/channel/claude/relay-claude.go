package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/reasonmap"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	WebSearchMaxUsesLow    = 1
	WebSearchMaxUsesMedium = 5
	WebSearchMaxUsesHigh   = 10
)

func stopReasonClaude2OpenAI(reason string) string {
	return reasonmap.ClaudeStopReasonToOpenAIFinishReason(reason)
}

func maybeMarkClaudeRefusal(c *gin.Context, stopReason string) {
	if c == nil {
		return
	}
	if strings.EqualFold(stopReason, "refusal") {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")
	}
}

func supportsClaudeAdaptiveReasoning(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4-6") || strings.HasPrefix(model, "claude-opus-4-7")
}

// claude-opus-4-8 及 Fable 世代起，Anthropic 已废弃 temperature 参数，
// 透传会被上游以 400 "`temperature` is deprecated for this model" 拒绝
func claudeTemperatureDeprecated(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4-8") || strings.HasPrefix(model, "claude-fable")
}

func mergeStringMessageContents(left, right string) string {
	return strings.Trim(fmt.Sprintf("%s %s", left, right), "\"")
}

func functionParametersToClaudeInputSchema(parameters any) any {
	if raw, ok := parameters.(json.RawMessage); ok && common.GetJsonType(raw) == "object" {
		var fields map[string]json.RawMessage
		if common.Unmarshal(raw, &fields) == nil {
			out := raw
			if len(fields["type"]) == 0 || common.GetJsonType(fields["type"]) == "null" {
				out, _ = sjson.SetRawBytes(out, "type", []byte(`"object"`))
			}
			if len(fields["properties"]) == 0 || common.GetJsonType(fields["properties"]) == "null" {
				out, _ = sjson.SetRawBytes(out, "properties", []byte(`{}`))
			}
			return json.RawMessage(out)
		}
	}
	params, _ := parameters.(map[string]any)
	schema := make(map[string]any, len(params)+2)
	for key, value := range params {
		schema[key] = value
	}
	if schema["type"] == nil {
		schema["type"] = "object"
	}
	if schema["properties"] == nil {
		schema["properties"] = map[string]any{}
	}
	return schema
}

func RequestOpenAI2ClaudeMessage(c *gin.Context, textRequest dto.GeneralOpenAIRequest) (*dto.ClaudeRequest, error) {
	for _, message := range textRequest.Messages {
		if len(message.Tools) > 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("message-level dynamic tools cannot be converted to Claude Messages; use Chat Completions for this request"),
				types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		for _, part := range message.ParseContent() {
			if part.Type == dto.ContentTypeVideoUrl {
				return nil, types.NewErrorWithStatusCode(
					fmt.Errorf("video_url cannot be converted to Claude Messages; use a video-capable Chat Completions channel"),
					types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		}
	}
	claudeTools := make([]any, 0, len(textRequest.Tools))

	for _, tool := range textRequest.Tools {
		if !dto.IsSchemaObject(tool.Function.Parameters) && tool.Type != "function" {
			continue
		}
		claudeTools = append(claudeTools, &dto.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: functionParametersToClaudeInputSchema(tool.Function.Parameters),
		})
	}

	// Web search tool
	// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool
	if textRequest.WebSearchOptions != nil {
		webSearchTool := dto.ClaudeWebSearchTool{
			Type: "web_search_20250305",
			Name: "web_search",
		}

		// 处理 user_location
		if textRequest.WebSearchOptions.UserLocation != nil {
			if anthropicUserLocation := dto.ParseApproximateWebSearchUserLocation(textRequest.WebSearchOptions.UserLocation); anthropicUserLocation != nil {
				webSearchTool.UserLocation = anthropicUserLocation
			}
		}

		// 处理 search_context_size 转换为 max_uses
		if textRequest.WebSearchOptions.SearchContextSize != "" {
			switch textRequest.WebSearchOptions.SearchContextSize {
			case "low":
				webSearchTool.MaxUses = WebSearchMaxUsesLow
			case "medium":
				webSearchTool.MaxUses = WebSearchMaxUsesMedium
			case "high":
				webSearchTool.MaxUses = WebSearchMaxUsesHigh
			}
		}

		claudeTools = append(claudeTools, &webSearchTool)
	}

	claudeRequest := dto.ClaudeRequest{
		Model:         textRequest.Model,
		MaxTokens:     textRequest.GetMaxTokens(),
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		TopK:          textRequest.TopK,
		Stream:        textRequest.Stream,
	}
	if len(claudeTools) > 0 {
		claudeRequest.Tools = claudeTools
	}
	if textRequest.TopP != 0 {
		claudeRequest.TopP = common.GetPointer(textRequest.TopP)
	}

	// 处理 tool_choice 和 parallel_tool_calls
	if textRequest.ToolChoice != nil || textRequest.ParallelTooCalls != nil {
		claudeToolChoice := mapToolChoice(textRequest.ToolChoice, textRequest.ParallelTooCalls)
		if claudeToolChoice != nil {
			claudeRequest.ToolChoice = claudeToolChoice
		}
	}

	if claudeRequest.MaxTokens == 0 {
		claudeRequest.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(textRequest.Model))
	}

	if baseModel, effortLevel, ok := reasoning.TrimEffortSuffix(textRequest.Model); ok && effortLevel != "" &&
		supportsClaudeAdaptiveReasoning(textRequest.Model) {
		claudeRequest.Model = baseModel
		claudeRequest.Thinking = &dto.Thinking{
			Type: "adaptive",
		}
		claudeRequest.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, effortLevel))
		if strings.HasPrefix(baseModel, "claude-opus-4-7") {
			claudeRequest.Thinking.Display = "summarized"
			claudeRequest.Temperature = nil
			claudeRequest.TopP = nil
			claudeRequest.TopK = 0
		} else {
			claudeRequest.TopP = nil
			claudeRequest.Temperature = common.GetPointer[float64](1.0)
		}
	} else if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(textRequest.Model, "-thinking") {
		trimmedModel := strings.TrimSuffix(textRequest.Model, "-thinking")
		if strings.HasPrefix(trimmedModel, "claude-opus-4-7") {
			claudeRequest.Thinking = &dto.Thinking{
				Type:    "adaptive",
				Display: "summarized",
			}
			claudeRequest.OutputConfig = json.RawMessage(`{"effort":"high"}`)
			claudeRequest.Temperature = nil
			claudeRequest.TopP = nil
			claudeRequest.TopK = 0
		} else {
			// 因为BudgetTokens 必须大于1024
			if claudeRequest.MaxTokens < 1280 {
				claudeRequest.MaxTokens = 1280
			}

			// BudgetTokens 为 max_tokens 的 80%
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](int(float64(claudeRequest.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)),
			}
			// TODO: 临时处理
			// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
			claudeRequest.TopP = nil
			claudeRequest.Temperature = common.GetPointer[float64](1.0)
		}
		if !model_setting.ShouldPreserveThinkingSuffix(textRequest.Model) {
			claudeRequest.Model = trimmedModel
		}
	}

	if textRequest.ReasoningEffort != "" {
		switch textRequest.ReasoningEffort {
		case "low":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](1280),
			}
		case "medium":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](2048),
			}
		case "high":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](4096),
			}
		}
	}

	// 指定了 reasoning 参数,覆盖 budgetTokens
	if textRequest.Reasoning != nil {
		var reasoning openrouter.RequestReasoning
		if err := common.Unmarshal(textRequest.Reasoning, &reasoning); err != nil {
			return nil, err
		}

		budgetTokens := reasoning.MaxTokens
		if budgetTokens > 0 {
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: &budgetTokens,
			}
		}
	}

	if textRequest.Stop != nil {
		// stop maybe string/array string, convert to array string
		switch textRequest.Stop.(type) {
		case string:
			claudeRequest.StopSequences = []string{textRequest.Stop.(string)}
		case []interface{}:
			stopSequences := make([]string, 0)
			for _, stop := range textRequest.Stop.([]interface{}) {
				stopSequences = append(stopSequences, stop.(string))
			}
			claudeRequest.StopSequences = stopSequences
		}
	}
	formatMessages := make([]dto.Message, 0)
	lastMessage := dto.Message{
		Role: "tool",
	}
	lastMessageHasPlaceholder := false
	for i, message := range textRequest.Messages {
		if message.Role == "" {
			textRequest.Messages[i].Role = "user"
		}
		fmtMessage := dto.Message{
			Role:    message.Role,
			Content: message.Content,
		}
		if !message.IsStringContent() {
			if parsedContent := message.ParseContent(); len(parsedContent) > 0 {
				fmtMessage.SetMediaContent(parsedContent)
			}
		}
		if message.Role == "tool" {
			fmtMessage.ToolCallId = message.ToolCallId
		}
		if message.Role == "assistant" && message.ToolCalls != nil {
			fmtMessage.ToolCalls = message.ToolCalls
		}
		if lastMessage.Role == message.Role && lastMessage.Role != "tool" {
			if lastMessage.IsStringContent() && (message.IsStringContent() || message.Role == "assistant" && message.Content == nil) {
				previousToolCalls := lastMessage.ParseToolCalls()
				currentToolCalls := fmtMessage.ParseToolCalls()
				if len(previousToolCalls) > 0 || len(currentToolCalls) > 0 {
					texts := make([]string, 0, 2)
					if previousText := lastMessage.StringContent(); previousText != "" && !(lastMessageHasPlaceholder && len(previousToolCalls) > 0) {
						texts = append(texts, previousText)
					}
					if currentText := message.StringContent(); currentText != "" {
						texts = append(texts, currentText)
					}
					fmtMessage.SetStringContent(strings.Join(texts, " "))
					fmtMessage.SetToolCalls(append(previousToolCalls, currentToolCalls...))
				} else {
					fmtMessage.SetStringContent(mergeStringMessageContents(lastMessage.StringContent(), message.StringContent()))
				}
				// delete last message
				formatMessages = formatMessages[:len(formatMessages)-1]
			}
		}
		hasPlaceholder := fmtMessage.Content == nil || (fmtMessage.IsStringContent() && fmtMessage.StringContent() == "")
		if hasPlaceholder {
			fmtMessage.SetStringContent("...")
		}
		formatMessages = append(formatMessages, fmtMessage)
		lastMessage = fmtMessage
		lastMessageHasPlaceholder = hasPlaceholder
	}

	claudeMessages := make([]dto.ClaudeMessage, 0)
	isFirstMessage := true
	// 初始化system消息数组，用于累积多个system消息
	var systemMessages []dto.ClaudeMediaMessage

	for _, message := range formatMessages {
		if message.Role == "system" {
			// 根据Claude API规范，system字段使用数组格式更有通用性
			if message.IsStringContent() {
				if text := message.StringContent(); text != "" {
					systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
						Type: "text",
						Text: common.GetPointer[string](text),
					})
				}
			} else {
				// 支持复合内容的system消息（虽然不常见，但需要考虑完整性）
				for _, ctx := range message.ParseContent() {
					if ctx.Type == "text" && ctx.Text != "" {
						systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
							Type:         "text",
							Text:         common.GetPointer[string](ctx.Text),
							CacheControl: ctx.CacheControl,
						})
					}
					// 未来可以在这里扩展对图片等其他类型的支持
				}
			}
		} else {
			if isFirstMessage {
				isFirstMessage = false
				if message.Role != "user" {
					// fix: first message is assistant, add user message
					claudeMessage := dto.ClaudeMessage{
						Role: "user",
						Content: []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string]("..."),
							},
						},
					}
					claudeMessages = append(claudeMessages, claudeMessage)
				}
			}
			claudeMessage := dto.ClaudeMessage{
				Role: message.Role,
			}
			if message.Role == "tool" {
				if len(claudeMessages) > 0 && claudeMessages[len(claudeMessages)-1].Role == "user" {
					lastMessage := claudeMessages[len(claudeMessages)-1]
					if content, ok := lastMessage.Content.(string); ok {
						lastMessage.Content = []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string](content),
							},
						}
					}
					lastMessage.Content = append(lastMessage.Content.([]dto.ClaudeMediaMessage), dto.ClaudeMediaMessage{
						Type:      "tool_result",
						ToolUseId: message.ToolCallId,
						Content:   message.Content,
					})
					claudeMessages[len(claudeMessages)-1] = lastMessage
					continue
				} else {
					claudeMessage.Role = "user"
					claudeMessage.Content = []dto.ClaudeMediaMessage{
						{
							Type:      "tool_result",
							ToolUseId: message.ToolCallId,
							Content:   message.Content,
						},
					}
				}
			} else if message.IsStringContent() && message.ToolCalls == nil {
				text := message.StringContent()
				if text == "" {
					text = "..."
				}
				claudeMessage.Content = text
			} else {
				claudeMediaMessages := make([]dto.ClaudeMediaMessage, 0)
				for _, mediaMessage := range message.ParseContent() {
					claudeMediaMessage := dto.ClaudeMediaMessage{
						Type:         mediaMessage.Type,
						CacheControl: mediaMessage.CacheControl,
					}
					if mediaMessage.Type == "text" {
						if mediaMessage.Text == "" {
							continue
						}
						claudeMediaMessage.Text = common.GetPointer[string](mediaMessage.Text)
					} else {
						imageUrl := mediaMessage.GetImageMedia()
						if imageUrl == nil {
							return nil, fmt.Errorf("unsupported media type for Claude Messages: %s", mediaMessage.Type)
						}
						claudeMediaMessage.Type = "image"
						claudeMediaMessage.Source = &dto.ClaudeMessageSource{
							Type: "base64",
						}
						// 使用统一的文件服务获取图片数据
						var source *types.FileSource
						if strings.HasPrefix(imageUrl.Url, "http") {
							source = types.NewURLFileSource(imageUrl.Url)
						} else {
							source = types.NewBase64FileSource(imageUrl.Url, "")
						}
						base64Data, mimeType, err := service.GetBase64Data(c, source, "formatting image for Claude")
						if err != nil {
							return nil, fmt.Errorf("get file data failed: %s", err.Error())
						}
						claudeMediaMessage.Source.MediaType = mimeType
						claudeMediaMessage.Source.Data = base64Data
					}
					claudeMediaMessages = append(claudeMediaMessages, claudeMediaMessage)
				}
				if message.ToolCalls != nil {
					for _, toolCall := range message.ParseToolCalls() {
						inputObj := make(map[string]any)
						if args := strings.TrimSpace(toolCall.Function.Arguments); args != "" {
							if err := common.DecodeJsonUseNumberStrict(strings.NewReader(args), &inputObj); err != nil {
								common.SysLog("tool call function arguments is not a map[string]any: " + fmt.Sprintf("%v", toolCall.Function.Arguments))
								inputObj = map[string]any{}
							}
						}
						claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
							Type:  "tool_use",
							Id:    toolCall.ID,
							Name:  toolCall.Function.Name,
							Input: inputObj,
						})
					}
				}
				claudeMessage.Content = claudeMediaMessages
			}
			claudeMessages = append(claudeMessages, claudeMessage)
		}
	}

	// 设置累积的system消息
	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}

	claudeRequest.Prompt = ""
	claudeRequest.Messages = claudeMessages

	// 放在所有分支之后统一剥离：thinking 分支可能把 temperature 重设为 1.0
	if claudeTemperatureDeprecated(claudeRequest.Model) {
		claudeRequest.Temperature = nil
	}

	return &claudeRequest, nil
}

func StreamResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *dto.ChatCompletionsStreamResponse {
	if claudeResponse == nil {
		return nil
	}
	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Model = claudeResponse.Model
	response.Choices = make([]dto.ChatCompletionsStreamResponseChoice, 0)
	tools := make([]dto.ToolCallResponse, 0)
	if claudeInfo != nil && claudeInfo.ToolCallStreamStates == nil {
		claudeInfo.ToolCallStreamStates = make(map[int]*ToolCallStreamState)
	}
	blockIndex := 0
	if claudeResponse.Index != nil {
		blockIndex = *claudeResponse.Index
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	if claudeResponse.Type == "message_start" {
		if claudeResponse.Message != nil {
			response.Id = claudeResponse.Message.Id
			response.Model = claudeResponse.Message.Model
		}
		//claudeUsage = &claudeResponse.Message.Usage
		choice.Delta.SetContentString("")
		choice.Delta.Role = "assistant"
	} else if claudeResponse.Type == "content_block_start" {
		if claudeResponse.ContentBlock != nil {
			// 如果是文本块，尽可能发送首段文本（若存在）
			if claudeResponse.ContentBlock.Type == "text" && claudeResponse.ContentBlock.Text != nil {
				choice.Delta.SetContentString(*claudeResponse.ContentBlock.Text)
			}
			if claudeResponse.ContentBlock.Type == "tool_use" {
				if claudeInfo != nil {
					// Claude indexes all content blocks, including text/thinking.
					// OpenAI indexes tool calls only; keep a request-local mapping.
					if _, exists := claudeInfo.ToolCallStreamStates[blockIndex]; exists {
						return nil
					}
					claudeInfo.ToolCallStreamStates[blockIndex] = &ToolCallStreamState{
						ID:    claudeResponse.ContentBlock.Id,
						Name:  claudeResponse.ContentBlock.Name,
						Index: claudeInfo.NextToolCallIndex,
					}
					claudeInfo.NextToolCallIndex++
					return nil
				}
				tools = append(tools, dto.ToolCallResponse{
					Index: common.GetPointer(blockIndex),
					ID:    claudeResponse.ContentBlock.Id,
					Type:  "function",
					Function: dto.FunctionResponse{
						Name:      claudeResponse.ContentBlock.Name,
						Arguments: "",
					},
				})
			}
		} else {
			return nil
		}
	} else if claudeResponse.Type == "content_block_delta" {
		if claudeResponse.Delta != nil {
			choice.Delta.Content = claudeResponse.Delta.Text
			switch claudeResponse.Delta.Type {
			case "input_json_delta":
				if claudeResponse.Delta.PartialJson == nil {
					return nil
				}
				arguments := *claudeResponse.Delta.PartialJson
				// A whitespace-only delta can be part of a JSON string. Only
				// the truly empty fragment is safe to omit.
				if arguments == "" {
					return nil
				}
				toolCall := dto.ToolCallResponse{
					Type:  "function",
					Index: common.GetPointer(blockIndex),
					Function: dto.FunctionResponse{
						Arguments: arguments,
					},
				}
				if claudeInfo != nil {
					state, ok := claudeInfo.ToolCallStreamStates[blockIndex]
					if !ok {
						// Do not attach orphan/late argument deltas to another tool.
						return nil
					}
					toolCall.Index = common.GetPointer(state.Index)
					if !state.Emitted {
						toolCall.ID = state.ID
						toolCall.Function.Name = state.Name
						state.Emitted = true
					}
				}
				tools = append(tools, toolCall)
			case "signature_delta":
				// 加密的不处理
				signatureContent := "\n"
				choice.Delta.ReasoningContent = &signatureContent
			case "thinking_delta":
				choice.Delta.ReasoningContent = claudeResponse.Delta.Thinking
			}
		}
	} else if claudeResponse.Type == "content_block_stop" {
		if claudeInfo == nil {
			return nil
		}
		state, ok := claudeInfo.ToolCallStreamStates[blockIndex]
		if !ok {
			return nil
		}
		delete(claudeInfo.ToolCallStreamStates, blockIndex)
		if state.Emitted {
			return nil
		}
		tools = append(tools, dto.ToolCallResponse{
			Index: common.GetPointer(state.Index),
			ID:    state.ID,
			Type:  "function",
			Function: dto.FunctionResponse{
				Name:      state.Name,
				Arguments: "{}",
			},
		})
	} else if claudeResponse.Type == "message_delta" {
		if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
			finishReason := stopReasonClaude2OpenAI(*claudeResponse.Delta.StopReason)
			if finishReason != "null" {
				choice.FinishReason = &finishReason
			}
		}
		//claudeUsage = &claudeResponse.Usage
	} else if claudeResponse.Type == "message_stop" {
		return nil
	} else {
		return nil
	}
	if len(tools) > 0 {
		choice.Delta.Content = nil // compatible with other OpenAI derivative applications, like LobeOpenAICompatibleFactory ...
		choice.Delta.ToolCalls = tools
	}
	response.Choices = append(response.Choices, choice)

	return &response
}

func ResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse) *dto.OpenAITextResponse {
	choices := make([]dto.OpenAITextResponseChoice, 0)
	fullTextResponse := dto.OpenAITextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", common.GetUUID()),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
	}
	var responseText string
	var responseThinking string
	if len(claudeResponse.Content) > 0 {
		responseText = claudeResponse.Content[0].GetText()
		if claudeResponse.Content[0].Thinking != nil {
			responseThinking = *claudeResponse.Content[0].Thinking
		}
	}
	tools := make([]dto.ToolCallResponse, 0)
	thinkingContent := ""

	fullTextResponse.Id = claudeResponse.Id
	for _, message := range claudeResponse.Content {
		switch message.Type {
		case "tool_use":
			args, _ := json.Marshal(message.Input)
			tools = append(tools, dto.ToolCallResponse{
				ID:   message.Id,
				Type: "function", // compatible with other OpenAI derivative applications
				Function: dto.FunctionResponse{
					Name:      message.Name,
					Arguments: string(args),
				},
			})
		case "thinking":
			// 加密的不管， 只输出明文的推理过程
			if message.Thinking != nil {
				thinkingContent = *message.Thinking
			}
		case "text":
			responseText = message.GetText()
		}
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role: "assistant",
		},
		FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
	}
	choice.SetStringContent(responseText)
	if len(responseThinking) > 0 {
		choice.ReasoningContent = responseThinking
	}
	if len(tools) > 0 {
		choice.Message.SetToolCalls(tools)
	}
	choice.Message.ReasoningContent = thinkingContent
	fullTextResponse.Model = claudeResponse.Model
	choices = append(choices, choice)
	fullTextResponse.Choices = choices
	return &fullTextResponse
}

type ClaudeResponseInfo struct {
	UsageError           error
	ResponseId           string
	Created              int64
	Model                string
	ResponseText         strings.Builder
	Usage                *dto.Usage
	HasInputUsage        bool
	HasFinalOutputUsage  bool
	Done                 bool
	MessageStarted       bool
	MessageStopped       bool
	RequireMessageStop   bool
	PendingFinish        *dto.ChatCompletionsStreamResponse
	ToolCallStreamStates map[int]*ToolCallStreamState
	NextToolCallIndex    int
}

type ToolCallStreamState struct {
	ID      string
	Name    string
	Index   int
	Emitted bool
}

func buildMessageDeltaPatchUsage(claudeResponse *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *dto.ClaudeUsage {
	usage := &dto.ClaudeUsage{}
	if claudeResponse != nil && claudeResponse.Usage != nil {
		*usage = *claudeResponse.Usage
	}

	if claudeInfo == nil || claudeInfo.Usage == nil {
		return usage
	}

	if usage.InputTokens == 0 && claudeInfo.Usage.PromptTokens > 0 {
		usage.InputTokens = claudeInfo.Usage.PromptTokens
	}
	if usage.CacheReadInputTokens == 0 && claudeInfo.Usage.PromptTokensDetails.CachedTokens > 0 {
		usage.CacheReadInputTokens = claudeInfo.Usage.PromptTokensDetails.CachedTokens
	}
	if usage.CacheCreationInputTokens == 0 && claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens > 0 {
		usage.CacheCreationInputTokens = claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens
	}
	if usage.CacheCreation == nil && (claudeInfo.Usage.ClaudeCacheCreation5mTokens > 0 || claudeInfo.Usage.ClaudeCacheCreation1hTokens > 0) {
		usage.CacheCreation = &dto.ClaudeCacheCreationUsage{
			Ephemeral5mInputTokens: claudeInfo.Usage.ClaudeCacheCreation5mTokens,
			Ephemeral1hInputTokens: claudeInfo.Usage.ClaudeCacheCreation1hTokens,
		}
	}
	return usage
}

func shouldSkipClaudeMessageDeltaUsagePatch(info *relaycommon.RelayInfo) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	if info == nil {
		return false
	}
	return info.ChannelSetting.PassThroughBodyEnabled
}

func patchClaudeMessageDeltaUsageData(data string, usage *dto.ClaudeUsage) string {
	if data == "" || usage == nil {
		return data
	}

	data = setMessageDeltaUsageInt(data, "usage.input_tokens", usage.InputTokens)
	data = setMessageDeltaUsageInt(data, "usage.cache_read_input_tokens", usage.CacheReadInputTokens)
	data = setMessageDeltaUsageInt(data, "usage.cache_creation_input_tokens", usage.CacheCreationInputTokens)

	if usage.CacheCreation != nil {
		data = setMessageDeltaUsageInt(data, "usage.cache_creation.ephemeral_5m_input_tokens", usage.CacheCreation.Ephemeral5mInputTokens)
		data = setMessageDeltaUsageInt(data, "usage.cache_creation.ephemeral_1h_input_tokens", usage.CacheCreation.Ephemeral1hInputTokens)
	}

	return data
}

func setMessageDeltaUsageInt(data string, path string, localValue int) string {
	if localValue <= 0 {
		return data
	}

	upstreamValue := gjson.Get(data, path)
	if upstreamValue.Exists() && upstreamValue.Int() > 0 {
		return data
	}

	patchedData, err := sjson.Set(data, path, localValue)
	if err != nil {
		return data
	}
	return patchedData
}

func FormatClaudeResponseInfo(claudeResponse *dto.ClaudeResponse, oaiResponse *dto.ChatCompletionsStreamResponse, claudeInfo *ClaudeResponseInfo) bool {
	if claudeInfo == nil {
		return false
	}
	if claudeInfo.Usage == nil {
		claudeInfo.Usage = &dto.Usage{}
	}
	if claudeResponse.Type == "message_start" {
		if claudeResponse.Message != nil {
			claudeInfo.MessageStarted = true
			claudeInfo.ResponseId = claudeResponse.Message.Id
			claudeInfo.Model = claudeResponse.Message.Model
		}

		if claudeResponse.Message != nil {
			mergeClaudeUsage(claudeInfo, claudeResponse.Message.Usage, false)
		}
	} else if claudeResponse.Type == "content_block_delta" {
		appendClaudeUsageText(claudeInfo, claudeResponse.Delta)
	} else if claudeResponse.Type == "message_delta" || claudeResponse.Type == "message_stop" {
		if claudeResponse.Type == "message_stop" {
			claudeInfo.MessageStopped = true
		}
		// Some compatible providers attach their final usage to message_stop.
		mergeClaudeUsage(claudeInfo, claudeResponse.Usage, true)
		if claudeResponse.Type == "message_stop" ||
			(claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil && *claudeResponse.Delta.StopReason != "") {
			claudeInfo.Done = true
		}
	} else if claudeResponse.Type == "content_block_start" {
		appendClaudeUsageText(claudeInfo, claudeResponse.ContentBlock)
	} else if claudeResponse.Type != "content_block_stop" {
		return false
	}
	if oaiResponse != nil {
		oaiResponse.Id = claudeInfo.ResponseId
		oaiResponse.Created = claudeInfo.Created
		oaiResponse.Model = claudeInfo.Model
	}
	return true
}

func HandleStreamResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, data string, fallbackStatus ...int) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.UnmarshalJsonStr(data, &claudeResponse)
	if err != nil {
		common.SysLog("error unmarshalling stream response: " + err.Error())
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		statusCode := 0
		if len(fallbackStatus) > 0 {
			statusCode = fallbackStatus[0]
		}
		statusCode = resolveClaudeErrorStatusCode(claudeError, statusCode)
		logClaudeUpstreamError("stream", claudeError, statusCode)
		return types.WithClaudeError(*claudeError, statusCode)
	}
	if claudeInfo.RequireMessageStop && claudeResponse.Type == "message_stop" && !claudeInfo.MessageStarted {
		return types.NewErrorWithStatusCode(errors.New("Claude stream message_stop preceded message_start"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	if claudeResponse.StopReason != "" {
		maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	}
	if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
		maybeMarkClaudeRefusal(c, *claudeResponse.Delta.StopReason)
	}
	if isEffectiveClaudeStreamOutput(&claudeResponse) {
		info.SetFirstEffectiveOutputTime()
	}
	info.ObserveResponseModel(claudeResponse.Model)
	if claudeResponse.Message != nil {
		info.ObserveResponseModel(claudeResponse.Message.Model)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		FormatClaudeResponseInfo(&claudeResponse, nil, claudeInfo)
		if claudeInfo.UsageError != nil {
			return types.NewError(claudeInfo.UsageError, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
		}

		if claudeResponse.Type == "message_start" {
			// message_start, 获取usage
			if claudeResponse.Message != nil {
				info.UpstreamModelName = claudeResponse.Message.Model
			}
		} else if claudeResponse.Type == "message_delta" {
			// 确保 message_delta 的 usage 包含完整的 input_tokens 和 cache 相关字段
			// 解决 AWS Bedrock 等上游返回的 message_delta 缺少这些字段的问题
			if !shouldSkipClaudeMessageDeltaUsagePatch(info) {
				data = patchClaudeMessageDeltaUsageData(data, buildMessageDeltaPatchUsage(&claudeResponse, claudeInfo))
			}
		}
		if err := helper.ClaudeChunkData(c, claudeResponse, data); err != nil {
			return newClaudeDeliveryError(c, err)
		}
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		// Account for terminal frames even when they produce no OpenAI chunk.
		if !FormatClaudeResponseInfo(&claudeResponse, nil, claudeInfo) {
			return nil
		}
		if claudeInfo.UsageError != nil {
			return types.NewError(claudeInfo.UsageError, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
		}
		response := StreamResponseClaude2OpenAI(&claudeResponse, claudeInfo)
		if response == nil {
			return nil
		}
		response.Id = claudeInfo.ResponseId
		response.Created = claudeInfo.Created
		response.Model = claudeInfo.Model
		if claudeInfo.RequireMessageStop && claudeResponse.Type == "message_delta" && claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil && *claudeResponse.Delta.StopReason != "" {
			// A Chat finish_reason is a success signal too. Hold it until the
			// provider sends the actual Messages terminal (and any final usage).
			claudeInfo.PendingFinish = response
			return nil
		}

		err = helper.ObjectData(c, response)
		if err != nil {
			return newClaudeDeliveryError(c, err)
		}
	}
	return nil
}

func isEffectiveClaudeStreamOutput(response *dto.ClaudeResponse) bool {
	if response == nil || response.Delta == nil {
		return false
	}
	delta := response.Delta
	return (delta.Text != nil && strings.TrimSpace(*delta.Text) != "") ||
		(delta.Thinking != nil && strings.TrimSpace(*delta.Thinking) != "") ||
		(delta.PartialJson != nil && strings.TrimSpace(*delta.PartialJson) != "")
}

func HandleStreamFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo) error {
	completeClaudeUsage(c, info, claudeInfo)

	if info.RelayFormat == types.RelayFormatClaude {
		//
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		if claudeInfo.PendingFinish != nil {
			if err := helper.ObjectData(c, claudeInfo.PendingFinish); err != nil {
				return err
			}
			claudeInfo.PendingFinish = nil
		}
		if info.ShouldIncludeUsage {
			response := helper.GenerateFinalUsageResponse(claudeInfo.ResponseId, claudeInfo.Created, info.UpstreamModelName, *claudeInfo.Usage)
			err := helper.ObjectData(c, response)
			if err != nil {
				return err
			}
		}
		return helper.StringData(c, "[DONE]")
	}
	return nil
}

func ClaudeStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	claudeInfo := &ClaudeResponseInfo{
		ResponseId:         helper.GetResponseID(c),
		Created:            common.GetTimestamp(),
		Model:              info.UpstreamModelName,
		ResponseText:       strings.Builder{},
		Usage:              &dto.Usage{},
		RequireMessageStop: true,
	}
	var err *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		info.StreamStatus.RequireTerminal()
		err = HandleStreamResponseData(c, info, claudeInfo, data, resp.StatusCode)
		if err != nil {
			return false
		}
		if claudeInfo.MessageStopped {
			if !claudeInfo.MessageStarted {
				return false
			}
			if writeErr := HandleStreamFinalResponse(c, info, claudeInfo); writeErr != nil {
				err = newClaudeDeliveryError(c, writeErr)
				return false
			}
			// The scanner stops and joins its reader before returning. Do not wait
			// for EOF after message_stop or let cleanup cancellation undo delivery.
			info.StreamStatus.MarkDeliveredCompletion()
			return false
		}
		return true
	})
	if info.StreamStatus.HasDeliveredCompletion() {
		return claudeInfo.Usage, nil
	}
	if (err == nil && (c.Request.Context().Err() != nil || info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone)) ||
		(err != nil && types.ClassifyFailure(err) == types.FailureClientCanceled) {
		// Keep the existing cancellation settlement policy. A client must not
		// obtain a free partial answer by disconnecting; no completion is sent.
		info.StreamStatus.RequireTerminal()
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeCancelled)
		info.StreamStatus.RecordError("downstream canceled before complete Claude delivery")
		completeClaudeUsage(c, info, claudeInfo)
		service.CaptureClaudeStreamFailure(c, info.StreamStatus, claudeInfo.MessageStopped)
		return claudeInfo.Usage, nil
	}
	if err != nil {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeFailed)
		service.CaptureClaudeStreamFailure(c, info.StreamStatus, claudeInfo.MessageStopped)
		return nil, err
	}

	info.StreamStatus.RequireTerminal()
	info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeIncomplete)
	info.StreamStatus.RecordError("Claude stream ended without a delivered message_stop")
	// Retain evidence for support, never use an interrupted snapshot for a
	// successful charge or send synthetic successful usage/terminal frames.
	completeClaudeUsage(c, info, claudeInfo)
	claudeInfo.Usage.InterruptedOutput = true
	service.CaptureUnbilledRequestUsage(c, claudeInfo.Usage)
	service.CaptureClaudeStreamFailure(c, info.StreamStatus, claudeInfo.MessageStopped)
	status := http.StatusBadGateway
	message := "Claude upstream stream interrupted before message_stop; the response is incomplete"
	if info.StreamStatus.EndReason == relaycommon.StreamEndReasonTimeout {
		status = http.StatusGatewayTimeout
		message = "Claude upstream stream timed out before message_stop; the response is incomplete"
	}
	return nil, types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponseBody, status, types.ErrOptionWithSkipRetry())
}

func newClaudeDeliveryError(c *gin.Context, err error) *types.NewAPIError {
	if errors.Is(err, context.Canceled) || (c.Request != nil && errors.Is(c.Request.Context().Err(), context.Canceled)) {
		return types.NewClientCanceledError()
	}
	return types.NewErrorWithStatusCode(errors.New("failed to deliver Claude stream to client"), helper.DownstreamWriteErrorCode, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}

func HandleClaudeResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, httpResp *http.Response, data []byte) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.Unmarshal(data, &claudeResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		statusCode := http.StatusInternalServerError
		if httpResp != nil {
			statusCode = httpResp.StatusCode
		}
		statusCode = resolveClaudeErrorStatusCode(claudeError, statusCode)
		logClaudeUpstreamError("non_stream", claudeError, statusCode)
		return types.WithClaudeError(*claudeError, statusCode)
	}
	maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	info.ObserveResponseModel(claudeResponse.Model)
	if claudeInfo.Usage == nil {
		claudeInfo.Usage = &dto.Usage{}
	}
	mergeClaudeUsage(claudeInfo, claudeResponse.Usage, true)
	if claudeInfo.UsageError != nil {
		return types.NewError(claudeInfo.UsageError, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	for i := range claudeResponse.Content {
		appendClaudeUsageText(claudeInfo, &claudeResponse.Content[i])
	}
	if claudeResponse.Type == "message" || len(claudeResponse.Content) > 0 {
		claudeInfo.Done = claudeResponse.StopReason != ""
		completeClaudeUsage(c, info, claudeInfo)
	}
	var responseData []byte
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		openaiResponse := ResponseClaude2OpenAI(&claudeResponse)
		openaiResponse.Usage = *claudeInfo.Usage
		responseData, err = common.Marshal(openaiResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		responseData = data
	}

	if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}

	service.IOCopyBytesGracefully(c, httpResp, responseData)
	return nil
}

func resolveClaudeErrorStatusCode(claudeError *types.ClaudeError, fallbackStatus int) int {
	if fallbackStatus >= 400 && fallbackStatus <= 599 {
		return fallbackStatus
	}
	if claudeError == nil {
		return http.StatusInternalServerError
	}

	errorType := strings.ToLower(strings.TrimSpace(claudeError.Type))
	message := strings.ToLower(strings.TrimSpace(claudeError.Message))

	switch errorType {
	case "authentication_error":
		return http.StatusUnauthorized
	case "permission_error":
		return http.StatusForbidden
	case "invalid_request_error":
		return http.StatusBadRequest
	case "rate_limit_error":
		return http.StatusTooManyRequests
	}

	if containsClaudeErrorHint(message,
		"overloaded",
		"overload",
		"capacity",
		"too many concurrent",
		"temporarily unavailable",
		"service unavailable",
	) {
		return 529
	}

	if errorType == "overloaded_error" {
		return http.StatusTooManyRequests
	}

	if containsClaudeErrorHint(message,
		"rate limit",
		"rate-limited",
		"ratelimit",
		"too many requests",
	) {
		return http.StatusTooManyRequests
	}

	return http.StatusInternalServerError
}

func containsClaudeErrorHint(message string, hints ...string) bool {
	for _, hint := range hints {
		if hint != "" && strings.Contains(message, hint) {
			return true
		}
	}
	return false
}

func logClaudeUpstreamError(source string, claudeError *types.ClaudeError, statusCode int) {
	if !common.DebugEnabled || claudeError == nil {
		return
	}
	common.SysLog(fmt.Sprintf(
		"claude upstream error detected: source=%s status=%d type=%s message=%s",
		source,
		statusCode,
		claudeError.Type,
		common.MaskSensitiveInfo(claudeError.Message),
	))
}

func ClaudeHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if common.DebugEnabled {
		println("responseBody: ", string(responseBody))
	}
	handleErr := HandleClaudeResponseData(c, info, claudeInfo, resp, responseBody)
	if handleErr != nil {
		return nil, handleErr
	}
	return claudeInfo.Usage, nil
}

func mapToolChoice(toolChoice any, parallelToolCalls *bool) *dto.ClaudeToolChoice {
	var claudeToolChoice *dto.ClaudeToolChoice

	// 处理 tool_choice 字符串值
	if toolChoiceStr, ok := toolChoice.(string); ok {
		switch toolChoiceStr {
		case "auto":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		case "required":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "any",
			}
		case "none":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "none",
			}
		}
	} else if toolChoiceMap, ok := toolChoice.(map[string]interface{}); ok {
		// 处理 tool_choice 对象值
		if function, ok := toolChoiceMap["function"].(map[string]interface{}); ok {
			if toolName, ok := function["name"].(string); ok {
				claudeToolChoice = &dto.ClaudeToolChoice{
					Type: "tool",
					Name: toolName,
				}
			}
		}
	}

	// 处理 parallel_tool_calls
	if parallelToolCalls != nil {
		if claudeToolChoice == nil {
			// 如果没有 tool_choice，但有 parallel_tool_calls，创建默认的 auto 类型
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		}

		// Anthropic schema: tool_choice.type=none does not accept extra fields.
		// When tools are disabled, parallel_tool_calls is irrelevant, so we drop it.
		if claudeToolChoice.Type != "none" {
			// 如果 parallel_tool_calls 为 true，则 disable_parallel_tool_use 为 false
			claudeToolChoice.DisableParallelToolUse = !*parallelToolCalls
		}
	}

	return claudeToolChoice
}
