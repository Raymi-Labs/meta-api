package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenRoutingAffinityAndSpecifiedChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldRedis, oldMemory := model.DB, common.RedisEnabled, common.MemoryCacheEnabled
	oldGroups, oldRatios := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	affinity := operation_setting.GetChannelAffinitySetting()
	oldAffinity := *affinity
	model.DB, common.RedisEnabled, common.MemoryCacheEnabled = db, false, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.MemoryCacheEnabled = oldDB, oldRedis, oldMemory
		*affinity = oldAffinity
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"primary":"","backup":"","outside":""}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"primary":1,"backup":3,"outside":7}`))
	for i, group := range []string{"primary", "backup", "outside"} {
		require.NoError(t, db.Create(&model.Channel{Id: i + 1, Name: group, Key: "fixture", Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Group: group, Models: "scoped-model"}).Error)
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: "scoped-model", ChannelId: i + 1, Enabled: true}).Error)
	}
	*affinity = operation_setting.ChannelAffinitySetting{Enabled: true, DefaultTTLSeconds: 60, Rules: []operation_setting.ChannelAffinityRule{{Name: "token-route-test", ModelRegex: []string{"^scoped-model$"}, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "context_string", Key: "test_affinity"}}, IncludeRuleName: true}}}
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "primary")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "primary")
		require.NoError(t, SetupContextForToken(c, &model.Token{Group: "primary", RoutingConfig: `{"groups":["primary","backup"],"model_groups":{"scoped-model":["backup"]}}`}))
		c.Set("test_affinity", strconv.FormatInt(time.Now().UnixNano(), 10))
		c.Set("original_model", "scoped-model")
		return c
	}
	c := newContext()
	_, _ = service.GetPreferredChannelByAffinity(c, "scoped-model", "primary")
	service.RecordChannelAffinity(c, 2)
	channel, group, selectErr := selectChannelForRequest(c, "scoped-model", "primary", "", nil)
	require.Nil(t, selectErr)
	require.Equal(t, 2, channel.Id)
	require.Equal(t, "backup", group)
	info := &relaycommon.RelayInfo{UserGroup: "primary", UsingGroup: "primary"}
	price := helper.HandleGroupRatio(c, info)
	require.Equal(t, "backup", info.UsingGroup)
	require.Equal(t, float64(3), price.GroupRatio)
	c = newContext()
	_, _ = service.GetPreferredChannelByAffinity(c, "scoped-model", "primary")
	service.RecordChannelAffinity(c, 3)
	_, _, ok := tryAffinityChannel(c, "scoped-model", "primary", "", nil)
	require.False(t, ok, "affinity must not escape the model's allowed groups")
	_, _, selectErr = selectChannelForRequest(c, "scoped-model", "primary", "", common.GetPointer(3))
	require.NotNil(t, selectErr)
	require.Equal(t, http.StatusForbidden, selectErr.StatusCode)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"primary":"","outside":""}`))
	require.Empty(t, service.RequestModelRoutingGroups(c, "primary", "scoped-model"), "revocation must not fall back to primary or outside")
	c = newContext()
	common.SetContextKey(c, constant.ContextKeyTokenRoutingConfig, `{"groups":[]}`)
	require.Empty(t, service.RequestModelRoutingGroups(c, "primary", "scoped-model"))
}
