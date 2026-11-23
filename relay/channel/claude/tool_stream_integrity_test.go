package claude

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func toolStreamStart(state *ClaudeResponseInfo, block int, id, name string) {
	StreamResponseClaude2OpenAI(&dto.ClaudeResponse{
		Type: "content_block_start", Index: common.GetPointer(block),
		ContentBlock: &dto.ClaudeMediaMessage{Type: "tool_use", Id: id, Name: name},
	}, state)
}

func toolStreamDelta(state *ClaudeResponseInfo, block int, part string) *dto.ChatCompletionsStreamResponse {
	return StreamResponseClaude2OpenAI(&dto.ClaudeResponse{
		Type: "content_block_delta", Index: common.GetPointer(block),
		Delta: &dto.ClaudeMediaMessage{Type: "input_json_delta", PartialJson: &part},
	}, state)
}

func toolStreamStop(state *ClaudeResponseInfo, block int) *dto.ChatCompletionsStreamResponse {
	return StreamResponseClaude2OpenAI(&dto.ClaudeResponse{
		Type: "content_block_stop", Index: common.GetPointer(block),
	}, state)
}

// Reassemble the serialized wire deltas like an append-based client. Testing
// only a single Go object would miss repeated identities and JSON omission bugs.
func appendToolStreamChunk(t *testing.T, calls map[int]*dto.ToolCallResponse, chunk *dto.ChatCompletionsStreamResponse) {
	t.Helper()
	if chunk == nil {
		return
	}
	data, err := common.Marshal(chunk)
	require.NoError(t, err)
	var wire dto.ChatCompletionsStreamResponse
	require.NoError(t, common.Unmarshal(data, &wire))
	for _, choice := range wire.Choices {
		for _, delta := range choice.Delta.ToolCalls {
			require.NotNil(t, delta.Index)
			index := *delta.Index
			if calls[index] == nil {
				calls[index] = &dto.ToolCallResponse{Type: "function"}
			}
			call := calls[index]
			call.ID += delta.ID
			call.Function.Name += delta.Function.Name
			call.Function.Arguments += delta.Function.Arguments
		}
	}
}

func TestToolStreamIntegrityFragmentation(t *testing.T) {
	payload := `{"q":"福建 hello world \"quoted\"","nested":{"n":3},"items":["a","b"]}`
	runes := []rune(payload)
	for split := 0; split <= len(runes); split++ {
		state := &ClaudeResponseInfo{}
		toolStreamStart(state, 2, "call_search", "web_search")
		calls := make(map[int]*dto.ToolCallResponse)
		appendToolStreamChunk(t, calls, toolStreamDelta(state, 2, string(runes[:split])))
		appendToolStreamChunk(t, calls, toolStreamDelta(state, 2, string(runes[split:])))
		appendToolStreamChunk(t, calls, toolStreamStop(state, 2))
		require.Len(t, calls, 1, "split=%d", split)
		require.NotNil(t, calls[0], "first tool must have index 0")
		require.Equal(t, "call_search", calls[0].ID)
		require.Equal(t, "web_search", calls[0].Function.Name)
		require.Equal(t, payload, calls[0].Function.Arguments, "split=%d", split)
		require.Empty(t, state.ToolCallStreamStates)
	}
}

func TestToolStreamIntegrityWhitespaceAndIdentityOmission(t *testing.T) {
	state := &ClaudeResponseInfo{}
	toolStreamStart(state, 0, "call_search", "web_search")
	calls := make(map[int]*dto.ToolCallResponse)
	parts := []string{`{"q":"hello`, " ", `world",`, "\n\t", `"n":2}`}
	for i, part := range parts {
		chunk := toolStreamDelta(state, 0, part)
		require.NotNil(t, chunk)
		data, err := common.Marshal(chunk.Choices[0].Delta.ToolCalls[0])
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, common.Unmarshal(data, &fields))
		if i > 0 {
			require.NotContains(t, fields, "id")
			require.NotContains(t, fields["function"], "name")
		}
		appendToolStreamChunk(t, calls, chunk)
	}
	require.Nil(t, toolStreamStop(state, 0))
	require.Equal(t, strings.Join(parts, ""), calls[0].Function.Arguments)
	require.Equal(t, "web_search", calls[0].Function.Name)
}

func TestToolStreamIntegrityContentIndexes(t *testing.T) {
	for _, blocks := range [][]int{{0, 1}, {1, 2}, {2, 4}, {4, 9}} {
		state := &ClaudeResponseInfo{}
		calls := make(map[int]*dto.ToolCallResponse)
		for _, kind := range []string{"thinking", "text"} {
			StreamResponseClaude2OpenAI(&dto.ClaudeResponse{
				Type: "content_block_start", Index: common.GetPointer(0),
				ContentBlock: &dto.ClaudeMediaMessage{Type: kind},
			}, state)
		}
		toolStreamStart(state, blocks[0], "call_a", "tool_a")
		appendToolStreamChunk(t, calls, toolStreamStop(state, blocks[0]))
		// Removing finished state must not cause the next index to be reused.
		toolStreamStart(state, blocks[1], "call_b", "tool_b")
		appendToolStreamChunk(t, calls, toolStreamDelta(state, blocks[1], `{"n":2}`))
		require.Nil(t, toolStreamStop(state, blocks[1]))
		require.Len(t, calls, 2, "blocks=%v", blocks)
		require.Equal(t, "call_a", calls[0].ID)
		require.Equal(t, "{}", calls[0].Function.Arguments)
		require.Equal(t, "call_b", calls[1].ID)
		require.Equal(t, `{"n":2}`, calls[1].Function.Arguments)
	}
}

func TestToolStreamIntegrityInterleavedArguments(t *testing.T) {
	state := &ClaudeResponseInfo{}
	calls := make(map[int]*dto.ToolCallResponse)
	toolStreamStart(state, 0, "call_a", "tool_a")
	toolStreamStart(state, 1, "call_b", "tool_b")
	appendToolStreamChunk(t, calls, toolStreamDelta(state, 1, `{"b":`))
	appendToolStreamChunk(t, calls, toolStreamDelta(state, 0, `{"a":`))
	appendToolStreamChunk(t, calls, toolStreamDelta(state, 1, `2}`))
	appendToolStreamChunk(t, calls, toolStreamDelta(state, 0, `1}`))
	require.Nil(t, toolStreamStop(state, 0))
	require.Nil(t, toolStreamStop(state, 1))
	require.Equal(t, "call_a", calls[0].ID)
	require.Equal(t, `{"a":1}`, calls[0].Function.Arguments)
	require.Equal(t, "call_b", calls[1].ID)
	require.Equal(t, `{"b":2}`, calls[1].Function.Arguments)
}

func TestToolStreamIntegrityEmptyAndOrphanEvents(t *testing.T) {
	state := &ClaudeResponseInfo{}
	require.Nil(t, StreamResponseClaude2OpenAI(nil, state))
	require.Nil(t, toolStreamDelta(state, 0, `{"orphan":true}`))
	require.Nil(t, toolStreamStop(state, 0))
	toolStreamStart(state, 0, "call_a", "tool_a")
	require.Nil(t, toolStreamDelta(state, 0, ""))
	// A duplicate start must not replace an in-flight identity.
	toolStreamStart(state, 0, "bad_id", "bad_tool")
	chunk := toolStreamStop(state, 0)
	call := chunk.Choices[0].Delta.ToolCalls[0]
	require.Equal(t, "call_a", call.ID)
	require.Equal(t, "tool_a", call.Function.Name)
	require.Equal(t, "{}", call.Function.Arguments)
	require.Nil(t, toolStreamStop(state, 0))
	require.Nil(t, toolStreamDelta(state, 0, `late`))
}

func TestToolStreamIntegrityRequestIsolation(t *testing.T) {
	first, second := &ClaudeResponseInfo{}, &ClaudeResponseInfo{}
	toolStreamStart(first, 0, "first", "tool_a")
	toolStreamStart(second, 0, "second", "tool_b")
	a := toolStreamStop(first, 0).Choices[0].Delta.ToolCalls[0]
	b := toolStreamStop(second, 0).Choices[0].Delta.ToolCalls[0]
	require.Equal(t, "first", a.ID)
	require.Equal(t, "second", b.ID)
	require.Zero(t, *a.Index)
	require.Zero(t, *b.Index)
}

func TestToolStreamIntegritySecondRound(t *testing.T) {
	state := &ClaudeResponseInfo{}
	calls := make(map[int]*dto.ToolCallResponse)
	toolStreamStart(state, 2, "call_search", "web_search")
	for _, part := range []string{`{"q":"hello`, " ", `world"}`} {
		appendToolStreamChunk(t, calls, toolStreamDelta(state, 2, part))
	}
	toolStreamStop(state, 2)
	toolCalls, err := common.Marshal([]*dto.ToolCallResponse{calls[0]})
	require.NoError(t, err)
	for _, content := range []any{"sample result", []any{map[string]any{"type": "text", "text": "sample result"}}} {
		req := dto.GeneralOpenAIRequest{
			Model: "claude-opus-4-6", MaxTokens: 2048,
			Messages: []dto.Message{
				{Role: "user", Content: "search"},
				{Role: "assistant", ToolCalls: toolCalls},
				{Role: "tool", ToolCallId: "call_search", Content: content},
			},
		}
		out, err := RequestOpenAI2ClaudeMessage(nil, req)
		require.NoError(t, err)
		var tool *dto.ClaudeMediaMessage
		for _, block := range out.Messages[1].Content.([]dto.ClaudeMediaMessage) {
			if block.Type == "tool_use" {
				copy := block
				tool = &copy
			}
		}
		require.NotNil(t, tool)
		require.Equal(t, "call_search", tool.Id)
		require.Equal(t, "web_search", tool.Name)
		require.Equal(t, map[string]any{"q": "hello world"}, tool.Input)
		data, err := common.Marshal(out.Messages[2])
		require.NoError(t, err)
		require.Contains(t, string(data), `"tool_use_id":"call_search"`)
		require.Contains(t, string(data), "sample result")
	}
}

func TestToolStreamIntegrityNativeAndOpenAIEnds(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_demo","model":"claude-opus-4-6","usage":{"input_tokens":10,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_demo","name":"clock","input":{}}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	}
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		info := &relaycommon.RelayInfo{RelayFormat: format, ChannelMeta: &relaycommon.ChannelMeta{}}
		state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
		for _, raw := range events {
			require.Nil(t, HandleStreamResponseData(ctx, info, state, raw))
		}
		HandleStreamFinalResponse(ctx, info, state)
		body := w.Body.String()
		if format == types.RelayFormatClaude {
			for _, raw := range events {
				if !strings.Contains(raw, `"type":"message_delta"`) {
					require.Contains(t, body, raw)
				}
			}
			require.Contains(t, body, `"stop_reason":"tool_use"`)
			require.NotContains(t, body, "[DONE]")
		} else {
			require.Contains(t, body, `"finish_reason":"tool_calls"`)
			require.Equal(t, 1, strings.Count(body, `"id":"call_demo"`))
			require.Equal(t, 1, strings.Count(body, "[DONE]"))
		}
	}
}

func TestToolOnlyChatStreamWithRefusalHold(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fragments []string
		wantArgs  string
	}{
		{"fragmented", []string{`{"city":"New`, " ", `York"}`}, `{"city":"New York"}`},
		{"parameterless", nil, "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ShouldIncludeUsage: true, ChannelMeta: &relaycommon.ChannelMeta{}}
			state := &ClaudeResponseInfo{RequireMessageStop: true}
			state.StartRefusalHold()
			send := func(event string) {
				t.Helper()
				require.Nil(t, HandleStreamResponseData(c, info, state, event))
			}
			send(usageStart)
			require.Empty(t, w.Body.String())
			send(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}`)
			for _, fragment := range tc.fragments {
				part, err := common.Marshal(fragment)
				require.NoError(t, err)
				send(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":` + string(part) + `}}`)
				require.Contains(t, w.Body.String(), `"arguments":`+string(part))
			}
			send(`{"type":"content_block_stop","index":1}`)
			require.Contains(t, w.Body.String(), `"id":"call_1"`)
			send(`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`)
			require.NotContains(t, w.Body.String(), `"finish_reason":"tool_calls"`)
			send(usageStop)
			require.NoError(t, HandleStreamFinalResponse(c, info, state))

			calls := make(map[int]*dto.ToolCallResponse)
			out := w.Body.String()
			for _, line := range strings.Split(out, "\n") {
				if !strings.HasPrefix(line, "data: {") {
					continue
				}
				var chunk dto.ChatCompletionsStreamResponse
				require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &chunk))
				appendToolStreamChunk(t, calls, &chunk)
			}
			require.Len(t, calls, 1)
			require.NotNil(t, calls[0])
			require.Equal(t, "call_1", calls[0].ID)
			require.Equal(t, "lookup", calls[0].Function.Name)
			require.Equal(t, tc.wantArgs, calls[0].Function.Arguments)
			require.Equal(t, 1, strings.Count(out, `"finish_reason":"tool_calls"`))
			require.Contains(t, out, `"completion_tokens":5`)
			require.True(t, strings.HasSuffix(strings.TrimSpace(out), "data: [DONE]"))
		})
	}
}
