package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCacheWriteTokensPresenceAndWireEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		present       bool
		want          int
	}{
		{"missing", `{}`, false, 0}, {"null", `{"cache_write_tokens":null}`, false, 0},
		{"zero", `{"cache_write_tokens":0}`, true, 0}, {"actual upstream", `{"cached_tokens":0,"cache_write_tokens":6019}`, true, 6019},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var d InputTokenDetails
			require.NoError(t, common.Unmarshal([]byte(tc.payload), &d))
			require.Equal(t, tc.present, d.CacheWriteTokens != nil)
			require.Equal(t, tc.want, d.CachedCreationTokens)
			clone := d.Clone()
			u := Usage{PromptTokensDetails: d}
			wire := NewOpenAIChatCompletionsUsage(&u)
			if tc.present {
				*clone.CacheWriteTokens = 9
				*wire.PromptTokensDetails.CacheWriteTokens = 8
				require.Equal(t, tc.want, *d.CacheWriteTokens)
			}
		})
	}
	for _, payload := range []string{`{"cache_write_tokens":-1}`, `{"cache_write_tokens":1.5}`, `{"cache_write_tokens":"5"}`, `{"cache_write_tokens":9223372036854775808}`} {
		var d InputTokenDetails
		require.Error(t, common.Unmarshal([]byte(payload), &d))
	}
	internal := Usage{PromptTokensDetails: InputTokenDetails{CachedCreationTokens: 100}}
	require.Nil(t, NewOpenAIChatCompletionsUsage(&internal).PromptTokensDetails.CacheWriteTokens)
}

func TestCacheWriteTokensDistinctRequestAggregation(t *testing.T) {
	var first, second InputTokenDetails
	require.NoError(t, common.Unmarshal([]byte(`{"cached_tokens":80,"cache_write_tokens":20}`), &first))
	require.NoError(t, common.Unmarshal([]byte(`{"cached_tokens":30,"cache_write_tokens":70}`), &second))
	original := first.Clone()
	first.Add(second)
	require.Equal(t, 110, first.CachedTokens)
	require.Equal(t, 90, first.CachedCreationTokens)
	require.Equal(t, 90, *first.CacheWriteTokens)
	require.Equal(t, 20, *original.CacheWriteTokens)
	*second.CacheWriteTokens = 999
	require.Equal(t, 90, *first.CacheWriteTokens)
	first.Add(InputTokenDetails{})
	require.Equal(t, 90, *first.CacheWriteTokens)
}
