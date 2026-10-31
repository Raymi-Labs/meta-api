package grouphealth

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestProbeFallbackFoldsUpstreamsAndReplicasBeforeWeighting(t *testing.T) {
	const ts = int64(1800000000)
	rows := []model.GroupHealthMetric{
		{GroupName: "a", ModelName: "m", BucketTs: ts, ProbeCount: 9},
		{GroupName: "a", ModelName: "m", BucketTs: ts, ProbeCount: 1, ProbeSuccessCount: 1},
		{GroupName: "a", ModelName: "m", BucketTs: ts + 3600, ProbeCount: 1, ProbeSuccessCount: 1},
		{GroupName: "a", ModelName: "m", BucketTs: ts + 3600, RequestCount: 2, ServiceFailureCount: 2},
		{GroupName: "a", ModelName: "m", BucketTs: ts + 7200, ProbeCount: 7},
		{GroupName: "b", ModelName: "m", BucketTs: ts, ProbeCount: 1},
		{GroupName: "private", ModelName: "m", BucketTs: ts, ProbeCount: 100, ProbeSuccessCount: 100},
	}
	result := buildResult([]string{"a", "b"}, rows, ts, ts+23*3600)
	a := result.Groups[0]
	require.Equal(t, 100.0, *a.Series[0].UptimeRate)
	require.EqualValues(t, 1, a.Series[0].UptimeSampleCount)
	require.Nil(t, a.Series[0].SuccessRate)
	require.Zero(t, a.Series[0].ObservedCount)
	require.Nil(t, a.Series[0].TotalTokens)
	require.InDelta(t, 100.0/3, *a.Series[1].UptimeRate, 1e-9, "low-sample real failures retain their weight alongside one successful probe")
	require.EqualValues(t, 1, a.Series[1].ProbeBucketCount)
	require.Equal(t, 0.0, *a.Series[2].UptimeRate)
	require.Nil(t, a.Series[3].UptimeRate, "unknown hours never fill forward")
	require.EqualValues(t, 2, a.RequestCount)
	require.EqualValues(t, 3, a.ProbeBucketCount)
	require.EqualValues(t, 5, a.UptimeSampleCount)
	require.Equal(t, 40.0, *a.UptimeRate)
	require.Equal(t, 0.0, *a.SuccessRate)
	require.Equal(t, 0.0, *result.Groups[1].UptimeRate, "another group cannot borrow a passing upstream")
	rows = append(rows, model.GroupHealthMetric{GroupName: "a", ModelName: "other", BucketTs: ts, ProbeCount: 1})
	result = buildResult([]string{"a"}, rows, ts, ts+23*3600)
	require.Equal(t, 50.0, *result.Groups[0].Series[0].UptimeRate, "different models remain independent")
}

func TestProbeOnlySnapshotsRetryAndPersistSuccessAfterFailure(t *testing.T) {
	now := time.Unix(1800000000, 0)
	var saved []model.GroupHealthMetric
	fail := true
	c := newCollector(func(_ context.Context, rows []model.GroupHealthMetric) error {
		if fail {
			return errors.New("offline")
		}
		saved = append(saved, rows...)
		return nil
	})
	c.record(Sample{Group: "g", Model: "m", Probe: true, Outcome: OutcomeServiceFailure, At: now})
	require.Error(t, c.flush(context.Background(), now))
	fail = false
	require.NoError(t, c.flush(context.Background(), now))
	require.Len(t, saved, 1)
	id := saved[0].ID
	require.EqualValues(t, 1, saved[0].ProbeCount)
	require.Zero(t, saved[0].RequestCount)
	c.record(Sample{Group: "g", Model: "m", Probe: true, Outcome: OutcomeSuccess, At: now})
	require.NoError(t, c.flush(context.Background(), now))
	require.Len(t, saved, 2)
	require.Equal(t, id, saved[1].ID)
	require.EqualValues(t, 2, saved[1].ProbeCount)
	require.EqualValues(t, 1, saved[1].ProbeSuccessCount)
	require.Zero(t, saved[1].SuccessCount)
	require.NoError(t, c.flush(context.Background(), now.Add(time.Hour)))
	require.Len(t, saved, 2)
	require.Empty(t, c.buckets)
}

func TestProbeSupplementUsesEffectiveHourlySampleThreshold(t *testing.T) {
	const ts = int64(1800000000)
	for _, count := range []int64{0, 1, 29, 30, 31} {
		for _, passed := range []int64{0, 1} {
			rows := []model.GroupHealthMetric{{GroupName: "g", ModelName: "m", BucketTs: ts, RequestCount: count, SuccessCount: count, ExcludedUserErrorCount: 1000, ProbeCount: 12, ProbeSuccessCount: passed}}
			got := buildResult([]string{"g"}, rows, ts, ts+23*3600).Groups[0]
			supplement := int64(0)
			if count < 30 {
				supplement = 1
			}
			require.Equal(t, supplement, got.ProbeBucketCount, "requests=%d passed=%d", count, passed)
			require.Equal(t, count+supplement, got.UptimeSampleCount)
			require.InDelta(t, float64(count+supplement*passed)/float64(count+supplement)*100, *got.UptimeRate, 1e-9)
			require.Equal(t, count, got.RequestCount)
			require.Equal(t, count, got.SuccessCount)
			if count > 0 {
				require.Equal(t, 100.0, *got.SuccessRate)
			}
		}
	}
	// A separate process's request can bring the same hour to the threshold.
	rows := []model.GroupHealthMetric{
		{GroupName: "g", ModelName: "m", BucketTs: ts, RequestCount: 29, ProbeCount: 1, ProbeSuccessCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: ts, RequestCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: ts + 3600, RequestCount: 29, ProbeCount: 1, ProbeSuccessCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: ts + 7200, RequestCount: 1},
	}
	got := buildResult([]string{"g"}, rows, ts, ts+23*3600).Groups[0]
	require.Zero(t, got.Series[0].ProbeBucketCount)
	require.Equal(t, 0.0, *got.Series[0].UptimeRate)
	require.EqualValues(t, 1, got.Series[1].ProbeBucketCount, "window total over 30 must not suppress a low-sample hour")
	require.Zero(t, got.Series[2].ProbeBucketCount, "no tests means no supplemental evidence")
	require.Equal(t, 0.0, *got.Series[2].UptimeRate)
	require.EqualValues(t, 1, got.ProbeBucketCount)
}
