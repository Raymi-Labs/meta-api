package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveOpenAIOutputIgnoresMetadata(t *testing.T) {
	for _, data := range []string{
		`{"choices":[{"delta":{"role":"assistant","content":""}}]}`,
		`{"choices":[],"usage":{"completion_tokens":2}}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function"}]}}]}`,
		`: ping`, `{`,
	} {
		info := &RelayInfo{}
		info.ObserveOpenAIStreamOutput(data)
		require.True(t, info.GroupHealthFirstOutputTime.IsZero(), data)
	}
	for _, data := range []string{
		`{"choices":[{"delta":{"content":"hello"}}]}`,
		`{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"search"}}]}}]}`,
		`{"choices":[{"delta":{"function_call":{"arguments":"{}"}}}]}`,
		`{"choices":[{"text":"completion"}]}`,
	} {
		info := &RelayInfo{}
		info.ObserveOpenAIStreamOutput(data)
		require.False(t, info.GroupHealthFirstOutputTime.IsZero(), data)
		require.True(t, info.FirstEffectiveOutputTime.IsZero(), "health collection must not expand routing-guard sampling")
		first := info.GroupHealthFirstOutputTime
		info.ObserveOpenAIStreamOutput(data)
		require.Equal(t, first, info.GroupHealthFirstOutputTime)
	}
}
