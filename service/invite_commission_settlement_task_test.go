package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestInviteCommissionRecoveryTriggers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wakeups := make(chan struct{}, 1)
	ticks := make(chan time.Time, 1)
	runs := make(chan struct{}, 3)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runInviteCommissionSettlementLoop(ctx, wakeups, ticks, func() { runs <- struct{}{} })
	}()
	expectRun := func() {
		t.Helper()
		select {
		case <-runs:
		case <-time.After(time.Second):
			t.Fatal("settlement was not triggered")
		}
	}
	expectRun() // startup recovers pending work even if the previous process lost its signal
	wakeups <- struct{}{}
	expectRun()
	ticks <- time.Now()
	expectRun() // fallback for lost wakeups / another instance / retries
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop did not stop")
	}
	require.Empty(t, runs)
}
