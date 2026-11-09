package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPaidServiceTierAdminValidation(t *testing.T) {
	for _, tc := range []struct {
		settings string
		valid    bool
	}{
		{`{}`, true}, {`{"allow_service_tier":true,"paid_service_tiers":{"astra":["priority","ultrafast"]}}`, true},
		{`{"paid_service_tiers":{"astra":["priority"]}}`, false},
		{`{"allow_service_tier":true,"paid_service_tiers":{"other":["priority"]}}`, false},
		{`{"allow_service_tier":true,"paid_service_tiers":{"astra":["fast"]}}`, false},
		{`{"allow_service_tier":true,"paid_service_tiers":{"astra":["priority","priority"]}}`, false},
	} {
		ch := &Channel{Type: constant.ChannelTypeOpenAI, Models: "astra", OtherSettings: tc.settings}
		err := ValidatePaidServiceTierSettings(ch)
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	ch := &Channel{Type: constant.ChannelTypeAnthropic, Models: "astra", OtherSettings: `{"allow_service_tier":true,"paid_service_tiers":{"astra":["priority"]}}`}
	require.Error(t, ValidatePaidServiceTierSettings(ch))
}
