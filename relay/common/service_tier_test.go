package common

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaidServiceTierFinalBodyContract(t *testing.T) {
	for _, tc := range []struct {
		selected, body string
		factor         float64
		valid          bool
	}{
		{"default", `{}`, 1, true}, {"default", `{"service_tier":"default"}`, 1, true},
		{"default", `{"service_tier":"priority"}`, 1, false},
		{"priority", `{"service_tier":"priority","input":"hi"}`, 2, true},
		{"priority", `{}`, 2, false}, {"priority", `{"service_tier":"default"}`, 2, false},
		{"priority", `{"service_tier":"ultrafast","service_tier":"priority"}`, 2, false},
		{"priority", `{"Service_Tier":"priority"}`, 2, false},
		{"ultrafast", `{"service_tier":"ultrafast"}`, 6, true}, {"ultrafast", `{"service_tier":"priority"}`, 6, false},
	} {
		info := &RelayInfo{ManagedServiceTier: true, RequestedServiceTier: tc.selected}
		require.Equal(t, tc.factor, info.ServiceTierMultiplier())
		reader, err := ValidatePaidServiceTierBody(info, strings.NewReader(tc.body))
		if tc.valid {
			require.NoError(t, err)
			raw, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, tc.body, string(raw))
		} else {
			require.Error(t, err)
			require.Nil(t, reader)
		}
	}
	original := strings.NewReader(`legacy-not-json`)
	reader, err := ValidatePaidServiceTierBody(&RelayInfo{}, original)
	require.NoError(t, err)
	require.Same(t, original, reader)
}
