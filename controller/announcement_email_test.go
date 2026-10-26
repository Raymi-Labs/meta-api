package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnnouncementEmailManagementAuthorizationAndSave(t *testing.T) {
	db, user := setupContentSafetyControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.AnnouncementEmailJob{}, &model.AnnouncementEmailIdentity{}, &model.AnnouncementEmailDelivery{}))
	oldMap, oldServer := common.OptionMap, common.SMTPServer
	oldFrom, oldPort := common.SMTPFrom, common.SMTPPort
	oldAnnouncements := console_setting.GetConsoleSetting().Announcements
	common.OptionMap, common.SMTPServer = map[string]string{}, ""
	t.Cleanup(func() {
		common.OptionMap, common.SMTPServer = oldMap, oldServer
		common.SMTPFrom, common.SMTPPort = oldFrom, oldPort
		console_setting.GetConsoleSetting().Announcements = oldAnnouncements
	})
	role := common.RoleCommonUser
	r := gin.New()
	r.Use(sessions.Sessions("fixture", cookie.NewStore([]byte("announcement-session-test-key"))))
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
	r.GET("/api/option/announcement-email/jobs", middleware.RootAuth(), ListAnnouncementEmailJobs)
	r.PUT("/api/option/", middleware.RootAuth(), UpdateOption)
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
	path := "/api/option/announcement-email/jobs"
	require.Equal(t, 401, request(http.MethodGet, path, "", false).Code)
	for _, testRole := range []int{common.RoleCommonUser, common.RoleAdminUser} {
		role = testRole
		require.Contains(t, request(http.MethodGet, path, "", true).Body.String(), `"success":false`)
		require.Contains(t, request(http.MethodPut, "/api/option/", `{}`, true).Body.String(), `"success":false`)
	}
	role = common.RoleRootUser
	raw := `[{"id":"uuid","content":"维护通知","publishDate":"2026-10-26T12:00:00+08:00","type":"warning"}]`
	body, err := common.Marshal(map[string]any{"key": model.AnnouncementOptionKey, "value": raw, "email_sync": true})
	require.NoError(t, err)
	w := request(http.MethodPut, "/api/option/", string(body), true)
	require.Contains(t, w.Body.String(), `"success":false`)
	require.Contains(t, w.Body.String(), "SMTP")
	// Existing integrations without email_sync save successfully and send none.
	body, err = common.Marshal(map[string]any{"key": model.AnnouncementOptionKey, "value": raw})
	require.NoError(t, err)
	w = request(http.MethodPut, "/api/option/", string(body), true)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Contains(t, w.Body.String(), `"email_jobs_queued":0`)
	// A new mail-enabled publication creates a durable job through the API;
	// this test never starts the background worker or connects to SMTP.
	common.SMTPServer, common.SMTPFrom, common.SMTPPort = "smtp.example.invalid", "sender@example.invalid", 587
	raw = `[{"id":"mail-enabled","content":"新的维护通知","publishDate":"2026-10-26T12:00:00+08:00","type":"warning"}]`
	body, err = common.Marshal(map[string]any{"key": model.AnnouncementOptionKey, "value": raw, "email_sync": true})
	require.NoError(t, err)
	w = request(http.MethodPut, "/api/option/", string(body), true)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Contains(t, w.Body.String(), `"email_jobs_queued":1`)
	var created model.AnnouncementEmailJob
	require.NoError(t, db.First(&created).Error)
	require.Equal(t, "新的维护通知", created.Content)
	for i := 0; i < 11; i++ {
		job := model.AnnouncementEmailJob{Identity: fmt.Sprint(i), Content: "通知", Total: 1}
		require.NoError(t, db.Create(&job).Error)
		require.NoError(t, db.Create(&model.AnnouncementEmailDelivery{JobID: job.ID, EmailHash: "test", Email: "private@example.invalid", Status: "submitted"}).Error)
	}
	w = request(http.MethodGet, path+"?p=2&page_size=10", "", true)
	require.Contains(t, w.Body.String(), `"total":12`)
	require.Contains(t, w.Body.String(), `"page":2`)
	require.NotContains(t, w.Body.String(), "private@example.invalid")
	w = request(http.MethodGet, path+"?p=-1&page_size=999", "", true)
	require.Contains(t, w.Body.String(), `"page":1`)
	require.Contains(t, w.Body.String(), `"page_size":100`)
	require.NoError(t, db.Migrator().DropTable(&model.AnnouncementEmailJob{}))
	w = request(http.MethodGet, path, "", true)
	require.Contains(t, w.Body.String(), `"success":false`)
}
