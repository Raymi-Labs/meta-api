package grouphealth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestRequestFoldsRetriesByActualGroup(t *testing.T) {
	start := time.Unix(1800000000, 0)
	now := start.Add(3 * time.Second)
	r := NewRequest("client-model", true)
	r.Failure("a")
	info := &relaycommon.RelayInfo{UsingGroup: "a", StartTime: start, GroupHealthFirstOutputTime: start.Add(time.Second)}
	r.Observe(info, false, now)
	info.UsingGroup = "b"
	r.Observe(info, false, now)
	info.GroupHealthFirstOutputTime = start.Add(2 * time.Second)
	r.Observe(info, true, now)
	samples := r.Samples(now)
	require.Len(t, samples, 2)
	byGroup := map[string]Sample{}
	for _, sample := range samples {
		byGroup[sample.Group] = sample
	}
	require.NotEqual(t, OutcomeSuccess, byGroup["a"].Outcome)
	require.Nil(t, byGroup["a"].TTFTMs)
	require.Equal(t, OutcomeSuccess, byGroup["b"].Outcome)
	require.EqualValues(t, 2000, *byGroup["b"].TTFTMs)
	require.Equal(t, "client-model", byGroup["b"].Model)
	r.Failure("auto")
	require.Len(t, r.Samples(now), 2)
}

func TestCacheUsageWeightsTokensAndExcludesFailedAttempts(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := NewRequest("m", false)
	info := &relaycommon.RelayInfo{UsingGroup: "failed", GroupHealthCacheUsage: &relaycommon.GroupHealthCacheUsage{ReadTokens: 999, InputTokens: 999}}
	r.Observe(info, false, now)
	ResetAttempt(info)
	require.Nil(t, info.GroupHealthCacheUsage)
	info.UsingGroup = "g"
	info.GroupHealthCacheUsage = &relaycommon.GroupHealthCacheUsage{ReadTokens: 900, InputTokens: 1000}
	r.Observe(info, false, now)
	ResetAttempt(info)
	info.GroupHealthCacheUsage = &relaycommon.GroupHealthCacheUsage{ReadTokens: 80, InputTokens: 100}
	r.Observe(info, true, now)
	info.GroupHealthCacheUsage.ReadTokens = 99 // retained samples must be copies
	c := newCollector(func(context.Context, []model.GroupHealthMetric) error { return nil })
	for _, sample := range r.Samples(now) {
		c.record(sample)
	}
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeSuccess, At: now.Add(time.Hour), CacheUsage: &relaycommon.GroupHealthCacheUsage{ReadTokens: 30, InputTokens: 60}})
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeSuccess, At: now.Add(time.Hour), CacheUsage: &relaycommon.GroupHealthCacheUsage{InputTokens: 110}})
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeSuccess, At: now}) // legacy/missing usage
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeUnknownFailure, At: now, CacheUsage: &relaycommon.GroupHealthCacheUsage{ReadTokens: 999, InputTokens: 999}})
	var rows []model.GroupHealthMetric
	for _, row := range c.buckets {
		rows = append(rows, row)
	}
	start := now.Unix() / BucketSeconds * BucketSeconds
	result := buildResult([]string{"g", "failed"}, rows, start, start+23*BucketSeconds)
	require.Nil(t, result.Groups[0].CacheHitRate)
	g := result.Groups[1]
	require.EqualValues(t, 3, g.CacheSampleCount)
	require.InDelta(t, 110.0/270*100, *g.CacheHitRate, 0.00001)
	require.Equal(t, 80.0, *g.Series[0].CacheHitRate)
	require.Nil(t, g.Series[2].CacheHitRate)
	zero := summarize(model.GroupHealthMetric{CacheInputTokens: 110, CacheSampleCount: 1})
	require.NotNil(t, zero.CacheHitRate)
	require.Zero(t, *zero.CacheHitRate)
}

func TestFailedOrMissingEffectiveStreamNeverContributesLatency(t *testing.T) {
	start := time.Now().Add(-time.Second)
	for _, reason := range []relaycommon.StreamEndReason{relaycommon.StreamEndReasonScannerErr, relaycommon.StreamEndReasonTimeout, relaycommon.StreamEndReasonClientGone} {
		t.Run(string(reason), func(t *testing.T) {
			r := NewRequest("m", true)
			info := &relaycommon.RelayInfo{UsingGroup: "g", StartTime: start, GroupHealthFirstOutputTime: start.Add(time.Millisecond), StreamStatus: relaycommon.NewStreamStatus()}
			info.StreamStatus.SetEndReason(reason, nil)
			r.Observe(info, true, time.Now())
			sample := r.Samples(time.Now())[0]
			require.NotEqual(t, OutcomeSuccess, sample.Outcome)
			require.Nil(t, sample.TTFTMs)
		})
	}
	r := NewRequest("m", true)
	info := &relaycommon.RelayInfo{UsingGroup: "g", StartTime: start, FirstResponseTime: start.Add(time.Millisecond), StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	r.Observe(info, true, time.Now())
	require.Equal(t, OutcomeSuccess, r.Samples(time.Now())[0].Outcome)
	require.Nil(t, r.Samples(time.Now())[0].TTFTMs, "legacy first packet must not become TTFT")
	info.StreamStatus.RecordError("in-band error despite HTTP 200")
	r = NewRequest("m", true)
	r.Observe(info, true, time.Now())
	require.NotEqual(t, OutcomeSuccess, r.Samples(time.Now())[0].Outcome)
	info.FirstEffectiveOutputTime = start.Add(10 * time.Millisecond)
	info.FirstEffectiveOutputChannelId = 12
	ResetAttempt(info)
	require.Nil(t, info.StreamStatus)
	require.True(t, info.GroupHealthFirstOutputTime.IsZero())
	require.Equal(t, start.Add(10*time.Millisecond), info.FirstEffectiveOutputTime, "existing routing guard timestamps must stay unchanged")
	require.Equal(t, 12, info.FirstEffectiveOutputChannelId)
}

func TestImageCompletionAndUnsupportedAsync(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	r := NewRequest("image", false)
	info := &relaycommon.RelayInfo{UsingGroup: "image", StartTime: start, RelayMode: relayconstant.RelayModeImagesGenerations}
	r.Observe(info, true, start.Add(2*time.Second))
	sample := r.Samples(time.Now())[0]
	require.EqualValues(t, 2000, *sample.CompletionMs)
	require.Nil(t, sample.TTFTMs)
	info.IsStream = true
	r = NewRequest("image", false)
	r.Observe(info, true, start.Add(2*time.Second))
	require.Nil(t, r.Samples(time.Now())[0].CompletionMs)
	for _, path := range []string{"/v1/videos", "/mj/submit/imagine", "/v1/realtime", "/v1/videos/id"} {
		require.False(t, SupportsRequest(http.MethodPost, path))
	}
	require.True(t, SupportsRequest(http.MethodPost, "/v1beta/models/gemini-test:streamGenerateContent"))
	require.False(t, SupportsRequest(http.MethodGet, "/v1/chat/completions"))
}

func TestCollectorRetryIsIdempotentAndRetainsConcurrentSamples(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	stored := map[string]model.GroupHealthMetric{}
	fail := true
	var c *collector
	concurrent := false
	c = newCollector(func(_ context.Context, rows []model.GroupHealthMetric) error {
		for _, row := range rows {
			stored[row.ID] = row
		}
		if concurrent {
			concurrent = false
			c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeSuccess, At: now})
		}
		if fail {
			return errors.New("ambiguous commit response")
		}
		return nil
	})
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeSuccess, TTFTMs: common.GetPointer[int64](120), At: now})
	require.Error(t, c.flush(ctx, now.Add(time.Hour)))
	require.True(t, c.persistFailed.Load())
	require.Len(t, c.buckets, 1)
	fail = false
	concurrent = true
	require.NoError(t, c.flush(ctx, now.Add(time.Hour)))
	require.Len(t, c.buckets, 1, "sample received during save must stay pending")
	require.NoError(t, c.flush(ctx, now.Add(time.Hour)))
	require.Empty(t, c.buckets)
	require.False(t, c.persistFailed.Load())
	require.Len(t, stored, 1)
	for _, row := range stored {
		require.EqualValues(t, 2, row.RequestCount)
		require.EqualValues(t, 2, row.SuccessCount)
		require.EqualValues(t, 1, row.TTFTCount)
	}
}

func TestCollectorCapacityAndWeightedHourlySummary(t *testing.T) {
	c := newCollector(func(context.Context, []model.GroupHealthMetric) error { return nil })
	c.record(Sample{Group: "g", Model: strings.Repeat("x", 256), Outcome: OutcomeSuccess})
	require.Empty(t, c.buckets, "oversized names must not poison an entire batch")
	require.EqualValues(t, 1, c.dropped.Load())
	for i := 0; i < maxHotBuckets; i++ {
		c.buckets[bucketKey{ts: int64(i)}] = model.GroupHealthMetric{}
	}
	c.record(Sample{Group: "g", Model: "new", Outcome: OutcomeSuccess})
	require.EqualValues(t, 2, c.dropped.Load())
	require.Len(t, c.buckets, maxHotBuckets)
	start := int64(1800000000) / BucketSeconds * BucketSeconds
	result := buildResult([]string{"g", "empty"}, []model.GroupHealthMetric{
		{GroupName: "g", BucketTs: start, RequestCount: 1, SuccessCount: 0},
		{GroupName: "g", BucketTs: start + BucketSeconds, RequestCount: 99, SuccessCount: 99, TTFTSumMs: 19800, TTFTCount: 99},
		{GroupName: "private", BucketTs: start, RequestCount: 10000, SuccessCount: 10000},
	}, start, start+23*BucketSeconds+60)
	require.Len(t, result.Groups, 2)
	require.Len(t, result.Groups[0].Series, 24)
	require.Nil(t, result.Groups[0].SuccessRate)
	require.Equal(t, "g", result.Groups[1].Group)
	require.Equal(t, 99.0, *result.Groups[1].SuccessRate)
	require.Equal(t, 200.0, *result.Groups[1].AvgTTFTMs)
	require.EqualValues(t, 1, result.Groups[1].FailureCount)
}

func TestCollectorOutcomesKeepUnknownAndClientCancellationInDenominator(t *testing.T) {
	now := time.Unix(1800000000, 0)
	c := newCollector(func(context.Context, []model.GroupHealthMetric) error { return nil })
	for _, outcome := range []Outcome{OutcomeSuccess, OutcomeServiceFailure, OutcomeUnknownFailure, OutcomeClientCanceled, OutcomeUserError, "", "future-unknown-outcome"} {
		c.record(Sample{
			Group: "g", Model: "m", Outcome: outcome, At: now,
			TTFTMs: common.GetPointer[int64](120), CompletionMs: common.GetPointer[int64](500),
			CacheUsage: &relaycommon.GroupHealthCacheUsage{ReadTokens: 50, InputTokens: 100},
		})
	}
	require.Len(t, c.buckets, 1)
	for _, row := range c.buckets {
		result := summarize(row)
		require.EqualValues(t, 7, result.ObservedCount)
		require.EqualValues(t, 6, result.RequestCount)
		require.EqualValues(t, 1, result.SuccessCount)
		require.EqualValues(t, 5, result.FailureCount)
		require.EqualValues(t, 1, result.ServiceFailureCount)
		require.EqualValues(t, 4, result.UnknownFailureCount)
		require.EqualValues(t, 1, result.ClientCanceledCount)
		require.EqualValues(t, 1, result.ExcludedUserErrorCount)
		require.EqualValues(t, 1, result.ExcludedCount)
		require.InDelta(t, 100.0/6, *result.SuccessRate, 0.00001)
		require.EqualValues(t, 1, result.TTFTCount)
		require.Equal(t, 120.0, *result.AvgTTFTMs)
		require.EqualValues(t, 1, result.CompletionCount)
		require.EqualValues(t, 1, result.CacheSampleCount)
	}
}

func TestCollectorExcludedOnlySnapshotsSurviveRetryAndConcurrentSample(t *testing.T) {
	now := time.Unix(1800000000, 0)
	stored := map[string]model.GroupHealthMetric{}
	fail, concurrent := true, false
	var c *collector
	c = newCollector(func(_ context.Context, rows []model.GroupHealthMetric) error {
		for _, row := range rows {
			stored[row.ID] = row
		}
		if concurrent {
			concurrent = false
			c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeUserError, At: now})
		}
		if fail {
			return errors.New("ambiguous commit response")
		}
		return nil
	})
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeUserError, At: now})
	require.Error(t, c.flush(context.Background(), now.Add(time.Hour)))
	require.Len(t, c.buckets, 1, "excluded-only buckets still require durable snapshots")
	fail, concurrent = false, true
	require.NoError(t, c.flush(context.Background(), now.Add(time.Hour)))
	require.Len(t, c.buckets, 1, "concurrent exclusions must stay pending even when request_count is unchanged")
	require.NoError(t, c.flush(context.Background(), now.Add(time.Hour)))
	require.Empty(t, c.buckets)
	require.Len(t, stored, 1, "retries must overwrite the original snapshot ID")
	for _, row := range stored {
		require.Zero(t, row.RequestCount)
		require.EqualValues(t, 2, row.ExcludedUserErrorCount)
		result := summarize(row)
		require.Nil(t, result.SuccessRate)
		require.EqualValues(t, 2, result.ObservedCount)
		require.Zero(t, result.FailureCount)
		require.Zero(t, result.UnknownFailureCount)
	}
}

func TestHourlyAttributionPreservesLegacyUnknownAndExcludedOnlyGroups(t *testing.T) {
	start := int64(1800000000) / BucketSeconds * BucketSeconds
	result := buildResult([]string{"g", "excluded"}, []model.GroupHealthMetric{
		{GroupName: "g", BucketTs: start, RequestCount: 10, SuccessCount: 6},
		{GroupName: "g", BucketTs: start + BucketSeconds, RequestCount: 5, SuccessCount: 2, ServiceFailureCount: 1, ClientCanceledCount: 1, ExcludedUserErrorCount: 3},
		{GroupName: "excluded", BucketTs: start, ExcludedUserErrorCount: 2},
		{GroupName: "private", BucketTs: start, RequestCount: 100, ServiceFailureCount: 100, ExcludedUserErrorCount: 100},
		{GroupName: "g", BucketTs: start - BucketSeconds, RequestCount: 100, ServiceFailureCount: 100},
	}, start, start+23*BucketSeconds+60)
	require.Equal(t, WindowScope, result.WindowScope)
	require.Equal(t, MetricScope, result.MetricScope)
	require.Len(t, result.Groups, 2)
	excluded, group := result.Groups[0], result.Groups[1]
	require.Equal(t, "excluded", excluded.Group)
	require.Nil(t, excluded.SuccessRate)
	require.EqualValues(t, 2, excluded.ObservedCount)
	require.EqualValues(t, 2, excluded.ExcludedCount)
	require.Zero(t, excluded.RequestCount)
	require.EqualValues(t, 15, group.RequestCount)
	require.EqualValues(t, 8, group.SuccessCount)
	require.EqualValues(t, 7, group.FailureCount)
	require.EqualValues(t, 1, group.ServiceFailureCount)
	require.EqualValues(t, 6, group.UnknownFailureCount)
	require.EqualValues(t, 1, group.ClientCanceledCount)
	require.EqualValues(t, 3, group.ExcludedUserErrorCount)
	require.EqualValues(t, 18, group.ObservedCount)
	require.InDelta(t, 800.0/15, *group.SuccessRate, 0.00001)
	require.EqualValues(t, 4, group.Series[0].UnknownFailureCount, "legacy failures are never assigned a cause")
	require.EqualValues(t, 2, group.Series[1].UnknownFailureCount, "client cancellation is part of the unknown failures")
	var observed, failures, unknown int64
	for _, bucket := range group.Series {
		observed += bucket.ObservedCount
		failures += bucket.FailureCount
		unknown += bucket.UnknownFailureCount
	}
	require.Equal(t, group.ObservedCount, observed)
	require.Equal(t, group.FailureCount, failures)
	require.Equal(t, group.UnknownFailureCount, unknown)
}

func TestQueryWindowIsCurrentHourAndPrevious23Buckets(t *testing.T) {
	for _, secondsIntoHour := range []int64{0, 1, 1800, 3599} {
		hour := int64(1800000000) / BucketSeconds * BucketSeconds
		now := time.Unix(hour+secondsIntoHour, 0)
		result, err := Query(context.Background(), nil, "", 1, now)
		require.NoError(t, err)
		require.Equal(t, hour-23*BucketSeconds, result.StartTime)
		require.Equal(t, now.Unix(), result.EndTime)
		require.Equal(t, WindowScope, result.WindowScope)
		require.Less(t, result.EndTime-result.StartTime, 24*BucketSeconds)
		withGroup := buildResult([]string{"g"}, nil, result.StartTime, result.EndTime)
		require.Len(t, withGroup.Groups[0].Series, 24)
		require.Equal(t, hour, withGroup.Groups[0].Series[23].Ts)
	}
}
