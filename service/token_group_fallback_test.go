package service

import (
	"fmt"
	"net/http"
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

func TestPreferredGroupFallback(t *testing.T) {
	oldRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"primary":1,"backup":3,"last":5,"private":7}`))
	oldAuto, oldUsable := setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["private","backup","primary","backup","last","auto",""]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"primary":"","backup":"","last":"","auto":"","":""}`))
	require.Equal(t, []string{"primary", "backup", "last"}, TokenRoutingGroups("member", "primary", true))
	require.Equal(t, []string{"member", "backup", "primary", "last"}, TokenRoutingGroups("member", "", true))
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache_%v", cached), func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			oldDB, oldLog, oldCache := model.DB, model.LOG_DB, common.MemoryCacheEnabled
			model.DB, model.LOG_DB, common.MemoryCacheEnabled = db, db, cached
			t.Cleanup(func() {
				model.DB, model.LOG_DB, common.MemoryCacheEnabled = oldDB, oldLog, oldCache
				model.InitChannelCache()
				sqlDB, _ := db.DB()
				_ = sqlDB.Close()
			})
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
			for i, group := range []string{"primary", "backup", "last", "private"} {
				id := i + 1
				require.NoError(t, db.Create(&model.Channel{Id: id, Name: group, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: group, Models: "fallback-model", Priority: common.GetPointer(int64(1))}).Error)
				require.NoError(t, db.Create(&model.Ability{Group: group, Model: "fallback-model", ChannelId: id, Enabled: true, Priority: common.GetPointer(int64(1)), Weight: 1}).Error)
			}
			model.InitChannelCache()
			param := func(on bool) *RetryParam {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "member")
				common.SetContextKey(c, constant.ContextKeyTokenAutoGroupFallback, on)
				common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true) // Previously ignored for fixed tokens.
				return &RetryParam{Ctx: c, TokenGroup: "primary", ModelName: "fallback-model", Retry: common.GetPointer(0)}
			}
			scoped := param(true)
			common.SetContextKey(scoped.Ctx, constant.ContextKeyTokenRoutingConfig, `{"groups":["primary","backup"],"model_groups":{"fallback-model":["backup"]}}`)
			scopedChannel, scopedGroup, scopedErr := CacheGetRandomSatisfiedChannel(scoped)
			require.NoError(t, scopedErr)
			require.Equal(t, 2, scopedChannel.Id)
			require.Equal(t, "backup", scopedGroup)
			scoped.ExcludeChannels = []int{2}
			scoped.IncreaseRetry()
			scopedChannel, _, scopedErr = CacheGetRandomSatisfiedChannel(scoped)
			require.NoError(t, scopedErr)
			require.Nil(t, scopedChannel, "neither the primary group nor global auto groups may escape the model scope")
			require.Equal(t, 1, scoped.GetRetry())
			p := param(true)
			channel, group, err := CacheGetRandomSatisfiedChannel(p)
			require.NoError(t, err)
			require.Equal(t, 1, channel.Id)
			require.Equal(t, "primary", group)
			p.ExcludeChannels = []int{1}
			p.IncreaseRetry()
			channel, group, err = CacheGetRandomSatisfiedChannel(p)
			require.NoError(t, err)
			require.Equal(t, 2, channel.Id)
			require.Equal(t, "backup", group)
			require.Equal(t, 1, p.GetRetry(), "switching must not replenish the global retry budget")
			require.Equal(t, "backup", common.GetContextKeyString(p.Ctx, constant.ContextKeyAutoGroup))
			p.ExcludeChannels = []int{1, 2}
			p.IncreaseRetry()
			channel, group, err = CacheGetRandomSatisfiedChannel(p)
			require.NoError(t, err)
			require.Equal(t, 3, channel.Id)
			require.Equal(t, "last", group)
			require.Equal(t, 2, p.GetRetry())
			p.ExcludeChannels = []int{1, 2, 3}
			channel, _, err = CacheGetRandomSatisfiedChannel(p)
			require.NoError(t, err)
			require.Nil(t, channel, "private channel must never be used")
			off := param(false)
			off.ExcludeChannels = []int{1}
			channel, _, err = CacheGetRandomSatisfiedChannel(off)
			require.NoError(t, err)
			require.Nil(t, channel, "old cross_group_retry must not activate fixed-group fallback")
			limited := param(true)
			limited.ExcludeChannels = []int{1}
			limited.AllowedChannels = []int{1, 4}
			channel, _, err = CacheGetRandomSatisfiedChannel(limited)
			require.NoError(t, err)
			require.Nil(t, channel, "fallback must respect channel permissions")
			waiting := param(true)
			waiting.ExcludeChannels = []int{1}
			candidates, group, err := CacheGetSatisfiedChannelCandidates(waiting)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			require.Equal(t, "backup", group)
			require.Empty(t, common.GetContextKeyString(waiting.Ctx, constant.ContextKeyAutoGroup))
			waiting.ExcludeChannels = nil
			candidates, group, err = CacheGetSatisfiedChannelCandidates(waiting)
			require.NoError(t, err)
			require.Equal(t, "primary", group)
			require.Equal(t, 1, candidates[0].Id)
			waiting.ExcludeChannels = []int{1}
			_, group, err = CacheGetSatisfiedChannelCandidates(waiting)
			require.NoError(t, err)
			CommitChannelCandidateSelection(waiting, group)
			require.Equal(t, "backup", common.GetContextKeyString(waiting.Ctx, constant.ContextKeyAutoGroup))
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Update("status", common.ChannelStatusManuallyDisabled).Error)
			require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 1).Update("enabled", false).Error)
			model.InitChannelCache()
			channel, group, err = CacheGetRandomSatisfiedChannel(param(true))
			require.NoError(t, err)
			require.Equal(t, 2, channel.Id)
			require.Equal(t, "backup", group)
			channel, _, err = CacheGetRandomSatisfiedChannel(param(false))
			require.NoError(t, err)
			require.Nil(t, channel)
		})
	}
}
