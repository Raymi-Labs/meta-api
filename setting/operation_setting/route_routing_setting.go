package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

const (
	defaultUnhealthyBelowPercent = 90
	minUnhealthyBelowPercent     = 50
	maxUnhealthyBelowPercent     = 99
)

// RouteRoutingSetting controls live use of route health. The switch is off by
// default: shadow measurement runs regardless, and nothing changes routing
// until an administrator opts in after reviewing the shadow evidence.
type RouteRoutingSetting struct {
	// HealthRoutingEnabled is the smart routing master switch. Every token
	// keeps its group order, priorities and weights; unhealthy channels move
	// behind healthy ones inside a group, and a group with no healthy channel
	// for the model is skipped when the token's later groups have one. The
	// actual group is billed, as with ordinary group fallback. (A former
	// plan_routing_enabled key in the options table is ignored.)
	HealthRoutingEnabled bool `json:"health_routing_enabled"`
	// ProbePercent (0-20) of selections may still try an unhealthy/unknown
	// route so it can recover (half-open). 0 disables probing.
	ProbePercent int `json:"probe_percent"`
	// UnhealthyBelowPercent marks a route unhealthy when its recent success
	// rate (upstream evidence only, enough samples) is below this value.
	UnhealthyBelowPercent int `json:"unhealthy_below_percent"`
}

var routeRoutingSetting = RouteRoutingSetting{ProbePercent: 2, UnhealthyBelowPercent: defaultUnhealthyBelowPercent}

func init() {
	config.GlobalConfig.Register("route_routing_setting", &routeRoutingSetting)
}

func GetRouteRoutingSetting() RouteRoutingSetting {
	setting := routeRoutingSetting
	if setting.ProbePercent < 0 {
		setting.ProbePercent = 0
	}
	if setting.ProbePercent > 20 {
		setting.ProbePercent = 20
	}
	if setting.UnhealthyBelowPercent == 0 {
		setting.UnhealthyBelowPercent = defaultUnhealthyBelowPercent
	}
	if setting.UnhealthyBelowPercent < minUnhealthyBelowPercent {
		setting.UnhealthyBelowPercent = minUnhealthyBelowPercent
	}
	if setting.UnhealthyBelowPercent > maxUnhealthyBelowPercent {
		setting.UnhealthyBelowPercent = maxUnhealthyBelowPercent
	}
	return setting
}
