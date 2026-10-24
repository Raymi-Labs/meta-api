package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestProviderToOpenAIToResponsesPreservesSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","maximum":9007199254740993}},"required":["reasoning","answer"]}`
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5"}}
	for _, provider := range []string{"claude", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			var converted *dto.GeneralOpenAIRequest
			var err error
			if provider == "claude" {
				var request dto.ClaudeRequest
				require.NoError(t, common.Unmarshal([]byte(`{"model":"claude-test","messages":[{"role":"user","content":"answer"}],"tools":[{"name":"answer","input_schema":`+schema+`}]}`), &request))
				converted, err = ClaudeToOpenAIRequest(nil, request, info)
			} else {
				var request dto.GeminiChatRequest
				require.NoError(t, common.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"text":"answer"}]}],"tools":[{"functionDeclarations":[{"name":"answer","parameters":`+schema+`}]}]}`), &request))
				converted, err = GeminiToOpenAIRequest(&request, info)
			}
			require.NoError(t, err)
			encoded, err := common.Marshal(converted)
			require.NoError(t, err)
			require.Equal(t, schema, gjson.GetBytes(encoded, "tools.0.function.parameters").Raw)
			responses, err := openaicompat.ChatCompletionsRequestToResponsesRequest(converted)
			require.NoError(t, err)
			require.Equal(t, schema, gjson.GetBytes(responses.Tools, "0.parameters").Raw)
		})
	}
}
