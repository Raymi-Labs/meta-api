package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func normalizeChatImageURLToString(v any) any {
	switch vv := v.(type) {
	case string:
		return vv
	case map[string]any:
		if url := common.Interface2String(vv["url"]); url != "" {
			return url
		}
		return v
	case dto.MessageImageUrl:
		if vv.Url != "" {
			return vv.Url
		}
		return v
	case *dto.MessageImageUrl:
		if vv != nil && vv.Url != "" {
			return vv.Url
		}
		return v
	default:
		return v
	}
}

func responsesImageDetail(imageURL any) string {
	var detail string
	switch image := imageURL.(type) {
	case map[string]any:
		detail, _ = image["detail"].(string)
	case dto.MessageImageUrl:
		detail = image.Detail
	case *dto.MessageImageUrl:
		if image != nil {
			detail = image.Detail
		}
	}
	switch normalized := strings.ToLower(strings.TrimSpace(detail)); normalized {
	case "low", "high", "auto", "original":
		return normalized
	}
	return "auto"
}

// Read the source hints because ParseContent defaults absent image detail to
// high for token estimation; Responses uses auto when the source omitted it.
func responsesMessageImageDetails(msg dto.Message) []string {
	var details []string
	appendPart := func(part any) {
		switch value := part.(type) {
		case dto.MediaContent:
			if value.Type == dto.ContentTypeImageURL {
				details = append(details, responsesImageDetail(value.ImageUrl))
			}
		case map[string]any:
			if value["type"] == dto.ContentTypeImageURL {
				details = append(details, responsesImageDetail(value["image_url"]))
			}
		}
	}
	switch content := msg.Content.(type) {
	case []any:
		for _, part := range content {
			appendPart(part)
		}
	case []dto.MediaContent:
		for _, part := range content {
			appendPart(part)
		}
	}
	return details
}

func convertChatResponseFormatToResponsesText(reqFormat *dto.ResponseFormat) json.RawMessage {
	if reqFormat == nil || strings.TrimSpace(reqFormat.Type) == "" {
		return nil
	}

	format := map[string]any{
		"type": reqFormat.Type,
	}

	if reqFormat.Type == "json_schema" && len(reqFormat.JsonSchema) > 0 {
		var chatSchema map[string]json.RawMessage
		if err := common.Unmarshal(reqFormat.JsonSchema, &chatSchema); err == nil {
			for key, value := range chatSchema {
				if key == "type" {
					continue
				}
				format[key] = value
			}

			var nested map[string]json.RawMessage
			if raw := chatSchema["json_schema"]; len(raw) > 0 && common.Unmarshal(raw, &nested) == nil && nested != nil {
				for key, value := range nested {
					if _, exists := format[key]; !exists {
						format[key] = value
					}
				}
				delete(format, "json_schema")
			}
		} else {
			format["json_schema"] = reqFormat.JsonSchema
		}
	}

	textRaw, _ := common.Marshal(map[string]any{
		"format": format,
	})
	return textRaw
}

func buildResponsesWebSearchTool(options *dto.WebSearchOptions) map[string]any {
	tool := map[string]any{
		"type": dto.BuildInToolWebSearch,
	}
	if options == nil {
		return tool
	}
	if size := strings.TrimSpace(options.SearchContextSize); size != "" {
		tool["search_context_size"] = size
	}
	if len(options.UserLocation) > 0 {
		if userLocation := dto.ParseApproximateWebSearchUserLocation(options.UserLocation); userLocation != nil {
			var normalized any
			if raw := dto.BuildResponsesWebSearchUserLocation(userLocation); len(raw) > 0 && common.Unmarshal(raw, &normalized) == nil {
				tool["user_location"] = normalized
			}
		}
	}
	return tool
}

func buildResponsesInclude(webSearchEnabled bool) json.RawMessage {
	if !webSearchEnabled {
		return nil
	}
	includeRaw, _ := common.Marshal([]string{"web_search_call.action.sources"})
	return includeRaw
}

func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if req.N > 1 {
		return nil, fmt.Errorf("n>1 is not supported in responses compatibility mode")
	}

	var instructionsParts []string
	inputItems := make([]map[string]any, 0, len(req.Messages))

	for _, msg := range req.Messages {
		if len(msg.Tools) > 0 {
			return nil, fmt.Errorf("message-level dynamic tools cannot be converted to Responses; use Chat Completions for this request")
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			continue
		}

		if role == "tool" || role == "function" {
			callID := strings.TrimSpace(msg.ToolCallId)

			var output any
			if msg.Content == nil {
				output = ""
			} else if msg.IsStringContent() {
				output = msg.StringContent()
			} else {
				if b, err := common.Marshal(msg.Content); err == nil {
					output = string(b)
				} else {
					output = fmt.Sprintf("%v", msg.Content)
				}
			}

			if callID == "" {
				inputItems = append(inputItems, map[string]any{
					"role":    "user",
					"content": fmt.Sprintf("[tool_output_missing_call_id] %v", output),
				})
				continue
			}

			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			})
			continue
		}

		// Prefer mapping system/developer messages into `instructions`.
		if role == "system" || role == "developer" {
			if msg.Content == nil {
				continue
			}
			if msg.IsStringContent() {
				if s := strings.TrimSpace(msg.StringContent()); s != "" {
					instructionsParts = append(instructionsParts, s)
				}
				continue
			}
			parts := msg.ParseContent()
			var sb strings.Builder
			for _, part := range parts {
				if part.Type == dto.ContentTypeText && strings.TrimSpace(part.Text) != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(part.Text)
				}
			}
			if s := strings.TrimSpace(sb.String()); s != "" {
				instructionsParts = append(instructionsParts, s)
			}
			continue
		}

		item := map[string]any{
			"role": role,
		}

		if msg.Content == nil {
			item["content"] = ""
			inputItems = append(inputItems, item)

			if role == "assistant" {
				for _, tc := range msg.ParseToolCalls() {
					if strings.TrimSpace(tc.ID) == "" {
						continue
					}
					if tc.Type != "" && tc.Type != "function" {
						continue
					}
					name := strings.TrimSpace(tc.Function.Name)
					if name == "" {
						continue
					}
					inputItems = append(inputItems, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      name,
						"arguments": tc.Function.Arguments,
					})
				}
			}
			continue
		}

		if msg.IsStringContent() {
			item["content"] = msg.StringContent()
			inputItems = append(inputItems, item)

			if role == "assistant" {
				for _, tc := range msg.ParseToolCalls() {
					if strings.TrimSpace(tc.ID) == "" {
						continue
					}
					if tc.Type != "" && tc.Type != "function" {
						continue
					}
					name := strings.TrimSpace(tc.Function.Name)
					if name == "" {
						continue
					}
					inputItems = append(inputItems, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      name,
						"arguments": tc.Function.Arguments,
					})
				}
			}
			continue
		}

		parts := msg.ParseContent()
		imageDetails := responsesMessageImageDetails(msg)
		imageIndex := 0
		contentParts := make([]map[string]any, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case dto.ContentTypeText:
				textType := "input_text"
				if role == "assistant" {
					textType = "output_text"
				}
				contentParts = append(contentParts, map[string]any{
					"type": textType,
					"text": part.Text,
				})
			case dto.ContentTypeImageURL:
				detail := responsesImageDetail(part.ImageUrl)
				if imageIndex < len(imageDetails) {
					detail = imageDetails[imageIndex]
				}
				imageIndex++
				contentParts = append(contentParts, map[string]any{
					"type":      "input_image",
					"image_url": normalizeChatImageURLToString(part.ImageUrl),
					"detail":    detail,
				})
			case dto.ContentTypeInputAudio:
				contentParts = append(contentParts, map[string]any{
					"type":        "input_audio",
					"input_audio": part.InputAudio,
				})
			case dto.ContentTypeFile:
				contentParts = append(contentParts, map[string]any{
					"type": "input_file",
					"file": part.File,
				})
			case dto.ContentTypeVideoUrl:
				contentParts = append(contentParts, map[string]any{
					"type":      "input_video",
					"video_url": part.VideoUrl,
				})
			default:
				contentParts = append(contentParts, map[string]any{
					"type": part.Type,
				})
			}
		}
		item["content"] = contentParts
		inputItems = append(inputItems, item)

		if role == "assistant" {
			for _, tc := range msg.ParseToolCalls() {
				if strings.TrimSpace(tc.ID) == "" {
					continue
				}
				if tc.Type != "" && tc.Type != "function" {
					continue
				}
				name := strings.TrimSpace(tc.Function.Name)
				if name == "" {
					continue
				}
				inputItems = append(inputItems, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      name,
					"arguments": tc.Function.Arguments,
				})
			}
		}
	}

	inputRaw, err := common.Marshal(inputItems)
	if err != nil {
		return nil, err
	}

	var instructionsRaw json.RawMessage
	if len(instructionsParts) > 0 {
		instructions := strings.Join(instructionsParts, "\n\n")
		instructionsRaw, _ = common.Marshal(instructions)
	}

	var toolsRaw json.RawMessage
	webSearchEnabled := false
	if req.Tools != nil {
		tools := make([]map[string]any, 0, len(req.Tools))
		hasBuiltInWebSearch := false
		for _, tool := range req.Tools {
			switch tool.Type {
			case "function":
				entry := map[string]any{
					"type":        "function",
					"name":        tool.Function.Name,
					"description": tool.Function.Description,
					"parameters":  tool.Function.Parameters,
				}
				if len(tool.Function.Strict) > 0 {
					entry["strict"] = tool.Function.Strict
				}
				tools = append(tools, entry)
			default:
				// Best-effort: keep original tool shape for unknown types.
				var m map[string]any
				if b, err := common.Marshal(tool); err == nil {
					_ = common.Unmarshal(b, &m)
				}
				if len(m) == 0 {
					m = map[string]any{"type": tool.Type}
				}
				if dto.IsBuiltInWebSearchToolType(common.Interface2String(m["type"])) {
					hasBuiltInWebSearch = true
				}
				tools = append(tools, m)
			}
		}
		if req.WebSearchOptions != nil && !hasBuiltInWebSearch {
			tools = append(tools, buildResponsesWebSearchTool(req.WebSearchOptions))
			hasBuiltInWebSearch = true
		}
		webSearchEnabled = hasBuiltInWebSearch
		toolsRaw, _ = common.Marshal(tools)
	} else if req.WebSearchOptions != nil {
		webSearchEnabled = true
		toolsRaw, _ = common.Marshal([]map[string]any{buildResponsesWebSearchTool(req.WebSearchOptions)})
	}

	var toolChoiceRaw json.RawMessage
	if req.ToolChoice != nil {
		switch v := req.ToolChoice.(type) {
		case string:
			toolChoiceRaw, _ = common.Marshal(v)
		default:
			var m map[string]any
			if b, err := common.Marshal(v); err == nil {
				_ = common.Unmarshal(b, &m)
			}
			if m == nil {
				toolChoiceRaw, _ = common.Marshal(v)
			} else if t, _ := m["type"].(string); t == "function" {
				// Chat: {"type":"function","function":{"name":"..."}}
				// Responses: {"type":"function","name":"..."}
				if name, ok := m["name"].(string); ok && name != "" {
					toolChoiceRaw, _ = common.Marshal(map[string]any{
						"type": "function",
						"name": name,
					})
				} else if fn, ok := m["function"].(map[string]any); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						toolChoiceRaw, _ = common.Marshal(map[string]any{
							"type": "function",
							"name": name,
						})
					} else {
						toolChoiceRaw, _ = common.Marshal(v)
					}
				} else {
					toolChoiceRaw, _ = common.Marshal(v)
				}
			} else {
				toolChoiceRaw, _ = common.Marshal(v)
			}
		}
	}

	var parallelToolCallsRaw json.RawMessage
	if req.ParallelTooCalls != nil {
		parallelToolCallsRaw, _ = common.Marshal(*req.ParallelTooCalls)
	}

	var promptCacheKeyRaw json.RawMessage
	if strings.TrimSpace(req.PromptCacheKey) != "" {
		promptCacheKeyRaw, _ = common.Marshal(req.PromptCacheKey)
	}

	textRaw := convertChatResponseFormatToResponsesText(req.ResponseFormat)

	maxOutputTokens := req.GetMaxTokens()
	// OpenAI Responses API rejects max_output_tokens < 16 when explicitly provided.
	//if maxOutputTokens > 0 && maxOutputTokens < 16 {
	//	maxOutputTokens = 16
	//}

	var topP *float64
	if req.TopP != 0 {
		topP = common.GetPointer(req.TopP)
	}

	var topLogProbs *int
	if req.TopLogProbs != 0 {
		topLogProbs = common.GetPointer(req.TopLogProbs)
	}

	out := &dto.OpenAIResponsesRequest{
		Model:                req.Model,
		Input:                inputRaw,
		Include:              buildResponsesInclude(webSearchEnabled),
		Instructions:         instructionsRaw,
		MaxOutputTokens:      maxOutputTokens,
		TopLogProbs:          topLogProbs,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		Temperature:          req.Temperature,
		Text:                 textRaw,
		ToolChoice:           toolChoiceRaw,
		Tools:                toolsRaw,
		TopP:                 topP,
		User:                 req.User,
		ParallelToolCalls:    parallelToolCallsRaw,
		ServiceTier:          req.ServiceTier,
		SafetyIdentifier:     req.SafetyIdentifier,
		Store:                req.Store,
		PromptCacheKey:       promptCacheKeyRaw,
		PromptCacheRetention: req.PromptCacheRetention,
		PromptCacheOptions:   req.PromptCacheOptions,
		Metadata:             req.Metadata,
	}

	if req.ReasoningEffort != "" {
		out.Reasoning = &dto.Reasoning{
			Effort:  req.ReasoningEffort,
			Summary: "auto",
		}
	}

	return out, nil
}
