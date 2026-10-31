package common

import "slices"

var AliTTSModels = []string{
	"qwen-audio-3.1-tts-flash", "qwen-audio-3.1-tts-plus", "qwen-audio-3.0-tts-flash", "qwen-audio-3.0-tts-plus",
	"cosyvoice-v3.5-flash", "cosyvoice-v3.5-plus", "cosyvoice-v3-flash", "cosyvoice-v3-plus", "cosyvoice-v2",
}

func IsAliTTSModel(model string) bool { return slices.Contains(AliTTSModels, model) }
