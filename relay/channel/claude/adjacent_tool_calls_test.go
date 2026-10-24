package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestRequestOpenAI2ClaudeMessagePreservesAdjacentToolCalls(t *testing.T) {
	assistant := func(content any, id string) dto.Message {
		message := dto.Message{Role: "assistant", Content: content}
		if id != "" {
			message.SetToolCalls([]dto.ToolCallRequest{{ID: id, Type: "function", Function: dto.FunctionRequest{Name: "lookup_" + id, Arguments: `{"id":"` + id + `"}`}}})
		}
		return message
	}
	for _, tc := range []struct {
		name      string
		messages  []dto.Message
		wantText  string
		wantCalls []string
	}{
		{"two calls", []dto.Message{assistant("", "a"), assistant("", "b")}, "...", []string{"a", "b"}},
		{"three nil or empty calls", []dto.Message{assistant(nil, "a"), assistant(nil, "b"), assistant("", "c")}, "...", []string{"a", "b", "c"}},
		{"call followed by text", []dto.Message{assistant(nil, "a"), assistant("Checking.", "")}, "Checking.", []string{"a"}},
		{"literal ellipsis with call", []dto.Message{assistant("...", "a"), assistant("Checking.", "b")}, "... Checking.", []string{"a", "b"}},
		{"literal ellipsis replaces placeholder", []dto.Message{assistant(nil, "a"), assistant("...", ""), assistant("Checking.", "")}, "... Checking.", []string{"a"}},
		{"literal ellipsis before call", []dto.Message{assistant("...", ""), assistant("", "a"), assistant("Checking.", "")}, "... Checking.", []string{"a"}},
		{"plain text", []dto.Message{assistant("First.", ""), assistant("Second.", "")}, "First. Second.", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := append([]dto.Message{{Role: "user", Content: "Look up."}}, tc.messages...)
			for _, id := range tc.wantCalls {
				messages = append(messages, dto.Message{Role: "tool", ToolCallId: id, Content: "result " + id})
			}
			got, err := RequestOpenAI2ClaudeMessage(nil, dto.GeneralOpenAIRequest{Model: "claude-test", MaxTokens: 1024, Messages: messages})
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(got.Messages), 2)
			require.Equal(t, "assistant", got.Messages[1].Role)
			var text string
			var calls []string
			switch content := got.Messages[1].Content.(type) {
			case string:
				text = content
			case []dto.ClaudeMediaMessage:
				for _, part := range content {
					switch part.Type {
					case "text":
						text += part.GetText()
					case "tool_use":
						calls = append(calls, part.Id)
						require.Equal(t, "lookup_"+part.Id, part.Name)
						require.Equal(t, map[string]any{"id": part.Id}, part.Input)
					}
				}
			default:
				t.Fatalf("unexpected content %T", content)
			}
			require.Equal(t, tc.wantText, text)
			require.Equal(t, tc.wantCalls, calls)
			if len(tc.wantCalls) > 0 {
				require.Len(t, got.Messages, 3)
				results, ok := got.Messages[2].Content.([]dto.ClaudeMediaMessage)
				require.True(t, ok)
				require.Len(t, results, len(tc.wantCalls))
				for i, result := range results {
					require.Equal(t, "tool_result", result.Type)
					require.Equal(t, tc.wantCalls[i], result.ToolUseId)
				}
			} else {
				require.Len(t, got.Messages, 2)
			}
		})
	}
}
