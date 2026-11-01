package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesCacheWriteAggregationPreservesWireAndBilling(t *testing.T) {
	setResponsesStreamTestTimeout(t)
	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-6-luna"}}
	body := `data: {"type":"response.output_text.delta","delta":"OK"}` + "\n\n" +
		`data: {"type":"response.completed","response":{"id":"resp_cache_write","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":6022,"output_tokens":5,"total_tokens":6027,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":6019}}}}` + "\n\n"
	u, err := OaiResponsesAggregateHandler(c, info, newResponsesStreamHTTPResponse(body))
	require.Nil(t, err)
	require.Equal(t, 6019, u.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, int64(6019), gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cache_write_tokens").Int())
	var next dto.Usage
	require.NoError(t, common.Unmarshal([]byte(`{"prompt_tokens":100,"completion_tokens":2,"total_tokens":102,"prompt_tokens_details":{"cache_write_tokens":80}}`), &next))
	mergeResponsesStreamUsage(u, &next)
	require.Equal(t, 6099, u.PromptTokensDetails.CachedCreationTokens)
	public := normalizeResponsesUsage(u)
	require.Equal(t, 6099, *public.InputTokensDetails.CacheWriteTokens)
	*public.InputTokensDetails.CacheWriteTokens = 1
	require.Equal(t, 6099, *u.PromptTokensDetails.CacheWriteTokens)
}
