package routeinsight

import (
	"github.com/QuantumNous/new-api/common"
)

// LatencyBoundsMs are inclusive upper bounds of fixed histogram bins; the last
// bin is open-ended. Percentiles report the bin upper bound, a conservative
// estimate whose resolution is limited by these bounds.
var LatencyBoundsMs = []int64{250, 500, 1000, 2000, 3000, 5000, 8000, 12000, 20000, 30000, 60000, 120000}

type Histogram []int64

func newHistogram() Histogram {
	return make(Histogram, len(LatencyBoundsMs)+1)
}

func (h Histogram) Observe(ms int64) {
	if len(h) != len(LatencyBoundsMs)+1 || ms < 0 {
		return
	}
	for index, bound := range LatencyBoundsMs {
		if ms <= bound {
			h[index]++
			return
		}
	}
	h[len(LatencyBoundsMs)]++
}

func (h Histogram) Count() int64 {
	var total int64
	for _, value := range h {
		total += value
	}
	return total
}

func (h Histogram) Add(other Histogram) {
	if len(h) != len(other) {
		return
	}
	for index := range other {
		h[index] += other[index]
	}
}

// Quantile returns the upper bound of the bin containing q, nil without
// samples, and -1 when it falls in the open-ended overflow bin.
func (h Histogram) Quantile(q float64) *int64 {
	total := h.Count()
	if total == 0 || q <= 0 || q > 1 {
		return nil
	}
	rank := int64(float64(total)*q + 0.999999)
	if rank < 1 {
		rank = 1
	}
	var seen int64
	for index, value := range h {
		seen += value
		if seen >= rank {
			result := int64(-1)
			if index < len(LatencyBoundsMs) {
				result = LatencyBoundsMs[index]
			}
			return &result
		}
	}
	return nil
}

func (h Histogram) encode() string {
	if h.Count() == 0 {
		return ""
	}
	data, err := common.Marshal([]int64(h))
	if err != nil {
		return ""
	}
	return string(data)
}

// decodeHistogram ignores malformed or differently-sized arrays rather than
// guessing their bins; a bounds change starts a new, separate series.
func decodeHistogram(raw string) Histogram {
	h := newHistogram()
	if raw == "" {
		return h
	}
	var values []int64
	if err := common.UnmarshalJsonStr(raw, &values); err != nil || len(values) != len(h) {
		return h
	}
	copy(h, values)
	return h
}
