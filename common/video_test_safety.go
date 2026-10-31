package common

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// IsMiniMaxH3Model lists the exact V2 models supported by our text-to-video path.
// Safety detection below is intentionally broader than this capability check.
func IsMiniMaxH3Model(name string) bool {
	switch name {
	case "MiniMax-H3", "MiniMax-H3-Highspeed", "MiniMax-H3-Lite", "MiniMax-H3-Max":
		return true
	}
	return false
}

func IsVideoTaskChannel(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeAli, constant.ChannelTypeKling, constant.ChannelTypeJimeng, constant.ChannelTypeVertexAi, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine, constant.ChannelTypeSora, constant.ChannelTypeOpenAI, constant.ChannelTypeGemini, constant.ChannelTypeXai, constant.ChannelTypeMiniMax:
		return true
	}
	return false
}

// IsVideoGenerationTest is deliberately more conservative than relay routing.
// Recognising a family here prevents an accidental paid test; it does not claim
// that a particular provider/model version is implemented or callable.
func IsVideoGenerationTest(channelType int, modelName string) bool {
	switch channelType {
	case constant.ChannelTypeKling, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora:
		return true
	}
	if IsWanImageModel(modelName) {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	// Providers may qualify model IDs with an organisation prefix.
	parts := strings.Split(name, "/")
	name = parts[len(parts)-1]
	if name == "minimax-h3" || strings.HasPrefix(name, "minimax-h3-") || strings.HasPrefix(name, "minimax-h3.") {
		return true
	}
	for _, prefix := range []string{"wan", "wanx", "veo"} {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) && name[len(prefix)] >= '0' && name[len(prefix)] <= '9' {
			return true
		}
	}
	for _, prefix := range []string{"grok-imagine-video", "sora", "veo", "wan", "wanx", "happyhorse", "kling", "hailuo", "minimax-hailuo", "minimax-video", "seedance", "doubao-seedance", "vidu", "cogvideo", "cogvideox", "jimeng-video", "luma", "ray-2", "ray-3", "runway"} {
		if name == prefix || strings.HasPrefix(name, prefix+"-") || strings.HasPrefix(name, prefix+".") {
			return true
		}
	}
	return false
}
