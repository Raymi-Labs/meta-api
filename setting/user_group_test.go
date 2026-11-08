package setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUserGroupsRequiresDefaultAndCleanNames(t *testing.T) {
	groups, err := ParseUserGroups(`{"default":"默认","vip":"VIP 用户","分销商":""}`)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"default": "默认", "vip": "VIP 用户", "分销商": ""}, groups)

	for _, raw := range []string{
		`{"vip":"no default"}`,
		`{"default":"","":"empty"}`,
		`{"default":""," vip":"padded"}`,
		`["default"]`,
		`not json`,
		`{"default":"","` + strings.Repeat("长", 65) + `":""}`,
	} {
		_, err := ParseUserGroups(raw)
		require.Error(t, err, raw)
	}
}

func TestUpdateUserGroupsKeepsPreviousListOnError(t *testing.T) {
	original := UserGroups2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateUserGroupsByJSONString(original)) })

	require.NoError(t, UpdateUserGroupsByJSONString(`{"default":"","vip":"VIP"}`))
	require.True(t, IsRegisteredUserGroup("vip"))
	require.False(t, IsRegisteredUserGroup("svip"))

	require.Error(t, UpdateUserGroupsByJSONString(`{"svip":""}`))
	require.True(t, IsRegisteredUserGroup("vip"))
	require.Equal(t, map[string]string{"default": "", "vip": "VIP"}, GetUserGroupsCopy())
}
