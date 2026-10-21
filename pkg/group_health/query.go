package grouphealth

import (
	"context"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const MetricScope = "per_request_per_group_final_outcome"
const WindowScope = "current_hour_and_previous_23_hours"

type Metrics struct {
	CacheHitRate           *float64 `json:"cache_hit_rate"`
	CacheSampleCount       int64    `json:"cache_sample_count"`
	RequestCount           int64    `json:"request_count"`
	SuccessCount           int64    `json:"success_count"`
	FailureCount           int64    `json:"failure_count"`
	ServiceFailureCount    int64    `json:"service_failure_count"`
	UnknownFailureCount    int64    `json:"unknown_failure_count"`
	ClientCanceledCount    int64    `json:"client_canceled_count"`
	ExcludedUserErrorCount int64    `json:"excluded_user_error_count"`
	ExcludedCount          int64    `json:"excluded_count"`
	ObservedCount          int64    `json:"observed_count"`
	SuccessRate            *float64 `json:"success_rate"`
	AvgTTFTMs              *float64 `json:"avg_ttft_ms"`
	TTFTCount              int64    `json:"ttft_count"`
	AvgCompletionMs        *float64 `json:"avg_completion_ms"`
	CompletionCount        int64    `json:"completion_count"`
}

type Bucket struct {
	Ts int64 `json:"ts"`
	Metrics
}

type Group struct {
	Group string `json:"group"`
	Metrics
	Series []Bucket `json:"series"`
}

type Result struct {
	Enabled              bool    `json:"enabled"`
	StartTime            int64   `json:"start_time"`
	EndTime              int64   `json:"end_time"`
	UpdatedAt            int64   `json:"updated_at"`
	BucketSeconds        int64   `json:"bucket_seconds"`
	MetricScope          string  `json:"metric_scope"`
	WindowScope          string  `json:"window_scope"`
	CollectionIncomplete bool    `json:"collection_incomplete"`
	Groups               []Group `json:"groups"`
}

func Query(ctx context.Context, groups []string, modelName string, role int, now time.Time) (Result, error) {
	end := now.Unix()
	start := end/BucketSeconds*BucketSeconds - 23*BucketSeconds
	rows, err := model.QueryGroupHealthMetrics(ctx, groups, modelName, start, end)
	if err != nil {
		return Result{}, err
	}
	// Apply model permissions before aggregation, including the all-model view.
	visible := rows[:0]
	for _, row := range rows {
		name := ratio_setting.FormatMatchingModelName(row.ModelName)
		if model.IsModelVisibleToRole(name, role) && model.IsModelCallableByRole(name, role) {
			visible = append(visible, row)
		}
	}
	result := buildResult(groups, visible, start, end)
	result.CollectionIncomplete = CollectionIncomplete(now)
	return result, nil
}

func buildResult(groups []string, rows []model.GroupHealthMetric, start, end int64) Result {
	result := Result{Enabled: true, StartTime: start, EndTime: end, BucketSeconds: BucketSeconds, MetricScope: MetricScope, WindowScope: WindowScope, Groups: make([]Group, 0, len(groups))}
	byGroup := make(map[string]map[int64]model.GroupHealthMetric, len(groups))
	for _, group := range groups {
		byGroup[group] = make(map[int64]model.GroupHealthMetric)
	}
	for _, row := range rows {
		buckets, allowed := byGroup[row.GroupName]
		if !allowed || row.BucketTs < start || row.BucketTs > end {
			continue
		}
		aggregate := buckets[row.BucketTs]
		add(&aggregate, row)
		buckets[row.BucketTs] = aggregate
		if row.UpdatedAt > result.UpdatedAt {
			result.UpdatedAt = row.UpdatedAt
		}
	}
	ordered := append([]string(nil), groups...)
	sort.Strings(ordered)
	for _, name := range ordered {
		group := Group{Group: name, Series: make([]Bucket, 0, 24)}
		var total model.GroupHealthMetric
		for i := int64(0); i < 24; i++ {
			ts := start + i*BucketSeconds
			row := byGroup[name][ts]
			add(&total, row)
			group.Series = append(group.Series, Bucket{Ts: ts, Metrics: summarize(row)})
		}
		group.Metrics = summarize(total)
		result.Groups = append(result.Groups, group)
	}
	return result
}

func add(dst *model.GroupHealthMetric, src model.GroupHealthMetric) {
	dst.CacheReadTokens += src.CacheReadTokens
	dst.CacheInputTokens += src.CacheInputTokens
	dst.CacheSampleCount += src.CacheSampleCount
	dst.RequestCount += src.RequestCount
	dst.SuccessCount += src.SuccessCount
	dst.ServiceFailureCount += src.ServiceFailureCount
	dst.ExcludedUserErrorCount += src.ExcludedUserErrorCount
	dst.ClientCanceledCount += src.ClientCanceledCount
	dst.TTFTSumMs += src.TTFTSumMs
	dst.TTFTCount += src.TTFTCount
	dst.CompletionSumMs += src.CompletionSumMs
	dst.CompletionCount += src.CompletionCount
}

func summarize(row model.GroupHealthMetric) Metrics {
	result := Metrics{RequestCount: row.RequestCount, SuccessCount: row.SuccessCount, FailureCount: row.RequestCount - row.SuccessCount, TTFTCount: row.TTFTCount, CompletionCount: row.CompletionCount}
	result.ServiceFailureCount = row.ServiceFailureCount
	// The original schema cannot attribute historical failures. Residual failures
	// remain unknown, including disconnected clients; no history is reclassified.
	result.UnknownFailureCount = result.FailureCount - row.ServiceFailureCount
	result.ClientCanceledCount = row.ClientCanceledCount
	result.ExcludedUserErrorCount = row.ExcludedUserErrorCount
	result.ExcludedCount = row.ExcludedUserErrorCount
	result.ObservedCount = observedCount(row)
	result.CacheSampleCount = row.CacheSampleCount
	if row.CacheInputTokens > 0 && row.CacheSampleCount > 0 {
		value := float64(row.CacheReadTokens) / float64(row.CacheInputTokens) * 100
		result.CacheHitRate = &value
	}
	if row.RequestCount > 0 {
		value := float64(row.SuccessCount) / float64(row.RequestCount) * 100
		result.SuccessRate = &value
	}
	if row.TTFTCount > 0 {
		value := float64(row.TTFTSumMs) / float64(row.TTFTCount)
		result.AvgTTFTMs = &value
	}
	if row.CompletionCount > 0 {
		value := float64(row.CompletionSumMs) / float64(row.CompletionCount)
		result.AvgCompletionMs = &value
	}
	return result
}
