package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDistributeInputCapabilityPreservesScopeAndBody(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldDB, oldMemory, oldRedis := model.DB, common.MemoryCacheEnabled, common.RedisEnabled
	oldPass := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, common.MemoryCacheEnabled, common.RedisEnabled = db, false, false
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled, common.RedisEnabled = oldDB, oldMemory, oldRedis
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldPass
		model.InvalidateModelPermissionCache()
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.ModelPermission{}))
	channels := []model.Channel{
		{Id: 1, Name: "unsupported", Type: constant.ChannelTypeAnthropic, Priority: common.GetPointer[int64](10), Status: common.ChannelStatusEnabled, Key: "test", Models: "alias", Group: "public"},
		{Id: 2, Name: "compatible", Type: constant.ChannelTypeOpenAI, Priority: common.GetPointer[int64](0), Status: common.ChannelStatusEnabled, Key: "test", Models: "alias", Group: "public"},
		{Id: 3, Name: "private", Type: constant.ChannelTypeOpenAI, Priority: common.GetPointer[int64](0), Status: common.ChannelStatusEnabled, Key: "test", Models: "private-alias", Group: "private"},
	}
	require.NoError(t, db.Create(&channels).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "public", Model: "alias", ChannelId: 1, Enabled: true, Priority: common.GetPointer[int64](10), Weight: 100},
		{Group: "public", Model: "alias", ChannelId: 2, Enabled: true, Priority: common.GetPointer[int64](0), Weight: 100},
		{Group: "private", Model: "alias", ChannelId: 3, Enabled: true, Priority: common.GetPointer[int64](20), Weight: 100},
	}).Error)
	videoBody := `{"model":"alias","stream":true,"messages":[{"role":"user","content":[{"type":"video_url","video_url":{"url":"https://example.test/secret.mp4"}}]}]}`
	for _, cached := range []bool{false, true} {
		common.MemoryCacheEnabled = cached
		if cached {
			model.InitChannelCache()
		}
		for _, tc := range []struct {
			name, path, body, code, specific string
			allowed                          []int
			selected                         int
		}{
			{"lower priority compatible", "/v1/chat/completions", videoBody, "", "", nil, 2},
			{"text highest priority", "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"video_url"}]}`, "", "", nil, 1},
			{"whitelist rejects without expanding", "/v1/chat/completions", videoBody, "unsupported_video_input", "", []int{1}, 0},
			{"pinned rejects", "/v1/chat/completions", videoBody, "unsupported_video_input", "1", nil, 0},
			{"responses chooses compatible", "/v1/responses", `{"model":"alias","input":"test"}`, "", "", nil, 2},
			{"responses no compatible", "/v1/responses", `{"model":"alias","input":"test"}`, "unsupported_endpoint", "", []int{1}, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				engine := gin.New()
				called := false
				engine.POST(tc.path, func(c *gin.Context) {
					defer common.CleanupBodyStorage(c)
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "public")
					common.SetContextKey(c, constant.ContextKeyUserGroup, "public")
					common.SetContextKey(c, constant.ContextKeyUserRole, common.RoleCommonUser)
					if tc.specific != "" {
						common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, tc.specific)
					}
					if tc.allowed != nil {
						allowed := map[int]bool{}
						for _, id := range tc.allowed {
							allowed[id] = true
						}
						common.SetContextKey(c, constant.ContextKeyTokenChannelLimitEnabled, true)
						common.SetContextKey(c, constant.ContextKeyTokenChannelLimit, allowed)
					}
					c.Next()
				}, Distribute(), func(c *gin.Context) {
					called = true
					require.Equal(t, tc.selected, c.GetInt("channel_id"))
					storage, err := common.GetBodyStorage(c)
					require.NoError(t, err)
					body, err := storage.Bytes()
					require.NoError(t, err)
					require.Equal(t, tc.body, string(body))
					candidates, group, err := service.CacheGetSatisfiedChannelCandidates(&service.RetryParam{Ctx: c, ModelName: "alias", TokenGroup: "public", AllowedChannels: tc.allowed})
					require.NoError(t, err)
					require.Equal(t, "public", group)
					for _, ch := range candidates {
						require.Equal(t, tc.selected, ch.Id)
					}
					c.Status(204)
				})
				request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				if tc.code == "" {
					require.True(t, called)
					require.Equal(t, 204, recorder.Code)
				} else {
					require.False(t, called)
					require.Equal(t, 400, recorder.Code, recorder.Body.String())
					require.Contains(t, recorder.Body.String(), `"code":"`+tc.code+`"`)
					require.NotContains(t, recorder.Body.String(), "secret.mp4")
				}
			})
		}
	}
}
