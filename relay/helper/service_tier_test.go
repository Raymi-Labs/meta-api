package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaidServiceTierPreconsumeAndFrozenSettlement(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	base := `len <= 272000 ? tier("standard",p*10+c*50+cr+cc*12.5) : tier("long",p*20+c*75+cr*2+cc*25)`
	expressions, _ := common.Marshal(map[string]string{"tier-test": base})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{"tier-test":"tiered_expr"}`, "billing_setting.billing_expr": string(expressions)}))
	for _, length := range []int{1000, 300000} {
		var basePre, baseSettled int
		for _, tc := range []struct {
			tier   string
			factor int
		}{{"default", 1}, {"priority", 2}, {"ultrafast", 6}} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			c.Set("group", "default")
			info := &relaycommon.RelayInfo{ManagedServiceTier: true, RequestedServiceTier: tc.tier, OriginModelName: "tier-test", UserGroup: "default", UsingGroup: "default", BillingRequestInput: &billingexpr.RequestInput{}}
			price, err := ModelPriceHelper(c, info, length, &types.TokenCountMeta{MaxTokens: 100})
			require.NoError(t, err)
			result, err := billingexpr.ComputeTieredQuotaWithRequest(info.TieredBillingSnapshot, billingexpr.TokenParams{P: 1000, C: 100, CR: 100, CC: 100, Len: float64(length)}, *info.BillingRequestInput)
			require.NoError(t, err)
			if tc.factor == 1 {
				basePre = price.QuotaToPreConsume
				baseSettled = result.ActualQuotaAfterGroup
			}
			require.Equal(t, basePre*tc.factor, price.QuotaToPreConsume)
			require.Equal(t, baseSettled*tc.factor, result.ActualQuotaAfterGroup)
		}
	}
	expr, _ := common.Marshal(map[string]string{"tier-test": `param("service_tier")=="priority"?tier("fast",p*2):tier("base",p)`})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": string(expr)}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := &relaycommon.RelayInfo{ManagedServiceTier: true, RequestedServiceTier: "priority", OriginModelName: "tier-test", UserGroup: "default", UsingGroup: "default"}
	_, err := ModelPriceHelper(c, info, 100, &types.TokenCountMeta{})
	require.Error(t, err)
	require.Nil(t, info.TieredBillingSnapshot)
}
