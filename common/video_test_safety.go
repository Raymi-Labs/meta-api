package common

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// IsVideoGenerationTest is deliberately more conservative than relay routing.
// Recognising a family here prevents an accidental paid test; it does not claim
// that a particular provider/model version is implemented or callable.
func IsVideoGenerationTest(channelType int, modelName string) bool {
	switch channelType {
	case constant.ChannelTypeKling, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora:
		return true
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	// Providers may qualify model IDs with an organisation prefix.
	parts := strings.Split(name, "/")
	name = parts[len(parts)-1]
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
