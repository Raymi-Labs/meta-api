package common

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

var (
	// OpenAIResponseOnlyModels is a list of models that are only available for OpenAI responses.
	OpenAIResponseOnlyModels = []string{
		"o3-pro",
		"o3-deep-research",
		"o4-mini-deep-research",
	}
	ImageGenerationModels = []string{
		"dall-e-3",
		"dall-e-2",
		"gpt-image-1",
		"gpt-image-2",
		"grok-imagine-image",
		"prefix:imagen-",
		"flux-",
		"flux.1-",
	}
	VideoGenerationModels = []string{
		"grok-imagine-video",
		"sora-2",
	}
	OpenAITextModels = []string{
		"gpt-",
		"o1",
		"o3",
		"o4",
		"chatgpt",
	}
)

func IsOpenAIResponseOnlyModel(modelName string) bool {
	for _, m := range OpenAIResponseOnlyModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

func IsImageGenerationModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range ImageGenerationModels {
		if strings.Contains(modelName, m) {
			return true
		}
		if strings.HasPrefix(m, "prefix:") && strings.HasPrefix(modelName, strings.TrimPrefix(m, "prefix:")) {
			return true
		}
	}
	return false
}

func IsOpenAITextModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAITextModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

// IsVideoGenerationModel reports whether a model uses the asynchronous video
// generation API rather than a text or image endpoint.
func IsVideoGenerationModel(modelName string) bool {
	if isH3EndpointModel(modelName) {
		return true
	}
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	for _, model := range VideoGenerationModels {
		if modelName == model || strings.HasPrefix(modelName, model+"-") {
			return true
		}
	}
	return false
}

// IsVideoGenerationModelForChannel narrows automatic video endpoint
// selection to models that are actually supported by the channel. This keeps
// a model name such as sora-2 on a normal OpenAI-compatible channel from
// unexpectedly changing that channel's existing text path.
func IsVideoGenerationModelForChannel(channelType int, modelName string) bool {
	if channelType == constant.ChannelTypeMiniMax && isH3EndpointModel(modelName) {
		return true
	}
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch channelType {
	case constant.ChannelTypeKling, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora:
		return true
	case constant.ChannelTypeMiniMax:
		return strings.HasPrefix(modelName, "minimax-hailuo-") || strings.HasPrefix(modelName, "t2v-01") || strings.HasPrefix(modelName, "i2v-01") || modelName == "s2v-01"
	case constant.ChannelTypeAli:
		if !strings.HasPrefix(modelName, "wan2.") && !strings.HasPrefix(modelName, "wanx2.") {
			return false
		}
		// The Wan family also contains image models. Match video task types,
		// not the shared vendor/version prefix.
		for _, part := range strings.Split(modelName, "-") {
			switch part {
			case "t2v", "i2v", "r2v", "v2v", "s2v", "flf2v":
				return true
			}
		}
		return false
	case constant.ChannelTypeGemini, constant.ChannelTypeVertexAi:
		return strings.HasPrefix(modelName, "veo-") || strings.HasPrefix(modelName, "veo3")
	case constant.ChannelTypeVolcEngine:
		return strings.HasPrefix(modelName, "doubao-seedance-") || strings.HasPrefix(modelName, "seedance-")
	case constant.ChannelTypeJimeng:
		return modelName == "jimeng_vgfm_t2v_l20" ||
			strings.HasPrefix(modelName, "jimeng_v30") ||
			strings.HasPrefix(modelName, "jimeng_t2v_") ||
			strings.HasPrefix(modelName, "jimeng_i2v_") ||
			strings.HasPrefix(modelName, "jimeng_ti2v_")
	case constant.ChannelTypeXai:
		return modelName == "grok-imagine-video" || strings.HasPrefix(modelName, "grok-imagine-video-")
	case constant.ChannelTypeOpenAI:
		return modelName == "sora-2" || strings.HasPrefix(modelName, "sora-2-")
	default:
		// A nil/unknown channel is used while building the token-model
		// capability map. Preserve the model-only fallback there; concrete
		// channel routing remains narrowed by the cases above.
		return IsVideoGenerationModel(modelName)
	}
}

// Endpoint discovery lowercases model names; request validation still requires
// canonical supported names after channel mapping.
func isH3EndpointModel(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "minimax-h3", "minimax-h3-highspeed", "minimax-h3-lite", "minimax-h3-max":
		return true
	default:
		return false
	}
}
