package claude

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatToolHistoryPreservesSerializedNumbers(t *testing.T) {
	const args = `{"id":9007199254740993,"nested":[9223372036854775807,0.1234567890123456789,1.234567890123456789e+30]}`
	message := dto.Message{Role: "assistant", Content: ""}
	message.SetToolCalls([]dto.ToolCallRequest{{ID: "call_1", Type: "function", Function: dto.FunctionRequest{Name: "lookup", Arguments: args}}})
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := RequestOpenAI2ClaudeMessage(ctx, dto.GeneralOpenAIRequest{Model: "claude-test", MaxTokens: 128, Messages: []dto.Message{{Role: "user", Content: "lookup"}, message}})
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	input := gjson.GetBytes(encoded, `messages.1.content.#(type=="tool_use").input`)
	for _, path := range []string{"id", "nested.0", "nested.1", "nested.2"} {
		require.Equal(t, gjson.Get(args, path).Raw, input.Get(path).Raw, path)
	}
}
