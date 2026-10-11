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
