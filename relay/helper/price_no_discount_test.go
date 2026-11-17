package helper

import (
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNoDiscountPriceAcrossGroupsAndSnapshots(t *testing.T) {
	oldModels := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModels)) })
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":2}`))
	old, groups, specials, times := ratio_setting.NoDiscountModelsJSON(), ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString(), ratio_setting.TimeRatioRules2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateNoDiscountModels(old))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(specials))
		require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(times))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0.3,"free":0,"enterprise":0.2}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"default":0.1,"free":0}}`))
	require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(`[{"id":"sale","enabled":true,"timezone":"UTC","start":"00:00","end":"23:59","ratio":0.5}]`))
	require.NoError(t, ratio_setting.UpdateNoDiscountModels(`["gpt-4o"]`))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, enterprise := range []bool{false, true} {
		info := &relaycommon.RelayInfo{OriginModelName: "gpt-4o", UserGroup: "vip", UsingGroup: "default", StartTime: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)}
		if enterprise {
			info.EnterpriseId = 1
			info.EnterpriseBillingGroup = "enterprise"
		}
		price, err := ModelPriceHelper(c, info, 100, &types.TokenCountMeta{})
		require.NoError(t, err)
		require.Equal(t, 1.0, price.GroupRatioInfo.GroupRatio)
		require.False(t, price.GroupRatioInfo.HasSpecialRatio)
		require.Equal(t, 1.0, price.TimeRatioInfo.EffectiveRatio())
		require.False(t, price.FreeModel)
		c.Set("auto_group", "free")
		perCall, err := ModelPriceHelperPerCall(c, info)
		require.NoError(t, err)
		require.Equal(t, 1.0, perCall.GroupRatioInfo.GroupRatio)
		require.Equal(t, "free", info.UsingGroup)
		require.NoError(t, ratio_setting.UpdateNoDiscountModels(`[]`))
		again, err := HandleGroupRatio(c, info)
		require.NoError(t, err)
		require.Equal(t, 1.0, again.GroupRatio)
		require.Equal(t, 1.0, info.ResolvePricingTimeRatio().EffectiveRatio())
		require.NoError(t, ratio_setting.UpdateNoDiscountModels(`["gpt-4o"]`))
	}
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	normal := &relaycommon.RelayInfo{OriginModelName: "gpt-4o-alias", UserGroup: "vip", UsingGroup: "default", StartTime: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)}
	got, err := HandleGroupRatio(c, normal)
	require.NoError(t, err)
	require.Equal(t, 0.1, got.GroupRatio)
	require.Equal(t, 0.5, normal.ResolvePricingTimeRatio().EffectiveRatio())
	require.NoError(t, ratio_setting.UpdateNoDiscountModels(`["gpt-4o-alias"]`))
	require.False(t, normal.UsesBaseModelPrice(), "previously discounted request retains its policy")
	require.True(t, (&relaycommon.RelayInfo{OriginModelName: "gpt-4o-alias"}).UsesBaseModelPrice())
}

func TestNoDiscountTieredSettlementPreservesPaidAcceleration(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	old := ratio_setting.NoDiscountModelsJSON()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		require.NoError(t, ratio_setting.UpdateNoDiscountModels(old))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"luna-test":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"luna-test":"p*2+c*8+cr*0.2"}`,
		"group_ratio_setting.group_ratio": `{"default":0.1}`,
	}))
	require.NoError(t, ratio_setting.UpdateNoDiscountModels(`["luna-test"]`))
	var basePre, baseSettled int
	for _, tier := range []string{"default", "priority"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		info := &relaycommon.RelayInfo{OriginModelName: "luna-test", UsingGroup: "default", UserGroup: "default", ManagedServiceTier: true, RequestedServiceTier: tier, BillingRequestInput: &billingexpr.RequestInput{}}
		price, err := ModelPriceHelper(c, info, 1000, &types.TokenCountMeta{MaxTokens: 100})
		require.NoError(t, err)
		result, err := billingexpr.ComputeTieredQuotaWithRequest(info.TieredBillingSnapshot, billingexpr.TokenParams{P: 1000, C: 100, CR: 100, Len: 1100}, *info.BillingRequestInput)
		require.NoError(t, err)
		require.Equal(t, 1.0, price.GroupRatioInfo.GroupRatio)
		if tier == "default" {
			basePre = price.QuotaToPreConsume
			baseSettled = result.ActualQuotaAfterGroup
		} else {
			require.Equal(t, basePre*2, price.QuotaToPreConsume)
			require.Equal(t, baseSettled*2, result.ActualQuotaAfterGroup)
		}
	}
}
