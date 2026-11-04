package dto

import "fmt"

// UpstreamCapacitySetting is opt-in; zero values preserve legacy routing.
type UpstreamCapacitySetting struct {
	Enabled        bool   `json:"enabled"`
	OverflowPolicy string `json:"overflow_policy,omitempty"`
	MaxWaitMs      int    `json:"max_wait_ms,omitempty"`
}

func (s UpstreamCapacitySetting) Validate() error {
	if s.OverflowPolicy != "" && s.OverflowPolicy != "wait" && s.OverflowPolicy != "reroute" {
		return fmt.Errorf("upstream_capacity.overflow_policy must be wait or reroute")
	}
	if s.MaxWaitMs < 0 || s.MaxWaitMs > 120000 {
		return fmt.Errorf("upstream_capacity.max_wait_ms must be between 0 and 120000")
	}
	return nil
}

func (s UpstreamCapacitySetting) WaitDurationMs() int {
	if s.MaxWaitMs > 0 {
		return s.MaxWaitMs
	}
	return 30000
}

func (s UpstreamCapacitySetting) PreferWait() bool { return s.OverflowPolicy != "reroute" }
