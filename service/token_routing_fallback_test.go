package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestTokenConfiguredGroupsAutoFallbackIsOptInPerConfig(t *testing.T) {
	oldRatios, oldAuto, oldUsable := ratio_setting.GroupRatio2JSONString(), setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"a":1,"b":2,"c":3,"d":4}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"a":"","b":"","c":"","d":""}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["c","a","d"]`))

	groups := func(token *model.Token, name string) []string {
		got, err := TokenConfiguredGroups("member", token, name)
		require.NoError(t, err)
		return got
	}

	legacy := &model.Token{RoutingConfig: `{"groups":["b","a"]}`, AutoGroupFallback: true}
	require.Equal(t, []string{"b", "a"}, groups(legacy, ""), "the legacy flag never expands a multi-group token")

	optIn := &model.Token{RoutingConfig: `{"groups":["b","a"],"auto_fallback":true}`}
	require.Equal(t, []string{"b", "a", "c", "d"}, groups(optIn, ""), "own order first, then remaining platform groups")
	require.Equal(t, []string{"b", "a", "c", "d"}, groups(optIn, "m"))

	scoped := &model.Token{RoutingConfig: `{"groups":["b","a"],"model_groups":{"m":["a"]},"auto_fallback":true}`}
	require.Equal(t, []string{"a"}, groups(scoped, "m"), "a per-model rule is never extended")
	require.Equal(t, []string{"b", "a", "c", "d"}, groups(scoped, "other"))

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"c":"","d":""}`))
	require.Empty(t, groups(optIn, ""), "a revoked scope is not refilled from fallback groups")
}
