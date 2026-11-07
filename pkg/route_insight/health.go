package routeinsight

import (
	"math"
)

// Health thresholds. The success-rate threshold is an administrator setting
// (route_routing_setting.unhealthy_below_percent, default 90%); the others
// are fixed initial values to be tuned against observed baselines.
const (
	MinSamples            = 10   // below this the route is unknown, never "healthy"
	DefaultUnhealthyBelow = 0.90 // success rate below this marks a route unhealthy
	BurstMinSamples       = 5    // 5m burst detector needs at least this many
	BurstFailureRate      = 0.5  // 5m upstream failure share that marks a burst
)

type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusUnknown   HealthStatus = "unknown"
)

// Window aggregates attempt evidence. Success, upstream failure, 429 and
// unknown outcomes form the success-rate denominator; user errors, client
// cancellations, local delivery failures and pre-execution capacity refusals
// are kept for reporting only.
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

// Evidence counts outcomes that say something about the upstream route. An
// unknown outcome (no classified error, but nothing delivered completely,
// e.g. a stream cut off mid-answer) counts as a failure: the customer did not
// get an answer, and client cancellations and local write failures are
// already separated out before this point.
func (w Window) Evidence() int64 {
	return w.Success + w.Failures()
}

// Failures counts evidence against the route.
func (w Window) Failures() int64 {
	return w.ServiceFailure + w.RateLimited + w.UnknownFailure
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

// Evaluate classifies a route from its 5m and 15m windows. unhealthyBelow is
// the success-rate threshold (0-1); 0 means DefaultUnhealthyBelow. Score is the
// Wilson lower bound, kept for display only: routing uses the tiers, not
// small score differences.
func Evaluate(short, long Window, unhealthyBelow float64) RouteHealth {
	if unhealthyBelow <= 0 || unhealthyBelow >= 1 {
		unhealthyBelow = DefaultUnhealthyBelow
	}
	health := RouteHealth{Short: short, Long: long, Status: StatusUnknown, Reason: "insufficient_samples"}
	if long.Evidence() < MinSamples {
		return health
	}
	health.Score = long.WilsonLower()
	if n := short.Evidence(); n >= BurstMinSamples && float64(short.Failures())/float64(n) >= BurstFailureRate {
		health.Status, health.Reason = StatusUnhealthy, "recent_failure_burst"
		return health
	}
	if rate := long.SuccessRate(); rate != nil && *rate < unhealthyBelow {
		health.Status, health.Reason = StatusUnhealthy, "low_success_rate"
		return health
	}
	health.Status, health.Reason = StatusHealthy, "ok"
	return health
}
