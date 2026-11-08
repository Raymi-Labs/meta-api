package service

import (
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResolveTokenGroup(t *testing.T) {
	oldUsers, oldRatios, oldUsable := setting.UserGroups2JSONString(), ratio_setting.GroupRatio2JSONString(), setting.UserUsableGroups2JSONString()
	oldSpecial := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		setting.UpdateUserGroupsByJSONString(oldUsers)
		ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
		setting.UpdateUserUsableGroupsByJSONString(oldUsable)
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(oldSpecial)
	})
	require.NoError(t, setting.UpdateUserGroupsByJSONString(`{"default":"","identity":""}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"legacy":2,"explicit":3}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":""}`))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(nil)
	for _, tc := range []struct{ user, token, want string }{
		{"identity", "", "default"}, {"legacy", "", "legacy"}, {"unknown", "", "unknown"}, {"identity", "explicit", "explicit"}, {"identity", "deleted", "deleted"}, {"identity", "auto", "auto"}, {"auto", "", "auto"},
	} {
		require.Equal(t, tc.want, ResolveTokenGroup(tc.user, tc.token))
	}
	require.Equal(t, []string{"default"}, TokenRoutingGroups("identity", "", false))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(map[string]map[string]string{"identity": {"-:default": ""}})
	require.Equal(t, "identity", ResolveTokenGroup("identity", ""))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(nil)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"legacy":2}`))
	require.Equal(t, "identity", ResolveTokenGroup("identity", ""))
}
