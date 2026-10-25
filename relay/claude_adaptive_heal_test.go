package relay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const adaptiveRejection = `{"error":{"type":"invalid_request_error","message":"claude-haiku-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort"}}`
const adaptiveRequest = `{"model":"claude-haiku-5-5","max_tokens":8192,"thinking":{"type":"enabled","budget_tokens":4096,"display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"}},"output_config":{"effort":"xhigh","format":{"type":"json_schema"}},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"valid history","signature":"signature"},{"type":"tool_use","id":"tool1","name":"f","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool1","content":"result"}]}],"opaque":9007199254740993}`

func TestClaudeAdaptiveThinkingHealBoundaries(t *testing.T) {
	oldPass := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	oldLog := constant.ErrorLogEnabled
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldPass
		constant.ErrorLogEnabled = oldLog
	})
	for _, tc := range []struct {
		name, request, rejection                                                 string
		disabledOrigin, globalPass, channelPass, written, canceled, networkError bool
		wrongChannel, wrongStatus                                                bool
		want                                                                     bool
	}{
		{name: "explicit_refusal", want: true},
		{name: "trailing_period", rejection: strings.ReplaceAll(adaptiveRejection, "output_config.effort\"", "output_config.effort.\""), want: true},
		{name: "other_model", rejection: strings.ReplaceAll(adaptiveRejection, "claude-haiku-5-5", "claude-opus-5-5")},
		{name: "wrong_error_type", rejection: strings.ReplaceAll(adaptiveRejection, "invalid_request_error", "api_error")},
		{name: "incidental_text", rejection: `{"error":{"type":"invalid_request_error","message":"bad request"},"detail":"claude-haiku-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort"}`},
		{name: "ambiguous_rejection", rejection: strings.ReplaceAll(adaptiveRejection, "output_config.effort\"", "output_config.effort or remove tools\"")},
		{name: "unknown_thinking_field", request: strings.Replace(adaptiveRequest, `"budget_tokens":4096`, `"budget_tokens":4096,"future_mode":true`, 1)},
		{name: "disabled", request: strings.Replace(adaptiveRequest, `"type":"enabled"`, `"type":"disabled"`, 1)},
		{name: "already_adaptive", request: strings.Replace(adaptiveRequest, `"type":"enabled"`, `"type":"adaptive"`, 1)},
		{name: "explicit_origin_disabled", disabledOrigin: true},
		{name: "unsupported_effort", request: strings.Replace(adaptiveRequest, `"effort":"xhigh"`, `"effort":"unknown"`, 1)},
		{name: "zero_budget", request: strings.Replace(adaptiveRequest, `"budget_tokens":4096`, `"budget_tokens":0`, 1)},
		{name: "fractional_budget", request: strings.Replace(adaptiveRequest, `"budget_tokens":4096`, `"budget_tokens":4096.5`, 1)},
		{name: "invalid_json", request: adaptiveRequest + "junk"},
		{name: "oversized_error", rejection: adaptiveRejection + strings.Repeat(" ", maxClaudeHealErrorBytes)},
		{name: "global_passthrough", globalPass: true},
		{name: "channel_passthrough", channelPass: true},
		{name: "already_written", written: true},
		{name: "already_canceled", canceled: true},
		{name: "wrong_channel", wrongChannel: true},
		{name: "wrong_status", wrongStatus: true},
		{name: "network_failure", networkError: true},
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
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAnthropic, ChannelId: 456, UpstreamModelName: "claude-haiku-5-5"}}
			info.ChannelSetting.PassThroughBodyEnabled = tc.channelPass
			if tc.disabledOrigin {
				info.Request = &dto.ClaudeRequest{Thinking: &dto.Thinking{Type: "disabled"}}
			}
			if tc.wrongChannel {
				info.ChannelType = constant.ChannelTypeOpenAI
			}
			model_setting.GetGlobalSettings().PassThroughRequestEnabled = tc.globalPass
			defer func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = false }()
			request := tc.request
			if request == "" {
				request = adaptiveRequest
			}
			rejection := tc.rejection
			if rejection == "" {
				rejection = adaptiveRejection
			}
			tracked := &healTrackedBody{Reader: strings.NewReader(rejection)}
			initial := &http.Response{StatusCode: 400, Body: tracked, Header: make(http.Header)}
			initial.Header.Set("X-Request-ID", "adaptive-refusal")
			if tc.wrongStatus {
				initial.StatusCode = 422
			}
			final := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}
			adaptor := &claudeThinkingHealTestAdaptor{response: final}
			if tc.networkError {
				adaptor.response = nil
				adaptor.err = errors.New("fixture connection error")
			}
			got, healed := tryHealClaudeParamError(c, info, adaptor, []byte(request), initial)
			require.Equal(t, tc.want, healed)
			if tc.want {
				require.Same(t, final, got)
				require.True(t, tracked.closed)
				require.Len(t, adaptor.requests, 1)
				expected := strings.Replace(request, `"type":"enabled"`, `"type":"adaptive"`, 1)
				expected = strings.Replace(expected, `"budget_tokens":4096,`, "", 1)
				require.JSONEq(t, expected, adaptor.requests[0])
				require.Contains(t, adaptor.requests[0], "9007199254740993")
				require.Equal(t, "xhigh", info.ReasoningEffort)
				require.Equal(t, claudeAdaptiveThinkingAdjustment, c.Writer.Header().Get("X-Request-Adjusted"))
				value, _ := c.Get("request_failure_attempts")
				attempts := value.([]map[string]interface{})
				require.Len(t, attempts, 1)
				require.Equal(t, claudeAdaptiveThinkingAdjustment, attempts[0]["adjustment"])
				require.Equal(t, "adaptive-refusal", attempts[0]["upstream"].(types.UpstreamDiagnostics).RequestID)
			} else {
				require.Same(t, initial, got)
				require.False(t, tracked.closed)
				restored, err := io.ReadAll(got.Body)
				require.NoError(t, err)
				require.Equal(t, rejection, string(restored))
				if !tc.networkError {
					require.Empty(t, adaptor.requests)
				}
			}
			if tc.want || tc.networkError {
				next := &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"temperature is not supported"}}`))}
				_, again := tryHealClaudeParamError(c, info, adaptor, []byte(`{"temperature":0.2}`), next)
				require.False(t, again)
				require.Len(t, adaptor.requests, 1, "adaptive and sampling share one adjustment budget")
			}
		})
	}
}

func TestClaudeAdaptiveThinkingEffortPreservesIntent(t *testing.T) {
	oldPass := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldPass })
	request := `{"model":"claude-haiku-5-5","thinking":{"type":"enabled","budget_tokens":1280},"max_tokens":4096,"messages":[]}`
	message := claudeHealErrorMessage([]byte(adaptiveRejection))
	for _, tc := range []struct {
		name, source, want string
		override           bool
		disabled           bool
	}{
		{name: "native_budget_keeps_default"},
		{name: "chat_low", source: "low", want: "low"},
		{name: "chat_high", source: "high", want: "high"},
		{name: "channel_override_prevents_inference", source: "low", override: true, disabled: true},
		{name: "unknown_chat_effort", source: "future", disabled: true},
		{name: "chat_explicit_off", source: "none", disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-haiku-5-5"}}
			if tc.source != "" {
				info.Request = &dto.GeneralOpenAIRequest{ReasoningEffort: tc.source}
			}
			if tc.override {
				info.ParamOverride = map[string]interface{}{"thinking.budget_tokens": 1280}
			}
			got, effort, ok := adaptRejectedClaudeThinking(info, []byte(request), message)
			require.Equal(t, !tc.disabled, ok)
			if tc.disabled {
				return
			}
			require.Equal(t, tc.want, effort)
			require.Equal(t, tc.want, gjson.GetBytes(got, "output_config.effort").String())
			require.Equal(t, tc.want != "", gjson.GetBytes(got, "output_config.effort").Exists())
		})
	}
	explicit := strings.Replace(request, `"max_tokens":4096`, `"output_config":{"effort":"max"},"max_tokens":4096`, 1)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-haiku-5-5"}, Request: &dto.GeneralOpenAIRequest{ReasoningEffort: "low"}}
	_, effort, ok := adaptRejectedClaudeThinking(info, []byte(explicit), message)
	require.True(t, ok)
	require.Equal(t, "max", effort)
	info.Request = &dto.GeneralOpenAIRequest{THINKING: []byte(`{"type":"disabled"}`)}
	_, _, ok = adaptRejectedClaudeThinking(info, []byte(request), message)
	require.False(t, ok)
}

func TestClaudeAdaptiveThinkingAllowsUnknownModelOnlyWithExactRefusal(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "supplier/private-alias"}}
	request := strings.ReplaceAll(adaptiveRequest, "claude-haiku-5-5", "supplier/private-alias")
	message := strings.ReplaceAll(claudeHealErrorMessage([]byte(adaptiveRejection)), "claude-haiku-5-5", "supplier/private-alias")
	adjusted, _, ok := adaptRejectedClaudeThinking(info, []byte(request), message)
	require.True(t, ok)
	require.Equal(t, "supplier/private-alias", gjson.GetBytes(adjusted, "model").String())
	_, _, ok = adaptRejectedClaudeThinking(info, []byte(request), "model requires adaptive thinking")
	require.False(t, ok)
	// Actual error codes may be absent or unknown; the structured error type is the boundary.
	body, err := common.Marshal(map[string]any{"error": map[string]any{"type": "invalid_request_error", "code": "unknown_error", "message": message}})
	require.NoError(t, err)
	require.Equal(t, message, claudeHealErrorMessage(body))
}
