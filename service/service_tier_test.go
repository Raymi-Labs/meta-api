package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPaidServiceTierRoutingAndRegistry(t *testing.T) {
	oldRatios := ratio_setting.GroupRatio2JSONString()
	oldAuto, oldUsable := setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"primary":1,"backup":2}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["primary","backup"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"primary":"","backup":"","auto":""}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
	})
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprint(cached), func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			oldDB, oldCache := model.DB, common.MemoryCacheEnabled
			model.DB, common.MemoryCacheEnabled = db, cached
			t.Cleanup(func() {
				model.DB, common.MemoryCacheEnabled = oldDB, oldCache
				if oldDB != nil {
					model.InitChannelCache()
				}
				conn, _ := db.DB()
				conn.Close()
			})
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
			for i, config := range []struct{ group, settings string }{{"primary", `{}`}, {"primary", `{"allow_service_tier":true,"paid_service_tiers":{"tier-test":["priority"]}}`}, {"backup", `{"allow_service_tier":true,"paid_service_tiers":{"tier-test":["priority","ultrafast"]}}`}} {
				ch := &model.Channel{Id: i + 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: config.group, Models: "tier-test", OtherSettings: config.settings, Priority: common.GetPointer(int64(3 - i))}
				require.NoError(t, model.ValidatePaidServiceTierSettings(ch))
				require.NoError(t, db.Create(ch).Error)
				require.NoError(t, db.Create(&model.Ability{Group: config.group, Model: "tier-test", ChannelId: i + 1, Enabled: true, Priority: ch.Priority, Weight: 1}).Error)
			}
			model.InitChannelCache()
			managed, err := model.PaidServiceTierManaged(context.Background(), "tier-test")
			require.NoError(t, err)
			require.True(t, managed)
			legacy, err := model.PaidServiceTierManaged(context.Background(), "legacy")
			require.NoError(t, err)
			require.False(t, legacy)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "member")
			common.SetContextKey(c, constant.ContextKeyTokenAutoGroupFallback, true)
			common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
			c.Set(ManagedServiceTierKey, true)
			for _, tc := range []struct {
				tier string
				id   int
			}{{"default", 1}, {"priority", 2}, {"ultrafast", 3}} {
				c.Set(RequestedServiceTierKey, tc.tier)
				p := &RetryParam{Ctx: c, TokenGroup: "primary", ModelName: "tier-test", Retry: common.GetPointer(0)}
				ch, _, err := CacheGetRandomSatisfiedChannel(p)
				require.NoError(t, err)
				require.NotNil(t, ch)
				require.Equal(t, tc.id, ch.Id)
			}
			c.Set(RequestedServiceTierKey, "ultrafast")
			p := &RetryParam{Ctx: c, TokenGroup: "primary", ModelName: "tier-test", Retry: common.GetPointer(0), ExcludeChannels: []int{3}}
			ch, _, err := CacheGetRandomSatisfiedChannel(p)
			require.NoError(t, err)
			require.Nil(t, ch, "never fall back to an ordinary or fast-only channel")
			require.NoError(t, db.Model(&model.Channel{}).Where("id > ?", 1).Update("status", common.ChannelStatusManuallyDisabled).Error)
			model.InitChannelCache()
			managed, err = model.PaidServiceTierManaged(context.Background(), "tier-test")
			require.NoError(t, err)
			require.True(t, managed, "disabled routes retain policy")
		})
	}
}

func TestPaidServiceTierAuditFieldsSurviveTieredOverlay(t *testing.T) {
	info := &relaycommon.RelayInfo{ManagedServiceTier: true, RequestedServiceTier: "ultrafast", TieredBillingSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("base",p)*6`, QuotaPerUnit: 500000}}
	other := map[string]interface{}{}
	InjectTieredBillingInfo(other, info, &billingexpr.TieredResult{MatchedTier: "base", ActualQuotaBeforeGroup: 300, ActualQuotaAfterGroup: 300}, &billingexpr.TokenParams{P: 100})
	require.Equal(t, "ultrafast", other["requested_service_tier"])
	require.Equal(t, float64(6), other["service_tier_multiplier"])
	require.Equal(t, "tiered_expr", other["billing_mode"])
}
