package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// RouteRoutingSetting controls live use of route health. Both switches are off
// by default: shadow measurement runs regardless, and nothing changes routing
// until an administrator opts in after reviewing the shadow evidence.
type RouteRoutingSetting struct {
	// HealthRoutingEnabled lets ordinary tokens skip channels whose recent
	// attempt evidence is unhealthy, inside the group the request already
	// uses. Groups, prices and permissions are unchanged.
	HealthRoutingEnabled bool `json:"health_routing_enabled"`
	// PlanRoutingEnabled turns on cross-group health/cost ranking for tokens
	// bound to a routing plan. When off, plan tokens fall back to the plan's
	// group order (still billed at the plan price).
	PlanRoutingEnabled bool `json:"plan_routing_enabled"`
	// ProbePercent (0-20) of selections may still try an unhealthy/unknown
	// route so it can recover (half-open). 0 disables probing.
	ProbePercent int `json:"probe_percent"`
}

var routeRoutingSetting = RouteRoutingSetting{ProbePercent: 2}

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
	return setting
}
