package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClaudeCacheDefaultRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		enabled bool
	}{
		{`{}`, true}, {`{"claude_incremental_cache_enabled":null}`, true},
		{`{"claude_incremental_cache_enabled":true}`, true}, {`{"claude_incremental_cache_enabled":false}`, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var settings ChannelSettings
			require.NoError(t, common.UnmarshalJsonStr(tc.raw, &settings))
			require.Equal(t, tc.enabled, settings.IsClaudeIncrementalCacheEnabled())
			raw, err := common.Marshal(settings)
			require.NoError(t, err)
			var saved ChannelSettings
			require.NoError(t, common.Unmarshal(raw, &saved))
			require.Equal(t, tc.enabled, saved.IsClaudeIncrementalCacheEnabled())
			if !tc.enabled {
				require.Contains(t, string(raw), `"claude_incremental_cache_enabled":false`)
			}
		})
	}
}
