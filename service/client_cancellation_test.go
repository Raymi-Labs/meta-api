package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNormalizeClientCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		for _, code := range []types.ErrorCode{types.ErrorCodeDoRequestFailed, types.ErrorCodeReadResponseBodyFailed, types.ErrorCodeReadRequestBodyFailed} {
			t.Run(string(code)+map[bool]string{false: "/active", true: "/canceled"}[canceled], func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				requestCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)
				if canceled {
					cancel()
				}
				// Reproduce the adapter's loss of context.Canceled when reading a
				// non-stream response, and the controller's later message rewrite.
				original := types.NewOpenAIError(context.Canceled, code, 500)
				result := NormalizeClientCancellation(ctx, original)
				if !canceled {
					require.Same(t, original, result)
					return
				}
				result.SetMessage(common.MessageWithRequestId(result.Error(), "req-canceled"))
				require.Equal(t, types.StatusClientClosedRequest, result.StatusCode)
				require.Equal(t, types.FailureClientCanceled, types.ClassifyFailure(result))
				require.True(t, types.IsSkipRetryError(result))
				require.False(t, ShouldRetryChannelError(ctx, result, 3))
			})
		}
	}
}

func TestClientCancellationKeepsCompletedResponsesAndUpstreamFailures(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)
	require.Nil(t, NormalizeClientCancellation(ctx, nil))
	upstream := types.NewOpenAIError(errors.New("provider error"), types.ErrorCodeReadResponseBodyFailed, 500)
	upstream.UpstreamStatusCode = 500
	require.Same(t, upstream, NormalizeClientCancellation(ctx, upstream))
	quota := types.NewErrorWithStatusCode(errors.New("quota exhausted"), types.ErrorCodeInsufficientUserQuota, 403)
	require.Same(t, quota, NormalizeClientCancellation(ctx, quota))
	deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	ctx.Request = ctx.Request.WithContext(deadlineCtx)
	timeout := types.NewErrorWithStatusCode(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, 504)
	require.Same(t, timeout, NormalizeClientCancellation(ctx, timeout))
	require.Equal(t, types.FailureService, types.ClassifyFailure(timeout))
}

func TestCanceledRequestStopsRetryBeforeChannelAndOverloadRules(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)
	for _, err := range []*types.NewAPIError{
		types.NewError(errors.New("invalid channel key"), types.ErrorCodeChannelInvalidKey),
		types.WithOpenAIError(types.OpenAIError{Code: "server_is_overloaded", Message: "server_is_overloaded"}, 503),
		types.NewOpenAIError(context.Canceled, types.ErrorCodeReadResponseBodyFailed, 500),
	} {
		require.False(t, ShouldRetryChannelError(ctx, err, 3))
	}
}

func TestCanceledResponseBodyProducesClientCancellationLog(t *testing.T) {
	ctx := failureLogContext(t)
	requestCtx, cancel := context.WithCancel(ctx.Request.Context())
	cancel()
	ctx.Request = ctx.Request.WithContext(requestCtx)
	err := NormalizeClientCancellation(ctx, types.NewOpenAIError(context.Canceled, types.ErrorCodeReadResponseBodyFailed, 500))
	MarkRequestFailure(ctx, err)
	err.SetMessage(common.MessageWithRequestId(err.Error(), "req-canceled"))
	ctx.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
	RecordFinalRequestFailure(ctx, 1460*time.Millisecond)
	var row model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-stream-failure").First(&row).Error)
	require.Equal(t, "客户端连接中断", row.Content)
	require.Zero(t, row.Quota)
	other, parseErr := common.StrToMap(row.Other)
	require.NoError(t, parseErr)
	require.Equal(t, "client_canceled", other["failure_class"])
	require.Equal(t, "client_canceled", other["failure_category"])
	require.Equal(t, "client_canceled", other["error_code"])
	require.EqualValues(t, 499, other["http_status"])
	require.EqualValues(t, 499, other["status_code"])
}
