package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const unsupportedChatBudget = "Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead."

func TestBudgetRejectionLogKeepsEachAttemptPrivate(t *testing.T) {
	c := failureLogContext(t)
	storage, err := common.CreateBodyStorage([]byte(`{"model":"gpt-6.1-terra","max_tokens":37,"messages":[{"content":"private fixture"}]}`))
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	var final *types.NewAPIError
	for i, key := range []string{"max_completion_tokens", "max_tokens"} {
		wire := []byte(`{"model":"fixture-model","` + key + `":37}`)
		sent := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
		resp := &http.Response{Request: sent, StatusCode: 400}
		RecordChatBudgetRejection(c, sent, resp, nil, wire)
		final = types.WithOpenAIError(types.OpenAIError{Code: "unsupported_parameter", Param: "max_tokens", Message: unsupportedChatBudget}, 400)
		final.UpstreamStatusCode = 400
		final.Upstream = buildUpstreamDiagnostics(resp, 0)
		AppendRequestFailureAttempt(c, *types.NewChannelError(i+1, 1, "fixture", false, "", false), final)
	}
	MarkRequestFailure(c, final)
	RecordFinalRequestFailure(c, time.Second)
	var row model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", c.GetString(common.RequestIdKey)).First(&row).Error)
	require.Equal(t, "output_budget_parameter", gjson.Get(row.Other, "failure_category").String())
	require.Contains(t, gjson.Get(row.Other, "failure_hint").String(), "max_completion_tokens")
	require.Equal(t, "max_completion_tokens", gjson.Get(row.Other, "admin_info.attempts.0.upstream.output_budget.outbound_fields.0").String())
	require.Equal(t, "max_tokens", gjson.Get(row.Other, "admin_info.attempts.1.upstream.output_budget.outbound_fields.0").String())
	require.Equal(t, "max_tokens", gjson.Get(row.Other, "admin_info.upstream.output_budget.outbound_fields.0").String())
	require.NotContains(t, row.Other, "private fixture")
	rows, _, err := model.GetUserLogs(row.UserId, model.LogTypeError, 0, 0, "", "", 0, 10, "", row.RequestId)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotContains(t, rows[0].Other, "admin_info")
	require.NotContains(t, rows[0].Other, "fixture-model")
}

func TestChatBudgetHintRequiresExactUpstreamRefusal(t *testing.T) {
	for _, tc := range []struct {
		protocol, code, param, message string
		status                         int
		want                           bool
	}{
		{"chat/completions", "unsupported_parameter", "", unsupportedChatBudget, 400, true},
		{"chat/completions", "unsupported_parameter", "temperature", unsupportedChatBudget, 400, false},
		{"responses", "unsupported_parameter", "max_tokens", unsupportedChatBudget, 400, false},
		{"chat/completions", "unsupported_parameter", "max_tokens", unsupportedChatBudget, 429, false},
		{"chat/completions", "invalid_value", "max_tokens", unsupportedChatBudget, 400, false},
		{"chat/completions", "unsupported_parameter", "max_tokens", "max_tokens too large", 400, false},
	} {
		e := types.WithOpenAIError(types.OpenAIError{Code: tc.code, Param: tc.param, Message: tc.message}, tc.status)
		e.UpstreamStatusCode = tc.status
		e.Upstream = &types.UpstreamDiagnostics{OutputBudget: &types.ChatOutputBudget{Protocol: tc.protocol}}
		kind, _, _ := describeRequestFailure(e, true)
		require.Equal(t, tc.want, kind == "output_budget_parameter", tc)
	}
	require.Nil(t, inspectBudgetBody(nil).fields)
	require.Empty(t, inspectBudgetBody([]byte(`{"model":"sk-private-key"}`)).model)
	require.Equal(t, []string{}, inspectBudgetBody([]byte(`{"tools":[{"max_tokens":3}]}`)).fields)
	require.Nil(t, inspectBudgetBody([]byte("   ")).fields)
}

func TestChatBudgetDoesNotAttributeRedirectOrOtherEndpoint(t *testing.T) {
	c := failureLogContext(t)
	for _, path := range []string{"/v1/messages", "/v1/chat/completions"} {
		sent := httptest.NewRequest(http.MethodPost, "https://upstream.example"+path, nil)
		actual := sent.Clone(sent.Context())
		if path == "/v1/chat/completions" {
			actual.Response = &http.Response{StatusCode: 307}
		}
		resp := &http.Response{Request: actual, StatusCode: 400}
		RecordChatBudgetRejection(c, sent, resp, nil, []byte(`{"max_tokens":1}`))
		require.Nil(t, buildUpstreamDiagnostics(resp, 0).OutputBudget)
	}
}
