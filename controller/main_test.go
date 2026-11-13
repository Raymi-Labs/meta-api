package controller

import (
	"os"
	"testing"

	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
)

func TestMain(m *testing.M) {
	// Controller tests own short-lived databases. Keep metrics recording and
	// explicit Flush assertions, but do not let background loops outlive them.
	grouphealth.UseManualFlushForTesting()
	routeinsight.SetHealthForTesting()
	os.Exit(m.Run())
}
