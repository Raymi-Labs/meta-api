package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeToolConversionsPreserveSerializedNumbers(t *testing.T) {
	const args = `{"id":9007199254740993,"nested":{"values":[9223372036854775807,0.1234567890123456789,1.234567890123456789e+30]},"type":"shutdown_response","approve":true,"reason":"done"}`
	for _, tool := range []string{"Read", "SendMessage"} {
		for _, source := range []string{"chat", "responses", "sanitizer"} {
			t.Run(tool+"/"+source, func(t *testing.T) {
				var encoded []byte
				var err error
				if source == "sanitizer" {
					encoded = []byte(SanitizeClaudeToolArguments(tool, args))
				} else {
					var response *dto.ClaudeResponse
					if source == "chat" {
						message := dto.Message{}
						message.SetToolCalls([]dto.ToolCallRequest{{ID: "call_1", Type: "function", Function: dto.FunctionRequest{Name: tool, Arguments: args}}})
						response = ResponseOpenAI2Claude(&dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: message, FinishReason: "tool_calls"}}}, &relaycommon.RelayInfo{})
					} else {
						response = ResponseOpenAIResponses2Claude(&dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "function_call", CallId: "call_1", Name: tool, Arguments: dto.ResponsesArguments(args)}}}, "msg_1")
					}
					require.Len(t, response.Content, 1)
					encoded, err = common.Marshal(response.Content[0].Input)
					require.NoError(t, err)
				}
				for _, path := range []string{"id", "nested.values.0", "nested.values.1", "nested.values.2"} {
					require.Equal(t, gjson.Get(args, path).Raw, gjson.GetBytes(encoded, path).Raw, path)
				}
				require.Equal(t, tool != "SendMessage", gjson.GetBytes(encoded, "reason").Exists())
				require.True(t, gjson.GetBytes(encoded, "approve").Bool())
			})
		}
	}
}

func TestClaudeToolSanitizerRejectsTrailingJSON(t *testing.T) {
	for _, raw := range []string{`{"type":"shutdown_response","approve":true,"reason":"done"} {}`, `{"id":`, `null`, `[]`} {
		require.Equal(t, raw, SanitizeClaudeToolArguments("SendMessage", raw))
	}
}
