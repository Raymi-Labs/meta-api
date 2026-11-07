package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRouteShadowCandidatesStayWithinPermittedScope(t *testing.T) {
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
		{Id: 1, Name: "primary", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Priority: &high},
		{Id: 2, Name: "backup", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Priority: &low},
		{Id: 3, Name: "other-group", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Priority: &high},
	}
	require.NoError(t, db.Create(&channels).Error)
	abilities := []model.Ability{
		{Group: "default", Model: "m", ChannelId: 1, Enabled: true, Priority: &high, Weight: 100},
		{Group: "default", Model: "m", ChannelId: 2, Enabled: true, Priority: &low, Weight: 100},
		{Group: "private", Model: "m", ChannelId: 3, Enabled: true, Priority: &high, Weight: 100},
	}
	require.NoError(t, db.Create(&abilities).Error)
	model.InitChannelCache()

	newParam := func() *RetryParam {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		return &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "m", Retry: common.GetPointer(0)}
	}

	param := newParam()
	candidates, ok := RouteShadowCandidates(param)
	require.True(t, ok)
	require.Len(t, candidates, 2, "all priorities of the permitted group, never another group")
	ids := map[int]bool{}
	for _, candidate := range candidates {
		ids[candidate.ChannelID] = true
		require.Equal(t, "default", candidate.Group)
	}
	require.True(t, ids[1] && ids[2])

	param = newParam()
	param.ExcludeChannels = []int{1}
	candidates, ok = RouteShadowCandidates(param)
	require.True(t, ok)
	require.Len(t, candidates, 1)
	require.Equal(t, 2, candidates[0].ChannelID, "retry exclusions apply to the shadow too")

	param = newParam()
	param.AllowedChannels = []int{2}
	candidates, _ = RouteShadowCandidates(param)
	require.Len(t, candidates, 1)
	require.Equal(t, 2, candidates[0].ChannelID, "token channel allow-list applies")

	param = newParam()
	param.Ctx.Set("specific_channel_id", "1")
	_, ok = RouteShadowCandidates(param)
	require.False(t, ok, "a pinned channel has no alternative to recommend")

	common.MemoryCacheEnabled = false
	_, ok = RouteShadowCandidates(newParam())
	require.False(t, ok, "without the memory cache the request path must not scan the database")
}
