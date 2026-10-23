package relay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const missingDeepSeekThinkingErrorJSON = "{\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The `content[].thinking` in the thinking mode must be passed back to the API. (request_id: fixture-123)\"}}"

type deepSeekThinkingHealTestAdaptor struct {
	claudeThinkingHealTestAdaptor
	url string
}

func (a *deepSeekThinkingHealTestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}

func TestDeepSeekThinkingHealBoundaries(t *testing.T) {
	oldPass := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldPass })
	for _, tc := range []struct {
		name, body, request, url, format, finalFormat, responseURL string
		status, channelType                                        int
		written, canceled, twice, transportError, pass, globalPass bool
		want                                                       bool
	}{
		{name: "explicit_refusal", want: true, twice: true},
		{name: "default_thinking", request: `{"model":"deepseek-v4-pro","messages":[],"output_config":{"effort":"high"}}`, want: true},
		{name: "already_disabled", request: `{"model":"deepseek-v4-pro","thinking":{"type":"disabled"}}`},
		{name: "wrong_type", body: strings.ReplaceAll(missingDeepSeekThinkingErrorJSON, "invalid_request_error", "authentication_error")},
		{name: "incidental_text", body: `{"error":{"type":"invalid_request_error","message":"bad request"},"detail":` + missingDeepSeekThinkingErrorJSON + `}`},
		{name: "additional_error", body: strings.ReplaceAll(missingDeepSeekThinkingErrorJSON, "The `content", "Other error. The `content")},
		{name: "invalid_json", body: missingDeepSeekThinkingErrorJSON + "junk"},
		{name: "oversized", body: missingDeepSeekThinkingErrorJSON + strings.Repeat(" ", maxClaudeHealErrorBytes)},
		{name: "invalid_request_json", request: "not JSON"},
		{name: "non_object", request: "[]"},
		{name: "missing_replay_body", request: "nil"},
		{name: "channel_passthrough", pass: true},
		{name: "global_passthrough", globalPass: true},
		{name: "wrong_status", status: 422},
		{name: "other_provider", channelType: constant.ChannelTypeAnthropic},
		{name: "third_party", url: "https://proxy.example/anthropic/v1/messages"},
		{name: "spoofed_host", url: "https://api.deepseek.com.example/anthropic/v1/messages"},
		{name: "wrong_scheme", url: "http://api.deepseek.com/anthropic/v1/messages"},
		{name: "wrong_path", url: "https://api.deepseek.com/v1/chat/completions"},
		{name: "redirected_elsewhere", responseURL: "https://proxy.example/anthropic/v1/messages"},
		{name: "chat_incoming", format: string(types.RelayFormatOpenAI)},
		{name: "chat_outgoing", finalFormat: string(types.RelayFormatOpenAI)},
		{name: "output_started", written: true},
		{name: "canceled", canceled: true},
		{name: "transport_error", transportError: true, twice: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			if tc.canceled {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			}
			if tc.written {
				c.Writer.WriteHeaderNow()
			}
			if tc.status == 0 {
				tc.status = 400
			}
			if tc.channelType == 0 {
				tc.channelType = constant.ChannelTypeDeepSeek
			}
			if tc.body == "" {
				tc.body = missingDeepSeekThinkingErrorJSON
			}
			if tc.url == "" {
				tc.url = "https://api.deepseek.com/anthropic/v1/messages"
			}
			if tc.format == "" {
				tc.format = string(types.RelayFormatClaude)
			}
			body := &healTrackedBody{Reader: strings.NewReader(tc.body)}
			original := &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}
			original.Header.Set("X-Request-ID", "thinking-rejection")
			if tc.responseURL != "" {
				original.Request = httptest.NewRequest(http.MethodPost, tc.responseURL, nil)
			}
			final := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}
			adaptor := &deepSeekThinkingHealTestAdaptor{claudeThinkingHealTestAdaptor: claudeThinkingHealTestAdaptor{response: final}, url: tc.url}
			if tc.transportError {
				adaptor.err = fmt.Errorf("fixture network error")
				adaptor.response = nil
			}
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormat(tc.format), FinalRequestRelayFormat: types.RelayFormat(tc.finalFormat), ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channelType, ChannelId: 456, UpstreamModelName: "deepseek-v4-pro"}}
			info.ChannelSetting.PassThroughBodyEnabled = tc.pass
			model_setting.GetGlobalSettings().PassThroughRequestEnabled = tc.globalPass
			t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = false })
			request := []byte(tc.request)
			if tc.request == "" {
				request = []byte(`{"model":"deepseek-v4-pro","thinking":{"type":"enabled","budget_tokens":4000,"block_binding":"preserve"},"output_config":{"effort":"high","opaque":true},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"preserved","signature":"keep"},{"type":"tool_use","id":"call_1","name":"echo","input":{"value":9007199254740993}}]}],"opaque":9007199254740993}`)
			}
			if tc.request == "nil" {
				request = nil
			}
			oldLog := constant.ErrorLogEnabled
			constant.ErrorLogEnabled = true
			t.Cleanup(func() { constant.ErrorLogEnabled = oldLog })
			got, healed := tryHealDeepSeekThinkingError(c, info, adaptor, request, original)
			require.Equal(t, tc.want, healed)
			if tc.want {
				require.Same(t, final, got)
				require.True(t, body.closed)
				require.Len(t, adaptor.requests, 1)
				if tc.name == "default_thinking" {
					require.JSONEq(t, `{"model":"deepseek-v4-pro","messages":[],"output_config":{"effort":"high"},"thinking":{"type":"disabled"}}`, adaptor.requests[0])
				} else {
					require.Equal(t, strings.Replace(string(request), `"type":"enabled"`, `"type":"disabled"`, 1), adaptor.requests[0])
				}
				require.Equal(t, deepSeekThinkingAdjustment, c.Writer.Header().Get("X-Request-Adjusted"))
				require.Equal(t, "none", info.ReasoningEffort)
				value, _ := c.Get("request_failure_attempts")
				attempts := value.([]map[string]interface{})
				require.Len(t, attempts, 1)
				require.Equal(t, deepSeekThinkingAdjustment, attempts[0]["adjustment"])
				require.Equal(t, "thinking-rejection", attempts[0]["upstream"].(types.UpstreamDiagnostics).RequestID)
			} else {
				require.Same(t, original, got)
				require.False(t, body.closed)
				restored, err := io.ReadAll(got.Body)
				require.NoError(t, err)
				require.Equal(t, tc.body, string(restored))
				require.NoError(t, got.Body.Close())
				require.True(t, body.closed)
				if !tc.transportError {
					require.Empty(t, adaptor.requests)
				}
			}
			if tc.twice {
				next := &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(missingDeepSeekThinkingErrorJSON))}
				_, healed = tryHealDeepSeekThinkingError(c, info, adaptor, request, next)
				require.False(t, healed)
				require.Len(t, adaptor.requests, 1)
			}
		})
	}
}
