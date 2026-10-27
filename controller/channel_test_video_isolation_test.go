package controller

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVideoEndpointDiscoveryPreservesExistingModelTests(t *testing.T) {
	for _, tc := range []struct {
		kind           int
		name, endpoint string
	}{
		{constant.ChannelTypeOpenAI, "gpt-4o", ""},
		{constant.ChannelTypeOpenAI, "gpt-image-1", string(constant.EndpointTypeImageGeneration)},
		{constant.ChannelTypeAnthropic, "claude-sonnet-4-5", ""},
		{constant.ChannelTypeMiniMax, "MiniMax-M2.5", ""},
		{constant.ChannelTypeJimeng, "jimeng_high_aes_general_v21_L", ""},
		{constant.ChannelTypeAli, "wan2.6-image", ""},
		{constant.ChannelTypeAli, "qwen-plus", ""},
		{constant.ChannelTypeGemini, "gemini-2.5-pro", ""},
		{constant.ChannelTypeVertexAi, "imagen-4.0-generate-001", string(constant.EndpointTypeImageGeneration)},
		{constant.ChannelTypeVolcEngine, "doubao-seedream-4-0", string(constant.EndpointTypeImageGeneration)},
		{constant.ChannelTypeMiniMax, "MiniMax-H3", string(constant.EndpointTypeOpenAIVideo)},
		{constant.ChannelTypeJimeng, "jimeng_v30_pro", string(constant.EndpointTypeOpenAIVideo)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := &model.Channel{Type: tc.kind}
			require.Equal(t, tc.endpoint, normalizeChannelTestEndpoint(ch, tc.name, ""))
			require.Equal(t, "custom-endpoint", normalizeChannelTestEndpoint(ch, tc.name, "custom-endpoint"))
		})
	}
}
