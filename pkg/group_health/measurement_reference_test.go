package grouphealth

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMeasurementReferencesCarryIndependentlyWithoutChangingSamples(t *testing.T) {
	const start = int64(1800000000)
	rows := []model.GroupHealthMetric{
		{GroupName: "g", ModelName: "m", BucketTs: start - 3600, RequestCount: 999, SuccessCount: 999, TTFTSumMs: 2400, TTFTCount: 2, CacheInputTokens: 100, CacheSampleCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: start, ProbeCount: 1, ProbeSuccessCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: start + 3600, RequestCount: 1, SuccessCount: 1, TTFTSumMs: 2500, TTFTCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: start + 7200, RequestCount: 1, SuccessCount: 1, CompletionSumMs: 8000, CompletionCount: 1, CacheReadTokens: 90, CacheInputTokens: 100, CacheSampleCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: start + 10800, RequestCount: 1, SuccessCount: 1, TTFTCount: 1, CacheInputTokens: 100, CacheSampleCount: 1},
		{GroupName: "future-only", ModelName: "m", BucketTs: start + 3600, TTFTSumMs: 9000, TTFTCount: 1},
		{GroupName: "expired", ModelName: "m", BucketTs: start - 200*3600, TTFTSumMs: 1000, TTFTCount: 1},
		{GroupName: "private", ModelName: "m", BucketTs: start, TTFTSumMs: 999999, TTFTCount: 1},
	}
	result := buildResult([]string{"g", "future-only", "expired"}, rows, start, start+23*3600)
	groups := make(map[string]Group)
	for _, group := range result.Groups {
		groups[group.Group] = group
	}
	g := groups["g"]
	require.Len(t, g.Series, 24)
	require.EqualValues(t, 3, g.RequestCount, "pre-window requests never enter totals")
	require.EqualValues(t, 2, g.TTFTCount)
	require.Equal(t, 1250.0, *g.AvgTTFTMs, "references do not affect measured average")
	require.Equal(t, 45.0, *g.CacheHitRate)
	first := g.Series[0]
	require.Nil(t, first.AvgTTFTMs)
	require.Nil(t, first.CacheHitRate)
	require.Zero(t, first.TTFTCount)
	require.Equal(t, &MeasurementReference{Value: 1200, Ts: start - 3600}, first.LastValid.TTFT)
	require.Equal(t, &MeasurementReference{Value: 0, Ts: start - 3600}, first.LastValid.Cache)
	require.Nil(t, first.LastValid.Completion)
	require.Equal(t, &MeasurementReference{Value: 2500, Ts: start + 3600}, g.Series[2].LastValid.TTFT)
	require.Equal(t, &MeasurementReference{Value: 90, Ts: start + 7200}, g.Series[2].LastValid.Cache)
	require.Equal(t, &MeasurementReference{Value: 8000, Ts: start + 7200}, g.Series[2].LastValid.Completion)
	require.Equal(t, &MeasurementReference{Value: 0, Ts: start + 10800}, g.Series[4].LastValid.TTFT)
	require.Equal(t, &MeasurementReference{Value: 0, Ts: start + 10800}, g.Series[4].LastValid.Cache)
	require.Nil(t, groups["future-only"].Series[0].LastValid, "later measurements never fill earlier hours")
	require.Nil(t, groups["expired"].LastValid, "references expire with retention")
	require.EqualValues(t, 1, g.ProbeBucketCount)
	require.EqualValues(t, 4, g.UptimeSampleCount)
}

func TestMeasurementReferenceQueriesHonorScopeAndWindow(t *testing.T) {
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() { model.DB = previous; model.InvalidateModelPermissionCache() })
	require.NoError(t, db.AutoMigrate(&model.GroupHealthMetric{}, &model.ModelPermission{}))
	permission := model.ModelPermission{ModelName: "hidden", NameRule: model.NameRuleExact, VisibilityScope: model.ModelPermissionScopeAdminOnly, CallScope: model.ModelPermissionScopeAll}
	require.NoError(t, permission.Insert())
	now := time.Unix(1800000000, 0)
	hour := now.Unix() / 3600 * 3600
	rows := []model.GroupHealthMetric{
		{ID: "old-a", GroupName: "public", ModelName: "m", BucketTs: hour - 80*3600, RequestCount: 99, SuccessCount: 99, TTFTSumMs: 1000, TTFTCount: 1, CacheInputTokens: 100, CacheSampleCount: 1, ThroughputOutputTokens: 100, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{ID: "old-b", GroupName: "own", ModelName: "m", BucketTs: hour - 80*3600, TTFTSumMs: 9000, TTFTCount: 3, ThroughputOutputTokens: 200, GenerationSumMs: 4000, ThroughputSampleCount: 1},
		{ID: "other", GroupName: "public", ModelName: "other", BucketTs: hour - 3600, TTFTSumMs: 5555, TTFTCount: 1, ThroughputOutputTokens: 5555, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{ID: "private", GroupName: "private", ModelName: "m", BucketTs: hour - 3600, TTFTSumMs: 8888, TTFTCount: 1, ThroughputOutputTokens: 8888, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{ID: "hidden", GroupName: "public", ModelName: "hidden", BucketTs: hour, TTFTSumMs: 9999, TTFTCount: 1, ThroughputOutputTokens: 9999, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{ID: "probe", GroupName: "public", ModelName: "m", BucketTs: hour, ProbeCount: 1, ProbeSuccessCount: 1},
	}
	require.NoError(t, model.SaveGroupHealthSnapshots(context.Background(), rows))
	for _, hours := range []int{24, 72} {
		result, err := Query(context.Background(), []string{"public"}, "m", common.RoleCommonUser, now, hours)
		require.NoError(t, err)
		g := result.Groups[0]
		require.Len(t, g.Series, hours)
		require.Zero(t, g.RequestCount)
		require.Nil(t, g.AvgTTFTMs)
		require.Nil(t, g.AvgOutputTPS)
		require.Zero(t, g.ThroughputSampleCount)
		require.Equal(t, &MeasurementReference{Value: 100, Ts: hour - 80*3600}, g.LastValid.OutputTPS)
		require.Equal(t, &MeasurementReference{Value: 1000, Ts: hour - 80*3600}, g.LastValid.TTFT)
		require.Equal(t, g.LastValid, g.Series[0].LastValid)
		batch, err := QueryModels(context.Background(), []string{"public"}, []string{"m", "hidden"}, common.RoleCommonUser, hours, now)
		require.NoError(t, err)
		require.Len(t, batch.Models, 1)
		require.Equal(t, g.Metrics, batch.Models[0].Metrics)
	}
	batch, err := QueryModels(context.Background(), []string{"public", "own"}, []string{"m"}, common.RoleCommonUser, 72, now)
	require.NoError(t, err)
	require.Equal(t, 2500.0, batch.Models[0].LastValid.TTFT.Value, "replica/group values at the latest hour are weighted before selection")
	require.Equal(t, 60.0, batch.Models[0].LastValid.OutputTPS.Value, "throughput weights generation time rather than averaging hourly rates")
	all, err := Query(context.Background(), []string{"public"}, "", common.RoleCommonUser, now, 72)
	require.NoError(t, err)
	require.Equal(t, 5555.0, all.Groups[0].LastValid.TTFT.Value, "all-model references cannot borrow hidden model data")
	require.Equal(t, 5555.0, all.Groups[0].LastValid.OutputTPS.Value)
}

func TestThroughputReferencesRequireEvidenceAndNeverBackfillFuture(t *testing.T) {
	const start = int64(1800000000)
	rows := []model.GroupHealthMetric{
		{GroupName: "g", ModelName: "m", BucketTs: start - 3600, ThroughputOutputTokens: 123, GenerationSumMs: 1669, ThroughputSampleCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: start + 3600, TTFTSumMs: 1000, TTFTCount: 1},
		{GroupName: "g", ModelName: "m", BucketTs: start + 7200, ThroughputOutputTokens: 80, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{GroupName: "future", ModelName: "m", BucketTs: start + 3600, ThroughputOutputTokens: 80, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{GroupName: "expired", ModelName: "m", BucketTs: start - 200*3600, ThroughputOutputTokens: 80, GenerationSumMs: 1000, ThroughputSampleCount: 1},
	}
	result := buildResult([]string{"g", "future", "expired"}, rows, start, start+23*3600)
	g := result.Groups[2]
	require.InDelta(t, 123000.0/1669, g.Series[0].LastValid.OutputTPS.Value, 0.00001)
	require.Nil(t, g.Series[0].AvgOutputTPS)
	require.Zero(t, g.Series[0].ThroughputSampleCount)
	require.Equal(t, g.Series[0].LastValid.OutputTPS, g.Series[1].LastValid.OutputTPS, "TTFT alone must not replace throughput history")
	require.Equal(t, 80.0, g.LastValid.OutputTPS.Value)
	require.Equal(t, 80.0, *g.AvgOutputTPS, "old values must not enter current aggregates")
	require.EqualValues(t, 1, g.ThroughputSampleCount)
	require.Nil(t, result.Groups[1].Series[0].LastValid)
	require.Nil(t, result.Groups[0].LastValid)
	for _, row := range []model.GroupHealthMetric{
		{ThroughputOutputTokens: 10, GenerationSumMs: 1000},
		{ThroughputOutputTokens: 10, ThroughputSampleCount: 1},
		{ThroughputOutputTokens: -1, GenerationSumMs: 1000, ThroughputSampleCount: 1},
	} {
		var r MeasurementReferences
		r.observe(row, start)
		require.Nil(t, r.snapshot())
	}
}
