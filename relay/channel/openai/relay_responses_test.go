package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var responsesStreamTokenEncoderOnce sync.Once

func TestDecorateResponsesPolicyFailureDataAddsVisibleWarningOnlyForSafety(t *testing.T) {
	policyData := `{"type":"response.failed","response":{"status":"failed","error":{"message":"rejected","type":"invalid_request","code":"cyber_policy"}}}`
	var policyEvent dto.ResponsesStreamResponse
	require.NoError(t, common.UnmarshalJsonStr(policyData, &policyEvent))
	decorated := decorateResponsesPolicyFailureData(policyEvent, policyData)
	require.Contains(t, decorated, "本站警告")
	require.Contains(t, decorated, "cyber_policy")

	contextData := `{"type":"error","error":{"message":"too long","type":"invalid_request","code":"context_length_exceeded"}}`
	var contextEvent dto.ResponsesStreamResponse
	require.NoError(t, common.UnmarshalJsonStr(contextData, &contextEvent))
	require.Equal(t, contextData, decorateResponsesPolicyFailureData(contextEvent, contextData))
}

func newResponsesStreamTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	responsesStreamTokenEncoderOnce.Do(service.InitTokenEncoders)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, recorder
}

func newResponsesStreamHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestResponsesUsageObservesDeclaredModelWithoutChangingBilling(t *testing.T) {
	c, _ := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{OriginModelName: "requested", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped"}}
	response := &dto.OpenAIResponsesResponse{Model: "declared"}
	buildResponsesUsage(c, info, response)
	require.Equal(t, "declared", info.ResponseModel.ReturnedModel)
	require.True(t, info.ResponseModel.Mismatch())
	require.Equal(t, "requested", info.OriginModelName)
	require.Equal(t, "mapped", info.UpstreamModelName)
	require.Equal(t, "declared", response.Model)
}

type errorAfterBodyReadCloser struct {
	reader *strings.Reader
	err    error
}

func (r *errorAfterBodyReadCloser) Read(p []byte) (int, error) {
	if r.reader.Len() > 0 {
		return r.reader.Read(p)
	}
	return 0, r.err
}

func (r *errorAfterBodyReadCloser) Close() error {
	return nil
}

func newResponsesStreamHTTPResponseWithReadError(body string, err error) *http.Response {
	resp := newResponsesStreamHTTPResponse("")
	resp.Body = &errorAfterBodyReadCloser{
		reader: strings.NewReader(body),
		err:    err,
	}
	return resp
}

func newResponsesStreamCooldownCounter() (*ResponsesStreamHandlerOptions, *int) {
	count := 0
	return &ResponsesStreamHandlerOptions{
		scheduleCooldown: func(string) {
			count++
		},
	}, &count
}

func TestShouldScheduleMissingResponsesCompletedCooldownSkipsClientGone(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, errors.New("context canceled"))

	require.False(t, shouldScheduleMissingResponsesCompletedCooldown(info))
}

func TestShouldScheduleMissingResponsesCompletedCooldownKeepsUpstreamErrors(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("upstream read timeout"))

	require.True(t, shouldScheduleMissingResponsesCompletedCooldown(info))
	require.True(t, shouldScheduleMissingResponsesCompletedCooldown(nil))
}

func TestOaiResponsesStreamHandler_ResponseFailedReturnsOriginalErrorWithoutCooldown(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"}}
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_policy","model":"gpt-5.5"}}`,
		`data: {"type":"response.failed","response":{"id":"resp_policy","status":"failed","error":{"message":"request rejected by policy","type":"invalid_request_error","code":"cyber_policy"}}}`,
		`data: [DONE]`,
	}, "\n")
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, types.ErrorCode("cyber_policy"), err.GetErrorCode())
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 0, *cooldowns)
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.Contains(t, recorder.Body.String(), `"code":"cyber_policy"`)
	require.NotContains(t, recorder.Body.String(), "event: response.completed")
	require.NotContains(t, recorder.Body.String(), service.ResponsesStreamMissingCompletedReason)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyResponsesStreamErrorWritten))
}

func TestOaiResponsesStreamHandler_EarlyOverloadIsRetryableBeforeOutput(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.6-luna"}}
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_overload","model":"gpt-5.6-luna"}}`,
		`data: {"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
	}, "\n")
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.Equal(t, types.ErrorCode("server_is_overloaded"), err.GetErrorCode())
	require.False(t, types.IsSkipRetryError(err))
	require.Equal(t, 0, *cooldowns)
	require.Empty(t, recorder.Body.String())
	require.False(t, common.GetContextKeyBool(c, constant.ContextKeyResponsesStreamErrorWritten))
}

func TestOaiResponsesStreamHandler_OverloadAfterOutputIsForwardedWithoutRetry(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.6-luna"}}
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_overload","model":"gpt-5.6-luna"}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.failed","response":{"id":"resp_overload","status":"failed","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
	}, "\n")
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)

	require.NotNil(t, usage)
	require.True(t, usage.InterruptedOutput)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 0, *cooldowns)
	require.Contains(t, recorder.Body.String(), "event: response.created")
	require.Contains(t, recorder.Body.String(), "event: response.output_text.delta")
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyResponsesStreamErrorWritten))
}

func TestOaiResponsesStreamHandler_EventOnlyResponseFailedIsRecognized(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"}}
	body := strings.Join([]string{
		`event: response.failed`,
		`data: {"response":{"id":"resp_policy","status":"failed","error":{"message":"request rejected by policy","type":"invalid_request_error","code":"cyber_policy"}},"provider_trace":"kept"}`,
	}, "\n")
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, types.ErrorCode("cyber_policy"), err.GetErrorCode())
	require.Equal(t, 0, *cooldowns)
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.Contains(t, recorder.Body.String(), `"type":"response.failed"`)
	require.Contains(t, recorder.Body.String(), `"provider_trace":"kept"`)
	require.NotContains(t, recorder.Body.String(), "event: response.completed")
}

func TestOaiResponsesStreamHandler_CompletedWithFailedStatusNeverSignalsSuccess(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"}}
	body := `data: {"type":"response.completed","response":{"id":"resp_failed","status":"failed","error":{"message":"upstream rejected request","type":"invalid_request_error","code":"invalid_request"}}}` + "\n"
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, 0, *cooldowns)
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.Contains(t, recorder.Body.String(), `"type":"response.failed"`)
	require.NotContains(t, recorder.Body.String(), "event: response.completed")
}

func TestOaiResponsesStreamHandler_TopLevelContextLimitErrorDoesNotCooldown(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"}}
	body := `data: {"error":{"message":"maximum context length exceeded","type":"invalid_request_error","code":"context_length_exceeded"}}` + "\n"
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, types.ErrorCode("context_length_exceeded"), err.GetErrorCode())
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 0, *cooldowns)
	require.Contains(t, recorder.Body.String(), "event: error")
	require.NotContains(t, recorder.Body.String(), "event: response.completed")
	require.NotContains(t, recorder.Body.String(), service.ResponsesStreamMissingCompletedReason)
}

func TestOaiResponsesStreamHandler_DoneWithoutSemanticTerminalIsChannelFault(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"}}
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse("data: [DONE]\n"), opts)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, 1, *cooldowns)
	require.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.NotContains(t, recorder.Body.String(), "event: response.completed")
}

func TestOaiResponsesStreamHandler_ClientCancelDoesNotCooldownOrBill(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"}}
	opts, cooldowns := newResponsesStreamCooldownCounter()

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponseWithReadError("", context.Canceled), opts)

	require.Nil(t, usage)
	require.Error(t, err)
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, types.ErrorCodeDoRequestFailed, err.GetErrorCode())
	require.Equal(t, 0, *cooldowns)
	require.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	require.Empty(t, recorder.Body.String())
	require.False(t, common.GetContextKeyBool(c, constant.ContextKeyResponsesStreamErrorWritten))
}

func TestOaiResponsesStreamHandler_FailsWhenMissingCompletedWithoutOutputAfterEOF(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
	}, "\n")

	opts, cooldowns := newResponsesStreamCooldownCounter()
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)
	require.Nil(t, usage)
	require.Error(t, err)
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 1, *cooldowns)
	require.NotNil(t, info.StreamStatus)
	require.True(t, info.StreamStatus.HasErrors())
	require.True(t, info.FirstEffectiveOutputTime.IsZero())

	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, "event: response.failed")
	require.Contains(t, responseBody, `"id":"resp_1"`)
	require.Contains(t, responseBody, `"status":"failed"`)
	require.Contains(t, responseBody, `stream end: eof`)
	require.NotContains(t, responseBody, "event: response.completed")
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyResponsesStreamErrorWritten))
}

func TestOaiResponsesStreamHandler_FailsInsteadOfSynthesizingCompletedWhenOutputIsPartial(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
		`data: {"type":"response.output_text.delta","delta":"partial output"}`,
	}, "\n")

	opts, cooldowns := newResponsesStreamCooldownCounter()
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)
	require.NotNil(t, usage)
	require.True(t, usage.InterruptedOutput)
	require.Error(t, err)
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 1, *cooldowns)
	require.NotNil(t, info.StreamStatus)
	require.True(t, info.StreamStatus.HasErrors())
	require.False(t, info.FirstEffectiveOutputTime.IsZero())

	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, "partial output")
	require.Contains(t, responseBody, "event: response.failed")
	require.NotContains(t, responseBody, "event: response.completed")
}

func TestOaiResponsesStreamHandler_FailsWhenMissingCompletedWithoutOutputAfterScannerError(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}` + "\n"

	opts, cooldowns := newResponsesStreamCooldownCounter()
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponseWithReadError(body, errors.New("upstream read timeout")), opts)
	require.Nil(t, usage)
	require.Error(t, err)
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 1, *cooldowns)
	require.NotNil(t, info.StreamStatus)
	require.Equal(t, relaycommon.StreamEndReasonScannerErr, info.StreamStatus.EndReason)
	require.True(t, info.StreamStatus.HasErrors())

	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, "event: response.failed")
	require.Contains(t, responseBody, `"status":"failed"`)
	require.Contains(t, responseBody, `stream end: scanner_error`)
	require.NotContains(t, responseBody, "event: response.completed")
}

func TestOaiResponsesStreamHandler_DoesNotSynthesizeCompletedWhenOutputExistsAfterScannerError(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
		`data: {"type":"response.output_text.delta","delta":"partial output"}`,
	}, "\n") + "\n"

	opts, cooldowns := newResponsesStreamCooldownCounter()
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponseWithReadError(body, errors.New("upstream read timeout")), opts)
	require.NotNil(t, usage)
	require.True(t, usage.InterruptedOutput)
	require.Error(t, err)
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 1, *cooldowns)
	require.NotNil(t, info.StreamStatus)
	require.Equal(t, relaycommon.StreamEndReasonScannerErr, info.StreamStatus.EndReason)
	require.True(t, info.StreamStatus.HasErrors())

	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, "partial output")
	require.Contains(t, responseBody, "event: response.failed")
	require.NotContains(t, responseBody, "event: response.completed")
}

func TestOaiResponsesStreamHandler_AutoContinuesBeforeSyntheticCompleted(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
		`data: {"type":"response.output_text.delta","delta":"partial output"}`,
	}, "\n") + "\n"

	called := false
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponseWithReadError(body, errors.New("upstream read timeout")), &ResponsesStreamHandlerOptions{
		AutoContinue: func(streamCtx ResponsesStreamAutoContinueContext) (*dto.Usage, bool) {
			called = true
			require.Equal(t, "partial output", streamCtx.OutputText)
			return &dto.Usage{PromptTokens: 5, CompletionTokens: 6, TotalTokens: 11}, true
		},
	})
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.True(t, called)
	require.GreaterOrEqual(t, usage.CompletionTokens, 6)

	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, "partial output")
	require.NotContains(t, responseBody, "event: response.completed")
	require.NotContains(t, responseBody, "event: response.failed")
}

func TestOaiResponsesStreamHandler_ContinuationOutputOnlyFiltersLifecycleEvents(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_2","model":"gpt-5.5","created_at":1700000001}}`,
		`data: {"type":"response.output_item.added","item":{"type":"message","id":"msg_1","role":"assistant"}}`,
		`data: {"type":"response.output_text.delta","delta":"continued"}`,
		`data: {"type":"response.completed","response":{"id":"resp_2","model":"gpt-5.5","created_at":1700000001,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"continued"}]}],"usage":{"input_tokens":5,"output_tokens":1,"total_tokens":6}}}`,
	}, "\n")

	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), &ResponsesStreamHandlerOptions{
		ContinuationOutputOnly: true,
	})
	require.Nil(t, err)
	require.Equal(t, 6, usage.TotalTokens)

	responseBody := recorder.Body.String()
	require.NotContains(t, responseBody, "event: response.created")
	require.NotContains(t, responseBody, "event: response.output_item.added")
	require.Contains(t, responseBody, "continued")
	require.Contains(t, responseBody, "event: response.completed")
}

func TestOaiResponsesStreamHandler_DoesNotDuplicateCompleted(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}`,
		`data: [DONE]`,
	}, "\n")

	opts, cooldowns := newResponsesStreamCooldownCounter()
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 0, *cooldowns)
	require.False(t, info.StreamStatus.HasErrors())

	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed"))
}

func TestOaiResponsesStreamHandler_RecordsCompletedSummary(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000,"status":"completed","output":[{"type":"reasoning"},{"type":"function_call","name":"shell","arguments":"secret_value"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]},{"type":"web_search_call","status":"completed"},{"type":"image_generation_call","status":"completed"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}`,
		`data: [DONE]`,
	}, "\n")

	usage, err := OaiResponsesStreamHandler(c, info, newResponsesStreamHTTPResponse(body))
	require.Nil(t, err)
	require.Equal(t, 7, usage.TotalTokens)

	summary := info.ResponsesCompletedSummary
	require.NotNil(t, summary)
	require.Equal(t, "completed", summary.Status)
	require.Equal(t, 5, summary.OutputCount)
	require.Equal(t, []string{"reasoning", "function_call", "message", "web_search_call", "image_generation_call"}, summary.OutputTypes)
	require.Equal(t, 1, summary.MessageCount)
	require.Equal(t, 1, summary.FunctionCallCount)
	require.Equal(t, 1, summary.WebSearchCallCount)
	require.Equal(t, 1, summary.ImageGenerationCallCount)
	require.Equal(t, 1, summary.ActionableToolCallCount)
	require.Equal(t, 4, summary.MessageTextChars)
	require.True(t, summary.HasActionableToolCall)

	summarySnapshot := fmt.Sprintf("%#v", summary)
	require.NotContains(t, summarySnapshot, "secret_value")
	require.NotContains(t, summarySnapshot, "done")
	require.Contains(t, recorder.Body.String(), "event: response.completed")
}

func TestOaiResponsesStreamHandler_RecordsMessageOnlyCompletedSummary(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, _ := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000,"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"top-secret"}]}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}`,
		`data: [DONE]`,
	}, "\n")

	usage, err := OaiResponsesStreamHandler(c, info, newResponsesStreamHTTPResponse(body))
	require.Nil(t, err)
	require.Equal(t, 7, usage.TotalTokens)

	summary := info.ResponsesCompletedSummary
	require.NotNil(t, summary)
	require.Equal(t, 1, summary.OutputCount)
	require.Equal(t, []string{"message"}, summary.OutputTypes)
	require.Equal(t, 1, summary.MessageCount)
	require.Zero(t, summary.FunctionCallCount)
	require.Zero(t, summary.ActionableToolCallCount)
	require.False(t, summary.HasActionableToolCall)
	require.Equal(t, len("top-secret"), summary.MessageTextChars)
	require.NotContains(t, fmt.Sprintf("%#v", summary), "top-secret")
}

func TestOaiResponsesStreamHandler_FailsWhenCompletedHasNoOutput(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000,"usage":{"input_tokens":3,"output_tokens":37,"total_tokens":40}}}`,
		`data: [DONE]`,
	}, "\n")

	opts, cooldowns := newResponsesStreamCooldownCounter()
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)
	require.Nil(t, usage)
	require.Error(t, err)
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, 0, *cooldowns, "empty completion must not cool down a shared upstream")
	require.True(t, info.StreamStatus.HasErrors())

	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, "event: response.failed")
	require.Contains(t, responseBody, `"status":"failed"`)
	require.Contains(t, responseBody, "responses stream completed without visible output")
	require.NotContains(t, responseBody, "event: response.completed")
}

func TestOaiResponsesStreamHandler_EmptyCompletionShapesDoNotCooldown(t *testing.T) {
	setResponsesStreamTestTimeout(t)
	for _, completed := range []string{
		`{"type":"response.completed"}`,
		`{"type":"response.completed","response":{"status":"completed","output":[]}}`,
		`{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[]}]}}`,
	} {
		t.Run(completed, func(t *testing.T) {
			c, recorder := newResponsesStreamTestContext()
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-empty"}}
			opts, cooldowns := newResponsesStreamCooldownCounter()
			usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse("data: "+completed+"\n\n"), opts)
			require.Nil(t, usage)
			require.Error(t, err)
			require.True(t, types.IsSkipRetryError(err))
			require.Equal(t, http.StatusBadGateway, err.StatusCode)
			require.Zero(t, *cooldowns)
			require.Contains(t, recorder.Body.String(), "event: response.failed")
			require.NotContains(t, recorder.Body.String(), "event: response.completed")
		})
	}
}

func TestOaiResponsesStreamHandler_ToolOnlyCompletionRemainsSuccessful(t *testing.T) {
	setResponsesStreamTestTimeout(t)
	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-tool"}}
	body := `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","name":"test_tool","call_id":"call_1","arguments":"{}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}` + "\n\n"
	opts, cooldowns := newResponsesStreamCooldownCounter()
	usage, err := OaiResponsesStreamHandlerWithOptions(c, info, newResponsesStreamHTTPResponse(body), opts)
	require.Nil(t, err)
	require.Equal(t, 3, usage.TotalTokens)
	require.Zero(t, *cooldowns)
	require.Contains(t, recorder.Body.String(), "event: response.completed")
	require.NotContains(t, recorder.Body.String(), "event: response.failed")
}

func TestOaiResponsesStreamHandler_ChannelTestFailsWhenCompletedMissing(t *testing.T) {
	t.Parallel()
	setResponsesStreamTestTimeout(t)

	c, recorder := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	body := `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5","created_at":1700000000}}`

	usage, err := OaiResponsesStreamHandler(c, info, newResponsesStreamHTTPResponse(body))
	require.NotNil(t, usage)
	require.Error(t, err)
	require.Contains(t, err.Error(), "response.completed")
	require.NotContains(t, recorder.Body.String(), "event: response.completed")
}
