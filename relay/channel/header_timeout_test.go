package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	basecommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelNonStreamHeaderTimeoutScope(t *testing.T) {
	require.Zero(t, channelNonStreamHeaderTimeout(nil))
	for _, mode := range []int{constant.RelayModeChatCompletions, constant.RelayModeCompletions, constant.RelayModeResponses, constant.RelayModeVideoSubmit} {
		info := &relaycommon.RelayInfo{RelayMode: mode, ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{NonStreamResponseHeaderTimeoutSec: 120},
		}}
		require.Equal(t, 120*time.Second, channelNonStreamHeaderTimeout(info))
		info.IsStream = true
		require.Zero(t, channelNonStreamHeaderTimeout(info))
		info.IsStream = false
		info.ChannelSetting.NonStreamResponseHeaderTimeoutSec = 0
		require.Zero(t, channelNonStreamHeaderTimeout(info))
		info.ChannelSetting.NonStreamResponseHeaderTimeoutSec = 601
		require.Zero(t, channelNonStreamHeaderTimeout(info))
	}
	for _, mode := range []int{constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits, constant.RelayModeEmbeddings, constant.RelayModeAudioSpeech, constant.RelayModeResponsesCompact, constant.RelayModeVideoFetchByID} {
		info := &relaycommon.RelayInfo{RelayMode: mode, ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{NonStreamResponseHeaderTimeoutSec: 120},
		}}
		require.Zero(t, channelNonStreamHeaderTimeout(info))
	}
	imageTool := &relaycommon.RelayInfo{
		RelayMode:   constant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{NonStreamResponseHeaderTimeoutSec: 120}},
		Request:     &dto.OpenAIResponsesRequest{Tools: []byte(`[{"type":"image_generation"}]`)},
	}
	require.Zero(t, channelNonStreamHeaderTimeout(imageTool))
}

func TestChannelHeaderOverrideActualRequest(t *testing.T) {
	oldHeader, oldTotal := basecommon.RelayResponseHeaderTimeout, basecommon.RelayTimeout
	t.Cleanup(func() {
		basecommon.RelayResponseHeaderTimeout, basecommon.RelayTimeout = oldHeader, oldTotal
		service.InitHttpClient()
	})
	basecommon.RelayResponseHeaderTimeout, basecommon.RelayTimeout = 1, 0
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1200 * time.Millisecond):
			_, _ = w.Write([]byte(`{"ok":true}`))
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	for _, tc := range []struct{ mode, seconds int }{{constant.RelayModeChatCompletions, 0}, {constant.RelayModeChatCompletions, 2}, {constant.RelayModeVideoSubmit, 0}, {constant.RelayModeVideoSubmit, 2}} {
		seconds := tc.seconds
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("{}"))
		req, err := http.NewRequestWithContext(c.Request.Context(), "POST", server.URL, strings.NewReader("{}"))
		require.NoError(t, err)
		info := &relaycommon.RelayInfo{RelayMode: tc.mode, ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{NonStreamResponseHeaderTimeoutSec: seconds},
		}}
		resp, err := doRequest(c, req, info)
		if seconds == 0 {
			require.ErrorContains(t, err, "upstream response header timeout")
		} else {
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Contains(t, string(body), `"ok":true`)
		}
		diagnostics, exists := c.Get("relay_http_timeout")
		require.True(t, exists)
		require.Equal(t, seconds > 0, diagnostics.(map[string]interface{})["channel_nonstream_override"])
	}
	require.Equal(t, time.Second, service.GetResponseHeaderTimeout())
}
