package openai

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatCapabilitiesPreserveLocalCompatibility(t *testing.T) {
	for _, tc := range []struct {
		model, effort                   string
		completion, sampling, developer bool
	}{
		{"gpt-6-astra", "", true, false, true},
		{"gpt-6-astra-high", "", true, false, true},
		{"gpt-6-astra-2026-09-03", "", true, false, true},
		{"gpt-6-sol", "", true, true, true},
		{"gpt-6-sol", "high", true, false, true},
		{"gpt-6-sol-high", "", true, false, true},
		{"gpt-6-luna", "none", true, true, true},
		{"gpt-6-luna", "medium", true, false, true},
		{"gpt-6-luna-2026-09-30", "", true, true, true},
		{"gpt-6.1-sol", "", true, false, true},
		{"gpt-6.1-sol", "none", true, false, true},
		{"gpt-6.1-sol-high", "", true, false, true},
		{"gpt-6.1-sol-2026-10-28", "", true, false, true},
		{"gpt-6.1-terra", "", true, false, true},
		{"gpt-6.1-terra", "none", true, true, true},
		{"gpt-6.1-terra", "medium", true, false, true},
		{"gpt-6.1-terra-none", "high", true, true, true},
		{"gpt-6.1-terra-2026-10-28", "none", true, true, true},
		{"gpt-6.1-sol-custom", "", false, true, false},
		{"gpt-6.1-terra-pro", "", false, true, false},
		{"gpt-6.1-terra-2026-02-30", "", false, true, false},
		{"gpt-6-sol-custom", "", false, true, false},
		{"gpt-6-luna-2026-02-30", "", false, true, false},
		{"gpt-5.2", "none", true, true, true},
		{"gpt-5.4", "high", true, false, true},
		{"gpt-5.2-high", "", true, false, true},
		{"gpt-5.2-pro", "none", true, false, true},
		{"gpt-5", "", true, false, true},
		{"gpt-7", "", false, true, false},
		{"gpt-6-astra-custom", "", false, true, false},
		{"gpt-50", "", false, true, false},
		{"openrouter-model", "", false, true, false},
		{"gpt-4.1", "", false, true, false},
	} {
		t.Run(tc.model+tc.effort, func(t *testing.T) {
			for _, channel := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeAzure} {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channel, UpstreamModelName: tc.model}}
				req := &dto.GeneralOpenAIRequest{Model: tc.model, ReasoningEffort: tc.effort,
					MaxTokens: 100, Temperature: common.GetPointer(0.7), TopP: 0.8, LogProbs: true, TopLogProbs: 3,
					Messages: []dto.Message{{Role: "system", Content: "instructions"}},
				}
				_, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, req)
				require.NoError(t, err)
				require.Equal(t, tc.completion, req.MaxCompletionTokens == 100)
				require.Equal(t, tc.completion, req.MaxTokens == 0)
				require.Equal(t, tc.sampling, req.Temperature != nil)
				require.Equal(t, tc.sampling, req.TopP == 0.8)
				require.Equal(t, tc.sampling, req.LogProbs)
				require.Equal(t, tc.sampling, req.TopLogProbs == 3)
				require.Equal(t, tc.developer, req.Messages[0].Role == "developer")
			}
		})
	}
}

func TestGPT61CapabilitiesUseMappedModelAndPreserveExplicitBudget(t *testing.T) {
	for _, tc := range []struct {
		requested, upstream string
		compatible          bool
	}{
		{"customer-alias", "gpt-6.1-sol", true},
		{"customer-alias", "gpt-6.1-terra", true},
		{"gpt-6.1-sol", "custom-model", false},
	} {
		t.Run(tc.requested+"/"+tc.upstream, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{OriginModelName: tc.requested, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: tc.upstream}}
			var req dto.GeneralOpenAIRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"`+tc.upstream+`","messages":[{"role":"system","content":"rules"},{"role":"user","content":"hi"}],"max_tokens":100,"max_completion_tokens":7,"temperature":0.7,"top_p":0.8,"logprobs":true,"top_logprobs":3}`), &req))
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, &req)
			require.NoError(t, err)
			wire, err := common.Marshal(converted)
			require.NoError(t, err)
			var actual map[string]any
			require.NoError(t, common.Unmarshal(wire, &actual))
			require.Equal(t, float64(7), actual["max_completion_tokens"])
			require.Equal(t, tc.upstream, actual["model"])
			if tc.compatible {
				for _, key := range []string{"max_tokens", "temperature", "top_p", "logprobs", "top_logprobs"} {
					require.NotContains(t, actual, key)
				}
				require.Equal(t, "developer", req.Messages[0].Role)
			} else {
				require.Equal(t, float64(100), actual["max_tokens"])
				require.Equal(t, "system", req.Messages[0].Role)
				require.Equal(t, 0.7, actual["temperature"])
			}
			require.Equal(t, "hi", req.Messages[1].Content)
		})
	}
}
