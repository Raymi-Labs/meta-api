package dto

import "testing"

func TestUpstreamCapacitySetting(t *testing.T) {
	for _, s := range []UpstreamCapacitySetting{{}, {Enabled: true}, {Enabled: true, OverflowPolicy: "reroute", MaxWaitMs: 120000}} {
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []UpstreamCapacitySetting{{OverflowPolicy: "unbounded"}, {MaxWaitMs: -1}, {MaxWaitMs: 120001}} {
		if s.Validate() == nil {
			t.Fatal("invalid setting accepted")
		}
	}
	if (UpstreamCapacitySetting{}).WaitDurationMs() != 30000 {
		t.Fatal("wait default")
	}
}
