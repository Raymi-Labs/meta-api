package service

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAffinityFallbackPriority(t *testing.T) {
	oldDB, oldLog, oldCache := model.DB, model.LOG_DB, common.MemoryCacheEnabled
	oldAuto, oldUsable := setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString()
	oldRatios := ratio_setting.GroupRatio2JSONString()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.MemoryCacheEnabled = oldDB, oldLog, oldCache
		model.InitChannelCache()
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["primary","backup"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"primary":"","backup":""}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"primary":1,"backup":2}`))
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	for groupIndex, group := range []string{"primary", "backup"} {
		for tier := 0; tier < 3; tier++ {
			id, priority := groupIndex*3+tier+1, int64(3-tier)
			require.NoError(t, db.Create(&model.Channel{Id: id, Name: fmt.Sprint(id), Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: group, Models: "affinity-test", Priority: &priority}).Error)
			require.NoError(t, db.Create(&model.Ability{Group: group, Model: "affinity-test", ChannelId: id, Enabled: true, Priority: &priority, Weight: 1}).Error)
		}
	}
	for _, cached := range []bool{false, true} {
		common.MemoryCacheEnabled = cached
		model.InitChannelCache()
		for _, mode := range []string{"fixed", "auto", "preferred", "model_groups"} {
			for _, capacity := range []bool{false, true} {
				t.Run(fmt.Sprintf("cache=%t/%s/capacity=%t", cached, mode, capacity), func(t *testing.T) {
					newParam := func(hit bool) *RetryParam {
						ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
						ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
						common.SetContextKey(ctx, constant.ContextKeyUserGroup, "member")
						p := &RetryParam{Ctx: ctx, TokenGroup: "primary", ModelName: "affinity-test", Retry: common.GetPointer(1), ExcludeChannels: []int{3}}
						if mode == "auto" {
							p.TokenGroup = "auto"
						}
						if mode == "preferred" {
							common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroupFallback, true)
						}
						if mode == "model_groups" {
							common.SetContextKey(ctx, constant.ContextKeyTokenRoutingConfig, `{"groups":["primary","backup"],"model_groups":{"affinity-test":["primary","backup"]}}`)
						}
						setChannelAffinityContext(ctx, channelAffinityMeta{RuleName: "test", UsingGroup: p.TokenGroup})
						if hit {
							MarkChannelAffinityUsed(ctx, "primary", 3)
						}
						return p
					}
					selectID := func(p *RetryParam) (int, string) {
						if capacity {
							channels, group, err := CacheGetSatisfiedChannelCandidates(p)
							require.NoError(t, err)
							require.Len(t, channels, 1)
							return channels[0].Id, group
						}
						channel, group, err := CacheGetRandomSatisfiedChannel(p)
						require.NoError(t, err)
						require.NotNil(t, channel)
						return channel.Id, group
					}
					miss := newParam(false)
					id, _ := selectID(miss)
					require.Equal(t, 2, id, "rule match alone must not change normal routing")
					hit := newParam(true)
					id, group := selectID(hit)
					require.Equal(t, 1, id, "affinity failure must start at highest available priority")
					require.Equal(t, "primary", group)
					require.Equal(t, 1, hit.GetRetry(), "priority adjustment must not replenish attempts")
					hit.IncreaseRetry()
					id, _ = selectID(hit)
					require.Equal(t, 2, id)
					require.Equal(t, 2, hit.GetRetry())
					limited := newParam(true)
					limited.AllowedChannels = []int{2}
					id, _ = selectID(limited)
					require.Equal(t, 2, id, "affinity offset never widens channel permission")
					if mode != "fixed" {
						if mode == "auto" {
							laterBinding := newParam(false)
							MarkChannelAffinityUsed(laterBinding.Ctx, "backup", 6)
							id, group = selectID(laterBinding)
							require.Equal(t, 1, id, "legacy auto starts normal routing from the first group even when affinity bound later")
							require.Equal(t, "primary", group)
						}
						moved := newParam(true)
						common.SetContextKey(moved.Ctx, constant.ContextKeyAutoGroupIndex, 1)
						id, group = selectID(moved)
						require.Equal(t, 5, id, "next group's own retry must not inherit the offset")
						require.Equal(t, "backup", group)
						if capacity {
							probe := newParam(true)
							probe.ExcludeChannels = []int{1, 2, 3}
							id, group = selectID(probe)
							require.Equal(t, 4, id)
							require.Equal(t, "backup", group)
							require.Equal(t, 0, common.GetContextKeyInt(probe.Ctx, constant.ContextKeyAutoGroupIndex))
							probe.ExcludeChannels = []int{3}
							id, _ = selectID(probe)
							require.Equal(t, 1, id, "capacity probes must preserve the original group and offset")
						}
					}
				})
			}
		}
	}
}
