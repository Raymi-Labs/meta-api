package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestPulseUsageLogRequirementIsRetired(t *testing.T) {
	t.Setenv("PULSE_USAGE_LOG_REQUIRED", "true")
	common.ReplacePulseConfig(nil)
	t.Cleanup(func() { common.ReplacePulseConfig(nil) })

	if common.IsPulseConfigKey("PulseUsageLogRequired") {
		t.Fatal("retired usage-log requirement remains a Pulse config key")
	}
	if _, ok := common.GetPulseConfig()["PulseUsageLogRequired"]; ok {
		t.Fatal("retired usage-log environment variable remains active")
	}
}
