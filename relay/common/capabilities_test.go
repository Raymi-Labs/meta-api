package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

func TestResolveChannelCapabilitiesOpenAIOfficialHost(t *testing.T) {
	caps := ResolveChannelCapabilities(constant.ChannelTypeOpenAI, "https://api.openai.com/v1", dto.ChannelOtherSettings{})

	if !caps.SupportsChatStreamOptions || !caps.SupportsResponsesAPI || !caps.SupportsResponsesStreamOptions {
		t.Fatalf("expected official openai host to support advanced capabilities, got %+v", caps)
	}
	if !caps.NativeTextFormats.Supports(types.RelayFormatOpenAI) || !caps.NativeTextFormats.Supports(types.RelayFormatOpenAIResponses) {
		t.Fatalf("expected official openai host to expose native text protocols, got %+v", caps)
	}
}

func TestResolveChannelCapabilitiesOpenAICompatibleHostDefaultsConservative(t *testing.T) {
	caps := ResolveChannelCapabilities(constant.ChannelTypeOpenAI, "https://nan.meta-api.vip/v1", dto.ChannelOtherSettings{})

	if caps.SupportsChatStreamOptions || caps.SupportsResponsesAPI || caps.SupportsResponsesStreamOptions {
		t.Fatalf("expected unknown openai-compatible host to default conservative, got %+v", caps)
	}
	if !caps.NativeTextFormats.Supports(types.RelayFormatOpenAI) {
		t.Fatalf("expected openai-compatible channel to keep its native chat protocol, got %+v", caps)
	}
}

func TestResolveChannelCapabilitiesXAINativeTextProtocols(t *testing.T) {
	caps := ResolveChannelCapabilities(constant.ChannelTypeXai, "https://api.x.ai", dto.ChannelOtherSettings{})

	if !caps.NativeTextFormats.Supports(types.RelayFormatOpenAI) || !caps.NativeTextFormats.Supports(types.RelayFormatOpenAIResponses) {
		t.Fatalf("expected xAI to expose chat and responses protocols, got %+v", caps)
	}
	if caps.NativeTextFormats.Supports(types.RelayFormatClaude) {
		t.Fatalf("expected Claude Messages to require conversion for xAI, got %+v", caps)
	}
}

func TestResolveChannelCapabilitiesMoonshotNativeTextProtocols(t *testing.T) {
	caps := ResolveChannelCapabilities(constant.ChannelTypeMoonshot, "https://api.kimi.com/coding", dto.ChannelOtherSettings{})

	if !caps.SupportsChatStreamOptions {
		t.Fatalf("expected Moonshot to support chat stream options, got %+v", caps)
	}
	if caps.SupportsResponsesAPI || caps.SupportsResponsesStreamOptions {
		t.Fatalf("expected Moonshot Responses support to remain disabled until implemented, got %+v", caps)
	}
	if !caps.NativeTextFormats.Supports(types.RelayFormatOpenAI) || !caps.NativeTextFormats.Supports(types.RelayFormatClaude) {
		t.Fatalf("expected Moonshot to expose native OpenAI and Claude protocols, got %+v", caps)
	}
}

func TestResolveChannelCapabilitiesHonorsOverrides(t *testing.T) {
	caps := ResolveChannelCapabilities(constant.ChannelTypeOpenAI, "https://nan.meta-api.vip/v1", dto.ChannelOtherSettings{
		ChatStreamOptionsMode:      dto.CapabilityModeEnabled,
		ResponsesAPIMode:           dto.CapabilityModeEnabled,
		ResponsesStreamOptionsMode: dto.CapabilityModeEnabled,
	})

	if !caps.SupportsChatStreamOptions || !caps.SupportsResponsesAPI || !caps.SupportsResponsesStreamOptions {
		t.Fatalf("expected overrides to force-enable capabilities, got %+v", caps)
	}
}

func TestResolveChannelCapabilitiesDeepSeekNativeResponses(t *testing.T) {
	caps := ResolveChannelCapabilities(constant.ChannelTypeDeepSeek, "https://api.deepseek.com", dto.ChannelOtherSettings{})
	if !caps.SupportsChatStreamOptions || !caps.SupportsResponsesAPI || caps.SupportsResponsesStreamOptions {
		t.Fatalf("incorrect DeepSeek stream/endpoint capabilities: %+v", caps)
	}
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatOpenAIResponses} {
		if !caps.NativeTextFormats.Supports(format) {
			t.Fatalf("missing native DeepSeek format %s", format)
		}
	}
}

func TestResolveChannelCapabilitiesResponsesStreamRequiresResponsesAPI(t *testing.T) {
	caps := ResolveChannelCapabilities(constant.ChannelTypeAzure, "https://example.openai.azure.com", dto.ChannelOtherSettings{
		ResponsesAPIMode:           dto.CapabilityModeDisabled,
		ResponsesStreamOptionsMode: dto.CapabilityModeEnabled,
	})

	if caps.SupportsResponsesAPI {
		t.Fatalf("expected responses api to be disabled, got %+v", caps)
	}
	if caps.SupportsResponsesStreamOptions {
		t.Fatalf("expected responses stream options to be disabled with responses api, got %+v", caps)
	}
}
