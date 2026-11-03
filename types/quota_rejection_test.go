package types

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQuotaRejectionReason(t *testing.T) {
	for _, tc := range []struct {
		value QuotaRejection
		want  string
	}{
		{QuotaRejection{Known: true, Available: 0, Required: 100}, QuotaReasonExhausted},
		{QuotaRejection{Known: true, Available: -1, Required: 100}, QuotaReasonExhausted},
		{QuotaRejection{Known: true, Available: 99, Required: 100}, QuotaReasonRequestTooLarge},
		{QuotaRejection{Known: true, Available: 0, Required: 100, Reason: QuotaReasonInFlight}, QuotaReasonInFlight},
		{QuotaRejection{Available: 0, Required: 100}, QuotaReasonUnknown},
	} {
		require.Equal(t, tc.want, tc.value.RejectionReason())
	}
}
