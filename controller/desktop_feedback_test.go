package controller

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDesktopFeedbackAuthorizationPersistenceAndManagement(t *testing.T) {
	db, user := setupContentSafetyControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.DesktopFeedback{}))
	role := common.RoleCommonUser
	r := gin.New()
	r.Use(sessions.Sessions("fixture", cookie.NewStore([]byte("desktop-feedback-test-session"))))
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
	r.POST("/api/user/desktop-feedback", middleware.UserAuth(), CreateDesktopFeedback)
	r.GET("/api/desktop-feedback/", middleware.AdminAuth(), GetDesktopFeedbackList)
	r.GET("/api/desktop-feedback/:id", middleware.AdminAuth(), GetDesktopFeedback)
	r.PATCH("/api/desktop-feedback/:id", middleware.AdminAuth(), UpdateDesktopFeedback)
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
	body := `{"reportId":"a7149591-4139-4c52-a0a2-1fa4ae45958b","description":"切换模型失败","appVersion":"0.1.108","platform":"macos","context":{"view":"home","tool":"dsh"},"errors":[{"occurredAt":"2026-10-26T01:00:00Z","source":"toast","message":"failed https://meta-api.vip/api/user/models?group=private-group"}],"userId":98765,"status":"resolved"}`
	require.Equal(t, 401, request("POST", "/api/user/desktop-feedback", body, false).Code)
	w := request("POST", "/api/user/desktop-feedback", body, true)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"status":"pending"`)
	require.NotContains(t, w.Body.String(), "切换模型")
	var stored model.DesktopFeedback
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, user.Id, stored.UserID)
	require.NotContains(t, stored.Errors, "private-group")
	require.Contains(t, stored.Errors, "/api/user/models")
	w = request("POST", "/api/user/desktop-feedback", strings.ReplaceAll(body, "切换模型失败", "do-not-overwrite"), true)
	require.Contains(t, w.Body.String(), fmt.Sprintf(`"id":%d`, stored.ID))
	var count int64
	require.NoError(t, db.Model(&model.DesktopFeedback{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, "切换模型失败", stored.Description)
	for _, path := range []string{"/api/desktop-feedback/", fmt.Sprintf("/api/desktop-feedback/%d", stored.ID)} {
		w = request("GET", path, "", true)
		require.Contains(t, w.Body.String(), `"success":false`)
		require.NotContains(t, w.Body.String(), "切换模型失败")
	}
	w = request("PATCH", fmt.Sprintf("/api/desktop-feedback/%d", stored.ID), `{"status":"resolved"}`, true)
	require.Contains(t, w.Body.String(), `"success":false`)
	role = common.RoleAdminUser
	w = request("GET", "/api/desktop-feedback/?status=pending&platform=macos&appVersion=0.1.108&userId="+fmt.Sprint(user.Id), "", true)
	require.Contains(t, w.Body.String(), `"total":1`)
	require.Contains(t, w.Body.String(), "切换模型失败")
	w = request("GET", "/api/desktop-feedback/?platform=windows", "", true)
	require.Contains(t, w.Body.String(), `"total":0`)
	w = request("GET", "/api/desktop-feedback/?p=2&page_size=1", "", true)
	require.Contains(t, w.Body.String(), `"items":[]`)
	for _, path := range []string{"?status=bad", "?platform=bad", "?userId=-1", "?page_size=-1", "?p=-2"} {
		require.Equal(t, 400, request("GET", "/api/desktop-feedback/"+path, "", true).Code)
	}
	for _, status := range []string{"in_progress", "resolved", "resolved", "pending"} {
		w = request("PATCH", fmt.Sprintf("/api/desktop-feedback/%d", stored.ID), fmt.Sprintf(`{"status":%q}`, status), true)
		require.Contains(t, w.Body.String(), fmt.Sprintf(`"status":%q`, status))
	}
	require.Equal(t, 400, request("PATCH", fmt.Sprintf("/api/desktop-feedback/%d", stored.ID), `{"status":"deleted"}`, true).Code)
	require.Equal(t, 404, request("GET", "/api/desktop-feedback/99999", "", true).Code)
	require.Equal(t, 404, request("PATCH", "/api/desktop-feedback/99999", `{"status":"resolved"}`, true).Code)
	require.Equal(t, 413, request("POST", "/api/user/desktop-feedback", strings.Repeat("x", desktopFeedbackBodyLimit+1), true).Code)
	require.Equal(t, 400, request("POST", "/api/user/desktop-feedback", "{", true).Code)
	require.NoError(t, db.Migrator().DropTable(&model.DesktopFeedback{}))
	w = request("POST", "/api/user/desktop-feedback", body, true)
	require.Equal(t, 500, w.Code)
	require.NotContains(t, w.Body.String(), "切换模型失败")
}
