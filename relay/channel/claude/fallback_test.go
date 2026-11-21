package claude

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOfficialFallbackIterationEvidence(t *testing.T) {
	const sticky = `{"type":"fallback_message","model":"claude-opus-4-8","input_tokens":0,"output_tokens":0,"cache_read_input_tokens":10}`
	attempts, ok := parseFallbackAttempts([]byte("[" + sticky + "]"))
	require.True(t, ok)
	require.Len(t, attempts, 1)
	require.Equal(t, 10, attempts[0].Usage.PromptTokensDetails.CachedTokens)
	for _, invalid := range []string{"null", "[]", "[" + sticky + "," + sticky + "]", `[{"type":"fallback_message","model":"other","input_tokens":1,"output_tokens":2}]`, `[{"type":"fallback_message","model":"claude-opus-4-8","input_tokens":1}]`} {
		_, ok = parseFallbackAttempts([]byte(invalid))
		require.False(t, ok, invalid)
	}
	info := &relaycommon.RelayInfo{OriginModelName: relaycommon.ClaudeFallbackPrimary, ClaudeFallback: &relaycommon.ClaudeFallback{ServingModel: relaycommon.ClaudeFallbackPrimary}}
	var response dto.ClaudeResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"type":"message","id":"msg_fixture","model":"claude-opus-4-8","stop_reason":"refusal","stop_details":{"category":"bio"},"usage":{"iterations":[`+sticky+`]}}`, &response))
	observeOfficialFallback(info, &response)
	require.Equal(t, relaycommon.ClaudeFallbackTarget, info.ClaudeFallback.ServingModel)
	require.Equal(t, "bio", info.ClaudeRefusalCategory)
	require.Equal(t, "msg_fixture", info.ClaudeMessageID)
	require.Equal(t, "refusal", info.ClaudeFallback.StopReason)
}
