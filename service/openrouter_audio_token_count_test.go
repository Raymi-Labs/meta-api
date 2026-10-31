package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenRouterJSONTranscriptionTokenEstimate(t *testing.T) {
	old := constant.CountToken
	constant.CountToken = true
	t.Cleanup(func() { constant.CountToken = old })
	for _, tc := range []struct {
		name, contentType string
		channel, mode     int
		fail              bool
	}{
		{"native JSON with alias", "application/json; charset=utf-8", constant.ChannelTypeOpenRouter, relayconstant.RelayModeAudioTranscription, false},
		{"other provider", "application/json", constant.ChannelTypeOpenAI, relayconstant.RelayModeAudioTranscription, true},
		{"translation unchanged", "application/json", constant.ChannelTypeOpenRouter, relayconstant.RelayModeAudioTranslation, true},
		{"multipart still parsed", "multipart/form-data", constant.ChannelTypeOpenRouter, relayconstant.RelayModeAudioTranscription, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/audio/transcriptions", strings.NewReader(`{"model":"custom-stt-alias","input_audio":{"data":"UklGRg==","format":"wav"}}`))
			c.Request.Header.Set("Content-Type", tc.contentType)
			common.SetContextKey(c, constant.ContextKeyChannelType, tc.channel)
			info := &relaycommon.RelayInfo{OriginModelName: "custom-stt-alias", RelayMode: tc.mode}
			n, err := EstimateRequestToken(c, &types.TokenCountMeta{}, info)
			require.Equal(t, tc.fail, err != nil)
			if !tc.fail {
				require.Zero(t, n, "unknown audio tokens must not be estimated as multipart duration")
				var body map[string]interface{}
				require.NoError(t, common.UnmarshalBodyReusable(c, &body))
				require.NotNil(t, body["input_audio"], "adaptor must still receive the original JSON")
			}
		})
	}
}
