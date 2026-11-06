package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestImageGIFFrameFieldsRoundTrip(t *testing.T) {
	for _, raw := range []string{
		`{"model":"gpt-image-2.6-sunburst","prompt":"cat","output_format":"gif","frame_count":9,"frame_duration_ms":200,"unknown_private_field":42}`,
		`{"model":"gpt-image-2.6-flare","prompt":"cat","output_format":"gif","frame_count":2,"frame_duration_ms":50,"images":["data:image/png;base64,AA=="]}`,
	} {
		var req ImageRequest
		require.NoError(t, common.Unmarshal([]byte(raw), &req))
		encoded, err := common.Marshal(req)
		require.NoError(t, err)
		for _, field := range []string{"output_format", "frame_count", "frame_duration_ms", "images"} {
			require.Equal(t, gjson.Get(raw, field).Raw, gjson.GetBytes(encoded, field).Raw, field)
		}
		require.False(t, gjson.GetBytes(encoded, "unknown_private_field").Exists())
	}
}

func TestImageGIFFrameOmissionAndInvalidTypes(t *testing.T) {
	var req ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"gpt-image-2","prompt":"cat"}`), &req))
	b, err := common.Marshal(req)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(b, "frame_count").Exists())
	require.False(t, gjson.GetBytes(b, "frame_duration_ms").Exists())
	for _, raw := range []string{`{"frame_count":-1}`, `{"frame_count":2.5}`, `{"frame_count":"9"}`, `{"frame_duration_ms":true}`, `{"frame_duration_ms":-20}`} {
		var invalid ImageRequest
		require.Error(t, common.Unmarshal([]byte(raw), &invalid), raw)
	}
}
