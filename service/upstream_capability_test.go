package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const capabilityModelError = `{"error":{"message":"unknown provider for model example-model","type":"invalid_request_error","code":"model_not_found","param":"model"}}`
const capabilityToolsError = `{"error":{"message":"example-model requires Responses for tool calls; this account only supports Chat Completions","type":"invalid_request_error"}}`
const capabilityFailureTail = "event: response.failed\ndata: " + `{"type":"response.failed","response":{"status":"failed","output":[],"error":{"code":"upstream_error","message":"Upstream request failed"}}}` + "\n\n"

func parseCapabilityTestError(body string, status int, showBody bool) *types.NewAPIError {
	return RelayErrorHandler(context.Background(), &http.Response{
		StatusCode: status,
		Header:     http.Header{"X-Request-Id": {"upstream-capability-test"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, showBody)
}

func TestUpstreamCapabilityErrorParsing(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       bool
	}{
		{"provider_json", capabilityModelError, 400, true},
		{"tools_json", capabilityToolsError, 400, true},
		{"tools_mixed", capabilityToolsError + capabilityFailureTail, 400, true},
		{"provider_mixed_crlf", " \n" + capabilityModelError + "\r\n" + strings.ReplaceAll(capabilityFailureTail, "\n", "\r\n"), 400, true},
		{"generic_400", `{"error":{"message":"invalid tool arguments","type":"invalid_request_error"}}`, 400, false},
		{"generic_model_missing", strings.ReplaceAll(capabilityModelError, "unknown provider for model", "model not found:"), 400, false},
		{"wrong_code", strings.ReplaceAll(capabilityModelError, "model_not_found", "invalid_argument"), 400, false},
		{"wrong_type", strings.ReplaceAll(capabilityModelError, "invalid_request_error", "authentication_error"), 400, false},
		{"tools_safety_code", strings.ReplaceAll(capabilityToolsError, `"type":"invalid_request_error"`, `"type":"invalid_request_error","code":"content_policy_violation"`), 400, false},
		{"quoted_message", strings.ReplaceAll(capabilityModelError, "unknown provider for model", "invalid prompt mentions unknown provider for model"), 400, false},
		{"status_401", capabilityModelError, 401, false},
		{"status_404", capabilityModelError, 404, false},
		{"status_200", capabilityModelError, 200, false},
		{"arbitrary_suffix", capabilityToolsError + "trailing text", 400, false},
		{"second_json", capabilityToolsError + `{}`, 400, false},
		{"success_event", capabilityToolsError + strings.ReplaceAll(capabilityFailureTail, "failed", "completed"), 400, false},
		{"failure_with_output", capabilityToolsError + strings.ReplaceAll(capabilityFailureTail, `"output":[]`, `"output":[{"type":"function_call","name":"lookup"}]`), 400, false},
		{"output_before_failure", capabilityToolsError + "event: response.output_text.delta\ndata: {}\n\n" + capabilityFailureTail, 400, false},
		{"output_after_failure", capabilityToolsError + capabilityFailureTail + "event: response.output_text.delta\ndata: {}\n\n", 400, false},
		{"missing_terminal_status", capabilityToolsError + strings.ReplaceAll(capabilityFailureTail, `"status":"failed",`, ""), 400, false},
		{"over_recovery_limit", capabilityToolsError + strings.Repeat(" ", 64*1024) + capabilityFailureTail, 400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, showBody := range []bool{false, true} {
				err := parseCapabilityTestError(tc.body, tc.status, showBody)
				require.Equal(t, tc.want, IsUpstreamCapabilityError(err))
				require.Equal(t, tc.status, err.UpstreamStatusCode)
				require.Equal(t, "upstream-capability-test", err.Upstream.RequestID)
				require.EqualValues(t, len(tc.body), err.Upstream.ResponseLength)
				if tc.want {
					require.NotContains(t, err.ToOpenAIError().Message, "event:")
				}
			}
		})
	}
	local := types.WithOpenAIError(types.OpenAIError{Message: "unknown provider for model example-model", Type: "invalid_request_error", Code: "model_not_found"}, 400)
	require.False(t, IsUpstreamCapabilityError(local), "local validation must not become an upstream retry")
}

func TestUpstreamCapabilityRetrySafety(t *testing.T) {
	oldRetry := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = oldRetry })
	for _, tc := range []struct {
		name   string
		mutate func(*gin.Context, *dto.OpenAIResponsesRequest, *types.NewAPIError)
		budget int
		want   bool
	}{
		{"self_contained", nil, 1, true},
		{"retry_disabled", nil, 0, false},
		{"fixed_channel", func(c *gin.Context, _ *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			c.Set("specific_channel_id", 1)
		}, 1, false},
		{"written_text", func(c *gin.Context, _ *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			_, _ = c.Writer.WriteString("data: text\n\n")
		}, 1, false},
		{"flushed_heartbeat", func(c *gin.Context, _ *dto.OpenAIResponsesRequest, _ *types.NewAPIError) { c.Writer.Flush() }, 1, false},
		{"previous_response", func(_ *gin.Context, req *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			req.PreviousResponseID = "resp_prior"
		}, 1, false},
		{"conversation", func(_ *gin.Context, req *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			req.Conversation = []byte(`"conv_prior"`)
		}, 1, false},
		{"null_conversation", func(_ *gin.Context, req *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			req.Conversation = []byte(` null `)
		}, 1, true},
		{"skip_retry", func(_ *gin.Context, _ *dto.OpenAIResponsesRequest, err *types.NewAPIError) {
			types.ErrOptionWithSkipRetry()(err)
		}, 1, false},
		{"total_budget_after_auto_reset", func(c *gin.Context, _ *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			c.Set("use_channel", []string{"1", "2"})
		}, 1, false},
		{"canceled", func(c *gin.Context, _ *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			ctx, cancel := context.WithCancel(c.Request.Context())
			cancel()
			c.Request = c.Request.WithContext(ctx)
		}, 1, false},
		{"affinity_without_provider_state", func(c *gin.Context, _ *dto.OpenAIResponsesRequest, _ *types.NewAPIError) {
			c.Set(ginKeyChannelAffinityMeta, &channelAffinityMeta{AffinityUsed: true, SkipRetry: true})
		}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set("use_channel", []string{"1"})
			req := &dto.OpenAIResponsesRequest{Input: []byte(`[{"type":"function_call_output","call_id":"call_test","output":"ok"}]`)}
			err := parseCapabilityTestError(capabilityModelError, 400, false)
			if tc.mutate != nil {
				tc.mutate(c, req, err)
			}
			err = GuardUpstreamCapabilityRetry(c, req, err)
			require.Equal(t, tc.want, ShouldRetryChannelError(c, err, tc.budget))
		})
	}
}
