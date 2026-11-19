package common

import "testing"

func TestInviteTierPartialSettings(t *testing.T) {
	original := GetInviteTierSettings()
	for _, raw := range []string{`{"enabled":false}`, `{"leader_slot_rates":[0.03,0.03,0.02,0.02]}`} {
		got, err := ParseInviteTierSettings(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got.FirstLevelRates != original.FirstLevelRates || got.SecondLevelMatrix != original.SecondLevelMatrix {
			t.Fatal("partial patch erased rates")
		}
	}
	for _, raw := range []string{`{"enabled":true}`, `null`, `[]`, `{"first_level_rates":[0.1]}`, `{"second_level_matrix":[[0.1]]}`, `{"leader_slot_rates":null}`, `{"leader_slot_rates":[0.2,0.2,0.2,0.2]}`} {
		if _, err := ParseInviteTierSettings(raw); err == nil {
			t.Fatalf("accepted invalid settings %s", raw)
		}
	}
}
