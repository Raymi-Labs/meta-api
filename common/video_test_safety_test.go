package common

import (
	"github.com/QuantumNous/new-api/constant"
	"testing"
)

func TestVideoTestSafetyDetection(t *testing.T) {
	for _, name := range []string{"MiniMax-H3", "org/MiniMax-H3-Max", "MiniMax-H3-Highspeed", "MiniMax-H3-Lite"} {
		if !IsVideoGenerationTest(1, name) {
			t.Errorf("missed H3 video model %s", name)
		}
	}
	for _, name := range []string{"MiniMax-H3", "MiniMax-H3-Max", "MiniMax-H3-Highspeed", "MiniMax-H3-Lite"} {
		endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeMiniMax, name)
		if len(endpoints) == 0 || endpoints[0] != constant.EndpointTypeOpenAIVideo {
			t.Errorf("H3 must advertise video endpoint: %s", name)
		}
	}
	if IsVideoGenerationModel("MiniMax-M2.5") {
		t.Fatal("M2.5 is not a video model")
	}
	for _, name := range []string{"wan2.6-t2v", "wanx2.1-i2v", "veo3.1", "org/veo-3.1", "kling-v2.6", "MiniMax-Hailuo-02", "grok-imagine-video-1.5", "happyhorse-1.1-t2v", "doubao-seedance-2.0", "sora-2"} {
		if !IsVideoGenerationTest(1, name) {
			t.Errorf("missed video model %s", name)
		}
	}
	for _, name := range []string{"gpt-4o", "kimi-k3", "qwen-vl", "MiniMax-M2.5", "seedream-4.0"} {
		if IsVideoGenerationTest(1, name) {
			t.Errorf("misclassified text/image model %s", name)
		}
	}
}

func TestVideoEndpointHintsRespectMixedChannelModels(t *testing.T) {
	for _, tc := range []struct {
		kind        int
		video, text string
	}{
		{constant.ChannelTypeAli, "wan2.5-t2v-preview", "qwen-plus"},
		{constant.ChannelTypeJimeng, "jimeng_vgfm_t2v_l20", "jimeng_high_aes_general_v21_L"},
		{constant.ChannelTypeJimeng, "jimeng_v30_pro", "jimeng_t2i_v40"},
		{constant.ChannelTypeGemini, "veo-3.0-generate-001", "gemini-2.5-pro"},
		{constant.ChannelTypeVolcEngine, "doubao-seedance-1-0-pro", "doubao-text"},
		{constant.ChannelTypeMiniMax, "MiniMax-Hailuo-2.3", "MiniMax-M2.5"},
	} {
		if !IsVideoGenerationModelForChannel(tc.kind, tc.video) {
			t.Errorf("missing video endpoint %s", tc.video)
		}
		if IsVideoGenerationModelForChannel(tc.kind, tc.text) {
			t.Errorf("text misclassified %s", tc.text)
		}
	}
}

func TestVideoChangesPreserveTextAndImageEndpointHints(t *testing.T) {
	for _, tc := range []struct {
		kind int
		name string
	}{
		{constant.ChannelTypeOpenAI, "gpt-4o"},
		{constant.ChannelTypeOpenAI, "gpt-image-1"},
		{constant.ChannelTypeAnthropic, "claude-sonnet-4-5"},
		{constant.ChannelTypeMiniMax, "MiniMax-M2.5"},
		{constant.ChannelTypeAli, "qwen-plus"},
		{constant.ChannelTypeAli, "wan2.6-image"},
		{constant.ChannelTypeAli, "wanx2.1-t2i-turbo"},
		{constant.ChannelTypeJimeng, "jimeng_high_aes_general_v21_L"},
		{constant.ChannelTypeGemini, "gemini-2.5-flash-image"},
		{constant.ChannelTypeVertexAi, "imagen-4.0-generate-001"},
		{constant.ChannelTypeVolcEngine, "doubao-seedream-4-0"},
		{constant.ChannelTypeXai, "grok-imagine-image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if IsVideoGenerationModelForChannel(tc.kind, tc.name) {
				t.Errorf("non-video model misclassified: %s", tc.name)
			}
			for _, endpoint := range GetEndpointTypesByChannelType(tc.kind, tc.name) {
				if endpoint == constant.EndpointTypeOpenAIVideo {
					t.Errorf("non-video model advertises video endpoint: %s", tc.name)
				}
			}
		})
	}
}

func TestH3EndpointHintsSurviveLowercaseNormalization(t *testing.T) {
	for _, name := range []string{"MiniMax-H3", "minimax-h3", "minimax-h3-max"} {
		if !IsVideoGenerationModelForChannel(constant.ChannelTypeMiniMax, name) {
			t.Errorf("missing normalized H3 hint: %s", name)
		}
	}
}
