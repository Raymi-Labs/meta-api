package types

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestSetMessagePreservesCause(t *testing.T) {
	sentinel := errors.New("internal failure")
	rejected := &QuotaRejection{Source: "wallet", Available: 11379, Required: 42210, Known: true, Message: "quota rejected", Cause: sentinel}
	apiErr := NewErrorWithStatusCode(fmt.Errorf("billing: %w", rejected), ErrorCodeInsufficientUserQuota, 403)
	for _, message := range []string{"safe message", "safe message (request id: fixture)"} {
		apiErr.SetMessage(message)
		require.Equal(t, message, apiErr.Error())
		require.Equal(t, message, apiErr.Err.Error())
		require.ErrorIs(t, apiErr, sentinel)
		var got *QuotaRejection
		require.ErrorAs(t, apiErr, &got)
		require.Same(t, rejected, got)
		for _, wire := range []any{apiErr.ToOpenAIError(), apiErr.ToClaudeError()} {
			data, err := common.Marshal(wire)
			require.NoError(t, err)
			require.Contains(t, string(data), message)
			require.NotContains(t, string(data), "internal failure")
			require.NotContains(t, string(data), "quota rejected")
		}
	}
	empty := &NewAPIError{}
	empty.SetMessage("fallback")
	require.Equal(t, "fallback", empty.Error())
	var got *QuotaRejection
	require.False(t, errors.As(empty, &got))
}
