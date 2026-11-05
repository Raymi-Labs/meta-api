package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDesktopLoginApprovalAndExchangeBoundary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLog, oldEnabled, oldOrigin := model.DB, model.LOG_DB, common.GitHubOAuthEnabled, system_setting.ServerAddress
	model.DB, model.LOG_DB, common.GitHubOAuthEnabled, system_setting.ServerAddress = db, db, true, "https://test.example"
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.GitHubOAuthEnabled, system_setting.ServerAddress = oldDB, oldLog, oldEnabled, oldOrigin
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.DesktopLoginGrant{}))
	user := model.User{Username: "oauth-fixture", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&user).Error)
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("desktop-login-fixture-secret-1234"))))
	r.GET("/seed", func(c *gin.Context) { s := sessions.Default(c); s.Set("id", user.Id); require.NoError(t, s.Save()) })
	r.GET("/info", DesktopLoginInfo)
	r.GET("/capabilities", DesktopLoginCapabilities)
	r.POST("/start", StartDesktopLogin)
	r.POST("/approve", ApproveDesktopLogin)
	r.POST("/exchange", ExchangeDesktopLogin)
	call := func(method, path string, body any, cookies []*http.Cookie, origin string) *httptest.ResponseRecorder {
		encoded, _ := common.Marshal(body)
		req := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	verifier, _ := model.DesktopLoginToken()
	require.Contains(t, call("GET", "/capabilities", nil, nil, "").Body.String(), `"github"`)
	require.Equal(t, 400, call("POST", "/start", gin.H{"provider": "github", "challenge": "invalid"}, nil, "").Code)
	require.Equal(t, 400, call("POST", "/start", gin.H{"provider": "not-allowed", "challenge": model.DesktopLoginChallenge(verifier)}, nil, "").Code)
	require.Equal(t, 200, call("POST", "/start", gin.H{"provider": "github", "challenge": model.DesktopLoginChallenge(verifier)}, nil, "").Code)
	id, g, err := model.NewDesktopLogin(model.DesktopLoginChallenge(verifier), "github")
	require.NoError(t, err)
	require.Contains(t, call("POST", "/exchange", gin.H{"request_id": id, "verifier": verifier}, nil, "").Body.String(), `"status":"pending"`)
	seed := call("GET", "/seed", nil, nil, "")
	info := call("GET", "/info?request_id="+id, nil, seed.Result().Cookies(), "")
	require.Equal(t, "no-store", info.Header().Get("Cache-Control"))
	var decoded struct {
		Data struct {
			CSRF string `json:"csrf"`
		}
	}
	require.NoError(t, common.Unmarshal(info.Body.Bytes(), &decoded))
	cookies := info.Result().Cookies()
	approve := map[string]any{"request_id": id, "display_code": g.DisplayCode, "csrf": decoded.Data.CSRF, "user_id": user.Id}
	require.Equal(t, 403, call("POST", "/approve", approve, cookies, "https://evil.example").Code)
	require.Equal(t, 403, call("POST", "/approve", approve, nil, "https://test.example").Code)
	approve["user_id"] = user.Id + 1
	require.Equal(t, 403, call("POST", "/approve", approve, cookies, "https://test.example").Code)
	approve["user_id"] = user.Id
	require.Equal(t, 200, call("POST", "/approve", approve, cookies, "https://test.example").Code)
	require.Equal(t, 409, call("POST", "/approve", approve, cookies, "https://test.example").Code)
	wrong, _ := model.DesktopLoginToken()
	require.Equal(t, 410, call("POST", "/exchange", gin.H{"request_id": id, "verifier": wrong}, nil, "").Code)
	result := call("POST", "/exchange", gin.H{"request_id": id, "verifier": verifier}, nil, "")
	require.Equal(t, 200, result.Code)
	require.NotEmpty(t, result.Result().Cookies())
	require.Contains(t, result.Body.String(), "oauth-fixture")
	require.Equal(t, 410, call("POST", "/exchange", gin.H{"request_id": id, "verifier": verifier}, nil, "").Code)
	id, g, err = model.NewDesktopLogin(model.DesktopLoginChallenge(verifier), "github")
	require.NoError(t, err)
	require.NoError(t, model.ApproveDesktopLogin(id, user.Id, g.DisplayCode, false))
	require.NoError(t, db.Model(&user).Update("status", common.UserStatusDisabled).Error)
	require.Equal(t, 403, call("POST", "/exchange", gin.H{"request_id": id, "verifier": verifier}, nil, "").Code)
}
