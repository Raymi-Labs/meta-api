package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestPaidServiceTierCapabilitiesScope(t *testing.T) {
	db := useControllerCapacityTestDB(t)
 oldRedis:=common.RedisEnabled;common.RedisEnabled=false;t.Cleanup(func(){common.RedisEnabled=oldRedis})
	oldCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldCache })
	oldGroups := setting.UserUsableGroups2JSONString()
	oldRatios := ratio_setting.GroupRatio2JSONString()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":""}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{"tier-test":"tiered_expr"}`, "billing_setting.billing_expr": `{"tier-test":"tier(\"base\",p*10+c*50)"}`}))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}))
	require.NoError(t, db.Create(&model.User{Id: 19, Username: "tier-fixture", AffCode: "tier-fixture", Group: "default", Role: common.RoleCommonUser}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "default", Models: "tier-test", OtherSettings: `{"allow_service_tier":true,"paid_service_tiers":{"tier-test":["priority","ultrafast"]}}`}).Error)
	require.NoError(t, db.Create(&model.Ability{ChannelId: 1, Group: "default", Model: "tier-test", Enabled: true, Weight: 1}).Error)
	call := func(id int, group string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/user/service-tiers?model=tier-test&group="+group+"&user_id=1", nil)
		c.Set("id", id)
		GetUserServiceTiers(c)
		return w
	}
	require.Equal(t, 401, call(0, "default").Code)
	require.Equal(t, 403, call(19, "private").Code)
	w := call(19, "default")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"id":"ultrafast"`)
	require.NotContains(t, w.Body.String(), "channel")
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Update("status", common.ChannelStatusManuallyDisabled).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 1).Update("enabled", false).Error)
	w = call(19, "default")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"managed":true`)
	require.NotContains(t, w.Body.String(), `"id":"ultrafast"`)
}
