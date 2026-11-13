package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useControllerCapacityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Midjourney{}))
	return db
}

func enableControllerCapacityForTest(t *testing.T) {
	t.Helper()
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedisEnabled })

	setting := operation_setting.GetChannelConcurrencySetting()
	original := *setting
	setting.Enabled = true
	setting.DefaultMaxConcurrency = 10
	setting.DefaultRpmLimit = 0
	setting.RpmWindowSeconds = 60
	setting.WaitTimeoutMs = 10
	setting.PollIntervalMs = 1
	t.Cleanup(func() { *setting = original })
}

func TestAcquireMidjourneyBoundChannelCapacityUsesOriginTaskChannel(t *testing.T) {
	enableControllerCapacityForTest(t)
	db := useControllerCapacityTestDB(t)

	channelSetting := `{"max_concurrency":1,"rpm_limit":10}`
	channel := model.Channel{
		Id:      993001,
		Name:    "origin",
		Type:    constant.ChannelTypeOpenAI,
		Status:  common.ChannelStatusEnabled,
		Key:     "test-key",
		Setting: &channelSetting,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Midjourney{
		UserId:    7,
		MjId:      "mj-origin",
		ChannelId: channel.Id,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 7)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/change", strings.NewReader(`{"taskId":"mj-origin","action":"UPSCALE","index":1}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	release, apiErr, handled := acquireMidjourneyBoundChannelCapacity(ctx, &relaycommon.RelayInfo{
		UserId:          7,
		OriginModelName: "midjourney-upscale",
		RelayMode:       relayconstant.RelayModeMidjourneyChange,
	})
	require.True(t, handled)
	require.Nil(t, apiErr)
	require.NotNil(t, release)
	defer release()
	require.Equal(t, channel.Id, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))

	inflight, err := middleware.QueryChannelConcurrency(channel.Id)
	require.NoError(t, err)
	require.EqualValues(t, 1, inflight)
}
