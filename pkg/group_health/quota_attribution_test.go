package grouphealth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestQuotaAttributionPreservesDenominatorAndUptime(t *testing.T) {
	now := time.Now()
	var stored []model.GroupHealthMetric
	c := newCollector(func(_ context.Context, rows []model.GroupHealthMetric) error { stored = rows; return nil })
	for i := 0; i < 120; i++ {
		r := NewRequest("quota-model", false)
		switch {
		case i < 90:
			r.Observe(&relaycommon.RelayInfo{UsingGroup: "quota-group"}, true, now)
		case i < 100:
			r.ObserveFailure("quota-group", types.NewError(errors.New("database failed"), types.ErrorCodeUpdateDataError))
		default:
			r.ObserveUnrecordedFailure("quota-group", types.NewErrorWithStatusCode(&types.QuotaRejection{Source: "token", Message: "fixture"}, types.ErrorCodePreConsumeTokenQuotaFailed, 403))
		}
		for _, sample := range r.Samples(now) {
			c.record(sample)
		}
	}
	require.NoError(t, c.flush(context.Background(), now))
	require.Len(t, stored, 1)
	m := summarize(stored[0])
	require.EqualValues(t, 100, m.RequestCount)
	require.EqualValues(t, 90, m.SuccessCount)
	require.EqualValues(t, 10, m.ServiceFailureCount)
	require.EqualValues(t, 20, m.ExcludedCount)
	require.EqualValues(t, 120, m.ObservedCount)
	require.Equal(t, float64(90), *m.SuccessRate)
	require.Equal(t, float64(90), *m.UptimeRate)
	only := model.GroupHealthMetric{ExcludedUserErrorCount: 20}
	m = summarize(only)
	require.Nil(t, m.SuccessRate)
	require.Nil(t, m.UptimeRate)
	// Low-volume probe supplementation remains separate from user quota exclusions.
	only.ProbeCount, only.ProbeSuccessCount, only.ProbeBucketCount, only.ProbeSuccessBucketCount = 1, 1, 1, 1
	m = summarize(only)
	require.Nil(t, m.SuccessRate)
	require.EqualValues(t, 1, m.UptimeSampleCount)
	require.Equal(t, float64(100), *m.UptimeRate)
	require.EqualValues(t, 20, m.ExcludedCount)
}

func TestQuotaRejectionCannotEraseServiceFailure(t *testing.T) {
	now := time.Now()
	quota := types.NewErrorWithStatusCode(&types.QuotaRejection{Source: "token", Message: "fixture"}, types.ErrorCodePreConsumeTokenQuotaFailed, 403)
	r := NewRequest("model", false)
	r.Failure("first")
	r.ObserveFailure("first", quota)
	r.ObserveUnrecordedFailure("second", quota)
	require.Equal(t, OutcomeServiceFailure, r.groups["first"].Outcome)
	require.Equal(t, OutcomeUserError, r.groups["second"].Outcome)
	r.Observe(&relaycommon.RelayInfo{UsingGroup: "second"}, true, now)
	require.Equal(t, OutcomeSuccess, r.groups["second"].Outcome)
}
