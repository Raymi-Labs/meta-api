package openaicompat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsRequestToResponsesRequest_MapsAssistantContentAndFields(t *testing.T) {
	t.Parallel()

	assistant := dto.Message{Role: "assistant"}
	assistant.SetMediaContent([]dto.MediaContent{{Type: dto.ContentTypeText, Text: "done"}})

	req := &dto.GeneralOpenAIRequest{
		Model:                "gpt-5",
		Messages:             []dto.Message{{Role: "system", Content: "be helpful"}, assistant, {Role: "user", Content: "next"}},
		Stream:               true,
		StreamOptions:        &dto.StreamOptions{IncludeUsage: true, IncludeObfuscation: true},
		ServiceTier:          "flex",
		SafetyIdentifier:     "user-1",
		PromptCacheKey:       "cache-key",
		PromptCacheRetention: []byte(`{"type":"ephemeral"}`),
		PromptCacheOptions:   []byte(`{"mode":"implicit","ttl":"30m"}`),
		TopLogProbs:          5,
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, req.StreamOptions, out.StreamOptions)
	require.Equal(t, "flex", out.ServiceTier)
	require.Equal(t, "user-1", out.SafetyIdentifier)
	require.NotNil(t, out.TopLogProbs)
	require.Equal(t, 5, *out.TopLogProbs)

	var promptCacheKey string
	err = common.Unmarshal(out.PromptCacheKey, &promptCacheKey)
	require.NoError(t, err)
	require.Equal(t, "cache-key", promptCacheKey)
	require.JSONEq(t, `{"type":"ephemeral"}`, string(out.PromptCacheRetention))
	require.JSONEq(t, `{"mode":"implicit","ttl":"30m"}`, string(out.PromptCacheOptions))

	var inputItems []map[string]any
	err = common.Unmarshal(out.Input, &inputItems)
	require.NoError(t, err)
	require.Len(t, inputItems, 2)

	assistantContent, ok := inputItems[0]["content"].([]any)
	require.True(t, ok)
	require.Len(t, assistantContent, 1)

	assistantPart, ok := assistantContent[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "output_text", assistantPart["type"])
	require.Equal(t, "done", assistantPart["text"])
}

func TestChatCompletionsRequestToResponsesRequest_AppendsBuiltInWebSearchTool(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-4.1",
		WebSearchOptions: &dto.WebSearchOptions{
			SearchContextSize: "high",
			UserLocation:      json.RawMessage(`{"type":"approximate","approximate":{"timezone":"Asia/Shanghai","country":"CN"}}`),
		},
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)

	var tools []map[string]any
	err = common.Unmarshal(out.Tools, &tools)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0]["type"])
	require.Equal(t, dto.BuildInToolWebSearch, tools[1]["type"])
	require.Equal(t, "high", tools[1]["search_context_size"])
	require.NotNil(t, tools[1]["user_location"])
	userLocation, ok := tools[1]["user_location"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "approximate", userLocation["type"])
	require.Equal(t, "Asia/Shanghai", userLocation["timezone"])
	require.Equal(t, "CN", userLocation["country"])

	var include []string
	err = common.Unmarshal(out.Include, &include)
	require.NoError(t, err)
	require.Equal(t, []string{"web_search_call.action.sources"}, include)
}

func TestChatCompletionsRequestToResponsesRequest_OmitsEmptyInstructionsWithoutSystemMessages(t *testing.T) {
	t.Parallel()

	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-5",
		Messages: []dto.Message{{Role: "user", Content: "hello"}},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Instructions, 0)
}

func TestChatCompletionsRequestToResponsesRequest_MapsSystemMessagesToInstructions(t *testing.T) {
	t.Parallel()

	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{Role: "system", Content: "be helpful"},
			{Role: "developer", Content: "format as json"},
			{Role: "user", Content: "hello"},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)

	var instructions string
	err = common.Unmarshal(out.Instructions, &instructions)
	require.NoError(t, err)
	require.Equal(t, "be helpful\n\nformat as json", instructions)
}

func TestChatToResponses_ToolStrictFieldPreserved(t *testing.T) {
	t.Parallel()

	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "weather?"}},
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
					Strict:      json.RawMessage(`true`),
				},
			},
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        "get_time",
					Description: "Get time",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)
	require.NotNil(t, out.Tools)

	var tools []map[string]any
	err = common.Unmarshal(out.Tools, &tools)
	require.NoError(t, err)
	require.Len(t, tools, 2)

	// First tool should have strict = true
	require.Equal(t, "function", tools[0]["type"])
	require.Equal(t, "get_weather", tools[0]["name"])
	strictVal, ok := tools[0]["strict"]
	require.True(t, ok, "strict field should be present")
	require.Equal(t, true, strictVal)

	// Second tool should NOT have strict field
	_, hasStrict := tools[1]["strict"]
	require.False(t, hasStrict, "strict field should be absent when not set")
}

func TestChatCompletionsRequestToResponsesRequest_UsesAutoReasoningSummary(t *testing.T) {
	t.Parallel()

	req := &dto.GeneralOpenAIRequest{
		Model:           "gpt-5",
		Messages:        []dto.Message{{Role: "user", Content: "hello"}},
		ReasoningEffort: "high",
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)
	require.NotNil(t, out.Reasoning)
	require.Equal(t, "high", out.Reasoning.Effort)
	require.Equal(t, "auto", out.Reasoning.Summary)
}

func TestResponsesToChat_ToolCallsExtractedWithText(t *testing.T) {
	t.Parallel()

	resp := &dto.OpenAIResponsesResponse{
		Model:     "gpt-4o",
		CreatedAt: 1700000000,
		Output: []dto.ResponsesOutput{
			{
				Type: "message",
				Role: "assistant",
				Content: []dto.ResponsesOutputContent{
					{Type: "output_text", Text: "Let me check the weather."},
				},
			},
			{
				Type:      "function_call",
				CallId:    "call_123",
				Name:      "get_weather",
				Arguments: dto.ResponsesArguments(`{"city":"Tokyo"}`),
			},
		},
	}

	out, _, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl-1")
	require.NoError(t, err)
	require.Len(t, out.Choices, 1)

	msg := out.Choices[0].Message

	// Both text and tool_calls should be present
	require.Equal(t, "Let me check the weather.", msg.Content)
	require.Equal(t, "tool_calls", out.Choices[0].FinishReason)

	var toolCalls []dto.ToolCallResponse
	err = common.Unmarshal(msg.ToolCalls, &toolCalls)
	require.NoError(t, err)
	require.Len(t, toolCalls, 1)
	require.Equal(t, "call_123", toolCalls[0].ID)
	require.Equal(t, "get_weather", toolCalls[0].Function.Name)
	require.Equal(t, `{"city":"Tokyo"}`, toolCalls[0].Function.Arguments)
}

func TestResponsesToChat_ToolCallsOnlyNilContent(t *testing.T) {
	t.Parallel()

	resp := &dto.OpenAIResponsesResponse{
		Model:     "gpt-4o",
		CreatedAt: 1700000000,
		Output: []dto.ResponsesOutput{
			{
				Type:      "function_call",
				CallId:    "call_456",
				Name:      "search",
				Arguments: dto.ResponsesArguments(`{"q":"test"}`),
			},
		},
	}

	out, _, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl-2")
	require.NoError(t, err)

	msg := out.Choices[0].Message
	require.Nil(t, msg.Content, "content should be nil when no text output")
	require.Equal(t, "tool_calls", out.Choices[0].FinishReason)

	var toolCalls []dto.ToolCallResponse
	err = common.Unmarshal(msg.ToolCalls, &toolCalls)
	require.NoError(t, err)
	require.Len(t, toolCalls, 1)
	require.Equal(t, "search", toolCalls[0].Function.Name)
}
