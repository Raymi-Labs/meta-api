package ratio_setting

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSpecialGroupRatiosValidateBeforeReplacing(t *testing.T) {
	old := GroupGroupRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateGroupGroupRatioByJSONString(old)) })
	const good = `{"vip":{"paid":0.0001,"free":0}}`
	require.NoError(t, UpdateGroupGroupRatioByJSONString(good))
	for _, bad := range []string{`null`, `[]`, `1`, `{"vip":null}`, `{"vip":{"paid":null}}`, `{"vip":{"paid":-1}}`, `{"vip":{"paid":"0.2"}}`, `{"vip":{"paid":1e999}}`, `{"vip":{"paid":0.5},"bad":3}`, `{"vip":`} {
		require.Error(t, UpdateGroupGroupRatioByJSONString(bad), bad)
		ratio, ok := GetGroupGroupRatio("vip", "paid")
		require.True(t, ok)
		require.Equal(t, 0.0001, ratio)
		ratio, ok = GetGroupGroupRatio("vip", "free")
		require.True(t, ok)
		require.Zero(t, ratio)
	}
	for _, empty := range []string{"", " \n\t", "{}"} {
		require.NoError(t, UpdateGroupGroupRatioByJSONString(good))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(empty))
		require.Equal(t, "{}", GroupGroupRatio2JSONString())
		normalized, err := NormalizeGroupGroupRatioJSON(empty)
		require.NoError(t, err)
		require.Equal(t, "{}", normalized)
	}
	require.NoError(t, UpdateGroupGroupRatioByJSONString(good))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"free":0}}`))
	_, ok := GetGroupGroupRatio("vip", "paid")
	require.False(t, ok, "removed rules must not survive replacement")
}
