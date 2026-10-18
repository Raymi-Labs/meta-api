package common

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

type ChannelCapabilities struct {
	SupportsChatStreamOptions      bool
	SupportsResponsesAPI           bool
	SupportsResponsesStreamOptions bool
	NativeTextFormats              TextProtocolSet
}

var defaultChannelCapabilities = map[int]ChannelCapabilities{
	constant.ChannelTypeOpenAI: {
		NativeTextFormats: NewTextProtocolSet(types.RelayFormatOpenAI),
	},
	constant.ChannelTypeAnthropic: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeAws: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeGemini: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelCloudflare: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeAzure: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeVolcEngine: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeOllama: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeXai: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
		NativeTextFormats: NewTextProtocolSet(
			types.RelayFormatOpenAI,
			types.RelayFormatOpenAIResponses,
		),
	},
	constant.ChannelTypeDeepSeek: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: false,
		NativeTextFormats: NewTextProtocolSet(
			types.RelayFormatOpenAI,
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		),
	},
	constant.ChannelTypeBaiduV2: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeZhipu_v4: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeAli: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeSubmodel: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeCodex: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeMoonshot: {
		SupportsChatStreamOptions: true,
		NativeTextFormats: NewTextProtocolSet(
			types.RelayFormatOpenAI,
			types.RelayFormatClaude,
		),
	},
	constant.ChannelTypeMiniMax: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
	constant.ChannelTypeSiliconFlow: {
		SupportsChatStreamOptions:      true,
		SupportsResponsesAPI:           true,
		SupportsResponsesStreamOptions: true,
	},
}

func ResolveChannelCapabilities(channelType int, baseURL string, settings dto.ChannelOtherSettings) ChannelCapabilities {
	caps := resolveDefaultChannelCapabilities(channelType, baseURL)

	caps.SupportsChatStreamOptions = applyCapabilityMode(
		caps.SupportsChatStreamOptions,
		settings.GetChatStreamOptionsMode(),
	)
	caps.SupportsResponsesAPI = applyCapabilityMode(
		caps.SupportsResponsesAPI,
		settings.GetResponsesAPIMode(),
	)
	caps.SupportsResponsesStreamOptions = applyCapabilityMode(
		caps.SupportsResponsesStreamOptions,
		settings.GetResponsesStreamOptionsMode(),
	)

	if !caps.SupportsResponsesAPI {
		caps.SupportsResponsesStreamOptions = false
	}

	return caps
}

func resolveDefaultChannelCapabilities(channelType int, baseURL string) ChannelCapabilities {
	if channelType == constant.ChannelTypeOpenAI && isOfficialOpenAIHost(baseURL) {
		return ChannelCapabilities{
			SupportsChatStreamOptions:      true,
			SupportsResponsesAPI:           true,
			SupportsResponsesStreamOptions: true,
			NativeTextFormats: NewTextProtocolSet(
				types.RelayFormatOpenAI,
				types.RelayFormatOpenAIResponses,
			),
		}
	}

	if caps, ok := defaultChannelCapabilities[channelType]; ok {
		return caps
	}

	return ChannelCapabilities{}
}

func applyCapabilityMode(defaultValue bool, mode dto.CapabilityMode) bool {
	switch dto.NormalizeCapabilityMode(mode) {
	case dto.CapabilityModeEnabled:
		return true
	case dto.CapabilityModeDisabled:
		return false
	default:
		return defaultValue
	}
}

func isOfficialOpenAIHost(baseURL string) bool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return true
	}

	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}

	return host == "openai.com" || host == "api.openai.com" || strings.HasSuffix(host, ".openai.com")
}
