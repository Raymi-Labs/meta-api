package controller

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuotaNoticeHistoryAuthorizationAndSettings(t *testing.T) {
	db, user := setupContentSafetyControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.QuotaNotice{}))
	for i := 0; i < 11; i++ {
		require.NoError(t, db.Create(&model.QuotaNotice{UserID: user.Id, Status: "submitted", Summary: "[]", SlotAt: 999, LeaseUntil: 999}).Error)
	}
	require.NoError(t, db.Create(&model.QuotaNotice{UserID: 98765, Status: "unknown", Summary: `[{"request_id":"other-user-private"}]`}).Error)
	role := common.RoleCommonUser
	r := gin.New()
	r.Use(sessions.Sessions("fixture", cookie.NewStore([]byte("quota-notice-test-cookie-secret"))))
	// Fixture identity is injected into the real session/auth middleware.
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Fixture-Logged-In") == "true" {
			s := sessions.Default(c)
			s.Set("id", user.Id)
			s.Set("username", user.Username)
			s.Set("role", role)
			s.Set("status", common.UserStatusEnabled)
		}
		c.Next()
	})
	r.GET("/api/user/quota-notices", middleware.UserAuth(), GetSelfQuotaNotices)
	r.GET("/api/user/:id/quota-notices", middleware.AdminAuth(), GetUserQuotaNotices)
	r.PUT("/api/user/setting", middleware.UserAuth(), UpdateUserSetting)
	request := func(method, path, body string, logged bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("New-Api-User", fmt.Sprint(user.Id))
		if logged {
			req.Header.Set("Fixture-Logged-In", "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w := request(http.MethodGet, "/api/user/quota-notices?user_id=98765&p=2", "", true)
	require.Contains(t, w.Body.String(), `"total":11`)
	require.Contains(t, w.Body.String(), `"page":2`)
	require.NotContains(t, w.Body.String(), "other-user-private")
	require.NotContains(t, w.Body.String(), "slot_at")
	require.NotContains(t, w.Body.String(), "lease_until")
	w = request(http.MethodGet, "/api/user/98765/quota-notices", "", true)
	require.Contains(t, w.Body.String(), `"success":false`)
	require.NotContains(t, w.Body.String(), "other-user-private")
	require.Equal(t, 401, request(http.MethodGet, "/api/user/quota-notices", "", false).Code)
	role = common.RoleAdminUser
	w = request(http.MethodGet, "/api/user/98765/quota-notices", "", true)
	require.Contains(t, w.Body.String(), "other-user-private")
	role = common.RoleCommonUser
	for _, body := range []string{`{"notify_type":"email","quota_warning_threshold":500000,"quota_blocked_notify_enabled":false}`, `{"notify_type":"email","quota_warning_threshold":500000}`} {
		w = request(http.MethodPut, "/api/user/setting", body, true)
		require.Contains(t, w.Body.String(), `"success":true`)
		var updated model.User
		require.NoError(t, db.First(&updated, user.Id).Error)
		require.False(t, updated.GetSetting().QuotaBlockedNotificationsEnabled())
	}
	require.NoError(t, db.Migrator().DropTable(&model.QuotaNotice{}))
	w = request(http.MethodGet, "/api/user/quota-notices", "", true)
	require.Contains(t, w.Body.String(), `"success":false`)
}
