package helper

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRoutePlanPriceIgnoresActualGroupAndSpecialRatios(t *testing.T) {
	old, oldSpecial := ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(old))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecial))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"cheap":0.5,"premium":3}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"premium":0.1}}`))

	plan := &types.RoutePlanSnapshot{ID: 7, Version: 3, Groups: []string{"cheap", "premium"}, SaleRatio: 1.2,
		ModelRatios: map[string]float64{"special-model": 2}}
	for _, group := range []string{"cheap", "premium"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("auto_group", group)
		info := &relaycommon.RelayInfo{UserGroup: "vip", UsingGroup: "cheap", OriginModelName: "gpt-x", RoutePlan: plan}
		got := HandleGroupRatio(c, info)
		require.Equal(t, 1.2, got.GroupRatio, "routing to %s must not change the contract price", group)
		require.False(t, got.HasSpecialRatio, "user-group special ratios do not apply to plans")
		require.Equal(t, group, info.UsingGroup, "the actual group is still recorded")
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{UsingGroup: "premium", OriginModelName: "special-model", RoutePlan: plan}
	require.Equal(t, 2.0, HandleGroupRatio(c, info).GroupRatio, "per-model plan price wins")

	require.Equal(t, types.TimeRatioInfo{}, info.PricingTimeRatio(), "plan price has no time rule")
	manual := &relaycommon.RelayInfo{UsingGroup: "premium", OriginModelName: "gpt-x", StartTime: time.Now()}
	require.Equal(t, ratio_setting.ResolveTimeRatio("gpt-x", "premium", "", manual.StartTime), manual.PricingTimeRatio())

	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	manual = &relaycommon.RelayInfo{UserGroup: "vip", UsingGroup: "premium", OriginModelName: "gpt-x"}
	require.Equal(t, 0.1, HandleGroupRatio(c, manual).GroupRatio, "manual tokens keep the existing contract")
}
