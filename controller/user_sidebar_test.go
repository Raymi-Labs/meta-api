package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestDefaultSidebarConfigRetiresPulse(t *testing.T) {
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		var config map[string]map[string]bool
		require.NoError(t, common.Unmarshal([]byte(generateDefaultSidebarConfig(role)), &config))
		require.True(t, config["personal"]["enabled"])
		require.True(t, config["personal"]["topup"])
		require.NotContains(t, config["personal"], "pulse")
		require.NotContains(t, config["admin"], "pulseOps")
	}
}
