package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekMissingThinkingDiagnostic(t *testing.T) {
	const message = "The `content[].thinking` in the thinking mode must be passed back to the API."
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{message, true},
		{message + " (request_id: 9a5ed54c-cb38-438b-a60f-75aadb15a652)", true},
		{" " + message + "\n", true},
		{"Example: " + message, false},
		{message + " Other error", false},
		{"each thinking block must contain thinking", false},
	} {
		require.Equal(t, tc.want, IsDeepSeekMissingThinkingHistory(tc.message))
	}
	err := types.WithOpenAIError(types.OpenAIError{Type: "invalid_request_error", Message: message}, 400)
	category, reason, hint := describeRequestFailure(err, true)
	require.Equal(t, "thinking_history", category)
	require.Equal(t, "对话历史缺少思考内容", reason)
	require.Contains(t, hint, "完整回传历史思考")
	require.NotContains(t, hint, "request_id")
	err.SetMessage(common.MessageWithRequestId(err.Error(), "local-fixture-request"))
	category, _, _ = describeRequestFailure(err, true)
	require.Equal(t, "thinking_history", category, "local response request ID must not change the original upstream classification")
	category, _, _ = describeRequestFailure(err, false)
	require.Equal(t, "invalid_request", category)
	wrongType := types.WithOpenAIError(types.OpenAIError{Type: "authentication_error", Message: message}, 400)
	category, _, _ = describeRequestFailure(wrongType, true)
	require.Equal(t, "invalid_request", category)
	err.StatusCode = 500
	category, _, _ = describeRequestFailure(err, true)
	require.Equal(t, "upstream_error", category)
}
