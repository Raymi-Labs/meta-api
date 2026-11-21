package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseClaude2OpenAIJoinsTextBlocks(t *testing.T) {
	var response dto.ClaudeResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"type":"message","content":[{"type":"text","text":"first "},{"type":"thinking","thinking":"step"},{"type":"text","text":"second"}],"stop_reason":"end_turn"}`, &response))
	converted := ResponseClaude2OpenAI(&response)
	require.Equal(t, "first second", converted.Choices[0].Message.StringContent())
}

func TestIsEffectiveClaudeStreamOutput(t *testing.T) {
	text := "hello"
	thinking := "reasoning"
	partialJSON := `{"name"`
	empty := "   "

	tests := []struct {
		name     string
		response *dto.ClaudeResponse
		want     bool
	}{
		{name: "nil", response: nil, want: false},
		{name: "lifecycle", response: &dto.ClaudeResponse{Type: "message_start"}, want: false},
		{name: "empty text", response: &dto.ClaudeResponse{Delta: &dto.ClaudeMediaMessage{Text: &empty}}, want: false},
		{name: "text", response: &dto.ClaudeResponse{Delta: &dto.ClaudeMediaMessage{Text: &text}}, want: true},
		{name: "thinking", response: &dto.ClaudeResponse{Delta: &dto.ClaudeMediaMessage{Thinking: &thinking}}, want: true},
		{name: "partial json", response: &dto.ClaudeResponse{Delta: &dto.ClaudeMediaMessage{PartialJson: &partialJSON}}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, isEffectiveClaudeStreamOutput(test.response))
		})
	}
}

func TestHandleClaudeResponseData_UsesUpstreamStatusCodeForClaudeError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	resp := &http.Response{StatusCode: http.StatusTooManyRequests}
	body := []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`)

	err := HandleClaudeResponseData(ctx, info, claudeInfo, resp, body)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	assert.Equal(t, "rate_limit_error", err.ToOpenAIError().Type)
	assert.Equal(t, "rate limited", err.ToOpenAIError().Message)
}

func TestHandleStreamResponseData_InfersClaudeErrorStatusCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	err := HandleStreamResponseData(ctx, info, claudeInfo, `{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`)
	require.NotNil(t, err)
	assert.Equal(t, 529, err.StatusCode)
	assert.Equal(t, "overloaded_error", err.ToOpenAIError().Type)
}

func TestHandleStreamResponseData_UsesBadRequestForInvalidRequestError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	err := HandleStreamResponseData(ctx, info, claudeInfo, `{"type":"error","error":{"type":"invalid_request_error","message":"bad input"}}`)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "invalid_request_error", err.ToOpenAIError().Type)
}

func TestRequestOpenAI2ClaudeMessageKeepsEmptyToolArguments(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	_ = recorder

	req := dto.GeneralOpenAIRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 128,
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: json.RawMessage(`[{
					"id":"call_1",
					"type":"function",
					"function":{"name":"noop","arguments":""}
				}]`),
			},
		},
	}

	claudeReq, err := RequestOpenAI2ClaudeMessage(ctx, req)
	require.NoError(t, err)
	require.Len(t, claudeReq.Messages, 2)

	blocks, ok := claudeReq.Messages[1].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	var toolUse *dto.ClaudeMediaMessage
	for i := range blocks {
		if blocks[i].Type == "tool_use" {
			toolUse = &blocks[i]
			break
		}
	}
	require.NotNil(t, toolUse)
	assert.Equal(t, "call_1", toolUse.Id)
	assert.Equal(t, "noop", toolUse.Name)
	assert.Equal(t, map[string]any{}, toolUse.Input)
}

func TestHandleStreamResponseData_UnknownClaudeErrorFallsBackToInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	err := HandleStreamResponseData(ctx, info, claudeInfo, `{"type":"error","error":{"type":"mystery_error","message":"unknown"}}`)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.Equal(t, "mystery_error", err.ToOpenAIError().Type)
}

func TestHandleStreamResponseData_PrefersHTTPStatusWhenProvided(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	err := HandleStreamResponseData(ctx, info, claudeInfo, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`, http.StatusTooManyRequests)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	assert.Equal(t, "rate_limit_error", err.ToOpenAIError().Type)
}

func TestFormatClaudeResponseInfo_MessageStart(t *testing.T) {
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{},
	}
	claudeResponse := &dto.ClaudeResponse{
		Type: "message_start",
		Message: &dto.ClaudeMediaMessage{
			Id:    "msg_123",
			Model: "claude-3-5-sonnet",
			Usage: &dto.ClaudeUsage{
				InputTokens:              100,
				OutputTokens:             1,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     30,
			},
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	if claudeInfo.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", claudeInfo.Usage.PromptTokens)
	}
	if claudeInfo.Usage.PromptTokensDetails.CachedTokens != 30 {
		t.Errorf("CachedTokens = %d, want 30", claudeInfo.Usage.PromptTokensDetails.CachedTokens)
	}
	if claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens != 50 {
		t.Errorf("CachedCreationTokens = %d, want 50", claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens)
	}
	if claudeInfo.ResponseId != "msg_123" {
		t.Errorf("ResponseId = %s, want msg_123", claudeInfo.ResponseId)
	}
	if claudeInfo.Model != "claude-3-5-sonnet" {
		t.Errorf("Model = %s, want claude-3-5-sonnet", claudeInfo.Model)
	}
}

func TestFormatClaudeResponseInfo_MessageDelta_FullUsage(t *testing.T) {
	// message_start 先积累 usage
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokens: 100,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         30,
				CachedCreationTokens: 50,
			},
			CompletionTokens: 1,
		},
	}

	// message_delta 带完整 usage（原生 Anthropic 场景）
	claudeResponse := &dto.ClaudeResponse{
		Type:  "message_delta",
		Delta: &dto.ClaudeMediaMessage{StopReason: common.GetPointer("end_turn")},
		Usage: &dto.ClaudeUsage{
			InputTokens:              100,
			OutputTokens:             200,
			CacheCreationInputTokens: 50,
			CacheReadInputTokens:     30,
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	if claudeInfo.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", claudeInfo.Usage.PromptTokens)
	}
	if claudeInfo.Usage.CompletionTokens != 200 {
		t.Errorf("CompletionTokens = %d, want 200", claudeInfo.Usage.CompletionTokens)
	}
	if claudeInfo.Usage.TotalTokens != 300 {
		t.Errorf("TotalTokens = %d, want 300", claudeInfo.Usage.TotalTokens)
	}
	if !claudeInfo.Done {
		t.Error("expected Done = true")
	}
}

func TestFormatClaudeResponseInfo_MessageDelta_OnlyOutputTokens(t *testing.T) {
	// 模拟 Bedrock: message_start 已积累 usage
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokens: 100,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         30,
				CachedCreationTokens: 50,
			},
			CompletionTokens:            1,
			ClaudeCacheCreation5mTokens: 10,
			ClaudeCacheCreation1hTokens: 20,
		},
	}

	// Bedrock 的 message_delta 只有 output_tokens，缺少 input_tokens 和 cache 字段
	claudeResponse := &dto.ClaudeResponse{
		Type:  "message_delta",
		Delta: &dto.ClaudeMediaMessage{StopReason: common.GetPointer("end_turn")},
		Usage: &dto.ClaudeUsage{
			OutputTokens: 200,
			// InputTokens, CacheCreationInputTokens, CacheReadInputTokens 都是 0
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	// PromptTokens 应保持 message_start 的值（因为 message_delta 的 InputTokens=0，不更新）
	if claudeInfo.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", claudeInfo.Usage.PromptTokens)
	}
	if claudeInfo.Usage.CompletionTokens != 200 {
		t.Errorf("CompletionTokens = %d, want 200", claudeInfo.Usage.CompletionTokens)
	}
	if claudeInfo.Usage.TotalTokens != 300 {
		t.Errorf("TotalTokens = %d, want 300", claudeInfo.Usage.TotalTokens)
	}
	// cache 字段应保持 message_start 的值
	if claudeInfo.Usage.PromptTokensDetails.CachedTokens != 30 {
		t.Errorf("CachedTokens = %d, want 30", claudeInfo.Usage.PromptTokensDetails.CachedTokens)
	}
	if claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens != 50 {
		t.Errorf("CachedCreationTokens = %d, want 50", claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens)
	}
	if claudeInfo.Usage.ClaudeCacheCreation5mTokens != 10 {
		t.Errorf("ClaudeCacheCreation5mTokens = %d, want 10", claudeInfo.Usage.ClaudeCacheCreation5mTokens)
	}
	if claudeInfo.Usage.ClaudeCacheCreation1hTokens != 20 {
		t.Errorf("ClaudeCacheCreation1hTokens = %d, want 20", claudeInfo.Usage.ClaudeCacheCreation1hTokens)
	}
	if !claudeInfo.Done {
		t.Error("expected Done = true")
	}
}

func TestFormatClaudeResponseInfo_NilClaudeInfo(t *testing.T) {
	claudeResponse := &dto.ClaudeResponse{Type: "message_start"}
	ok := FormatClaudeResponseInfo(claudeResponse, nil, nil)
	if ok {
		t.Error("expected false for nil claudeInfo")
	}
}

func TestFormatClaudeResponseInfo_ContentBlockDelta(t *testing.T) {
	text := "hello"
	claudeInfo := &ClaudeResponseInfo{
		Usage:        &dto.Usage{},
		ResponseText: strings.Builder{},
	}
	claudeResponse := &dto.ClaudeResponse{
		Type: "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{
			Text: &text,
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	if claudeInfo.ResponseText.String() != "hello" {
		t.Errorf("ResponseText = %q, want %q", claudeInfo.ResponseText.String(), "hello")
	}
}

func TestRequestOpenAI2ClaudeMessage_AssistantToolCallWithMalformedArguments(t *testing.T) {
	assistantMessage := dto.Message{
		Role:    "assistant",
		Content: "calling tool",
	}
	assistantMessage.SetToolCalls([]dto.ToolCallRequest{
		{
			ID:   "call_1",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "search_notes",
				Arguments: `{"query":`,
			},
		},
	})

	claudeReq, err := RequestOpenAI2ClaudeMessage(nil, dto.GeneralOpenAIRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "find it",
			},
			assistantMessage,
		},
	})
	require.NoError(t, err)
	require.Len(t, claudeReq.Messages, 2)

	content, err := claudeReq.Messages[1].ParseContent()
	require.NoError(t, err)
	require.Len(t, content, 2)
	assert.Equal(t, "tool_use", content[1].Type)
	assert.Equal(t, "call_1", content[1].Id)
	assert.Equal(t, "search_notes", content[1].Name)

	inputObj, ok := content[1].Input.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, inputObj)
}

func TestRequestOpenAI2ClaudeMessage_OmitsTopPForAdaptiveOpus(t *testing.T) {
	req, err := RequestOpenAI2ClaudeMessage(nil, dto.GeneralOpenAIRequest{
		Model: "claude-opus-4-6-high",
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)

	payload, err := json.Marshal(req)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), `"top_p"`)
}

func TestRequestOpenAI2ClaudeMessage_Opus47EffortSuffixUsesSummarizedAdaptiveReasoning(t *testing.T) {
	temperature := 0.2
	req, err := RequestOpenAI2ClaudeMessage(nil, dto.GeneralOpenAIRequest{
		Model:       "claude-opus-4-7-xhigh",
		Temperature: &temperature,
		TopP:        0.9,
		TopK:        7,
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, req.Thinking)

	assert.Equal(t, "claude-opus-4-7", req.Model)
	assert.Equal(t, "adaptive", req.Thinking.Type)
	assert.JSONEq(t, `{"effort":"xhigh"}`, string(req.OutputConfig))
	assert.Nil(t, req.Temperature)
	assert.Nil(t, req.TopP)
	assert.Zero(t, req.TopK)

	payload, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"display":"summarized"`)
}

func TestRequestOpenAI2ClaudeMessage_Opus47ThinkingSuffixMapsToAdaptiveHigh(t *testing.T) {
	temperature := 0.2
	req, err := RequestOpenAI2ClaudeMessage(nil, dto.GeneralOpenAIRequest{
		Model:       "claude-opus-4-7-thinking",
		Temperature: &temperature,
		TopP:        0.9,
		TopK:        7,
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, req.Thinking)

	assert.Equal(t, "claude-opus-4-7", req.Model)
	assert.Equal(t, "adaptive", req.Thinking.Type)
	assert.JSONEq(t, `{"effort":"high"}`, string(req.OutputConfig))
	assert.Nil(t, req.Temperature)
	assert.Nil(t, req.TopP)
	assert.Zero(t, req.TopK)

	payload, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"display":"summarized"`)
}

func TestRequestOpenAI2ClaudeMessage_EmptyAndTextMergeMatchesOfficialBranch(t *testing.T) {
	req, err := RequestOpenAI2ClaudeMessage(nil, dto.GeneralOpenAIRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []dto.Message{
			{Role: "user", Content: ""},
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "... hello", req.Messages[0].GetStringContent())
}

func TestRequestOpenAI2ClaudeMessage_SkipsEmptyTextBlocksInStructuredContent(t *testing.T) {
	systemMessage := dto.Message{Role: "system"}
	systemMessage.SetMediaContent([]dto.MediaContent{
		{Type: dto.ContentTypeText, Text: ""},
	})

	userMessage := dto.Message{Role: "user"}
	userMessage.SetMediaContent([]dto.MediaContent{
		{Type: dto.ContentTypeText, Text: ""},
		{Type: dto.ContentTypeText, Text: "hello"},
	})

	req, err := RequestOpenAI2ClaudeMessage(nil, dto.GeneralOpenAIRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []dto.Message{
			systemMessage,
			userMessage,
		},
	})
	require.NoError(t, err)
	assert.Nil(t, req.System)
	require.Len(t, req.Messages, 1)

	content, err := req.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, content, 1)
	require.NotNil(t, content[0].Text)
	assert.Equal(t, "hello", *content[0].Text)
}

func TestRequestOpenAI2ClaudeMessage_PreservesCacheControl(t *testing.T) {
	var request dto.GeneralOpenAIRequest
	err := common.Unmarshal([]byte(`{
		"model":"claude-opus-4-8",
		"messages":[
			{"role":"system","content":[{"type":"text","text":"stable system","cache_control":{"type":"ephemeral"}}]},
			{"role":"user","content":[{"type":"text","text":"stable context","cache_control":{"type":"ephemeral"}}]}
		]
	}`), &request)
	require.NoError(t, err)

	claudeRequest, err := RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)

	system, ok := claudeRequest.System.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, system, 1)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(system[0].CacheControl))

	require.Len(t, claudeRequest.Messages, 1)
	content, err := claudeRequest.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, content, 1)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(content[0].CacheControl))
}

func TestStreamResponseClaude2OpenAI_EmptyInputJSONDeltaIgnored(t *testing.T) {
	empty := ""
	resp := &dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: func() *int { v := 1; return &v }(),
		Delta: &dto.ClaudeMediaMessage{
			Type:        "input_json_delta",
			PartialJson: &empty,
		},
	}

	chunk := StreamResponseClaude2OpenAI(resp, &ClaudeResponseInfo{})
	require.Nil(t, chunk)
}

func TestStreamResponseClaude2OpenAI_NonEmptyInputJSONDeltaPreserved(t *testing.T) {
	state := &ClaudeResponseInfo{}
	StreamResponseClaude2OpenAI(&dto.ClaudeResponse{
		Type: "content_block_start", Index: common.GetPointer(1),
		ContentBlock: &dto.ClaudeMediaMessage{Type: "tool_use", Id: "call_test", Name: "search"},
	}, state)
	partial := `{"query":"today"}`
	resp := &dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: func() *int { v := 1; return &v }(),
		Delta: &dto.ClaudeMediaMessage{
			Type:        "input_json_delta",
			PartialJson: &partial,
		},
	}

	chunk := StreamResponseClaude2OpenAI(resp, state)
	require.NotNil(t, chunk)
	require.Len(t, chunk.Choices, 1)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, partial, chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)
}

func TestStreamResponseClaude2OpenAI_NoArgToolEmitsObjectAtStop(t *testing.T) {
	claudeInfo := &ClaudeResponseInfo{}
	start := &dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: func() *int { v := 1; return &v }(),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "tool_use",
			Id:   "toolu_1",
			Name: "get_current_time",
		},
	}
	stop := &dto.ClaudeResponse{
		Type:  "content_block_stop",
		Index: func() *int { v := 1; return &v }(),
	}

	startChunk := StreamResponseClaude2OpenAI(start, claudeInfo)
	require.Nil(t, startChunk)

	stopChunk := StreamResponseClaude2OpenAI(stop, claudeInfo)
	require.NotNil(t, stopChunk)
	require.Len(t, stopChunk.Choices, 1)
	require.Len(t, stopChunk.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, "toolu_1", stopChunk.Choices[0].Delta.ToolCalls[0].ID)
	assert.Equal(t, "get_current_time", stopChunk.Choices[0].Delta.ToolCalls[0].Function.Name)
	assert.Equal(t, "{}", stopChunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)
}

func TestStreamResponseClaude2OpenAI_ArgToolKeepsIDNameOnDelta(t *testing.T) {
	claudeInfo := &ClaudeResponseInfo{}
	start := &dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: func() *int { v := 1; return &v }(),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "tool_use",
			Id:   "toolu_2",
			Name: "search_notes",
		},
	}
	partial := `{"query":"today"}`
	delta := &dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: func() *int { v := 1; return &v }(),
		Delta: &dto.ClaudeMediaMessage{
			Type:        "input_json_delta",
			PartialJson: &partial,
		},
	}

	startChunk := StreamResponseClaude2OpenAI(start, claudeInfo)
	require.Nil(t, startChunk)

	deltaChunk := StreamResponseClaude2OpenAI(delta, claudeInfo)
	require.NotNil(t, deltaChunk)
	require.Len(t, deltaChunk.Choices, 1)
	require.Len(t, deltaChunk.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, "toolu_2", deltaChunk.Choices[0].Delta.ToolCalls[0].ID)
	assert.Equal(t, "search_notes", deltaChunk.Choices[0].Delta.ToolCalls[0].Function.Name)
	assert.Equal(t, partial, deltaChunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)
}
