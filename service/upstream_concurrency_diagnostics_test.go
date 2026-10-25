package service

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

const kimiConcurrencyMessage = "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."

func TestUpstreamConcurrencyDiagnosticBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, message, code, want string
		status                    int
		upstream                  bool
	}{
		{"kimi_403", kimiConcurrencyMessage, "unknown_error", "rate_limit", 403, true},
		{"pool_wrapper", "auth_unavailable: no auth available (last upstream error: access_terminated_error: " + kimiConcurrencyMessage + ")", "internal_server_error", "rate_limit", 503, true},
		{"gateway_code", "account capacity exceeded", "gateway_concurrency_limit", "rate_limit", 502, true},
		{"gateway_message", "Concurrency limit exceeded for account, please retry later", "unknown_error", "rate_limit", 502, true},
		{"ordinary_429", "too many requests", "unknown_error", "rate_limit", 429, true},
		{"invalid_key", "Invalid API key", "invalid_api_key", "upstream_auth", 403, true},
		{"permission_mentions_concurrency", "Permission denied: this plan does not allow concurrent requests", "permission_error", "upstream_auth", 403, true},
		{"unauthorized", kimiConcurrencyMessage, "invalid_api_key", "upstream_auth", 401, true},
		{"local_forbidden", kimiConcurrencyMessage, "permission", "permission", 403, false},
		{"unrelated_pool_failure", "auth_unavailable: no auth available", "internal_server_error", "unavailable", 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := types.WithOpenAIError(types.OpenAIError{Type: "api_error", Code: tc.code, Message: tc.message}, tc.status)
			category, _, _ := describeRequestFailure(err, tc.upstream)
			require.Equal(t, tc.want, category)
			require.Equal(t, tc.status, err.StatusCode)
			require.Equal(t, tc.code, string(err.GetErrorCode()))
		})
	}
	require.False(t, isExplicitUpstreamConcurrencyLimit(nil))
}

func TestFinalFailureConcurrencyKeepsOriginalStatusAndAttempts(t *testing.T) {
	ctx := failureLogContext(t)
	ctx.Writer.WriteHeader(http.StatusForbidden)
	ctx.Writer.WriteHeaderNow()
	err := types.WithOpenAIError(types.OpenAIError{Type: "access_terminated_error", Code: "unknown_error", Message: kimiConcurrencyMessage}, http.StatusForbidden)
	err.RetryAfter = 0
	channel := *types.NewChannelError(19, 25, "fixture", false, "secret-key", false)
	AppendRequestFailureAttempt(ctx, channel, err)
	MarkRequestFailure(ctx, err)
	RecordFinalRequestFailure(ctx, time.Second)
	RecordFinalRequestFailure(ctx, time.Second)
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-stream-failure").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Zero(t, rows[0].Quota)
	require.Equal(t, "请求受到限流或并发限制", rows[0].Content)
	other, parseErr := common.StrToMap(rows[0].Other)
	require.NoError(t, parseErr)
	require.Equal(t, "rate_limit", other["failure_category"])
	require.EqualValues(t, 403, other["status_code"])
	require.EqualValues(t, 403, other["http_status"])
	require.Equal(t, "unknown_error", other["error_code"])
	require.NotContains(t, other, "retry_after_seconds")
	require.NotContains(t, rows[0].Other, "secret-key")
	admin := other["admin_info"].(map[string]interface{})
	require.Contains(t, fmt.Sprint(admin["attempts"]), kimiConcurrencyMessage)
}
