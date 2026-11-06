package routeinsight

import (
	"math"
)

// Health thresholds are initial, uncalibrated parameters for the shadow phase.
// They must be tuned against observed baselines before any live routing use.
const (
	MinSamples          = 10   // below this the route is unknown, never "healthy"
	UnhealthyLowerBound = 0.80 // Wilson 95% lower bound on 15m evidence success
	BurstMinSamples     = 5    // 5m burst detector needs at least this many
	BurstFailureRate    = 0.5  // 5m upstream failure share that marks a burst
	ScoreTolerance      = 0.02 // healthy routes within this score are "equivalent"
)

type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusUnknown   HealthStatus = "unknown"
)

// Window aggregates attempt evidence. Unknown and local outcomes are kept for
// reporting but excluded from the success-rate denominator.
type Window struct {
	Attempts         int64     `json:"attempts"`
	Success          int64     `json:"success"`
	ServiceFailure   int64     `json:"service_failure"`
	RateLimited      int64     `json:"rate_limited"`
	CapacityRejected int64     `json:"capacity_rejected"`
	UnknownFailure   int64     `json:"unknown_failure"`
	UserError        int64     `json:"user_error"`
	ClientCanceled   int64     `json:"client_canceled"`
	LocalDelivery    int64     `json:"local_delivery"`
	TTFT             Histogram `json:"-"`
	Duration         Histogram `json:"-"`
}

func newWindow() Window {
	return Window{TTFT: newHistogram(), Duration: newHistogram()}
}

func (w *Window) addRow(attempts, success, serviceFailure, rateLimited, capacity, unknown, user, canceled, local int64, ttft, duration Histogram) {
	w.Attempts += attempts
	w.Success += success
	w.ServiceFailure += serviceFailure
	w.RateLimited += rateLimited
	w.CapacityRejected += capacity
	w.UnknownFailure += unknown
	w.UserError += user
	w.ClientCanceled += canceled
	w.LocalDelivery += local
	w.TTFT.Add(ttft)
	w.Duration.Add(duration)
}

// Evidence counts outcomes that say something about the upstream route.
func (w Window) Evidence() int64 {
	return w.Success + w.ServiceFailure + w.RateLimited
}

func (w Window) SuccessRate() *float64 {
	n := w.Evidence()
	if n == 0 {
		return nil
	}
	rate := float64(w.Success) / float64(n)
	return &rate
}

// WilsonLower is the 95% lower confidence bound of the evidence success rate,
// so 20/20 does not read as 100% reliable.
func (w Window) WilsonLower() float64 {
	n := float64(w.Evidence())
	if n == 0 {
		return 0
	}
	const z = 1.96
	p := float64(w.Success) / n
	denominator := 1 + z*z/n
	centre := p + z*z/(2*n)
	margin := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n))
	return math.Max(0, (centre-margin)/denominator)
}

type RouteHealth struct {
	Status HealthStatus `json:"status"`
	Score  float64      `json:"score"`
	Reason string       `json:"reason"`
	Short  Window       `json:"short"` // last 5 minutes
	Long   Window       `json:"long"`  // last 15 minutes
}

func Evaluate(short, long Window) RouteHealth {
	health := RouteHealth{Short: short, Long: long, Status: StatusUnknown, Reason: "insufficient_samples"}
	if long.Evidence() < MinSamples {
		return health
	}
	health.Score = long.WilsonLower()
	if n := short.Evidence(); n >= BurstMinSamples && float64(short.ServiceFailure+short.RateLimited)/float64(n) >= BurstFailureRate {
		health.Status, health.Reason = StatusUnhealthy, "recent_failure_burst"
		return health
	}
	if health.Score < UnhealthyLowerBound {
		health.Status, health.Reason = StatusUnhealthy, "low_success_lower_bound"
		return health
	}
	health.Status, health.Reason = StatusHealthy, "ok"
	return health
}
