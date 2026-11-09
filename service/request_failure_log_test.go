package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func failureLogContext(t *testing.T) *gin.Context {
	t.Helper()
	truncate(t)
	previous := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = previous })
	seedUser(t, 992701, 1000)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions?api_key=not-recorded", nil)
	ctx.Set("id", 992701)
	ctx.Set("username", "log-user")
	ctx.Set("token_id", 7)
	ctx.Set("token_name", "my-token")
	ctx.Set("original_model", "claude-opus-5")
	ctx.Set("channel_id", 19)
	ctx.Set(common.RequestIdKey, "req-stream-failure")
	return ctx
}

func TestFinalFailureRecordsHTTP200StreamErrorOnce(t *testing.T) {
	ctx := failureLogContext(t)
	client := newRelayHttpClient(120 * time.Second)
	t.Cleanup(client.CloseIdleConnections)
	RecordRelayHTTPTimeout(ctx, client, true)
	ctx.Set(RequestLogStreamKey, true)
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Writer.WriteHeaderNow()
	ctx.Writer.Flush()
	err := types.WithOpenAIError(types.OpenAIError{Type: "invalid_request_error", Code: "content_policy_violation", Message: "blocked by content policy at https://private.example/api?key=secret with Bearer hidden-secret sk-another-secret"}, 400)
	err.Upstream = &types.UpstreamDiagnostics{URL: "https://private.example/api"}
	MarkRequestFailure(ctx, err)
	RecordFinalRequestFailure(ctx, 1554*time.Millisecond)
	RecordFinalRequestFailure(ctx, 2*time.Second)
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-stream-failure").Find(&rows).Error)
	require.Len(t, rows, 1)
	log := rows[0]
	require.Equal(t, model.LogTypeError, log.Type)
	require.True(t, log.IsStream)
	require.Zero(t, log.Quota)
	require.Equal(t, "请求被内容安全策略拒绝", log.Content)
	details, err2 := common.StrToMap(log.Other)
	require.NoError(t, err2)
	require.EqualValues(t, 200, details["http_status"])
	require.EqualValues(t, 400, details["status_code"])
	require.EqualValues(t, 1554, details["latency_ms"])
	require.Equal(t, "/v1/chat/completions", details["request_path"])
	require.Contains(t, details["admin_info"], "relay_http_timeout")
	require.NotContains(t, log.Other, "hidden-secret")
	require.NotContains(t, log.Other, "sk-another-secret")
}

func TestFinalFailureOmitsRecoveredRetryButKeepsFinalDiagnostics(t *testing.T) {
	ctx := failureLogContext(t)
	channel := *types.NewChannelError(19, 1, "private-channel", false, "secret-channel-key", false)
	for i := 0; i < 7; i++ {
		AppendRequestFailureAttempt(ctx, channel, types.NewErrorWithStatusCode(fmt.Errorf("attempt %d", i), "upstream_error", 502))
	}
	RecordFinalRequestFailure(ctx, time.Second)
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
	require.Zero(t, count, "successful retry must not create a failure row")
	MarkRequestFailure(ctx, types.NewErrorWithStatusCode(fmt.Errorf("unavailable"), "get_channel_failed", 503))
	RecordFinalRequestFailure(ctx, time.Second)
	var row model.Log
	require.NoError(t, model.LOG_DB.First(&row).Error)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	require.Len(t, other["admin_info"].(map[string]interface{})["attempts"], 5)
	require.NotContains(t, row.Other, "secret-channel-key")
}

func TestFinalFailureCapturesLocalCooldownAndHonorsDisabledSetting(t *testing.T) {
	ctx := failureLogContext(t)
	err := types.NewErrorWithStatusCode(fmt.Errorf("cooldown"), "content_safety_cooldown", 429, types.ErrOptionWithNoRecordErrorLog())
	err.RetryAfter = 3 * time.Second
	MarkRequestFailure(ctx, err)
	constant.ErrorLogEnabled = false
	RecordFinalRequestFailure(ctx, time.Second)
	constant.ErrorLogEnabled = true
	RecordFinalRequestFailure(ctx, time.Second)
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	other, e := common.StrToMap(rows[0].Other)
	require.NoError(t, e)
	require.EqualValues(t, 3, other["retry_after_seconds"])
	require.Equal(t, "content_policy", other["failure_category"])
}

func TestDescribeClientCancellation(t *testing.T) {
	err := types.NewErrorWithStatusCode(fmt.Errorf("client canceled while receiving responses stream"), types.ErrorCodeDoRequestFailed, 500)
	category, reason, _ := describeRequestFailure(err, true)
	require.Equal(t, "client_canceled", category)
	require.Equal(t, "客户端连接中断", reason)
}

func TestFinalFailureKeepsUnbilledUsageSnapshotOnlyInAdminDiagnostics(t *testing.T) {
	ctx := failureLogContext(t)
	usage := &dto.Usage{PromptTokens: 41093, CompletionTokens: 1, TotalTokens: 41094,
		InputTokensEstimated: true, InterruptedOutput: true, WebSearchRequests: 2}
	CaptureUnbilledRequestUsage(nil, usage)
	CaptureUnbilledRequestUsage(ctx, nil)
	CaptureUnbilledRequestUsage(ctx, usage)
	usage.PromptTokens = 999 // Later mutations must not change the diagnostic snapshot.
	MarkRequestFailure(ctx, types.NewErrorWithStatusCode(fmt.Errorf("upstream unavailable"), "upstream_error", 500))
	RecordFinalRequestFailure(ctx, time.Second)
	var row model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-stream-failure").First(&row).Error)
	require.Equal(t, model.LogTypeError, row.Type)
	require.Zero(t, row.Quota)
	require.Zero(t, row.PromptTokens)
	require.Zero(t, row.CompletionTokens)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	require.NotContains(t, other, "unbilled_usage")
	diagnostics := other["admin_info"].(map[string]interface{})["unbilled_usage"].(map[string]interface{})
	require.EqualValues(t, 41093, diagnostics["prompt_tokens"])
	require.EqualValues(t, 1, diagnostics["completion_tokens"])
	require.EqualValues(t, 41094, diagnostics["total_tokens"])
	require.EqualValues(t, 2, diagnostics["web_search_requests"])
	require.Equal(t, true, diagnostics["input_tokens_estimated"])
	require.Equal(t, true, diagnostics["interrupted_output"])
	require.Equal(t, false, diagnostics["charged"])
}
