package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setRouteRouting(t *testing.T, values map[string]string) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(values))
}

func seedRoutePool(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	originDB, originLogDB := model.DB, model.LOG_DB
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originDB, originLogDB
		common.MemoryCacheEnabled = originMemoryCacheEnabled
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	high, low := int64(10), int64(1)
	channels := []model.Channel{
		{Id: 1, Name: "a-cheap-broken", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Priority: &high},
		{Id: 2, Name: "a-healthy", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Priority: &low},
		{Id: 3, Name: "b-healthy-cheap", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Priority: &high},
	}
	require.NoError(t, db.Create(&channels).Error)
	abilities := []model.Ability{
		{Group: "pool-a", Model: "m", ChannelId: 1, Enabled: true, Priority: &high, Weight: 100},
		{Group: "pool-a", Model: "m", ChannelId: 2, Enabled: true, Priority: &low, Weight: 100},
		{Group: "pool-b", Model: "m", ChannelId: 3, Enabled: true, Priority: &high, Weight: 100},
	}
	require.NoError(t, db.Create(&abilities).Error)
	model.InitChannelCache()
}

func healthy(score float64) routeinsight.RouteHealth {
	return routeinsight.RouteHealth{Status: routeinsight.StatusHealthy, Score: score}
}

func routeParam() *RetryParam {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return &RetryParam{Ctx: ctx, TokenGroup: "pool-a", ModelName: "m", Retry: common.GetPointer(0)}
}

func TestHealthRoutingSkipsUnhealthyOnlyWhenReplaceable(t *testing.T) {
	seedRoutePool(t)
	restore := routeinsight.SetHealthForTesting(
		routeinsight.HealthEntry{ChannelID: 1, Model: "m", Health: routeinsight.RouteHealth{Status: routeinsight.StatusUnhealthy}},
		routeinsight.HealthEntry{ChannelID: 2, Model: "m", Health: healthy(0.9)},
	)
	t.Cleanup(restore)

	channel, _, err := CacheGetRandomSatisfiedChannel(routeParam())
	require.NoError(t, err)
	require.Equal(t, 1, channel.Id, "off by default: production priority stands")

	setRouteRouting(t, map[string]string{"route_routing_setting.health_routing_enabled": "true", "route_routing_setting.probe_percent": "0"})
	for i := 0; i < 5; i++ {
		channel, group, err := CacheGetRandomSatisfiedChannel(routeParam())
		require.NoError(t, err)
		require.Equal(t, 2, channel.Id, "unhealthy higher-priority line is skipped inside the same group")
		require.Equal(t, "pool-a", group, "health routing never changes the group")
	}
	param := routeParam()
	param.ExcludeChannels = []int{2}
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, 1, channel.Id, "the only remaining line is still served")

	setRouteRouting(t, map[string]string{"route_routing_setting.health_routing_enabled": "true", "route_routing_setting.probe_percent": "20"})
	param = routeParam()
	param.routeProbe, param.routeProbeKey = 0.01, 1
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, 1, channel.Id, "a won probe lets the unhealthy line recover")
	require.True(t, RouteHealthAvoids(routeParam().Ctx, "m", 1), "session affinity releases an unhealthy line")
	require.False(t, RouteHealthAvoids(routeParam().Ctx, "m", 2))
}

func TestHealthRoutingSkipsWholeUnhealthyGroupInTokenOrder(t *testing.T) {
	seedRoutePool(t)
	oldRatios, oldUsable := ratio_setting.GroupRatio2JSONString(), setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"pool-a":1,"pool-b":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"pool-a":"","pool-b":""}`))
	unhealthy := routeinsight.RouteHealth{Status: routeinsight.StatusUnhealthy}
	multiGroup := func() *RetryParam {
		param := routeParam()
		common.SetContextKey(param.Ctx, constant.ContextKeyUserGroup, "member")
		common.SetContextKey(param.Ctx, constant.ContextKeyTokenRoutingConfig, `{"groups":["pool-a","pool-b"]}`)
		return param
	}

	restore := routeinsight.SetHealthForTesting(
		routeinsight.HealthEntry{ChannelID: 1, Model: "m", Health: unhealthy},
		routeinsight.HealthEntry{ChannelID: 2, Model: "m", Health: unhealthy},
		routeinsight.HealthEntry{ChannelID: 3, Model: "m", Health: healthy(0.99)},
	)
	t.Cleanup(restore)

	channel, group, err := CacheGetRandomSatisfiedChannel(multiGroup())
	require.NoError(t, err)
	require.Equal(t, 1, channel.Id, "switch off: the token's first group is used as configured")
	require.Equal(t, "pool-a", group)

	setRouteRouting(t, map[string]string{"route_routing_setting.health_routing_enabled": "true", "route_routing_setting.probe_percent": "0"})
	channel, group, err = CacheGetRandomSatisfiedChannel(multiGroup())
	require.NoError(t, err)
	require.Equal(t, 3, channel.Id, "a group with no healthy channel is skipped for the token's next group")
	require.Equal(t, "pool-b", group)
	candidates, group, err := CacheGetSatisfiedChannelCandidates(multiGroup())
	require.NoError(t, err)
	require.Equal(t, "pool-b", group, "capacity-admission candidates follow the same rule")
	require.Len(t, candidates, 1)

	channel, group, err = CacheGetRandomSatisfiedChannel(routeParam())
	require.NoError(t, err)
	require.Equal(t, 1, channel.Id, "a single-group token has no later group, so its group is still served")
	require.Equal(t, "pool-a", group)

	restore()
	restore = routeinsight.SetHealthForTesting(
		routeinsight.HealthEntry{ChannelID: 1, Model: "m", Health: unhealthy},
		routeinsight.HealthEntry{ChannelID: 2, Model: "m", Health: unhealthy},
		routeinsight.HealthEntry{ChannelID: 3, Model: "m", Health: unhealthy},
	)
	channel, group, err = CacheGetRandomSatisfiedChannel(multiGroup())
	require.NoError(t, err)
	require.Equal(t, 1, channel.Id, "when every group is unhealthy the normal order is kept")
	require.Equal(t, "pool-a", group)

	restore()
	restore = routeinsight.SetHealthForTesting(
		routeinsight.HealthEntry{ChannelID: 1, Model: "m", Health: unhealthy},
		routeinsight.HealthEntry{ChannelID: 3, Model: "m", Health: healthy(0.99)},
	)
	channel, group, err = CacheGetRandomSatisfiedChannel(multiGroup())
	require.NoError(t, err)
	require.Equal(t, 2, channel.Id, "an unknown channel in the first group keeps the request in that group")
	require.Equal(t, "pool-a", group)
	restore()
}
