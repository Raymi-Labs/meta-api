package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// RouteRoutingSetting controls live use of route health. The switch is off by
// default: shadow measurement runs regardless, and nothing changes routing
// until an administrator opts in after reviewing the shadow evidence.
type RouteRoutingSetting struct {
	// HealthRoutingEnabled is the smart routing master switch: every token
	// keeps its group order, but skips channels whose recent attempt evidence
	// is unhealthy inside the group the request uses. Groups, prices and
	// permissions are unchanged. (A former plan_routing_enabled key in the
	// options table is ignored by the config loader.)
	HealthRoutingEnabled bool `json:"health_routing_enabled"`
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
