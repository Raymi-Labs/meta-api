package routeinsight

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestLowVolumeRecoveryEvidence(t *testing.T) {
	badHistory := Window{Success: 1, ServiceFailure: 25}
	for _, tc := range []struct {
		name                        string
		short, long, history, probe Window
		status                      HealthStatus
	}{
		{name: "three_failures_without_ten", short: Window{ServiceFailure: 3}, long: Window{ServiceFailure: 3}, status: StatusUnhealthy},
		{name: "independent_burst", short: Window{Success: 2, ServiceFailure: 3}, long: Window{Success: 2, ServiceFailure: 3}, status: StatusUnhealthy},
		{name: "quiet_three_failures_not_recovered", history: Window{ServiceFailure: 3}, status: StatusUnhealthy},
		{name: "quiet_fault_not_recovered", history: badHistory, status: StatusUnhealthy},
		{name: "one_probe_does_not_erase_fault", history: badHistory, probe: Window{Success: 1}, status: StatusUnhealthy},
		{name: "repeated_exact_probe_recovers", history: badHistory, probe: Window{Success: 2}, status: StatusHealthy},
		{name: "recent_failure_blocks_probe_recovery", short: Window{ServiceFailure: 1}, long: Window{ServiceFailure: 1}, history: badHistory, probe: Window{Success: 20}, status: StatusUnhealthy},
		{name: "failed_probe_overrides_old_success", long: Window{Success: 20}, probe: Window{ServiceFailure: 1}, status: StatusUnhealthy},
		{name: "new_route_unknown", status: StatusUnknown},
		{name: "actual_success_recovers_low_volume", short: Window{Success: 3}, long: Window{Success: 3}, history: badHistory, status: StatusHealthy},
		{name: "customer_failure_not_erased_by_probes", short: Window{Success: 3}, long: Window{Success: 3, ServiceFailure: 10}, probe: Window{Success: 20}, history: badHistory, status: StatusUnhealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.status, EvaluateEvidence(tc.short, tc.long, tc.history, tc.probe, .9).Status)
		})
	}
}

func TestSnapshotSeparatesExactModelProbesAndHistory(t *testing.T) {
	useTestDB(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Minute).Add(30 * time.Second)
	c := newAttemptCollector(model.SaveRouteAttemptSnapshots)
	for i := 0; i < 12; i++ {
		c.record(Attempt{ChannelID: 1, Group: "g", Model: "argon", Outcome: OutcomeServiceFailure, At: now.Add(-time.Hour)})
	}
	// Another model's successful tests cannot recover argon.
	for i := 0; i < 2; i++ {
		c.record(Attempt{ChannelID: 1, Group: "g", Model: "carbon", Probe: true, Outcome: OutcomeSuccess, At: now})
	}
	require.NoError(t, c.flush(ctx, now))
	require.NoError(t, refreshSnapshot(ctx, now))
	snapshotAt.Store(time.Now().Unix())
	require.Equal(t, StatusUnhealthy, Health(1, "argon").Status)
	require.Equal(t, StatusHealthy, Health(1, "carbon").Status)
	for i := 0; i < 2; i++ {
		c.record(Attempt{ChannelID: 1, Group: "g", Model: "argon", Probe: true, Outcome: OutcomeSuccess, At: now})
	}
	require.NoError(t, c.flush(ctx, now))
	require.NoError(t, refreshSnapshot(ctx, now))
	require.Equal(t, StatusHealthy, Health(1, "argon").Status)
	rows, err := model.QueryRouteAttemptHistory(ctx, now.Add(-24*time.Hour).Unix(), now.Unix())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 12, rows[0].ServiceFailureCount)
	require.Zero(t, rows[0].SuccessCount)
	reports, err := ChannelReports(ctx, now, 1440, "argon")
	require.NoError(t, err)
	require.Len(t, reports, 1)
	require.EqualValues(t, 12, reports[0].Window.Attempts)
	require.EqualValues(t, 2, reports[0].ProbeWindow.Success)
	// Expired tests cannot keep a historically bad route healthy indefinitely.
	require.NoError(t, refreshSnapshot(ctx, now.Add(6*time.Minute)))
	snapshotAt.Store(time.Now().Unix())
	require.Equal(t, StatusUnhealthy, Health(1, "argon").Status)
}

func TestSnapshotScheduledProbeFreshness(t *testing.T) {
	useTestDB(t)
	monitor := operation_setting.GetMonitorSetting()
	original := *monitor
	t.Cleanup(func() { *monitor = original })
	monitor.AutoTestChannelEnabled = true
	monitor.AutoTestChannelMinutes = 30
	require.Equal(t, 61*time.Minute, probeEvidenceWindow())
	ctx := context.Background()
	now := time.Now().Truncate(time.Minute).Add(30 * time.Second)
	c := newAttemptCollector(model.SaveRouteAttemptSnapshots)
	for i := 0; i < 3; i++ {
		c.record(Attempt{ChannelID: 1, Group: "g", Model: "m", Outcome: OutcomeServiceFailure, At: now.Add(-2 * time.Hour)})
	}
	for _, ago := range []time.Duration{45 * time.Minute, 15 * time.Minute} {
		c.record(Attempt{ChannelID: 1, Group: "g", Model: "m", Probe: true, Outcome: OutcomeSuccess, At: now.Add(-ago)})
	}
	require.NoError(t, c.flush(ctx, now))
	require.NoError(t, refreshSnapshot(ctx, now))
	snapshotAt.Store(time.Now().Unix())
	require.Equal(t, StatusHealthy, Health(1, "m").Status)
	require.Zero(t, Health(1, "m").Long.Success, "test evidence never becomes live traffic")
	require.NoError(t, refreshSnapshot(ctx, now.Add(17*time.Minute)))
	require.Equal(t, StatusUnhealthy, Health(1, "m").Status, "one remaining test cannot hold recovery")
	monitor.AutoTestChannelMinutes = 1e30
	require.Equal(t, 24*time.Hour, probeEvidenceWindow())
	monitor.AutoTestChannelMinutes = -1
	require.Equal(t, ShortWindow, probeEvidenceWindow())
}
