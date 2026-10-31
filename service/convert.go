package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/reasonmap"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const claudeToOpenAIReasoningMapOption = "ClaudeToOpenAIReasoningMap"

var defaultClaudeToOpenAIReasoningMap = map[string]string{
	"low":    "low",
	"medium": "medium",
	"high":   "high",
	"max":    "xhigh",
}

var allowedOpenAIReasoningEfforts = map[string]struct{}{
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
}

func ClaudeToOpenAIRequest(c *gin.Context, claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	return ClaudeToOpenAIRequestForProtocol(c, claudeRequest, info, "chat")
}

func ClaudeToOpenAIRequestForProtocol(c *gin.Context, claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo, protocol string) (*dto.GeneralOpenAIRequest, error) {
	openAIRequest := dto.GeneralOpenAIRequest{
		Model:       claudeRequest.Model,
		MaxTokens:   claudeRequest.MaxTokens,
		Temperature: claudeRequest.Temperature,
		Stream:      claudeRequest.Stream,
	}
	if claudeRequest.TopP != nil {
		openAIRequest.TopP = *claudeRequest.TopP
	}

	isOpenRouter := info.ChannelType == constant.ChannelTypeOpenRouter

	reasoningEffort := claudeReasoningForProtocol(claudeRequest, info, protocol)
	if info != nil && reasoningEffort != "" {
		info.ReasoningEffort = reasoningEffort
	}

	if claudeRequest.Thinking != nil && claudeRequest.Thinking.Type == "enabled" {
		if isOpenRouter {
			reasoning := openrouter.RequestReasoning{
				MaxTokens: claudeRequest.Thinking.GetBudgetTokens(),
			}
			reasoningJSON, err := common.Marshal(reasoning)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal reasoning: %w", err)
			}
			openAIRequest.Reasoning = reasoningJSON
		} else {
			if reasoningEffort != "" {
				openAIRequest.ReasoningEffort = reasoningEffort
			}
			thinkingSuffix := "-thinking"
			if strings.HasSuffix(info.OriginModelName, thinkingSuffix) &&
				!strings.HasSuffix(openAIRequest.Model, thinkingSuffix) {
				openAIRequest.Model = openAIRequest.Model + thinkingSuffix
			}
		}
	} else if !isOpenRouter && reasoningEffort != "" {
		openAIRequest.ReasoningEffort = reasoningEffort
	}

	// Convert stop sequences
	if len(claudeRequest.StopSequences) == 1 {
		openAIRequest.Stop = claudeRequest.StopSequences[0]
	} else if len(claudeRequest.StopSequences) > 1 {
		openAIRequest.Stop = claudeRequest.StopSequences
	}

	// Convert tools
	openAITools := make([]dto.ToolCallRequest, 0)
	normalTools, webSearchTools := dto.ProcessTools(claudeRequest.GetTools())
	for _, claudeTool := range normalTools {
		openAITool := dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  claudeTool.InputSchema,
			},
		}
		openAITools = append(openAITools, openAITool)
	}
	openAIRequest.Tools = openAITools
	toolChoice, parallelCalls, err := claudeToolChoiceToChat(claudeRequest.ToolChoice)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	openAIRequest.ToolChoice = toolChoice
	openAIRequest.ParallelTooCalls = parallelCalls
	if len(webSearchTools) > 0 {
		openAIRequest.WebSearchOptions = convertClaudeWebSearchToolToOpenAIOptions(webSearchTools[0])
	}

	// Convert messages
	openAIMessages := make([]dto.Message, 0)

	// Add system message if present
	if claudeRequest.System != nil {
		if claudeRequest.IsStringSystem() && claudeRequest.GetStringSystem() != "" {
			openAIMessage := dto.Message{
				Role: "system",
			}
			openAIMessage.SetStringContent(claudeRequest.GetStringSystem())
			openAIMessages = append(openAIMessages, openAIMessage)
		} else {
			systems := claudeRequest.ParseSystem()
			if len(systems) > 0 {
				openAIMessage := dto.Message{
					Role: "system",
				}
				isOpenRouterClaude := isOpenRouter && strings.HasPrefix(info.UpstreamModelName, "anthropic/claude")
				if isOpenRouterClaude {
					systemMediaMessages := make([]dto.MediaContent, 0, len(systems))
					for _, system := range systems {
						message := dto.MediaContent{
							Type:         "text",
							Text:         system.GetText(),
							CacheControl: system.CacheControl,
						}
						systemMediaMessages = append(systemMediaMessages, message)
					}
					openAIMessage.SetMediaContent(systemMediaMessages)
				} else {
					systemStr := ""
					for _, system := range systems {
						if system.Text != nil {
							systemStr += *system.Text
						}
					}
					openAIMessage.SetStringContent(systemStr)
				}
				openAIMessages = append(openAIMessages, openAIMessage)
			}
		}
	}
	type unnamedToolResult struct {
		index int
		id    string
	}
	toolNames := make(map[string]string)
	var unnamedToolResults []unnamedToolResult
	var pendingToolMedia []dto.MediaContent
	flushToolMedia := func() {
		if len(pendingToolMedia) > 0 {
			message := dto.Message{Role: "user"}
			message.SetMediaContent(pendingToolMedia)
			openAIMessages = append(openAIMessages, message)
			pendingToolMedia = nil
		}
	}
	for _, claudeMessage := range claudeRequest.Messages {
		openAIMessage := dto.Message{
			Role: claudeMessage.Role,
		}

		//log.Printf("claudeMessage.Content: %v", claudeMessage.Content)
		if claudeMessage.IsStringContent() {
			flushToolMedia()
			openAIMessage.SetStringContent(claudeMessage.GetStringContent())
		} else {
			content, err := claudeMessage.ParseContent()
			if err != nil {
				return nil, err
			}
			contents := content
			hasToolResults := false
			for _, block := range contents {
				if block.Type == "tool_result" {
					hasToolResults = true
					break
				}
			}
			if claudeMessage.Role != "user" || !hasToolResults {
				flushToolMedia()
			}
			var toolCalls []dto.ToolCallRequest
			mediaMessages := make([]dto.MediaContent, 0, len(contents))

			for _, mediaMsg := range contents {
				if _, exists := toolNames[mediaMsg.Id]; !exists {
					toolNames[mediaMsg.Id] = mediaMsg.Name
				}
				switch mediaMsg.Type {
				case "thinking":
					if claudeMessage.Role == "assistant" && mediaMsg.Thinking != nil {
						openAIMessage.ReasoningContent += *mediaMsg.Thinking
					}
				case "text", "input_text":
					message := dto.MediaContent{
						Type:         "text",
						Text:         mediaMsg.GetText(),
						CacheControl: mediaMsg.CacheControl,
					}
					mediaMessages = append(mediaMessages, message)
				case "image":
					imageURL, err := ClaudeImageSourceToMessageImageURL(c, mediaMsg.Source)
					if err != nil {
						return nil, fmt.Errorf("convert claude image: %w", err)
					}
					if imageURL == nil {
						continue
					}
					mediaMessage := dto.MediaContent{
						Type:     "image_url",
						ImageUrl: imageURL,
					}
					mediaMessages = append(mediaMessages, mediaMessage)
				case "tool_use":
					toolCall := dto.ToolCallRequest{
						ID:   mediaMsg.Id,
						Type: "function",
						Function: dto.FunctionRequest{
							Name:      mediaMsg.Name,
							Arguments: toJSONString(mediaMsg.Input),
						},
					}
					toolCalls = append(toolCalls, toolCall)
				case "tool_result":
					// Add tool result as a separate message
					toolName := mediaMsg.Name
					if toolName == "" {
						unnamedToolResults = append(unnamedToolResults, unnamedToolResult{index: len(openAIMessages), id: mediaMsg.ToolUseId})
					}
					oaiToolMessage := dto.Message{
						Role:       "tool",
						Name:       &toolName,
						ToolCallId: mediaMsg.ToolUseId,
					}
					//oaiToolMessage.SetStringContent(*mediaMsg.GetMediaContent().Text)
					if mediaMsg.IsStringContent() {
						oaiToolMessage.SetStringContent(mediaMsg.GetStringContent())
					} else {
						text, media, err := claudeToolResultToChat(c, mediaMsg.Content)
						if err != nil {
							return nil, fmt.Errorf("convert claude tool result: %w", err)
						}
						oaiToolMessage.SetStringContent(text)
						mediaMessages = append(mediaMessages, media...)
					}
					openAIMessages = append(openAIMessages, oaiToolMessage)
				}
			}

			if len(toolCalls) > 0 {
				openAIMessage.SetToolCalls(toolCalls)
			}

			if len(mediaMessages) > 0 {
				if claudeMessage.Role == "user" && hasToolResults {
					// Keep all tool replies contiguous, even when Claude sends
					// their results in several adjacent user messages.
					pendingToolMedia = append(pendingToolMedia, mediaMessages...)
				} else {
					openAIMessage.SetMediaContent(mediaMessages)
				}
			}
		}
		if len(openAIMessage.ParseContent()) > 0 || len(openAIMessage.ToolCalls) > 0 || openAIMessage.ReasoningContent != "" {
			openAIMessages = append(openAIMessages, openAIMessage)
		}
	}
	flushToolMedia()

	for _, result := range unnamedToolResults {
		*openAIMessages[result.index].Name = toolNames[result.id]
	}
	openAIRequest.Messages = openAIMessages

	return &openAIRequest, nil
}

// Chat tool messages carry text. Move recognized images to a following user
// message, using the same data/bridge transport as ordinary Claude images.
// Keep unrecognized payloads intact instead of dropping unknown block fields.
func claudeToolResultToChat(c *gin.Context, content any) (string, []dto.MediaContent, error) {
	blocks, err := common.Any2Type[[]dto.ClaudeMediaMessage](content)
	fallback := func() (string, []dto.MediaContent, error) {
		encoded, err := common.Marshal(content)
		return string(encoded), nil, err
	}
	if err != nil || len(blocks) == 0 {
		return fallback()
	}
	for _, block := range blocks {
		if block.Type != "text" && block.Type != "input_text" &&
			(block.Type != "image" || block.Source == nil) {
			return fallback()
		}
	}
	var texts []string
	var media []dto.MediaContent
	for _, block := range blocks {
		if block.Type != "image" {
			if text := block.GetText(); text != "" {
				texts = append(texts, text)
			}
			continue
		}
		imageURL, err := ClaudeImageSourceToMessageImageURL(c, block.Source)
		if err != nil {
			return "", nil, err
		}
		if imageURL == nil {
			return fallback()
		}
		media = append(media, dto.MediaContent{Type: "image_url", ImageUrl: imageURL})
	}
	if len(texts) == 0 && len(media) > 0 {
		return "[image]", media, nil
	}
	return strings.Join(texts, "\n"), media, nil
}

func mapClaudeThinkingToOpenAIReasoningEffort(thinking *dto.Thinking) string {
	level := inferClaudeThinkingLevel(thinking)
	if level == "" {
		return ""
	}
	return getClaudeToOpenAIReasoningMap()[level]
}

func inferClaudeReasoningEffort(request dto.ClaudeRequest) string {
	if effort := mapClaudeOutputConfigToOpenAIReasoningEffort(request.OutputConfig); effort != "" {
		return effort
	}
	return mapClaudeThinkingToOpenAIReasoningEffort(request.Thinking)
}

func inferClaudeThinkingLevel(thinking *dto.Thinking) string {
	if thinking == nil || thinking.Type != "enabled" {
		return ""
	}

	budgetTokens := thinking.GetBudgetTokens()
	if budgetTokens <= 0 {
		return "medium"
	}
	if budgetTokens <= 1280 {
		return "low"
	}
	if budgetTokens <= 2048 {
		return "medium"
	}
	if budgetTokens <= 4096 {
		return "high"
	}
	return "max"
}

func getClaudeToOpenAIReasoningMap() map[string]string {
	mapping := make(map[string]string, len(defaultClaudeToOpenAIReasoningMap))
	for key, value := range defaultClaudeToOpenAIReasoningMap {
		mapping[key] = value
	}

	common.OptionMapRWMutex.RLock()
	raw := strings.TrimSpace(common.OptionMap[claudeToOpenAIReasoningMapOption])
	common.OptionMapRWMutex.RUnlock()
	if raw == "" {
		return mapping
	}

	var stored map[string]string
	if err := common.UnmarshalJsonStr(raw, &stored); err != nil {
		common.SysError(fmt.Sprintf("invalid %s option: %v", claudeToOpenAIReasoningMapOption, err))
		return mapping
	}
	for key, value := range stored {
		if _, ok := mapping[key]; !ok {
			continue
		}
		if _, ok := allowedOpenAIReasoningEfforts[value]; !ok {
			continue
		}
		mapping[key] = value
	}
	return mapping
}

func claudeOutputConfigEffort(outputConfig any) string {
	if outputConfig == nil {
		return ""
	}

	var payload struct {
		Effort string `json:"effort"`
	}
	switch value := outputConfig.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return ""
		}
		if err := common.UnmarshalJsonStr(value, &payload); err != nil {
			return ""
		}
	case []byte:
		if len(value) == 0 {
			return ""
		}
		if err := common.Unmarshal(value, &payload); err != nil {
			return ""
		}
	default:
		marshal, err := common.Marshal(value)
		if err != nil {
			return ""
		}
		if err := common.Unmarshal(marshal, &payload); err != nil {
			return ""
		}
	}

	return strings.TrimSpace(strings.ToLower(payload.Effort))
}

func mapClaudeOutputConfigToOpenAIReasoningEffort(outputConfig any) string {
	effort := claudeOutputConfigEffort(outputConfig)
	if effort == "" {
		return ""
	}
	if mapped, ok := getClaudeToOpenAIReasoningMap()[effort]; ok {
		return mapped
	}
	if _, ok := allowedOpenAIReasoningEfforts[effort]; ok {
		return effort
	}
	return ""
}

func convertClaudeWebSearchToolToOpenAIOptions(tool *dto.ClaudeWebSearchTool) *dto.WebSearchOptions {
	if tool == nil {
		return nil
	}

	options := &dto.WebSearchOptions{
		SearchContextSize: claudeWebSearchMaxUsesToContextSize(tool.MaxUses),
	}
	if tool.UserLocation != nil {
		if userLocationRaw := dto.BuildChatWebSearchUserLocation(tool.UserLocation); len(userLocationRaw) > 0 {
			options.UserLocation = userLocationRaw
		}
	}
	return options
}

func claudeWebSearchMaxUsesToContextSize(maxUses int) string {
	switch maxUses {
	case 1:
		return "low"
	case 5:
		return "medium"
	case 10:
		return "high"
	default:
		return "medium"
	}
}

func generateStopBlock(index int) *dto.ClaudeResponse {
	return &dto.ClaudeResponse{
		Type:  "content_block_stop",
		Index: common.GetPointer[int](index),
	}
}

func ResponseOpenAI2Claude(openAIResponse *dto.OpenAITextResponse, info *relaycommon.RelayInfo) *dto.ClaudeResponse {
	var stopReason string
	contents := make([]dto.ClaudeMediaMessage, 0)
	claudeResponse := &dto.ClaudeResponse{
		Id:    openAIResponse.Id,
		Type:  "message",
		Role:  "assistant",
		Model: openAIResponse.Model,
	}
	for _, choice := range openAIResponse.Choices {
		stopReason = stopReasonOpenAI2Claude(choice.FinishReason)
		reasoningContent := choice.Message.ReasoningContent
		if reasoningContent == "" {
			reasoningContent = choice.Message.Reasoning
		}
		if reasoningContent != "" {
			claudeThinking := dto.ClaudeMediaMessage{Type: "thinking"}
			claudeThinking.Thinking = &reasoningContent
			contents = append(contents, claudeThinking)
		}
		if choice.FinishReason == "tool_calls" {
			for _, toolUse := range choice.Message.ParseToolCalls() {
				claudeContent := dto.ClaudeMediaMessage{}
				claudeContent.Type = "tool_use"
				claudeContent.Id = toolUse.ID
				claudeContent.Name = toolUse.Function.Name
				var mapParams map[string]interface{}
				if err := common.DecodeJsonUseNumberStrict(strings.NewReader(toolUse.Function.Arguments), &mapParams); err == nil {
					claudeContent.Input = SanitizeClaudeToolInput(toolUse.Function.Name, mapParams)
				} else {
					claudeContent.Input = toolUse.Function.Arguments
				}
				contents = append(contents, claudeContent)
			}
		} else {
			claudeContent := dto.ClaudeMediaMessage{}
			claudeContent.Type = "text"
			claudeContent.SetText(choice.Message.StringContent())
			contents = append(contents, claudeContent)
		}
	}
	claudeResponse.Content = contents
	claudeResponse.StopReason = stopReason
	claudeResponse.Usage = buildClaudeUsageFromOpenAIUsage(&openAIResponse.Usage)

	return claudeResponse
}

func buildClaudeUsageFromOpenAIUsage(usage *dto.Usage) *dto.ClaudeUsage {
	if usage == nil {
		return nil
	}
	claudeUsage := &dto.ClaudeUsage{
		InputTokens:              usage.PromptTokens,
		OutputTokens:             usage.CompletionTokens,
		CacheCreationInputTokens: usage.PromptTokensDetails.CachedCreationTokens,
		CacheReadInputTokens:     usage.PromptTokensDetails.CachedTokens,
	}
	if usage.WebSearchRequests > 0 {
		claudeUsage.ServerToolUse = &dto.ClaudeServerToolUse{
			WebSearchRequests: usage.WebSearchRequests,
		}
	}
	return claudeUsage
}

func stopReasonOpenAI2Claude(reason string) string {
	return reasonmap.OpenAIFinishReasonToClaudeStopReason(reason)
}

func toJSONString(v interface{}) string {
	b, err := common.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func GeminiToOpenAIRequest(geminiRequest *dto.GeminiChatRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	openaiRequest := &dto.GeneralOpenAIRequest{
		Model:  info.UpstreamModelName,
		Stream: info.IsStream,
	}

	// 转换 messages
	var messages []dto.Message
	for _, content := range geminiRequest.Contents {
		message := dto.Message{
			Role: convertGeminiRoleToOpenAI(content.Role),
		}

		// 处理 parts
		var mediaContents []dto.MediaContent
		var toolCalls []dto.ToolCallRequest
		for _, part := range content.Parts {
			if part.Text != "" {
				mediaContent := dto.MediaContent{
					Type: "text",
					Text: part.Text,
				}
				mediaContents = append(mediaContents, mediaContent)
			} else if part.InlineData != nil {
				mediaContent := dto.MediaContent{
					Type: "image_url",
					ImageUrl: &dto.MessageImageUrl{
						Url:      fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data),
						Detail:   "auto",
						MimeType: part.InlineData.MimeType,
					},
				}
				mediaContents = append(mediaContents, mediaContent)
			} else if part.FileData != nil {
				mediaContent := dto.MediaContent{
					Type: "image_url",
					ImageUrl: &dto.MessageImageUrl{
						Url:      part.FileData.FileUri,
						Detail:   "auto",
						MimeType: part.FileData.MimeType,
					},
				}
				mediaContents = append(mediaContents, mediaContent)
			} else if part.FunctionCall != nil {
				// 处理 Gemini 的工具调用
				toolCall := dto.ToolCallRequest{
					ID:   fmt.Sprintf("call_%d", len(toolCalls)+1), // 生成唯一ID
					Type: "function",
					Function: dto.FunctionRequest{
						Name:      part.FunctionCall.FunctionName,
						Arguments: toJSONString(part.FunctionCall.Arguments),
					},
				}
				toolCalls = append(toolCalls, toolCall)
			} else if part.FunctionResponse != nil {
				// 处理 Gemini 的工具响应，创建单独的 tool 消息
				toolMessage := dto.Message{
					Role:       "tool",
					ToolCallId: fmt.Sprintf("call_%d", len(toolCalls)), // 使用对应的调用ID
				}
				toolMessage.SetStringContent(toJSONString(part.FunctionResponse.Response))
				messages = append(messages, toolMessage)
			}
		}

		// 设置消息内容
		if len(toolCalls) > 0 {
			// 如果有工具调用，设置工具调用
			message.SetToolCalls(toolCalls)
		} else if len(mediaContents) == 1 && mediaContents[0].Type == "text" {
			// 如果只有一个文本内容，直接设置字符串
			message.Content = mediaContents[0].Text
		} else if len(mediaContents) > 0 {
			// 如果有多个内容或包含媒体，设置为数组
			message.SetMediaContent(mediaContents)
		}

		// 只有当消息有内容或工具调用时才添加
		if len(message.ParseContent()) > 0 || len(message.ToolCalls) > 0 {
			messages = append(messages, message)
		}
	}

	openaiRequest.Messages = messages

	if geminiRequest.GenerationConfig.Temperature != nil {
		openaiRequest.Temperature = geminiRequest.GenerationConfig.Temperature
	}
	if geminiRequest.GenerationConfig.TopP > 0 {
		openaiRequest.TopP = geminiRequest.GenerationConfig.TopP
	}
	if geminiRequest.GenerationConfig.TopK > 0 {
		openaiRequest.TopK = int(geminiRequest.GenerationConfig.TopK)
	}
	if geminiRequest.GenerationConfig.MaxOutputTokens > 0 {
		openaiRequest.MaxTokens = geminiRequest.GenerationConfig.MaxOutputTokens
	}
	// gemini stop sequences 最多 5 个，openai stop 最多 4 个
	if len(geminiRequest.GenerationConfig.StopSequences) > 0 {
		openaiRequest.Stop = geminiRequest.GenerationConfig.StopSequences[:4]
	}
	if geminiRequest.GenerationConfig.CandidateCount > 0 {
		openaiRequest.N = geminiRequest.GenerationConfig.CandidateCount
	}

	// 转换工具调用
	if len(geminiRequest.GetTools()) > 0 {
		var tools []dto.ToolCallRequest
		for _, tool := range geminiRequest.GetTools() {
			if tool.FunctionDeclarations != nil {
				functionDeclarations, err := common.Any2Type[[]dto.FunctionRequest](tool.FunctionDeclarations)
				if err != nil {
					common.SysError(fmt.Sprintf("failed to parse gemini function declarations: %v (type=%T)", err, tool.FunctionDeclarations))
					continue
				}
				for _, function := range functionDeclarations {
					openAITool := dto.ToolCallRequest{
						Type: "function",
						Function: dto.FunctionRequest{
							Name:        function.Name,
							Description: function.Description,
							Parameters:  function.Parameters,
						},
					}
					tools = append(tools, openAITool)
				}
			}
		}
		if len(tools) > 0 {
			openaiRequest.Tools = tools
		}
	}

	// gemini system instructions
	if geminiRequest.SystemInstructions != nil {
		// 将系统指令作为第一条消息插入
		systemMessage := dto.Message{
			Role:    "system",
			Content: extractTextFromGeminiParts(geminiRequest.SystemInstructions.Parts),
		}
		openaiRequest.Messages = append([]dto.Message{systemMessage}, openaiRequest.Messages...)
	}

	return openaiRequest, nil
}

func convertGeminiRoleToOpenAI(geminiRole string) string {
	switch geminiRole {
	case "user":
		return "user"
	case "model":
		return "assistant"
	case "function":
		return "function"
	default:
		return "user"
	}
}

func extractTextFromGeminiParts(parts []dto.GeminiPart) string {
	var texts []string
	for _, part := range parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// ResponseOpenAI2Gemini 将 OpenAI 响应转换为 Gemini 格式
func ResponseOpenAI2Gemini(openAIResponse *dto.OpenAITextResponse, info *relaycommon.RelayInfo) *dto.GeminiChatResponse {
	geminiResponse := &dto.GeminiChatResponse{
		Candidates: make([]dto.GeminiChatCandidate, 0, len(openAIResponse.Choices)),
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     openAIResponse.PromptTokens,
			CandidatesTokenCount: openAIResponse.CompletionTokens,
			TotalTokenCount:      openAIResponse.PromptTokens + openAIResponse.CompletionTokens,
		},
	}

	for _, choice := range openAIResponse.Choices {
		candidate := dto.GeminiChatCandidate{
			Index:         int64(choice.Index),
			SafetyRatings: []dto.GeminiChatSafetyRating{},
		}

		// 设置结束原因
		var finishReason string
		switch choice.FinishReason {
		case "stop":
			finishReason = "STOP"
		case "length":
			finishReason = "MAX_TOKENS"
		case "content_filter":
			finishReason = "SAFETY"
		case "tool_calls":
			finishReason = "STOP"
		default:
			finishReason = "STOP"
		}
		candidate.FinishReason = &finishReason

		// 转换消息内容
		content := dto.GeminiChatContent{
			Role:  "model",
			Parts: make([]dto.GeminiPart, 0),
		}

		// 处理工具调用
		toolCalls := choice.Message.ParseToolCalls()
		if len(toolCalls) > 0 {
			for _, toolCall := range toolCalls {
				// 解析参数
				var args map[string]interface{}
				if toolCall.Function.Arguments != "" {
					if err := common.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
						args = map[string]interface{}{"arguments": toolCall.Function.Arguments}
					}
				} else {
					args = make(map[string]interface{})
				}

				part := dto.GeminiPart{
					FunctionCall: &dto.FunctionCall{
						FunctionName: toolCall.Function.Name,
						Arguments:    args,
					},
				}
				content.Parts = append(content.Parts, part)
			}
		} else {
			// 处理文本内容
			textContent := choice.Message.StringContent()
			if textContent != "" {
				part := dto.GeminiPart{
					Text: textContent,
				}
				content.Parts = append(content.Parts, part)
			}
		}

		candidate.Content = content
		geminiResponse.Candidates = append(geminiResponse.Candidates, candidate)
	}

	return geminiResponse
}

// StreamResponseOpenAI2Gemini 将 OpenAI 流式响应转换为 Gemini 格式
func StreamResponseOpenAI2Gemini(openAIResponse *dto.ChatCompletionsStreamResponse, info *relaycommon.RelayInfo) *dto.GeminiChatResponse {
	// 检查是否有实际内容或结束标志
	hasContent := false
	hasFinishReason := false
	for _, choice := range openAIResponse.Choices {
		if len(choice.Delta.GetContentString()) > 0 || (choice.Delta.ToolCalls != nil && len(choice.Delta.ToolCalls) > 0) {
			hasContent = true
		}
		if choice.FinishReason != nil {
			hasFinishReason = true
		}
	}

	// 如果没有实际内容且没有结束标志，跳过。主要针对 openai 流响应开头的空数据
	if !hasContent && !hasFinishReason {
		return nil
	}

	geminiResponse := &dto.GeminiChatResponse{
		Candidates: make([]dto.GeminiChatCandidate, 0, len(openAIResponse.Choices)),
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     info.GetEstimatePromptTokens(),
			CandidatesTokenCount: 0, // 流式响应中可能没有完整的 usage 信息
			TotalTokenCount:      info.GetEstimatePromptTokens(),
		},
	}

	if openAIResponse.Usage != nil {
		geminiResponse.UsageMetadata.PromptTokenCount = openAIResponse.Usage.PromptTokens
		geminiResponse.UsageMetadata.CandidatesTokenCount = openAIResponse.Usage.CompletionTokens
		geminiResponse.UsageMetadata.TotalTokenCount = openAIResponse.Usage.TotalTokens
	}

	for _, choice := range openAIResponse.Choices {
		candidate := dto.GeminiChatCandidate{
			Index:         int64(choice.Index),
			SafetyRatings: []dto.GeminiChatSafetyRating{},
		}

		// 设置结束原因
		if choice.FinishReason != nil {
			var finishReason string
			switch *choice.FinishReason {
			case "stop":
				finishReason = "STOP"
			case "length":
				finishReason = "MAX_TOKENS"
			case "content_filter":
				finishReason = "SAFETY"
			case "tool_calls":
				finishReason = "STOP"
			default:
				finishReason = "STOP"
			}
			candidate.FinishReason = &finishReason
		}

		// 转换消息内容
		content := dto.GeminiChatContent{
			Role:  "model",
			Parts: make([]dto.GeminiPart, 0),
		}

		// 处理工具调用
		if choice.Delta.ToolCalls != nil {
			for _, toolCall := range choice.Delta.ToolCalls {
				// 解析参数
				var args map[string]interface{}
				if toolCall.Function.Arguments != "" {
					if err := common.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
						args = map[string]interface{}{"arguments": toolCall.Function.Arguments}
					}
				} else {
					args = make(map[string]interface{})
				}

				part := dto.GeminiPart{
					FunctionCall: &dto.FunctionCall{
						FunctionName: toolCall.Function.Name,
						Arguments:    args,
					},
				}
				content.Parts = append(content.Parts, part)
			}
		} else {
			// 处理文本内容
			textContent := choice.Delta.GetContentString()
			if textContent != "" {
				part := dto.GeminiPart{
					Text: textContent,
				}
				content.Parts = append(content.Parts, part)
			}
		}

		candidate.Content = content
		geminiResponse.Candidates = append(geminiResponse.Candidates, candidate)
	}

	return geminiResponse
}
