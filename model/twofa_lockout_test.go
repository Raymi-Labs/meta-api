package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIncrementFailedAttemptsRestartsWindowAfterLockoutExpires(t *testing.T) {
	for _, tc := range []struct {
		name            string
		lockedUntil     time.Duration
		failures        int
		wantAttempts    int
		wantLocked      bool
		keepLockedUntil bool
	}{
		{name: "first failure after expiry", lockedUntil: -time.Minute, failures: 1, wantAttempts: 1, wantLocked: false},
		{name: "max failures after expiry", lockedUntil: -time.Minute, failures: common.MaxFailAttempts, wantAttempts: common.MaxFailAttempts, wantLocked: true},
		{name: "failure during active lockout", lockedUntil: time.Minute, failures: 1, wantAttempts: common.MaxFailAttempts, wantLocked: true, keepLockedUntil: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)

			user := User{Username: "twofa-expiry-user", Password: "password"}
			require.NoError(t, DB.Create(&user).Error)
			lockedUntil := time.Now().Add(tc.lockedUntil)
			twoFA := TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: true, FailedAttempts: common.MaxFailAttempts, LockedUntil: &lockedUntil}
			require.NoError(t, DB.Create(&twoFA).Error)
			var stored TwoFA
			require.NoError(t, DB.First(&stored, twoFA.Id).Error)

			for range tc.failures {
				require.NoError(t, (&TwoFA{Id: twoFA.Id}).IncrementFailedAttempts())
			}

			var reloaded TwoFA
			require.NoError(t, DB.First(&reloaded, twoFA.Id).Error)
			assert.Equal(t, tc.wantAttempts, reloaded.FailedAttempts)
			assert.Equal(t, tc.wantLocked, reloaded.IsLocked())
			if !tc.wantLocked {
				assert.Nil(t, reloaded.LockedUntil)
			}
			if tc.keepLockedUntil {
				require.NotNil(t, reloaded.LockedUntil)
				assert.True(t, stored.LockedUntil.Equal(*reloaded.LockedUntil), "lockout must not be extended")
			}
		})
	}
}
