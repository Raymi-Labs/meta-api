package helper

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseBillingIdentityPricePaths(t *testing.T) {
	oldGroup, oldSpecial := ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	oldModel, oldPrice, oldTime := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString(), ratio_setting.TimeRatioRules2JSONString()
	oldBundle := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecial))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModel))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
		require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(oldTime))
		billing_setting.ReplaceBundle(oldBundle)
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"enterprise-price":0.25,"enterprise-free":0}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"personal-auth":{"default":0.8},"enterprise-price":{"default":9},"enterprise-free":{"default":9}}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"payer-text":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"payer-call":0.2,"payer-image":0.2}`))
	require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(`[
		{"id":"enterprise-window","enabled":true,"timezone":"UTC","start":"10:00","end":"11:00","ratio":1.5,
		"models":["payer-*"],"groups":["default"],"user_groups":["enterprise-price"],"priority":1}
	]`))
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{
		BillingMode: map[string]string{"payer-task": billing_setting.BillingModeTieredExpr, "payer-tiered": billing_setting.BillingModeTieredExpr, "payer-image-expr": billing_setting.BillingModeTieredExpr},
		BillingExpr: map[string]string{"payer-task": `tier("base", 1000000)`, "payer-tiered": `tier("base", p * 2)`, "payer-image-expr": `tier("base", param("n") * 1000000)`},
	})
	for _, payer := range []struct {
		group            string
		ratio, timeRatio float64
	}{{"", 0.8, 1}, {"enterprise-price", 0.25, 1.5}, {"enterprise-free", 0, 1}} {
		for _, path := range []string{"text", "tiered", "image", "image-expr", "call", "task"} {
			t.Run(payer.group+"/"+path, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{UserGroup: "personal-auth", EnterpriseBillingGroup: payer.group, UsingGroup: "default",
					OriginModelName: "payer-" + path, StartTime: time.Date(2026, 7, 7, 10, 30, 0, 0, time.UTC),
					BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"n":1}`)},
					ChannelMeta:         &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}
				if payer.group != "" {
					info.EnterpriseId, info.EnterpriseMemberId = 11, 21
					info.BillingSource = service.BillingSourceEnterprise
				}
				if strings.HasPrefix(path, "image") {
					info.Request = &dto.ImageRequest{}
				}
				var price types.PriceData
				var err error
				switch path {
				case "text", "tiered", "image", "image-expr":
					price, err = ModelPriceHelper(c, info, 100, &types.TokenCountMeta{MaxTokens: 50})
				case "call":
					price, err = ModelPriceHelperPerCall(c, info)
				case "task":
					price, err = ModelPriceHelperTask(c, info)
				}
				require.NoError(t, err)
				require.Equal(t, payer.ratio, price.GroupRatioInfo.GroupRatio)
				require.Equal(t, payer.timeRatio, price.TimeRatioInfo.EffectiveRatio())
				require.Equal(t, "personal-auth", info.UserGroup)
				require.Equal(t, "default", info.UsingGroup)
				// Image re-reservation and log generation must consume the frozen
				// enterprise prices even if settings change after request admission.
				frozenSpecial, frozenTime := ratio_setting.GroupGroupRatio2JSONString(), ratio_setting.TimeRatioRules2JSONString()
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(frozenSpecial))
					require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(frozenTime))
				})
				require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"personal-auth":{"default":9},"enterprise-price":{"default":9}}`))
				require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(`[]`))
				if strings.HasPrefix(path, "image") {
					require.Nil(t, service.PrepareImageBilling(c, info, 2, false, false))
					unit := 0.2
					if path == "image-expr" {
						unit = 1
					}
					expected := int(math.Round(2 * unit * common.QuotaPerUnit * payer.ratio * payer.timeRatio))
					require.Equal(t, expected, info.PriceData.QuotaToPreConsume)
					require.Nil(t, service.PrepareImageBilling(c, info, 2, false, false))
					require.Equal(t, expected, info.PriceData.QuotaToPreConsume)
				}
				other := service.GenerateTextOtherInfo(c, info, price.ModelRatio, price.GroupRatioInfo.GroupRatio,
					price.CompletionRatio, 0, 0, price.ModelPrice, price.GroupRatioInfo.GroupSpecialRatio)
				require.Equal(t, payer.ratio, other["group_ratio"])
				if payer.group == "" {
					require.Equal(t, payer.ratio, other["user_group_ratio"])
				} else {
					require.Equal(t, float64(-1), other["user_group_ratio"])
					require.Equal(t, payer.group, other["enterprise_billing_group"])
				}
				if payer.timeRatio != 1 {
					require.Equal(t, payer.timeRatio, other["time_ratio"])
					require.Equal(t, "enterprise-window", other["time_ratio_rule"])
				}
				if payer.group != "" {
					require.Equal(t, service.BillingSourceEnterprise, other["billing_source"])
					require.Equal(t, 11, other["enterprise_id"])
					require.Equal(t, 21, other["enterprise_member_id"])
				}
				if info.TieredBillingSnapshot != nil {
					require.Equal(t, payer.ratio, info.TieredBillingSnapshot.GroupRatio)
					service.InjectTieredBillingInfo(other, info, nil, nil)
					require.Equal(t, "tiered_expr", other["billing_mode"])
				}
				if path == "call" || path == "task" {
					callLog := service.GenerateMjOtherInfo(info, price)
					require.Equal(t, payer.ratio, callLog["group_ratio"])
					if payer.group == "" {
						require.Equal(t, payer.ratio, callLog["user_group_ratio"])
					} else {
						require.NotContains(t, callLog, "user_group_ratio")
					}
					if payer.timeRatio != 1 {
						require.Equal(t, payer.timeRatio, callLog["time_ratio"])
					}
				}
			})
		}
	}
}
