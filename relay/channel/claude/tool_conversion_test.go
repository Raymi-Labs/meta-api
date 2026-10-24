package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionParametersToClaudeInputSchema(t *testing.T) {
	tests := []struct {
		name       string
		parameters any
		want       map[string]any
	}{
		{
			name:       "omitted parameters",
			parameters: nil,
			want: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			name:       "typed nil map",
			parameters: map[string]any(nil),
			want: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			name: "missing type and properties",
			parameters: map[string]any{
				"additionalProperties": false,
			},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			name: "nil type and properties",
			parameters: map[string]any{
				"type":       nil,
				"properties": nil,
				"required":   []any{"city"},
			},
			want: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{"city"},
			},
		},
		{
			name: "preserve complete schema and extensions",
			parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required":             []any{"city"},
				"additionalProperties": false,
				"$defs":                map[string]any{"unit": map[string]any{"enum": []any{"c", "f"}}},
			},
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required":             []any{"city"},
				"additionalProperties": false,
				"$defs":                map[string]any{"unit": map[string]any{"enum": []any{"c", "f"}}},
			},
		},
		{
			name: "non-string type does not panic",
			parameters: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
			want: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
		},
		{
			name:       "non-object parameters fall back to empty object",
			parameters: []any{"invalid"},
			want: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, functionParametersToClaudeInputSchema(test.parameters))
		})
	}
}

func TestFunctionParametersToClaudeInputSchemaDoesNotMutateSource(t *testing.T) {
	parameters := map[string]any{"additionalProperties": false}

	schema := functionParametersToClaudeInputSchema(parameters).(map[string]any)

	assert.NotContains(t, parameters, "type")
	assert.NotContains(t, parameters, "properties")
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, map[string]any{}, schema["properties"])
}

func TestRequestOpenAI2ClaudeMessageNormalizesParameterlessFunctionTool(t *testing.T) {
	request := dto.GeneralOpenAIRequest{
		Model:     "claude-test",
		MaxTokens: 16,
		Messages:  []dto.Message{{Role: "user", Content: "Call the tool."}},
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        "get_current_time",
					Description: "Get the current time",
				},
			},
		},
	}

	got, err := RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)

	tools, ok := got.Tools.([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	tool, ok := tools[0].(*dto.Tool)
	require.True(t, ok)
	assert.Equal(t, "get_current_time", tool.Name)
	assert.Equal(t, "Get the current time", tool.Description)
	assert.Equal(t, map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, tool.InputSchema)
}

func TestRequestOpenAI2ClaudeMessageToolEmission(t *testing.T) {
	tests := []struct {
		name      string
		request   dto.GeneralOpenAIRequest
		wantTools int
	}{
		{
			name: "omitted tools",
			request: dto.GeneralOpenAIRequest{
				Model: "claude-test", MaxTokens: 16,
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
			},
		},
		{
			name: "explicit empty tools",
			request: dto.GeneralOpenAIRequest{
				Model: "claude-test", MaxTokens: 16,
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
				Tools:    []dto.ToolCallRequest{},
			},
		},
		{
			name: "unsupported non-function tool without schema",
			request: dto.GeneralOpenAIRequest{
				Model: "claude-test", MaxTokens: 16,
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
				Tools: []dto.ToolCallRequest{{
					Type: "custom",
					Function: dto.FunctionRequest{
						Name: "custom_tool",
					},
				}},
			},
		},
		{
			name: "parameterless function tool",
			request: dto.GeneralOpenAIRequest{
				Model: "claude-test", MaxTokens: 16,
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
				Tools: []dto.ToolCallRequest{{
					Type: "function",
					Function: dto.FunctionRequest{
						Name: "noop",
					},
				}},
			},
			wantTools: 1,
		},
		{
			name: "mixed valid and unsupported tools",
			request: dto.GeneralOpenAIRequest{
				Model: "claude-test", MaxTokens: 16,
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
				Tools: []dto.ToolCallRequest{
					{Type: "custom", Function: dto.FunctionRequest{Name: "skip_me"}},
					{Type: "function", Function: dto.FunctionRequest{Name: "keep_me"}},
				},
			},
			wantTools: 1,
		},
		{
			name: "web search only",
			request: dto.GeneralOpenAIRequest{
				Model:            "claude-test",
				MaxTokens:        16,
				Messages:         []dto.Message{{Role: "user", Content: "hello"}},
				WebSearchOptions: &dto.WebSearchOptions{SearchContextSize: "low"},
			},
			wantTools: 1,
		},
		{
			name: "legacy non-function object schema remains supported",
			request: dto.GeneralOpenAIRequest{
				Model: "claude-test", MaxTokens: 16,
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
				Tools: []dto.ToolCallRequest{{
					Type: "custom",
					Function: dto.FunctionRequest{
						Name:       "legacy_tool",
						Parameters: map[string]any{"type": "object"},
					},
				}},
			},
			wantTools: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := RequestOpenAI2ClaudeMessage(nil, test.request)
			require.NoError(t, err)

			body, err := common.Marshal(got)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(body, &payload))

			tools, present := payload["tools"]
			if test.wantTools == 0 {
				assert.Nil(t, got.Tools)
				assert.False(t, present, "serialized request must omit empty tools: %s", body)
				return
			}

			assert.NotNil(t, got.Tools)
			require.True(t, present)
			serializedTools, ok := tools.([]any)
			require.True(t, ok)
			assert.Len(t, serializedTools, test.wantTools)
		})
	}
}

func TestRequestOpenAI2ClaudeMessagePreservesWireParameterlessFunctionTool(t *testing.T) {
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.UnmarshalJsonStr(`{
		"model":"claude-test",
		"max_tokens":16,
		"messages":[{"role":"user","content":"Call noop."}],
		"tools":[{"type":"function","function":{"name":"noop"}}]
	}`, &request))

	got, err := RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)

	body, err := common.Marshal(got)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	tools, ok := payload["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "noop", tool["name"])
	assert.Equal(t, map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, tool["input_schema"])
	assert.NotContains(t, tool["input_schema"], "required")
}
