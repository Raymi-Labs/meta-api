package router

import (
	"os"
	"testing"

	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
)

func TestMain(m *testing.M) {
	// HTTP tests replace the global database per fixture. Metrics may record,
	// but their process-lifetime flush ticker must not outlive a test database.
	grouphealth.UseManualFlushForTesting()
	os.Exit(m.Run())
}
