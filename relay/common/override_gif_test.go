package common

import (
	jsoncommon "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"os"
	"testing"
)

func TestGIFPresetPreservesStaticPolicy(t *testing.T) {
	raw, err := os.ReadFile("testdata/gpt26_gif_preset.json")
	require.NoError(t, err)
	var override map[string]interface{}
	require.NoError(t, jsoncommon.Unmarshal(raw, &override))
	for _, input := range []string{
		`{"model":"gpt-image-2.6-sunburst","prompt":"cat"}`,
		`{"model":"gpt-image-2.6-flare","prompt":"cat","output_format":"jpeg","size":"2048x2048"}`,
		`{"model":"gpt-image-2.6-sunburst","prompt":"cat","output_format":"gif"}`,
		`{"model":"gpt-image-2.6-sunburst","prompt":"cat","output_format":"gif","frame_count":99,"frame_duration_ms":800,"n":4}`,
	} {
		out, err := ApplyParamOverride([]byte(input), override, nil)
		require.NoError(t, err)
		require.Equal(t, "cat", gjson.GetBytes(out, "prompt").String())
		require.Equal(t, int64(1), gjson.GetBytes(out, "n").Int())
		if gjson.Get(input, "output_format").String() == "gif" {
			require.Equal(t, "gif", gjson.GetBytes(out, "output_format").String())
			require.Equal(t, "512x512", gjson.GetBytes(out, "size").String())
			require.Equal(t, "medium", gjson.GetBytes(out, "quality").String())
			require.Equal(t, int64(9), gjson.GetBytes(out, "frame_count").Int())
			require.Equal(t, int64(200), gjson.GetBytes(out, "frame_duration_ms").Int())
		} else {
			require.Equal(t, "png", gjson.GetBytes(out, "output_format").String())
			require.Equal(t, "1024x1024", gjson.GetBytes(out, "size").String())
			require.Equal(t, "high", gjson.GetBytes(out, "quality").String())
			require.False(t, gjson.GetBytes(out, "frame_count").Exists())
		}
	}
}
