package service

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
)

// ObserveGroupHealthCacheUsage captures cache and total upstream token counts
// before settlement. The existing estimated flag also covers output fallbacks.
// Request.Observe only collects this snapshot if the group's final attempt succeeds.
func ObserveGroupHealthCacheUsage(info *relaycommon.RelayInfo, usage *dto.Usage) {
	if info == nil {
		return
	}
	info.GroupHealthCacheUsage = nil
	info.GroupHealthTokenUsage = nil
	info.GroupHealthUsageObservedAt = time.Time{}
	if usage == nil || usage.InputTokensEstimated || usage.InterruptedOutput {
		return
	}
	// These endpoints may use image counts, characters or audio duration as input
	// tokens. Do not mix those billing units with text context cache measurements.
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits,
		relayconstant.RelayModeAudioSpeech, relayconstant.RelayModeAudioTranscription,
		relayconstant.RelayModeAudioTranslation, relayconstant.RelayModeRealtime:
		return
	}
	input := int64(usage.PromptTokens)
	read := int64(usage.PromptTokensDetails.CachedTokens)
	if read == 0 {
		read = int64(usage.PromptCacheHitTokens)
	}
	if input == 0 && usage.InputTokens > 0 {
		input = int64(usage.InputTokens)
		if usage.InputTokensDetails != nil {
			read = int64(usage.InputTokensDetails.CachedTokens)
		}
	}
	if input < 0 || read < 0 {
		return
	}
	if IsClaudeUsageSemantic(info, usage) {
		creation := int64(usage.PromptTokensDetails.CachedCreationTokens)
		if creation == 0 {
			five, hour := int64(usage.ClaudeCacheCreation5mTokens), int64(usage.ClaudeCacheCreation1hTokens)
			if five < 0 || hour < 0 || five > math.MaxInt64-hour {
				return
			}
			creation = five + hour
		}
		if creation < 0 || input > math.MaxInt64-read || input+read > math.MaxInt64-creation {
			return
		}
		// Anthropic input excludes cache reads and writes. Cache creation is
		// part of the denominator, never a cache hit, and splits are not doubled.
		input += read + creation
	}
	if input > 0 && read <= input {
		info.GroupHealthCacheUsage = &relaycommon.GroupHealthCacheUsage{ReadTokens: read, InputTokens: input}
		output := int64(usage.CompletionTokens)
		if output == 0 && usage.OutputTokens > 0 {
			output = int64(usage.OutputTokens)
		}
		if output >= 0 && input <= math.MaxInt64-output {
			info.GroupHealthTokenUsage = &relaycommon.GroupHealthTokenUsage{InputTokens: input, OutputTokens: output}
			info.GroupHealthUsageObservedAt = time.Now()
		}
	}
}
