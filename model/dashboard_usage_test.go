package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDashboardUsageScreenshotAndCacheSemantics(t *testing.T) {
	a := newDashboardUsageAccumulator(1, 100, false)
	err := a.add(dashboardUsageLog{Type: LogTypeConsume, Quota: 92473150, PromptTokens: 5021, CompletionTokens: 183092,
		Other: `{"prompt_tokens_semantic":"non_cache","cache_tokens":21815522,"cache_creation_tokens":5078380,"cache_creation_tokens_5m":5078380,"model_ratio":66}`})
	require.NoError(t, err)
	s := a.finish().Summary
	require.EqualValues(t, 27082015, s.Tokens)
	require.InDelta(t, 6.829118882, *s.UnitPrice/500000, 0.000000001)

	for _, tc := range []struct {
		name, other          string
		prompt, output, want int
	}{
		{"inclusive", `{"prompt_tokens_semantic":"inclusive","cache_tokens":80,"cache_creation_tokens":10,"model_ratio":1}`, 100, 20, 120},
		{"cache-only", `{"prompt_tokens_semantic":"non_cache","cache_tokens":80,"cache_creation_tokens":20,"cache_creation_tokens_5m":5,"cache_creation_tokens_1h":15,"model_ratio":1}`, 0, 0, 100},
		{"split-only", `{"claude":true,"cache_creation_tokens_5m":5,"cache_creation_tokens_1h":15,"model_ratio":1}`, 3, 2, 25},
		{"legacy-claude", `{"claude":true,"cache_tokens":80,"model_ratio":1}`, 20, 2, 102},
		{"legacy-conversion", `{"claude":true,"cache_tokens":80,"model_ratio":1,"billing_mode":"tiered_expr","request_conversion":["Claude Messages","OpenAI Responses"],"tiered_params":{"len":100}}`, 100, 2, 102},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newDashboardUsageAccumulator(1, 100, false)
			require.NoError(t, a.add(dashboardUsageLog{Type: LogTypeConsume, Quota: 100, PromptTokens: tc.prompt, CompletionTokens: tc.output, Other: tc.other}))
			s := a.finish().Summary
			require.EqualValues(t, tc.want, s.Tokens)
			require.NotNil(t, s.UnitPrice)
		})
	}
}

func TestDashboardUsageMixedBillingRefundsAndAdjustments(t *testing.T) {
	a := newDashboardUsageAccumulator(1, 100, false)
	for _, row := range []dashboardUsageLog{
		{Type: LogTypeConsume, Quota: 100, PromptTokens: 100, Other: `{"model_ratio":1,"prompt_tokens_semantic":"inclusive"}`},
		{Type: LogTypeConsume, Quota: 500, PromptTokens: 1, Other: `{"model_price":9.9,"request_path":"/v1/images/generations"}`},
		{Type: LogTypeConsume, Quota: 50, Other: `{"task_id":"task1","billing_unit":"task"}`},
		{Type: LogTypeRefund, Quota: 800, Other: `{"task_id":"task1","billing_unit":"task"}`},
	} {
		require.NoError(t, a.add(row))
	}
	s := a.finish().Summary
	require.EqualValues(t, -150, s.Quota)
	require.EqualValues(t, 2, s.Requests)
	require.EqualValues(t, 101, s.Tokens)
	require.EqualValues(t, 100, s.PricedTokens)
	require.EqualValues(t, 100, s.TokenQuota)
	require.EqualValues(t, -250, s.OtherQuota)
	require.Equal(t, float64(1000000), *s.UnitPrice)
	rows := a.result.Rows
	require.Equal(t, -150, rows[0].Quota)
	require.Equal(t, 2, rows[0].Count)
}

func TestDashboardUsageUnknownAndCorruptEvidence(t *testing.T) {
	for _, other := range []string{`{"cache_tokens":80,"model_ratio":1}`, `{"prompt_tokens_semantic":"inclusive","cache_tokens":101,"model_ratio":1}`} {
		a := newDashboardUsageAccumulator(1, 100, false)
		require.NoError(t, a.add(dashboardUsageLog{Type: LogTypeConsume, PromptTokens: 100, Quota: 10, Other: other}))
		s := a.finish().Summary
		require.EqualValues(t, 1, s.UnclassifiedRecords)
		require.Nil(t, s.UnitPrice)
		require.Nil(t, s.CacheRate)
	}
	for _, other := range []string{`{broken`, `{"cache_tokens":-1}`, `{"cache_creation_tokens":1,"cache_creation_tokens_1h":2}`} {
		a := newDashboardUsageAccumulator(1, 100, false)
		require.Error(t, a.add(dashboardUsageLog{Type: LogTypeConsume, Other: other}))
	}
}

func TestDashboardUsagePartialMetricsHaveIndependentDenominators(t *testing.T) {
	a := newDashboardUsageAccumulator(1, 100, false)
	rows := []dashboardUsageLog{
		{Type: LogTypeConsume, Quota: 100, PromptTokens: 100, CompletionTokens: 20, Other: `{"model_ratio":1,"prompt_tokens_semantic":"inclusive","cache_tokens":80}`},
		// Unknown cached input: exclude both fee and tokens, not just denominator.
		{Type: LogTypeConsume, Quota: 900, PromptTokens: 100, Other: `{"model_ratio":1,"cache_tokens":80}`},
		// Output-only is valid for price, not an input cache rate observation.
		{Type: LogTypeConsume, Quota: 100, CompletionTokens: 80, Other: `{"model_ratio":1,"prompt_tokens_semantic":"inclusive"}`},
		{Type: LogTypeConsume, Quota: 700, Other: `{"model_ratio":1,"prompt_tokens_semantic":"inclusive"}`},
		{Type: LogTypeConsume, Quota: 500, PromptTokens: 10, Other: `{"model_price":9.9}`},
		// Clearly token-priced refund changes price only; never cache numerator/count.
		{Type: LogTypeRefund, Quota: 50, Other: `{"billing_unit":"token"}`},
	}
	for _, row := range rows {
		require.NoError(t, a.add(row))
	}
	s := a.finish().Summary
	require.EqualValues(t, 2250, s.Quota)
	require.EqualValues(t, 5, s.Requests)
	require.EqualValues(t, 150, s.TokenQuota)
	require.EqualValues(t, 2100, s.OtherQuota)
	require.EqualValues(t, 200, s.PricedTokens)
	require.InDelta(t, .8, *s.CacheRate, 1e-9)
	require.InDelta(t, 750000, *s.UnitPrice, 1e-9)
	require.EqualValues(t, 1, s.CacheIncludedRecords)
	require.EqualValues(t, 3, s.CacheExcludedRecords)
	require.EqualValues(t, 3, s.PriceIncludedRecords)
	require.EqualValues(t, 2, s.PriceExcludedRecords)
}

func TestDashboardUsageAllUnknownCannotBecomeFalseZero(t *testing.T) {
	a := newDashboardUsageAccumulator(1, 100, false)
	require.NoError(t, a.add(dashboardUsageLog{Type: LogTypeConsume, Quota: 99, PromptTokens: 1, Other: `{"model_ratio":1,"cache_tokens":200}`}))
	s := a.finish().Summary
	require.Nil(t, s.CacheRate)
	require.Nil(t, s.UnitPrice)
	require.EqualValues(t, 99, s.Quota)
	require.EqualValues(t, 99, s.OtherQuota)
	require.EqualValues(t, 1, s.CacheExcludedRecords)
	require.EqualValues(t, 1, s.PriceExcludedRecords)
	require.Zero(t, s.PricedTokens)
	require.Zero(t, s.CacheReadTokens)
}

func TestDashboardUsageExactRangeIdentityAndCancellation(t *testing.T) {
	setupCacheRateScanDB(t)
	// Both endpoints are inside hours. Filter events before bucketing.
	for _, row := range []Log{
		{UserId: 1, Username: "123", CreatedAt: 99, Type: LogTypeConsume, Quota: 999},
		{UserId: 1, Username: "123", CreatedAt: 100, Type: LogTypeConsume, Quota: 10, PromptTokens: 5, Other: `{"model_ratio":1}`},
		{UserId: 1, Username: "123", CreatedAt: 199, Type: LogTypeRefund, Quota: 20},
		{UserId: 1, Username: "123", CreatedAt: 200, Type: LogTypeConsume, Quota: 999},
		{UserId: 123, Username: "other", CreatedAt: 150, Type: LogTypeConsume, Quota: 999},
	} {
		require.NoError(t, LOG_DB.Create(&row).Error)
	}
	r, err := GetDashboardUsage(context.Background(), 100, 200, 1, "", false)
	require.NoError(t, err)
	require.EqualValues(t, -10, r.Summary.Quota)
	require.EqualValues(t, 1, r.Summary.Requests)
	r, err = GetDashboardUsage(context.Background(), 100, 200, 0, "123", false)
	require.NoError(t, err)
	require.EqualValues(t, -10, r.Summary.Quota)
	_, err = GetDashboardUsage(context.Background(), 100, 200, 1, "other", false)
	require.Error(t, err)
	r, err = GetDashboardUsage(context.Background(), 100, 200, 2, "", false)
	require.NoError(t, err)
	require.Empty(t, r.Rows)
	require.Nil(t, r.Summary.UnitPrice)
	r, err = scanDashboardUsage(context.Background(), 100, 200, 1, "", false, 1)
	require.ErrorContains(t, err, "记录过多")
	require.Empty(t, r.Rows)
	require.Zero(t, r.Summary.Quota)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetDashboardUsage(ctx, 100, 200, 0, "", false)
	require.Error(t, err)
	for _, bounds := range [][2]int64{{0, 1}, {100, 100}, {200, 100}, {1, 32 * 86400}} {
		_, err = GetDashboardUsage(context.Background(), bounds[0], bounds[1], 0, "", false)
		require.Error(t, err)
	}
}

func TestDashboardUsageBillingClassification(t *testing.T) {
	for _, other := range []string{
		`{"billing_unit":"request","model_ratio":1}`, `{"model_price":1,"model_ratio":1}`,
		`{"billing_unit":"token","request_path":"/v1/audio/speech"}`, `{"task_billing":{},"model_ratio":1}`,
		`{"task_id":"x","model_ratio":1}`, `{"billing_mode":"video_second"}`, `{}`,
	} {
		a := newDashboardUsageAccumulator(1, 100, false)
		require.NoError(t, a.add(dashboardUsageLog{Type: LogTypeConsume, Quota: 100, PromptTokens: 1, Other: other}))
		require.Nil(t, a.finish().Summary.UnitPrice, other)
	}
}

func TestDashboardUsageLimitDoesNotReturnPartialTotals(t *testing.T) {
	a := newDashboardUsageAccumulator(1, 100, false)
	for i := 0; i < dashboardUsageBucketLimit; i++ {
		a.buckets[dashboardUsageKey{Hour: int64(i + 1)}] = &QuotaData{}
	}
	err := a.add(dashboardUsageLog{Type: LogTypeConsume, ModelName: "new"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "分组过多"))
	dashboardUsageSlots <- struct{}{}
	dashboardUsageSlots <- struct{}{}
	t.Cleanup(func() { <-dashboardUsageSlots; <-dashboardUsageSlots })
	_, err = GetDashboardUsage(context.Background(), 1, 100, 0, "", false)
	require.ErrorContains(t, err, "繁忙")
}
