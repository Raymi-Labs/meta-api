package controller

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const relayMatrixProcessKey = "NEWAPI_RELAY_MATRIX_TEST_PROCESS"

// A full Relay starts process-wide health collection. Its background worker
// outlives per-case databases, so keep each complete matrix in its own process
// instead of leaking that worker into unrelated controller tests. Reusing this
// executable also preserves race instrumentation without changing production.
func runRelayMatrixInSubprocess(t *testing.T) bool {
	t.Helper()
	if os.Getenv(relayMatrixProcessKey) == t.Name() {
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	timeout := 2 * time.Minute
	if deadline, ok := t.Deadline(); ok {
		if remaining := time.Until(deadline) - time.Second; remaining < timeout {
			timeout = remaining
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1", "-test.timeout="+timeout.String())
	cmd.Env = append(os.Environ(), relayMatrixProcessKey+"="+t.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated Relay matrix failed: %v\n%s", err, output)
	}
	return true
}

func useRelayMatrixProcessDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.Equal(t, t.Name(), os.Getenv(relayMatrixProcessKey), "database must live in its dedicated matrix process")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled = false
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Midjourney{}, &model.Ability{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.GroupHealthMetric{}))
	// Keep this pointer and connection valid through the race detector's exit
	// wait too. The process, rather than a test cleanup, owns their lifetime.
	return db
}

func resetRelayMatrixFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Health snapshots belong to the running process. Only reset business rows
	// between cases; the collector keeps its real table and stable DB pointer.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for _, row := range []any{&model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.ContentSafetyViolation{}, &model.Log{}, &model.Token{}, &model.User{}, &model.Ability{}, &model.Channel{}} {
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(row).Error; err != nil {
				return err
			}
		}
		return nil
	}))
}
