package sora

import (
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVideoExplicitParametersReachWire(t *testing.T) {
	for _, model := range []string{"muse-video", "flow-video-gemini-omni-flash-1.1-1080p"} {
		t.Run(model, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"`+model+`","prompt":"scene","seconds":"181","resolution":"1080p","aspect_ratio":"21:9"}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			a := &TaskAdaptor{}
			require.Nil(t, a.ValidateRequestAndSetAction(ctx, info))
			reader, err := a.BuildRequestBody(ctx, info)
			require.NoError(t, err)
			raw, err := io.ReadAll(reader)
			require.NoError(t, err)
			var body map[string]interface{}
			require.NoError(t, common.Unmarshal(raw, &body))
			require.Equal(t, "2520x1080", body["size"])
			require.Equal(t, "1080p", body["resolution"])
			require.Equal(t, "181", body["seconds"])
		})
	}
}

func TestVideoDimensionsDoNotInventOrContradictExplicitSpecs(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
		bad       bool
	}{
		{`{"resolution":"1080p","aspect_ratio":"16:9"}`, "1920x1080", false},
		{`{"resolution":"1080p","aspect_ratio":"9:16"}`, "1080x1920", false},
		{`{"aspect_ratio":"16:9"}`, "1280x720", false},
		{`{"resolution":"cinema","aspect_ratio":"21:9"}`, "", false},
		{`{"resolution":"1080p"}`, "", false},
		{`{}`, "720x1280", false},
		{`{"size":"1920x1080","resolution":"720p"}`, "", true},
		{`{"size":"1920x1080","aspect_ratio":"9:16"}`, "", true},
		{`{"aspect_ratio":"0:1"}`, "", true},
		{`{"resolution":"1080p","aspect_ratio":"9:7"}`, "", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var body map[string]interface{}
			require.NoError(t, common.Unmarshal([]byte(tc.raw), &body))
			err := normalizeVideoDimensions(body, "720x1280")
			if tc.bad {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.want == "" {
				require.NotContains(t, body, "size")
			} else {
				require.Equal(t, tc.want, body["size"])
			}
		})
	}
}

func TestMuseExplicitParametersAreNotCapabilityGated(t *testing.T) {
	refs := []interface{}{"https://example.test/1.png", "https://example.test/2.png", "https://example.test/3.png", "https://example.test/4.png", "https://example.test/5.png"}
	body := map[string]interface{}{"prompt": strings.Repeat("x", 4001), "seconds": float64(181), "resolution": "1080p", "aspect_ratio": "1:1", "images": refs}
	require.NoError(t, normalizeMuseRequest(body))
	require.Equal(t, "1080x1080", body["size"])
	require.Len(t, body["images"], 5)
}
