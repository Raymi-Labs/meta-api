package vidu

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestViduMetadataCannotOverrideBilledFields(t *testing.T) {
	for _, tc := range []struct {
		name, model, action, size, wantModel, wantSize string
		duration, wantDuration                         int
	}{
		{name: "mapped", model: "vidu1.5", size: "360p", duration: 4, wantModel: "vidu1.5", wantSize: "360p", wantDuration: 4},
		{name: "defaults", wantModel: "viduq1", wantSize: "1080p", wantDuration: 5},
		{name: "reference", model: "viduq2-pro", action: constant.TaskActionReferenceGenerate, size: "720p", duration: 8, wantModel: "viduq2", wantSize: "720p", wantDuration: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", relaycommon.TaskSubmitReq{
				Prompt: "test", Duration: tc.duration, Size: tc.size,
				Metadata: map[string]any{"model": "viduq2-turbo", "duration": 16, "resolution": "4k", "seed": 7, "bgm": true, "callback_url": "https://callback.example/task"},
			})
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}, TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: tc.action}}
			body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
			require.NoError(t, err)
			var got requestPayload
			require.NoError(t, common.DecodeJson(body, &got))
			require.Equal(t, tc.wantModel, got.Model)
			require.Equal(t, tc.wantDuration, got.Duration)
			require.Equal(t, tc.wantSize, got.Resolution)
			require.Equal(t, 7, got.Seed)
			require.True(t, got.Bgm)
			require.Equal(t, "https://callback.example/task", got.CallbackUrl)
			require.Equal(t, tc.model, info.UpstreamModelName)
		})
	}
}
