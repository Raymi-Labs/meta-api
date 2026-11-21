package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiReplyKeepsPartBytes(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	parts := []dto.GeminiPart{
		{Text: "{\"name\":\""}, {Text: "思", Thought: true}, {Text: "模"}, {Text: "型\""}, {Text: "\n"},
		{Text: "考", Thought: true}, {Text: "}"}, {ExecutableCode: &dto.GeminiPartExecutableCode{Language: "go", Code: "x"}},
	}
	response := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Parts: parts}}}}
	full := responseGeminiChat2OpenAI(c, response).Choices[0].Message
	want := "{\"name\":\"模型\"\n}\n" + "```go\nx\n```"
	require.Equal(t, want, full.StringContent())
	require.Equal(t, "思考", full.ReasoningContent)
	chunk, _ := streamResponseGeminiChat2OpenAI(response)
	require.Equal(t, want+"\n", chunk.Choices[0].Delta.GetContentString())
	require.Equal(t, "思考", chunk.Choices[0].Delta.GetReasoningContent())
}

func TestGemini4ThinkingOnWire(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, tc := range []struct{ model, effort, want string }{
		{"gemini-4-carbon-preview", "minimal", "low"},
		{"gemini-4-argon-cyber-permissive", "max", "high"},
		{"gemini-4-carbon", "xhigh", "high"},
		{"gemini-4-hydrogen-preview", "max", "xhigh"},
		{"gemini-4-neon-v2", "xhigh", "xhigh"},
		{"gemini-4-argon", "medium", "medium"},
	} {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}}
		req := dto.GeneralOpenAIRequest{Model: tc.model, ReasoningEffort: tc.effort, Messages: []dto.Message{{Role: "user", Content: "fixture"}}}
		converted, err := CovertOpenAI2Gemini(c, req, info)
		require.NoError(t, err)
		require.NotNil(t, converted.GenerationConfig.ThinkingConfig)
		require.Equal(t, tc.want, converted.GenerationConfig.ThinkingConfig.ThinkingLevel)
		native := &dto.GeminiChatRequest{}
		native.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{ThinkingLevel: tc.effort}
		_, err = (&Adaptor{}).ConvertGeminiRequest(c, info, native)
		require.NoError(t, err)
		require.Equal(t, tc.want, native.GenerationConfig.ThinkingConfig.ThinkingLevel)
	}
}
