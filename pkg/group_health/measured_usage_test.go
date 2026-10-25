package grouphealth

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestMeasuredUsageUsesSuccessfulFinalAttemptAndEffectiveOutput(t *testing.T) {
	start := time.Unix(1800000000, 0)
	r := NewRequest("m", true)
	info := &relaycommon.RelayInfo{UsingGroup: "g", StartTime: start, GroupHealthFirstOutputTime: start.Add(time.Second), GroupHealthTokenUsage: &relaycommon.GroupHealthTokenUsage{InputTokens: 999, OutputTokens: 999}}
	r.Observe(info, false, start.Add(2*time.Second))
	ResetAttempt(info)
	require.Nil(t, info.GroupHealthTokenUsage)
	info.GroupHealthFirstOutputTime = start.Add(3 * time.Second)
	info.GroupHealthTokenUsage = &relaycommon.GroupHealthTokenUsage{InputTokens: 100, OutputTokens: 20}
	info.GroupHealthUsageObservedAt = start.Add(5 * time.Second)
	r.Observe(info, true, start.Add(7*time.Second))
	info.GroupHealthTokenUsage.OutputTokens = 999
	samples := r.Samples(start)
	require.Len(t, samples, 1)
	require.EqualValues(t, 20, samples[0].TokenUsage.OutputTokens, "retained usage is immutable")
	require.EqualValues(t, 2000, *samples[0].GenerationMs)
	require.EqualValues(t, 3000, *samples[0].TTFTMs)

	c := newCollector(func(context.Context, []model.GroupHealthMetric) error { return nil })
	c.record(samples[0])
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeSuccess, At: start, TokenUsage: &relaycommon.GroupHealthTokenUsage{InputTokens: 50, OutputTokens: 30}, GenerationMs: common.GetPointer[int64](1000)})
	c.record(Sample{Group: "g", Model: "m", Outcome: OutcomeSuccess, At: start, TokenUsage: &relaycommon.GroupHealthTokenUsage{InputTokens: 50}})
	for _, outcome := range []Outcome{OutcomeServiceFailure, OutcomeClientCanceled, OutcomeUserError} {
		c.record(Sample{Group: "g", Model: "m", Outcome: outcome, At: start, TokenUsage: &relaycommon.GroupHealthTokenUsage{InputTokens: 999, OutputTokens: 999}, GenerationMs: common.GetPointer[int64](1)})
	}
	for _, row := range c.buckets {
		stats := summarize(row)
		require.EqualValues(t, 3, stats.UsageSampleCount)
		require.EqualValues(t, 200, *stats.InputTokens)
		require.EqualValues(t, 50, *stats.OutputTokens)
		require.EqualValues(t, 250, *stats.TotalTokens)
		require.EqualValues(t, 2, stats.ThroughputSampleCount)
		require.InDelta(t, 50.0/3, *stats.AvgOutputTPS, 0.0001, "speed is weighted by generation time")
	}
}

func TestMeasuredUsageMissingTimingRemainsUnknown(t *testing.T) {
	start := time.Unix(1800000000, 0)
	for _, stream := range []bool{false, true} {
		r := NewRequest("m", stream)
		info := &relaycommon.RelayInfo{UsingGroup: "g", StartTime: start, FirstResponseTime: start.Add(time.Second), GroupHealthTokenUsage: &relaycommon.GroupHealthTokenUsage{InputTokens: 100, OutputTokens: 10}}
		r.Observe(info, true, start.Add(2*time.Second))
		sample := r.Samples(start)[0]
		require.NotNil(t, sample.TokenUsage)
		require.Nil(t, sample.GenerationMs, "legacy first packet is not an effective output timestamp")
	}
	legacy := summarize(model.GroupHealthMetric{RequestCount: 10, SuccessCount: 9})
	require.Nil(t, legacy.InputTokens)
	require.Nil(t, legacy.OutputTokens)
	require.Nil(t, legacy.TotalTokens)
	require.Nil(t, legacy.AvgOutputTPS)
	measuredZero := summarize(model.GroupHealthMetric{UsageSampleCount: 1, InputTokens: 100})
	require.NotNil(t, measuredZero.OutputTokens)
	require.Zero(t, *measuredZero.OutputTokens)
	require.Nil(t, measuredZero.AvgOutputTPS)
}

func TestHealthWindowIncludesSeventyTwoExplicitHourlyBuckets(t *testing.T) {
	start := int64(1800000000) / BucketSeconds * BucketSeconds
	result := buildResult([]string{"g"}, []model.GroupHealthMetric{
		{GroupName: "g", BucketTs: start, RequestCount: 1, SuccessCount: 1, UsageSampleCount: 1, InputTokens: 10, OutputTokens: 5},
		{GroupName: "g", BucketTs: start - BucketSeconds, RequestCount: 1000, UsageSampleCount: 1, InputTokens: 999},
	}, start, start+71*BucketSeconds+100)
	require.Equal(t, 72, result.WindowHours)
	require.Equal(t, "current_hour_and_previous_71_hours", result.WindowScope)
	require.Len(t, result.Groups[0].Series, 72)
	require.EqualValues(t, 15, *result.Groups[0].TotalTokens)
	require.Nil(t, result.Groups[0].Series[1].TotalTokens)
	require.Nil(t, result.Groups[0].Series[71].SuccessRate)
}
