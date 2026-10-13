package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestEmptyResponsesFailureDiagnosticsAreBoundedSnapshots(t *testing.T) {
	ctx := failureLogContext(t)
	ctx.Writer.WriteHeaderNow()
	secret := "sk-private-value-never-log"
	summary := &relaycommon.ResponsesCompletedSummary{
		Status: secret, OutputCount: 25, MessageCount: 1, MessageTextChars: 17,
		OutputTypes:      []string{"message", secret, strings.Repeat("private-data", 2000), "message"},
		IncompleteReason: secret,
	}
	CaptureResponsesEmptyOutputDiagnostics(nil, summary, 2)
	CaptureResponsesEmptyOutputDiagnostics(ctx, summary, 2)
	summary.OutputCount = 999
	summary.OutputTypes[0] = secret
	err := types.NewOpenAIError(errors.New(ResponsesStreamEmptyCompletedReason), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	MarkRequestFailure(ctx, err)
	RecordFinalRequestFailure(ctx, time.Second)
	var row model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-stream-failure").First(&row).Error)
	require.Zero(t, row.Quota)
	require.NotContains(t, row.Other, secret)
	require.NotContains(t, row.Other, "private-data")
	details, e := common.StrToMap(row.Other)
	require.NoError(t, e)
	require.Equal(t, "empty_response", details["failure_category"])
	diagnostics := details["admin_info"].(map[string]interface{})["responses_stream_diagnostics"].(map[string]interface{})
	require.EqualValues(t, 2, diagnostics["json_decode_errors"])
	require.Equal(t, true, diagnostics["response_written"])
	require.Equal(t, true, diagnostics["completed_response_present"])
	completed := diagnostics["responses_completed_summary"].(map[string]interface{})
	require.EqualValues(t, 25, completed["output_count"])
	require.Equal(t, "unknown", completed["status"])
	require.Equal(t, []interface{}{"message", "unknown"}, completed["output_types"])
	logs, total, e := model.GetUserLogs(992701, model.LogTypeError, 0, 0, "", "", 0, 10, "", "req-stream-failure")
	require.NoError(t, e)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.NotContains(t, logs[0].Other, "responses_stream_diagnostics")
	require.NotContains(t, logs[0].Other, "admin_info")
	require.Equal(t, "响应结束但未识别到有效输出", logs[0].Content)
}

func TestEmptyResponsesDiagnosticsLimitTypesAndHandleMissingResponse(t *testing.T) {
	ctx := failureLogContext(t)
	summary := &relaycommon.ResponsesCompletedSummary{Status: "completed", OutputTypes: []string{
		"message", "reasoning", "function_call", "web_search_call", "image_generation_call",
		"computer_call", "custom_tool_call", "file_search_call", "code_interpreter_call",
		"local_shell_call", "shell_call", "shell_call_output", "apply_patch_call",
		"apply_patch_call_output", "mcp_call", "mcp_list_tools", "mcp_approval_request",
	}}
	CaptureResponsesEmptyOutputDiagnostics(ctx, summary, 0)
	value, exists := ctx.Get(requestResponsesStreamFailureKey)
	require.True(t, exists)
	diagnostics := value.(map[string]interface{})
	completed := diagnostics["responses_completed_summary"].(map[string]interface{})
	require.Len(t, completed["output_types"], 16)
	require.Equal(t, true, completed["output_types_truncated"])
	require.Equal(t, false, diagnostics["response_written"])
	CaptureResponsesEmptyOutputDiagnostics(ctx, nil, 1)
	value, exists = ctx.Get(requestResponsesStreamFailureKey)
	require.True(t, exists)
	diagnostics = value.(map[string]interface{})
	require.Equal(t, false, diagnostics["completed_response_present"])
	require.NotContains(t, diagnostics, "responses_completed_summary", "a missing response must not inherit an earlier summary")
}
