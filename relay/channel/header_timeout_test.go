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

func TestChannelStreamHeaderTimeoutScope(t *testing.T) {
	require.Zero(t, channelResponseHeaderTimeout(nil))
	require.Zero(t, channelResponseHeaderTimeout(&relaycommon.RelayInfo{IsStream: true}))
	for _, mode := range []int{constant.RelayModeChatCompletions, constant.RelayModeCompletions, constant.RelayModeResponses, constant.RelayModeGemini} {
		info := &relaycommon.RelayInfo{IsStream: true, RelayMode: mode, ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{NonStreamResponseHeaderTimeoutSec: 600},
		}}
		// Existing nonstream configuration must never change streaming behavior.
		require.Zero(t, channelResponseHeaderTimeout(info))
		info.ChannelSetting.StreamResponseHeaderTimeoutSec = 180
		require.Equal(t, 180*time.Second, channelResponseHeaderTimeout(info))
		info.IsStream = false
		if mode == constant.RelayModeGemini {
			require.Zero(t, channelResponseHeaderTimeout(info))
		} else {
			require.Equal(t, 600*time.Second, channelResponseHeaderTimeout(info))
		}
		info.IsStream = true
		for _, invalid := range []int{-1, 601} {
			info.ChannelSetting.StreamResponseHeaderTimeoutSec = invalid
			require.Zero(t, channelResponseHeaderTimeout(info))
		}
	}
	for _, mode := range []int{constant.RelayModeUnknown, constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits, constant.RelayModeEmbeddings, constant.RelayModeAudioSpeech, constant.RelayModeRealtime, constant.RelayModeVideoSubmit, constant.RelayModeVideoFetchByID, constant.RelayModeResponsesCompact} {
		info := &relaycommon.RelayInfo{IsStream: true, RelayMode: mode, ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{StreamResponseHeaderTimeoutSec: 180},
		}}
		require.Zero(t, channelResponseHeaderTimeout(info))
	}
	imageTool := &relaycommon.RelayInfo{IsStream: true, RelayMode: constant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{StreamResponseHeaderTimeoutSec: 180}},
		Request:     &dto.OpenAIResponsesRequest{Tools: []byte(`[{"type":"image_generation"}]`)},
	}
	require.Zero(t, channelResponseHeaderTimeout(imageTool))
}

func TestGeminiStreamHeaderOverrideIsolatedRequests(t *testing.T) {
	oldHeader, oldTotal := basecommon.RelayResponseHeaderTimeout, basecommon.RelayTimeout
	t.Cleanup(func() {
		basecommon.RelayResponseHeaderTimeout, basecommon.RelayTimeout = oldHeader, oldTotal
		service.InitHttpClient()
	})
	basecommon.RelayResponseHeaderTimeout, basecommon.RelayTimeout = 1, 0
	service.InitHttpClient()
	base := service.GetHttpClient()
	base.Transport.(*http.Transport).ResponseHeaderTimeout = 50 * time.Millisecond
	t.Cleanup(base.CloseIdleConnections)
	const terminal = "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"complete\"}]},\"finishReason\":\"STOP\"}]}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(150 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// The body remains readable beyond the base response-header deadline.
		select {
		case <-time.After(100 * time.Millisecond):
			_, _ = w.Write([]byte(terminal))
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	type result struct {
		override    int
		body        string
		err         error
		diagnostics map[string]interface{}
	}
	results := make(chan result, 2)
	for _, seconds := range []int{0, 1} {
		go func(seconds int) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-test:streamGenerateContent?alt=sse", strings.NewReader("{}"))
			req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, server.URL, strings.NewReader("{}"))
			if err != nil {
				results <- result{override: seconds, err: err}
				return
			}
			info := &relaycommon.RelayInfo{IsStream: true, RelayMode: constant.RelayModeGemini, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelSetting: dto.ChannelSettings{StreamResponseHeaderTimeoutSec: seconds},
			}}
			resp, err := doRequest(c, req, info)
			var body []byte
			if err == nil {
				body, err = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}
			diagnostic, _ := c.Get("relay_http_timeout")
			results <- result{override: seconds, body: string(body), err: err, diagnostics: diagnostic.(map[string]interface{})}
		}(seconds)
	}
	for range 2 {
		r := <-results
		require.Equal(t, false, r.diagnostics["channel_nonstream_override"])
		require.Equal(t, r.override > 0, r.diagnostics["channel_stream_override"])
		if r.override == 0 {
			require.ErrorContains(t, r.err, "upstream response header timeout")
		} else {
			require.NoError(t, r.err)
			require.Equal(t, terminal, r.body)
			require.Equal(t, float64(1), r.diagnostics["response_header_timeout_seconds"])
		}
	}
	require.Equal(t, 50*time.Millisecond, service.GetResponseHeaderTimeout())
}
