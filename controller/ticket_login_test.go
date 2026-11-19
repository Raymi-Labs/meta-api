package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTicketAppealLoginRequiresCredentialsAndSecondFactor(t *testing.T) {
	i18n.Init()
	setupRegisterRiskControllerTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TwoFA{}, &model.TwoFABackupCode{}))
	old := common.PasswordLoginEnabled
	common.PasswordLoginEnabled = true
	t.Cleanup(func() { common.PasswordLoginEnabled = old })
	hash, err := common.Password2Hash("secret-password")
	require.NoError(t, err)
	user := model.User{Username: "appeal-test", Password: hash, Status: common.UserStatusDisabled, Role: common.RoleCommonUser, AffCode: "appeal-test"}
	require.NoError(t, model.DB.Create(&user).Error)
	second := model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}
	require.NoError(t, model.DB.Create(&second).Error)
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("ticket-login-test-key"))))
	r.POST("/login", Login)
	r.POST("/second", Verify2FALogin)
	r.GET("/scope", func(c *gin.Context) {
		s := sessions.Default(c)
		c.JSON(200, gin.H{"appeal": s.Get("appeal_user_id"), "expires": s.Get("appeal_expires_at"), "id": s.Get("id"), "pending": s.Get("pending_user_id")})
	})
	call := func(path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	bad := call("/login", `{"username":"appeal-test","password":"wrong"}`, nil)
	require.NotContains(t, bad.Body.String(), "appeal_available")
	login := call("/login", `{"username":"appeal-test","password":"secret-password"}`, nil)
	require.Contains(t, login.Body.String(), "require_2fa")
	cookies := login.Result().Cookies()
	wrong := call("/second", `{"code":"invalid"}`, cookies)
	require.NotContains(t, wrong.Body.String(), "appeal_available")
	code, err := totp.GenerateCode(second.Secret, time.Now())
	require.NoError(t, err)
	verified := call("/second", `{"code":"`+code+`"}`, cookies)
	require.Contains(t, verified.Body.String(), "appeal_available")
	req := httptest.NewRequest("GET", "/scope", nil)
	for _, cookie := range verified.Result().Cookies() {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var scope struct {
		Appeal  int   `json:"appeal"`
		Expires int64 `json:"expires"`
		ID      any   `json:"id"`
		Pending any   `json:"pending"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &scope))
	require.Equal(t, user.Id, scope.Appeal)
	require.Nil(t, scope.ID)
	require.Nil(t, scope.Pending)
	require.InDelta(t, time.Now().Add(15*time.Minute).Unix(), scope.Expires, 2)
}
