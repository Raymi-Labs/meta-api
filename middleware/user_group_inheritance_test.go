package middleware

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIdentityGroupWithInheritedToken(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldRedis, oldMemory := model.DB, common.RedisEnabled, common.MemoryCacheEnabled
	oldUsable := setting.UserUsableGroups2JSONString()
	oldGroups, oldRatios := setting.UserGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	model.DB, common.RedisEnabled, common.MemoryCacheEnabled = db, false, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.MemoryCacheEnabled = oldDB, oldRedis, oldMemory
		setting.UpdateUserUsableGroupsByJSONString(oldUsable)
		setting.UpdateUserGroupsByJSONString(oldGroups)
		ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":""}`))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, setting.UpdateUserGroupsByJSONString(`{"default":"","vip":""}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	user := model.User{Id: 998201, Username: "audit", Group: "vip", Status: common.UserStatusEnabled, Quota: 10000}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{Id: 998202, UserId: user.Id, Key: "auditidentitytoken", Group: "", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, token.Insert())
	r := gin.New()
	r.Use(TokenAuth())
	var selected string
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		selected = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		require.Equal(t, "vip", common.GetContextKeyString(c, constant.ContextKeyUserGroup))
		require.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
		c.Status(200)
	})
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fixture"}`))
	req.Header.Set("Authorization", "Bearer sk-"+token.Key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	t.Logf("identity=vip token.group=empty selected_route_group=%s configured_channel_group=%v", selected, ratio_setting.ContainsGroupRatio(selected))
	require.Equal(t, "default", selected)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{}`))
	req = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fixture"}`))
	req.Header.Set("Authorization", "Bearer sk-"+token.Key)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 403, w.Code, "must not grant a revoked default group")
}

func TestDeletedUserTokenReturnsUnauthorized(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	prevDB, prevRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = prevDB, prevRedis
		conn, _ := db.DB()
		_ = conn.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	user := model.User{Username: "deleted-token-test", Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "deletedkeyfixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, token.Insert())
	r := gin.New()
	ok := func(c *gin.Context) { c.Status(200) }
	r.GET("/v1/models", TokenAuth(), ok)
	r.GET("/api/usage/token", TokenAuthQuery(), ok)
	check := func(status int) {
		for _, path := range []string{"/v1/models", "/api/usage/token"} {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", "Bearer sk-"+token.Key)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, status, w.Code, w.Body.String())
			if status == 401 {
				require.Contains(t, w.Body.String(), "无效的令牌")
			}
		}
	}
	check(200)
	// Exercise revocation with the same warm owner and key hashes used in production.
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	previous := common.RDB
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previous, false; _ = client.Close() })
	userKey := fmt.Sprintf("user:%d", user.Id)
	tokenKey := "token:" + common.GenerateHMAC(token.Key)
	cachedToken := token
	cachedToken.Clean()
	require.NoError(t, common.RedisHSetObj(userKey, user.ToBaseUser(), time.Minute))
	require.NoError(t, common.RedisHSetObj(tokenKey, &cachedToken, time.Minute))
	check(200)
	require.NoError(t, model.DeleteUserById(user.Id))
	require.False(t, srv.Exists(userKey))
	require.False(t, srv.Exists(tokenKey))
	// The token row survives deletion. Its subsequent cache hit must also return 401.
	require.NoError(t, common.RedisHSetObj(tokenKey, &cachedToken, time.Minute))
	check(401)
	common.RedisEnabled = false
	check(401)
}
