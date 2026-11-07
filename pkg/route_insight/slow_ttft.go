package routeinsight

// MinTTFTSamples is the fewest first-token samples (successful streamed
// attempts in the 15-minute window) a route, and separately its peers, need
// before first-token speed can mark it unhealthy.
const (
	MinTTFTSamples   = 10
	slowTTFTQuantile = 0.75
	// overflowTTFTMs stands for the open ">120s" histogram bin.
	overflowTTFTMs = int64(240000)
)

func ttftQuantile(h Histogram) int64 {
	value := h.Quantile(slowTTFTQuantile)
	if value == nil {
		return 0
	}
	if *value < 0 {
		return overflowTTFTMs
	}
	return *value
}

// ApplySlowTTFT marks otherwise healthy routes unhealthy when their first
// token is clearly slower than other channels serving the same model.
//
// The statistic is P75 (the start of the slowest quarter): the median misses
// channels that are usually fine but often stall, while P90 swings on a few
// outliers. A route is slow when its P75 is at least minMs and at least ratio
// times the P75 of all other channels for that model combined. Only
// comparisons within one model are made, so models that are slow everywhere
// (long reasoning) are not penalized. ratio <= 0 disables the rule.
//
// Values are histogram bin upper bounds, so the comparison is coarse: e.g. a
// 12s bin against a 5s peer bin passes a 2x ratio, 8s against 5s does not.
func ApplySlowTTFT(health map[routeKey]RouteHealth, windows map[routeKey]*Window, ratio float64, minMs int64) {
	totals := map[string]Histogram{}
	for key, window := range windows {
		total, ok := totals[key.model]
		if !ok {
			total = newHistogram()
			totals[key.model] = total
		}
		total.Add(window.TTFT)
	}
	for key, window := range windows {
		own := window.TTFT
		if own.Count() < MinTTFTSamples {
			continue
		}
		peers := newHistogram()
		peers.Add(totals[key.model])
		for index := range peers {
			peers[index] -= own[index]
		}
		if peers.Count() < MinTTFTSamples {
			continue
		}
		ownP75, peerP75 := ttftQuantile(own), ttftQuantile(peers)
		h := health[key]
		h.TTFTP75Ms, h.PeerTTFTP75Ms = &ownP75, &peerP75
		if h.Status == StatusHealthy && ratio > 0 && ownP75 >= minMs && float64(ownP75) >= ratio*float64(peerP75) {
			h.Status, h.Reason = StatusUnhealthy, "slow_ttft"
		}
		health[key] = h
	}
}
