package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestUpdateUsageFromStreamDataRetainsUsageBeforeTrailingMetadata(t *testing.T) {
	usage := &dto.Usage{}
	containStreamUsage := false

	usageChunk := `{"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":4584,"completion_tokens":1,"total_tokens":4585,"prompt_cache_hit_tokens":4480,"prompt_cache_miss_tokens":104,"prompt_tokens_details":{"cached_tokens":4480}}}`
	metadataChunk := `{"choices":[],"x-opencode-type":"inference-cost","normalizedUsage":{"inputTokens":4584,"outputTokens":1,"cacheReadTokens":4480}}`

	updateUsageFromStreamData(usageChunk, &usage, &containStreamUsage)
	updateUsageFromStreamData(metadataChunk, &usage, &containStreamUsage)

	require.True(t, containStreamUsage)
	require.Equal(t, 4584, usage.PromptTokens)
	require.Equal(t, 4480, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 4480, usage.PromptCacheHitTokens)
}

func TestUpdateUsageFromStreamDataNormalizesResponsesStyleUsage(t *testing.T) {
	usage := &dto.Usage{}
	containStreamUsage := false

	chunk := `{"choices":[],"usage":{"input_tokens":120,"output_tokens":8,"total_tokens":128,"input_tokens_details":{"cached_tokens":96}}}`
	updateUsageFromStreamData(chunk, &usage, &containStreamUsage)

	require.True(t, containStreamUsage)
	require.Equal(t, 120, usage.PromptTokens)
	require.Equal(t, 8, usage.CompletionTokens)
	require.Equal(t, 96, usage.PromptTokensDetails.CachedTokens)
}

func TestUpdateUsageFromStreamDataRetainsMiMoSearchOnlyUsage(t *testing.T) {
	usage := &dto.Usage{}
	containStreamUsage := false

	chunk := `{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"web_search_usage":{"tool_usage":3,"page_usage":3}}}`
	updateUsageFromStreamData(chunk, &usage, &containStreamUsage)

	require.True(t, containStreamUsage)
	require.Equal(t, 3, usage.WebSearchRequests)
	require.Nil(t, usage.WebSearchUsage)
}

func TestCacheWriteStreamSnapshotsAreNotAccumulated(t *testing.T) {
	usage := &dto.Usage{}
	present := false
	chunk := `{"choices":[],"usage":{"prompt_tokens":6022,"completion_tokens":5,"total_tokens":6027,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":6019}}}`
	updateUsageFromStreamData(chunk, &usage, &present)
	updateUsageFromStreamData(chunk, &usage, &present)
	updateUsageFromStreamData(`{"choices":[],"metadata":"done"}`, &usage, &present)
	require.True(t, present)
	require.Equal(t, 6022, usage.PromptTokens)
	require.Equal(t, 6019, usage.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 6019, *usage.PromptTokensDetails.CacheWriteTokens)

	updateUsageFromStreamData(`{"choices":[],"usage":{"input_tokens":20,"output_tokens":1,"total_tokens":21,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":17}}}`, &usage, &present)
	require.Equal(t, 17, usage.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
}
