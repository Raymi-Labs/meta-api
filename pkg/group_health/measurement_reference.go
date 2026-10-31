package grouphealth

import "github.com/QuantumNous/new-api/model"

// MeasurementReference is a measured hourly value, not a synthetic request.
type MeasurementReference struct {
	Value float64 `json:"value"`
	Ts    int64   `json:"ts"`
}

type MeasurementReferences struct {
	TTFT       *MeasurementReference `json:"ttft,omitempty"`
	Completion *MeasurementReference `json:"completion,omitempty"`
	Cache      *MeasurementReference `json:"cache,omitempty"`
}

func measurementStart(end int64) int64 {
	return end/BucketSeconds*BucketSeconds - (MeasurementLookbackHours-1)*BucketSeconds
}

func (r *MeasurementReferences) observe(row model.GroupHealthMetric, ts int64) {
	if row.TTFTCount > 0 && row.TTFTSumMs >= 0 && (r.TTFT == nil || ts > r.TTFT.Ts) {
		r.TTFT = &MeasurementReference{Value: float64(row.TTFTSumMs) / float64(row.TTFTCount), Ts: ts}
	}
	if row.CompletionCount > 0 && row.CompletionSumMs >= 0 && (r.Completion == nil || ts > r.Completion.Ts) {
		r.Completion = &MeasurementReference{Value: float64(row.CompletionSumMs) / float64(row.CompletionCount), Ts: ts}
	}
	if row.CacheSampleCount > 0 && row.CacheInputTokens > 0 && row.CacheReadTokens >= 0 && row.CacheReadTokens <= row.CacheInputTokens && (r.Cache == nil || ts > r.Cache.Ts) {
		r.Cache = &MeasurementReference{Value: float64(row.CacheReadTokens) / float64(row.CacheInputTokens) * 100, Ts: ts}
	}
}

func (r MeasurementReferences) snapshot() *MeasurementReferences {
	if r.TTFT == nil && r.Completion == nil && r.Cache == nil {
		return nil
	}
	// Values are immutable; replacing a later pointer cannot change earlier hours.
	return &r
}
