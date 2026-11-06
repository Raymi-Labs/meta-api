package routeinsight

import (
	"context"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type ChannelReport struct {
	ChannelID     int          `json:"channel_id"`
	Group         string       `json:"group"`
	Model         string       `json:"model"`
	Window        Window       `json:"window"`
	SuccessRate   *float64     `json:"success_rate"`
	WilsonLower   *float64     `json:"success_lower_bound"`
	TTFTP50Ms     *int64       `json:"ttft_p50_ms"`
	TTFTP95Ms     *int64       `json:"ttft_p95_ms"`
	DurationP50Ms *int64       `json:"duration_p50_ms"`
	DurationP95Ms *int64       `json:"duration_p95_ms"`
	TTFTSamples   int64        `json:"ttft_samples"`
	CurrentStatus HealthStatus `json:"current_status"`
	CurrentReason string       `json:"current_reason"`
}

// ChannelReports summarizes [now-minutes, now] per channel/group/model. The
// window is persisted data only; the latest few seconds may not be flushed.
func ChannelReports(ctx context.Context, now time.Time, minutes int, modelName string) ([]ChannelReport, error) {
	rows, err := model.QueryRouteAttemptMetrics(ctx, now.Add(-time.Duration(minutes)*time.Minute).Unix(), now.Unix(), modelName)
	if err != nil {
		return nil, err
	}
	type key struct {
		channelID    int
		group, model string
	}
	windows := map[key]*Window{}
	for _, row := range rows {
		k := key{row.ChannelID, row.GroupName, row.ModelName}
		w := windows[k]
		if w == nil {
			nw := newWindow()
			w = &nw
			windows[k] = w
		}
		w.addRow(row.AttemptCount, row.SuccessCount, row.ServiceFailureCount, row.RateLimitedCount, row.CapacityRejectedCount,
			row.UnknownFailureCount, row.UserErrorCount, row.ClientCanceledCount, row.LocalDeliveryCount,
			decodeHistogram(row.TTFTHistogram), decodeHistogram(row.DurationHistogram))
	}
	reports := make([]ChannelReport, 0, len(windows))
	for k, w := range windows {
		current := Health(k.channelID, k.model)
		report := ChannelReport{ChannelID: k.channelID, Group: k.group, Model: k.model, Window: *w,
			SuccessRate: w.SuccessRate(), TTFTP50Ms: w.TTFT.Quantile(0.5), TTFTP95Ms: w.TTFT.Quantile(0.95),
			DurationP50Ms: w.Duration.Quantile(0.5), DurationP95Ms: w.Duration.Quantile(0.95), TTFTSamples: w.TTFT.Count(),
			CurrentStatus: current.Status, CurrentReason: current.Reason}
		if w.Evidence() > 0 {
			lower := w.WilsonLower()
			report.WilsonLower = &lower
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].Model != reports[j].Model {
			return reports[i].Model < reports[j].Model
		}
		if reports[i].Window.Attempts != reports[j].Window.Attempts {
			return reports[i].Window.Attempts > reports[j].Window.Attempts
		}
		return reports[i].ChannelID < reports[j].ChannelID
	})
	return reports, nil
}

type ShadowReport struct {
	Model string                  `json:"model"`
	Total model.RouteShadowMetric `json:"total"`
	// Failure rates of attempts where shadow agreed / disagreed with production.
	// A clearly higher disagree failure rate is evidence the shadow would help;
	// it is correlation over production-chosen traffic, not a counterfactual.
	AgreeFailureRate    *float64 `json:"agree_failure_rate"`
	DisagreeFailureRate *float64 `json:"disagree_failure_rate"`
	DeliveryRate        *float64 `json:"delivery_rate"`
	AvgAttempts         *float64 `json:"avg_attempts"`
}

func ShadowReports(ctx context.Context, now time.Time, hours int, modelName string) ([]ShadowReport, error) {
	rows, err := model.QueryRouteShadowMetrics(ctx, now.Add(-time.Duration(hours)*time.Hour).Unix(), now.Unix(), modelName)
	if err != nil {
		return nil, err
	}
	totals := map[string]*model.RouteShadowMetric{}
	for _, row := range rows {
		t := totals[row.ModelName]
		if t == nil {
			t = &model.RouteShadowMetric{ModelName: row.ModelName}
			totals[row.ModelName] = t
		}
		t.RequestCount += row.RequestCount
		t.DeliveredCount += row.DeliveredCount
		t.RetriedCount += row.RetriedCount
		t.TotalAttemptCount += row.TotalAttemptCount
		t.DecisionCount += row.DecisionCount
		t.AgreeCount += row.AgreeCount
		t.DisagreeCount += row.DisagreeCount
		t.CrossGroupCount += row.CrossGroupCount
		t.NoDataCount += row.NoDataCount
		t.AvoidUnhealthyCount += row.AvoidUnhealthyCount
		t.CheaperCount += row.CheaperCount
		t.CostUnknownCount += row.CostUnknownCount
		t.AgreeSuccessCount += row.AgreeSuccessCount
		t.AgreeFailureCount += row.AgreeFailureCount
		t.DisagreeSuccessCount += row.DisagreeSuccessCount
		t.DisagreeFailureCount += row.DisagreeFailureCount
		if row.UpdatedAt > t.UpdatedAt {
			t.UpdatedAt = row.UpdatedAt
		}
	}
	reports := make([]ShadowReport, 0, len(totals))
	for name, t := range totals {
		report := ShadowReport{Model: name, Total: *t,
			AgreeFailureRate:    ratio(t.AgreeFailureCount, t.AgreeSuccessCount+t.AgreeFailureCount),
			DisagreeFailureRate: ratio(t.DisagreeFailureCount, t.DisagreeSuccessCount+t.DisagreeFailureCount),
			DeliveryRate:        ratio(t.DeliveredCount, t.RequestCount),
			AvgAttempts:         ratio(t.TotalAttemptCount, t.RequestCount)}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Total.DecisionCount > reports[j].Total.DecisionCount })
	return reports, nil
}

func ratio(numerator, denominator int64) *float64 {
	if denominator == 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}
