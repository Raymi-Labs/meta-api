package grouphealth

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const MetricScope = "per_request_per_group_final_outcome"
const WindowScope = "current_hour_and_previous_23_hours"

// Keep this threshold aligned with the request-health UI sample minimum.
const MinUptimeRequestSamples = int64(30)

// Matches aggregate retention; older observations must not live on indefinitely.
const MeasurementLookbackHours = 7 * 24

// A normalized billing family must never erase a restriction on the exact
// model ID exposed by pricing. Require both names when they differ.
func IsModelAuthorized(name string, role int) bool {
	if !model.IsModelVisibleToRole(name, role) || !model.IsModelCallableByRole(name, role) {
		return false
	}
	matchingName := ratio_setting.FormatMatchingModelName(name)
	return matchingName == name || (model.IsModelVisibleToRole(matchingName, role) && model.IsModelCallableByRole(matchingName, role))
}

// PerformanceReference is display-only: it never contributes to measured metrics.
type PerformanceReference struct {
	Source     string   `json:"source"`
	Model      string   `json:"model"`
	Provider   string   `json:"provider"`
	Percentile int      `json:"percentile"`
	FetchedAt  int64    `json:"fetched_at"`
	ExpiresAt  int64    `json:"expires_at"`
	TTFTMs     *float64 `json:"ttft_ms"`
	OutputTPS  *float64 `json:"output_tps"`
}

type Metrics struct {
	PerformanceReference    *PerformanceReference  `json:"performance_reference,omitempty"`
	LastValid               *MeasurementReferences `json:"last_valid,omitempty"`
	ProbeCount              int64                  `json:"probe_count"`
	ProbeSuccessCount       int64                  `json:"probe_success_count"`
	ProbeBucketCount        int64                  `json:"probe_bucket_count"`
	ProbeSuccessBucketCount int64                  `json:"probe_success_bucket_count"`
	UptimeSampleCount       int64                  `json:"uptime_sample_count"`
	UptimeRate              *float64               `json:"uptime_rate"`
	InputTokens             *int64                 `json:"input_tokens"`
	OutputTokens            *int64                 `json:"output_tokens"`
	TotalTokens             *int64                 `json:"total_tokens"`
	UsageSampleCount        int64                  `json:"usage_sample_count"`
	AvgOutputTPS            *float64               `json:"avg_output_tps"`
	ThroughputSampleCount   int64                  `json:"throughput_sample_count"`
	CacheHitRate            *float64               `json:"cache_hit_rate"`
	CacheSampleCount        int64                  `json:"cache_sample_count"`
	RequestCount            int64                  `json:"request_count"`
	SuccessCount            int64                  `json:"success_count"`
	FailureCount            int64                  `json:"failure_count"`
	ServiceFailureCount     int64                  `json:"service_failure_count"`
	UnknownFailureCount     int64                  `json:"unknown_failure_count"`
	ClientCanceledCount     int64                  `json:"client_canceled_count"`
	ExcludedUserErrorCount  int64                  `json:"excluded_user_error_count"`
	ExcludedCount           int64                  `json:"excluded_count"`
	ObservedCount           int64                  `json:"observed_count"`
	SuccessRate             *float64               `json:"success_rate"`
	AvgTTFTMs               *float64               `json:"avg_ttft_ms"`
	TTFTCount               int64                  `json:"ttft_count"`
	AvgCompletionMs         *float64               `json:"avg_completion_ms"`
	CompletionCount         int64                  `json:"completion_count"`
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

// ModelUsageReference is public model-wide activity, never this site's group usage.
type ModelUsagePoint struct {
	Ts           int64  `json:"ts"`
	TotalTokens  *int64 `json:"total_tokens"`
	RequestCount *int64 `json:"request_count"`
}
type ModelUsageReference struct {
	Source        string            `json:"source"`
	Model         string            `json:"model"`
	Permaslug     string            `json:"permaslug"`
	Variant       string            `json:"variant"`
	Scope         string            `json:"scope"`
	Groups        []string          `json:"groups"`
	BucketSeconds int64             `json:"bucket_seconds"`
	FetchedAt     int64             `json:"fetched_at"`
	UpdatedAt     int64             `json:"updated_at"`
	ExpiresAt     int64             `json:"expires_at"`
	Series        []ModelUsagePoint `json:"series"`
}

type Result struct {
	ModelUsageReference  *ModelUsageReference `json:"model_usage_reference,omitempty"`
	Enabled              bool                 `json:"enabled"`
	StartTime            int64                `json:"start_time"`
	EndTime              int64                `json:"end_time"`
	UpdatedAt            int64                `json:"updated_at"`
	BucketSeconds        int64                `json:"bucket_seconds"`
	MetricScope          string               `json:"metric_scope"`
	WindowScope          string               `json:"window_scope"`
	WindowHours          int                  `json:"window_hours"`
	CollectionIncomplete bool                 `json:"collection_incomplete"`
	Groups               []Group              `json:"groups"`
}

func Query(ctx context.Context, groups []string, modelName string, role int, now time.Time, windowHours ...int) (Result, error) {
	hours := 24
	if len(windowHours) > 0 && windowHours[0] == 72 {
		hours = 72
	}
	end := now.Unix()
	start := end/BucketSeconds*BucketSeconds - int64(hours-1)*BucketSeconds
	rows, err := model.QueryGroupHealthMetrics(ctx, groups, modelName, measurementStart(end), end)
	if err != nil {
		return Result{}, err
	}
	// Apply model permissions before aggregation, including the all-model view.
	visible := rows[:0]
	for _, row := range rows {
		if IsModelAuthorized(row.ModelName, role) {
			visible = append(visible, row)
		}
	}
	result := buildResult(groups, visible, start, end)
	result.CollectionIncomplete = CollectionIncomplete(now, hours)
	return result, nil
}

type ModelSummary struct {
	Model string `json:"model"`
	Metrics
}

type ModelResult struct {
	StartTime            int64          `json:"start_time"`
	EndTime              int64          `json:"end_time"`
	UpdatedAt            int64          `json:"updated_at"`
	WindowHours          int            `json:"window_hours"`
	CollectionIncomplete bool           `json:"collection_incomplete"`
	Models               []ModelSummary `json:"models"`
}

// QueryModels performs one bounded aggregate-table query for a catalog page.
// It never widens the caller's authorized groups or model list.
func QueryModels(ctx context.Context, groups, modelNames []string, role, hours int, now time.Time) (ModelResult, error) {
	if hours != 72 {
		hours = 24
	}
	end := now.Unix()
	start := end/BucketSeconds*BucketSeconds - int64(hours-1)*BucketSeconds
	result := ModelResult{StartTime: start, EndTime: end, WindowHours: hours, CollectionIncomplete: CollectionIncomplete(now, hours), Models: make([]ModelSummary, 0, len(modelNames))}
	allowed := make([]string, 0, len(modelNames))
	for _, name := range modelNames {
		if IsModelAuthorized(name, role) {
			allowed = append(allowed, name)
		}
	}
	rows, err := model.QueryGroupHealthMetricsForModels(ctx, groups, allowed, measurementStart(end), end)
	if err != nil {
		return ModelResult{}, err
	}
	totals := make(map[string]model.GroupHealthMetric, len(allowed))
	measured := make(map[string]map[int64]model.GroupHealthMetric, len(allowed))
	for _, row := range withProbeBuckets(rows) {
		if measured[row.ModelName] == nil {
			measured[row.ModelName] = make(map[int64]model.GroupHealthMetric)
		}
		hour := measured[row.ModelName][row.BucketTs]
		add(&hour, row)
		measured[row.ModelName][row.BucketTs] = hour
		if row.BucketTs >= start {
			total := totals[row.ModelName]
			add(&total, row)
			totals[row.ModelName] = total
		}
		if row.UpdatedAt > result.UpdatedAt {
			result.UpdatedAt = row.UpdatedAt
		}
	}
	for _, name := range allowed {
		var latest MeasurementReferences
		for ts, row := range measured[name] {
			latest.observe(row, ts)
		}
		metrics := summarize(totals[name])
		metrics.LastValid = latest.snapshot()
		result.Models = append(result.Models, ModelSummary{Model: name, Metrics: metrics})
	}
	return result, nil
}

func buildResult(groups []string, rows []model.GroupHealthMetric, start, end int64) Result {
	hours := int((end/BucketSeconds*BucketSeconds-start)/BucketSeconds) + 1
	result := Result{Enabled: true, StartTime: start, EndTime: end, BucketSeconds: BucketSeconds, MetricScope: MetricScope, WindowScope: fmt.Sprintf("current_hour_and_previous_%d_hours", hours-1), WindowHours: hours, Groups: make([]Group, 0, len(groups))}
	byGroup := make(map[string]map[int64]model.GroupHealthMetric, len(groups))
	for _, group := range groups {
		byGroup[group] = make(map[int64]model.GroupHealthMetric)
	}
	for _, row := range withProbeBuckets(rows) {
		buckets, allowed := byGroup[row.GroupName]
		if !allowed || row.BucketTs < measurementStart(end) || row.BucketTs > end {
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
		group := Group{Group: name, Series: make([]Bucket, 0, hours)}
		var total model.GroupHealthMetric
		var latest MeasurementReferences
		for ts := measurementStart(end); ts <= end; ts += BucketSeconds {
			row := byGroup[name][ts]
			latest.observe(row, ts)
			if ts < start {
				continue
			}
			add(&total, row)
			metrics := summarize(row)
			metrics.LastValid = latest.snapshot()
			group.Series = append(group.Series, Bucket{Ts: ts, Metrics: metrics})
		}
		group.Metrics = summarize(total)
		group.LastValid = latest.snapshot()
		result.Groups = append(result.Groups, group)
	}
	return result
}

// Fold before applying the low-sample threshold so replicas share one request
// count. N upstreams contribute at most one supplemental sample; real outcomes
// retain their original weight.
func withProbeBuckets(rows []model.GroupHealthMetric) []model.GroupHealthMetric {
	merged := make(map[bucketKey]model.GroupHealthMetric)
	for _, row := range rows {
		key := bucketKey{group: row.GroupName, model: row.ModelName, ts: row.BucketTs}
		total := merged[key]
		add(&total, row)
		total.GroupName, total.ModelName, total.BucketTs = key.group, key.model, key.ts
		if row.UpdatedAt > total.UpdatedAt {
			total.UpdatedAt = row.UpdatedAt
		}
		merged[key] = total
	}
	result := make([]model.GroupHealthMetric, 0, len(merged))
	for _, row := range merged {
		row.ProbeBucketCount, row.ProbeSuccessBucketCount = 0, 0
		if row.RequestCount < MinUptimeRequestSamples && row.ProbeCount > 0 {
			row.ProbeBucketCount = 1
			if row.ProbeSuccessCount > 0 {
				row.ProbeSuccessBucketCount = 1
			}
		}
		result = append(result, row)
	}
	return result
}

func add(dst *model.GroupHealthMetric, src model.GroupHealthMetric) {
	dst.ProbeCount += src.ProbeCount
	dst.ProbeSuccessCount += src.ProbeSuccessCount
	dst.ProbeBucketCount += src.ProbeBucketCount
	dst.ProbeSuccessBucketCount += src.ProbeSuccessBucketCount
	dst.InputTokens += src.InputTokens
	dst.OutputTokens += src.OutputTokens
	dst.UsageSampleCount += src.UsageSampleCount
	dst.ThroughputOutputTokens += src.ThroughputOutputTokens
	dst.GenerationSumMs += src.GenerationSumMs
	dst.ThroughputSampleCount += src.ThroughputSampleCount
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
	result.ProbeCount, result.ProbeSuccessCount = row.ProbeCount, row.ProbeSuccessCount
	result.ProbeBucketCount, result.ProbeSuccessBucketCount = row.ProbeBucketCount, row.ProbeSuccessBucketCount
	result.UptimeSampleCount = row.RequestCount + row.ProbeBucketCount
	if result.UptimeSampleCount > 0 {
		value := float64(row.SuccessCount+row.ProbeSuccessBucketCount) / float64(result.UptimeSampleCount) * 100
		result.UptimeRate = &value
	}
	result.UsageSampleCount = row.UsageSampleCount
	result.ThroughputSampleCount = row.ThroughputSampleCount
	if row.UsageSampleCount > 0 {
		input, output := row.InputTokens, row.OutputTokens
		total := input + output
		result.InputTokens, result.OutputTokens, result.TotalTokens = &input, &output, &total
	}
	if row.ThroughputSampleCount > 0 && row.GenerationSumMs > 0 {
		value := float64(row.ThroughputOutputTokens) * 1000 / float64(row.GenerationSumMs)
		result.AvgOutputTPS = &value
	}
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
