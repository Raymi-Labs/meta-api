package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelParamOverridePreviewPermissionAndIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser} {
		router := gin.New()
		router.Use(sessions.Sessions("preview", cookie.NewStore([]byte("test-preview-session"))))
		router.Use(func(c *gin.Context) {
			if role != 0 {
				s := sessions.Default(c)
				s.Set("username", "preview")
				s.Set("role", role)
				s.Set("id", 1)
				s.Set("status", common.UserStatusEnabled)
			}
			c.Next()
		})
		router.POST("/api/channel/param_override/preview", middleware.AdminAuth(), PreviewChannelParamOverride)
		body, err := common.Marshal(map[string]interface{}{"body": `{"n":9007199254740993}`, "override": `{"operations":[{"mode":"copy_header","from":"authorization","to":"x-key"}]}`, "context": map[string]interface{}{}})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/channel/param_override/preview", bytes.NewReader(body))
		r.Header.Set("New-Api-User", "1")
		if role != 0 {
			r.Header.Set("Authorization", "secret-not-a-sample")
		}
		router.ServeHTTP(w, r)
		var result struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
		require.Equal(t, role == common.RoleAdminUser, result.Success)
		require.NotContains(t, w.Body.String(), "secret-not-a-sample")
		if role == common.RoleAdminUser {
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Contains(t, w.Body.String(), `"header_override":{}`)
			require.Contains(t, w.Body.String(), "9007199254740993")
		}
	}
}

func TestChannelParamOverridePreviewRejectsInvalidAndOversizedInput(t *testing.T) {
	for _, body := range []string{`{} {}`, `{"body":"{}","override":"[]"}`, strings.Repeat(" ", 256*1024+1)} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		PreviewChannelParamOverride(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
}
