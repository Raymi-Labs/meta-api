package relay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type claudeThinkingHealTestAdaptor struct {
	requests []string
	response *http.Response
	err      error
}

var _ channel.Adaptor = (*claudeThinkingHealTestAdaptor)(nil)

func (a *claudeThinkingHealTestAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *claudeThinkingHealTestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return "", nil
}

func (a *claudeThinkingHealTestAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}

func (a *claudeThinkingHealTestAdaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, fmt.Errorf("unexpected ConvertOpenAIRequest call")
}

func (a *claudeThinkingHealTestAdaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, fmt.Errorf("unexpected ConvertRerankRequest call")
}

func (a *claudeThinkingHealTestAdaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, fmt.Errorf("unexpected ConvertEmbeddingRequest call")
}

func (a *claudeThinkingHealTestAdaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, fmt.Errorf("unexpected ConvertAudioRequest call")
}

func (a *claudeThinkingHealTestAdaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, fmt.Errorf("unexpected ConvertImageRequest call")
}

func (a *claudeThinkingHealTestAdaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, fmt.Errorf("unexpected ConvertOpenAIResponsesRequest call")
}

func (a *claudeThinkingHealTestAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	a.requests = append(a.requests, string(body))
	return a.response, a.err
}

func (a *claudeThinkingHealTestAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, types.NewOpenAIError(fmt.Errorf("unexpected DoResponse call"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
}

func (a *claudeThinkingHealTestAdaptor) GetModelList() []string { return nil }

func (a *claudeThinkingHealTestAdaptor) GetChannelName() string { return "test" }

func (a *claudeThinkingHealTestAdaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, fmt.Errorf("unexpected ConvertClaudeRequest call")
}

func (a *claudeThinkingHealTestAdaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, fmt.Errorf("unexpected ConvertGeminiRequest call")
}

func TestExtractHealableParam(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "anthropic temperature deprecated",
			body: `{"type":"error","error":{"type":"invalid_request_error","message":"` + "`temperature`" + ` is deprecated for this model."}}`,
			want: "temperature",
		},
		{
			name: "top_k not supported",
			body: `{"type":"error","error":{"type":"invalid_request_error","message":"` + "`top_k`" + ` is not supported with thinking."}}`,
			want: "top_k",
		},
		{
			name: "openai style unsupported parameter",
			body: `{"error":{"type":"invalid_request_error","message":"Unsupported parameter: top_p"}}`,
			want: "top_p",
		},
		{
			name: "semantic param never healed",
			body: `{"type":"error","error":{"type":"invalid_request_error","message":"` + "`max_tokens`" + ` is not supported."}}`,
			want: "",
		},
		{
			name: "unrelated 400",
			body: `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 210000 tokens > 200000 maximum"}}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractHealableParam(tc.body); got != tc.want {
				t.Fatalf("extractHealableParam() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTryHealClaudeParamErrorRemovesEmptyThinkingAndRetriesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	requestJSON := []byte(`{"model":"claude-test","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":""},{"type":"thinking","thinking":"valid","signature":"keep"},{"type":"text","text":"answer"}]}]}`)
	originalResp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"each thinking block must contain thinking"}}`)),
	}
	healedResp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}
	adaptor := &claudeThinkingHealTestAdaptor{response: healedResp}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:       constant.ChannelTypeAnthropic,
		ChannelId:         123,
		UpstreamModelName: "claude-test",
	}}

	got, healed := tryHealClaudeParamError(context, info, adaptor, requestJSON, originalResp)
	if !healed || got != healedResp {
		t.Fatalf("tryHealClaudeParamError() = (%v, %v), want healed response", got, healed)
	}
	if len(adaptor.requests) != 1 {
		t.Fatalf("retry requests = %d, want 1", len(adaptor.requests))
	}
	if strings.Contains(adaptor.requests[0], `"thinking":""`) {
		t.Fatalf("retry still contains empty thinking: %s", adaptor.requests[0])
	}
	if !strings.Contains(adaptor.requests[0], `"thinking":"valid"`) || !strings.Contains(adaptor.requests[0], `"signature":"keep"`) {
		t.Fatalf("retry lost valid thinking fields: %s", adaptor.requests[0])
	}
	if context.Writer.Header().Get("X-Request-Adjusted") != "empty_thinking" {
		t.Fatalf("X-Request-Adjusted = %q", context.Writer.Header().Get("X-Request-Adjusted"))
	}

	secondResp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"each thinking block must contain thinking"}}`)),
	}
	if got, healed := tryHealClaudeParamError(context, info, adaptor, requestJSON, secondResp); healed || got != secondResp {
		t.Fatalf("second self-heal should be blocked, got (%v, %v)", got, healed)
	}
	if len(adaptor.requests) != 1 {
		t.Fatalf("second self-heal retried unexpectedly: %d requests", len(adaptor.requests))
	}
}

type healTrackedBody struct {
	io.Reader
	closed bool
}

func (b *healTrackedBody) Close() error { b.closed = true; return nil }

type healReadError struct{ sent bool }

func (r *healReadError) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	return copy(p, "partial-error"), io.ErrUnexpectedEOF
}

func TestClaudeSamplingHealBoundaries(t *testing.T) {
	rejection := `{"type":"error","error":{"type":"invalid_request_error","message":"claude-haiku-5-5 does not support non-default temperature"}}`
	for _, tc := range []struct {
		name, body, request                      string
		status, channelType                      int
		written, canceled, twice, transportError bool
		want                                     bool
	}{
		{name: "explicit_refusal", body: rejection, want: true, twice: true},
		{name: "wrong_type", body: strings.ReplaceAll(rejection, "invalid_request_error", "authentication_error")},
		{name: "incidental_text", body: `{"error":{"type":"invalid_request_error","message":"bad request"},"detail":"claude-haiku-5-5 does not support non-default temperature"}`},
		{name: "invalid_json", body: rejection + "junk"},
		{name: "oversized", body: rejection + strings.Repeat(" ", maxClaudeHealErrorBytes)},
		{name: "multiple_parameters", body: strings.ReplaceAll(rejection, "non-default temperature", "non-default temperature with top_p")},
		{name: "missing_parameter", body: rejection, request: `{"model":"unknown","messages":[]}`},
		{name: "passthrough", body: rejection, request: "pass"},
		{name: "wrong_status", body: rejection, status: 422},
		{name: "other_provider", body: rejection, channelType: constant.ChannelTypeOpenAI},
		{name: "output_started", body: rejection, written: true},
		{name: "canceled", body: rejection, canceled: true},
		{name: "transport_error", body: rejection, transportError: true, twice: true},
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
				tc.channelType = constant.ChannelTypeAnthropic
			}
			body := &healTrackedBody{Reader: strings.NewReader(tc.body)}
			original := &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}
			original.Header.Set("X-Request-ID", "sampling-rejection")
			final := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}
			adaptor := &claudeThinkingHealTestAdaptor{response: final}
			if tc.transportError {
				adaptor.err = fmt.Errorf("fixture network error")
				adaptor.response = nil
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channelType, ChannelId: 456, UpstreamModelName: "claude-haiku-5-5"}}
			request := []byte(tc.request)
			if tc.request == "" {
				request = []byte(`{"model":"claude-haiku-5-5","temperature":0.2,"top_p":0.9,"max_tokens":24,"messages":[{"role":"user","content":"preserve"}],"opaque":9007199254740993}`)
			}
			if tc.request == "pass" {
				request = nil
			}
			oldLog := constant.ErrorLogEnabled
			constant.ErrorLogEnabled = true
			t.Cleanup(func() { constant.ErrorLogEnabled = oldLog })
			got, healed := tryHealClaudeParamError(c, info, adaptor, request, original)
			require.Equal(t, tc.want, healed)
			if tc.want {
				require.Same(t, final, got)
				require.True(t, body.closed)
				require.Len(t, adaptor.requests, 1)
				expected := strings.ReplaceAll(string(request), `"temperature":0.2,`, "")
				require.JSONEq(t, expected, adaptor.requests[0])
				require.Contains(t, adaptor.requests[0], "9007199254740993")
				require.Equal(t, "temperature", c.Writer.Header().Get("X-Param-Adjusted"))
				value, _ := c.Get("request_failure_attempts")
				attempts := value.([]map[string]interface{})
				require.Len(t, attempts, 1)
				require.Equal(t, "remove_temperature", attempts[0]["adjustment"])
				require.Equal(t, "sampling-rejection", attempts[0]["upstream"].(types.UpstreamDiagnostics).RequestID)
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
				next := &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(rejection))}
				_, healed = tryHealClaudeParamError(c, info, adaptor, request, next)
				require.False(t, healed)
				require.Len(t, adaptor.requests, 1)
			}
		})
	}
}

func TestPeekClaudeHealErrorPreservesReadFailure(t *testing.T) {
	original := &healTrackedBody{Reader: &healReadError{}}
	resp := &http.Response{Body: original}
	_, ok := peekClaudeHealError(resp)
	require.False(t, ok)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "partial-error", string(body))
	require.NoError(t, resp.Body.Close())
	require.True(t, original.closed)
}

func TestExtractHealableParamRequiresStructuredError(t *testing.T) {
	for _, body := range []string{
		`{"error":{"type":"invalid_request_error","message":"bad request"},"other":"Unsupported parameter: top_p"}`,
		`{"error":{"type":"api_error","message":"Unsupported parameter: top_p"}}`,
		`invalid_request_error Unsupported parameter: top_p`,
	} {
		require.Empty(t, extractHealableParam(body))
	}
	payload, err := common.Marshal(map[string]any{"error": map[string]string{"type": "invalid_request_error", "message": "claude-haiku-5-5 does not support non-default temperature"}})
	require.NoError(t, err)
	require.Equal(t, "temperature", extractHealableParam(string(payload)))
}
