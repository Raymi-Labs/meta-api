package middleware

import (
	"context"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRouteOutageRequestScope(t *testing.T) {
	oldGroups, oldAuto := setting.UserUsableGroups2JSONString(), setting.GetAutoGroups()
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(oldGroups)
		data, _ := common.Marshal(oldAuto)
		_ = setting.UpdateAutoGroupsByJsonString(string(data))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public","backup":"Backup"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["public","backup","private"]`))
	for _, tc := range []struct {
		name, group                                         string
		restricted, specific, canceled, anonymous, fallback bool
		want                                                []string
	}{
		{name: "fixed", group: "public", want: []string{"public"}},
		{name: "auto_all_exhausted", group: "auto", want: []string{"public", "backup"}},
		{name: "preferred_all_exhausted", group: "backup", fallback: true, want: []string{"backup", "public"}},
		{name: "channel_whitelist", group: "public", restricted: true},
		{name: "specific_channel", group: "public", specific: true},
		{name: "canceled", group: "public", canceled: true},
		{name: "anonymous", group: "public", anonymous: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			if !tc.anonymous {
				c.Set("id", 1)
			}
			common.SetContextKey(c, constant.ContextKeyUserGroup, "public")
			common.SetContextKey(c, constant.ContextKeyTokenAutoGroupFallback, tc.fallback)
			if tc.restricted {
				common.SetContextKey(c, constant.ContextKeyTokenChannelLimitEnabled, true)
				common.SetContextKey(c, constant.ContextKeyTokenChannelLimit, map[int]bool{1: true})
			}
			var specific *int
			if tc.specific {
				specific = common.GetPointer(1)
			}
			if tc.canceled {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			}
			require.Equal(t, tc.want, routeOutageRequestGroups(c, tc.group, specific))
		})
	}
}

func TestRouteOutageDistributeFinalOutcome(t *testing.T) {
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	oldDB, oldMemory, oldRedis := model.DB, common.MemoryCacheEnabled, common.RedisEnabled
	oldObserver := routeOutageObserver
	oldGroups, oldAuto := setting.UserUsableGroups2JSONString(), setting.GetAutoGroups()
	model.DB, common.MemoryCacheEnabled, common.RedisEnabled = db, false, false
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled, common.RedisEnabled = oldDB, oldMemory, oldRedis
		routeOutageObserver = oldObserver
		model.InvalidateModelPermissionCache()
		_ = setting.UpdateUserUsableGroupsByJSONString(oldGroups)
		data, _ := common.Marshal(oldAuto)
		_ = setting.UpdateAutoGroupsByJsonString(string(data))
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ModelPermission{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public","backup":"Backup"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["public","backup","private"]`))
	require.NoError(t, db.Create(&[]model.Channel{{Id: 1, Key: "fixture", Type: 1, Status: 3, Group: "public", Models: "m,down"}, {Id: 2, Key: "fixture", Type: 1, Status: 1, Group: "backup", Models: "m"}, {Id: 3, Key: "fixture", Type: 1, Status: 3, Group: "backup", Models: "down"}}).Error)
	require.NoError(t, db.Create(&[]model.Ability{{Group: "public", Model: "m", ChannelId: 1, Enabled: false}, {Group: "public", Model: "down", ChannelId: 1, Enabled: false}, {Group: "backup", Model: "m", ChannelId: 2, Enabled: true}, {Group: "backup", Model: "down", ChannelId: 3, Enabled: false}}).Error)
	for _, tc := range []struct {
		name, group, model           string
		fallback, restricted, denied bool
		wantStatus                   int
		wantGroups                   []string
	}{
		{name: "whole_group_down", group: "public", model: "m", wantStatus: 503, wantGroups: []string{"public"}},
		{name: "successful_fallback", group: "public", model: "m", fallback: true, wantStatus: 200},
		{name: "auto_all_exhausted", group: "auto", model: "down", wantStatus: 503, wantGroups: []string{"public", "backup"}},
		{name: "whitelist_failure", group: "public", model: "m", restricted: true, wantStatus: 503},
		{name: "unknown_model", group: "public", model: "typo", wantStatus: 404},
		{name: "model_permission", group: "public", model: "m", denied: true, wantStatus: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var observed []string
			routeOutageObserver = func(groups []string, name string, tokenID int) {
				require.Nil(t, observed)
				observed = groups
				require.Equal(t, tc.model, name)
				require.Equal(t, 19, tokenID)
			}
			router := gin.New()
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				c.Set("id", 1)
				c.Set("token_id", 19)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, tc.group)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "public")
				common.SetContextKey(c, constant.ContextKeyUserRole, common.RoleCommonUser)
				common.SetContextKey(c, constant.ContextKeyTokenAutoGroupFallback, tc.fallback)
				if tc.restricted {
					common.SetContextKey(c, constant.ContextKeyTokenChannelLimitEnabled, true)
					common.SetContextKey(c, constant.ContextKeyTokenChannelLimit, map[int]bool{1: true})
				}
				if tc.denied {
					common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
					common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{})
				}
			}, Distribute(), func(c *gin.Context) { c.Status(http.StatusOK) })
			body, _ := common.Marshal(map[string]any{"model": tc.model, "messages": []map[string]string{{"role": "user", "content": "fixture"}}})
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			require.Equal(t, tc.wantGroups, observed)
		})
	}
}
