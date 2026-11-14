package helper

import (
	"math"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseSelectedGroupOverridesRoutesAndFreezesRequest(t *testing.T) {
	groups, special := ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(special))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"preferred":0.8,"cheap":0.2,"free":0}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"personal":{"default":0.4,"cheap":0.1},"preferred":{"default":9,"cheap":9}}`))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{EnterpriseId: 1, EnterpriseBillingGroup: "preferred", UserGroup: "personal", UsingGroup: "default"}
	got, err := HandleGroupRatio(c, info)
	require.NoError(t, err)
	require.Equal(t, 0.8, got.GroupRatio)
	require.False(t, got.HasSpecialRatio)
	require.Equal(t, "personal", info.UserGroup)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"preferred":0.6,"cheap":0.2,"free":0}`))
	c.Set("auto_group", "cheap")
	got, err = HandleGroupRatio(c, info)
	require.NoError(t, err)
	require.Equal(t, "cheap", info.UsingGroup)
	require.Equal(t, 0.8, got.GroupRatio, "an admitted request keeps its selected enterprise price across routing retries")
	next := &relaycommon.RelayInfo{EnterpriseId: 1, EnterpriseBillingGroup: "preferred", UserGroup: "personal", UsingGroup: "default"}
	got, err = HandleGroupRatio(c, next)
	require.NoError(t, err)
	require.Equal(t, 0.6, got.GroupRatio, "new requests use the updated group's multiplier")
	for group, want := range map[string]float64{"default": 1, "free": 0} {
		got, err = HandleGroupRatio(c, &relaycommon.RelayInfo{EnterpriseId: 1, EnterpriseBillingGroup: group, UserGroup: "personal", UsingGroup: "cheap"})
		require.NoError(t, err)
		require.Equal(t, want, got.GroupRatio)
	}
	got, err = HandleGroupRatio(c, &relaycommon.RelayInfo{UserGroup: "personal", UsingGroup: "default"})
	require.NoError(t, err)
	require.Equal(t, 0.1, got.GroupRatio, "personal requests retain actual-route special pricing")
}

func TestEnterpriseInvalidBillingGroupFailsBeforeAllPricePaths(t *testing.T) {
	old := ratio_setting.GetGroupRatioCopy()
	t.Cleanup(func() { ratio_setting.GetGroupRatioSetting().GroupRatio.ReplaceAll(old) })
	ratio_setting.GetGroupRatioSetting().GroupRatio.ReplaceAll(map[string]float64{"default": 1, "auto": 0.8, "negative": -1, "nan": math.NaN(), "inf": math.Inf(1)})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, group := range []string{"", "missing", "auto", "negative", "nan", "inf"} {
		t.Run(group, func(t *testing.T) {
			info := &relaycommon.RelayInfo{EnterpriseId: 1, EnterpriseBillingGroup: group, UserGroup: "default", UsingGroup: "default", OriginModelName: "missing-test-model"}
			_, err := ModelPriceHelper(c, info, 100, &types.TokenCountMeta{})
			require.Error(t, err)
			_, err = ModelPriceHelperPerCall(c, info)
			require.Error(t, err)
			_, err = ModelPriceHelperTask(c, info)
			require.Error(t, err)
			require.Nil(t, info.EnterpriseBillingRatio)
		})
	}
}
