package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestOpenRouterFinanceRoutesRejectNonAdmin(t *testing.T) {
	oldLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = oldLimit })
	for _, role := range []int{0, common.RoleCommonUser} {
		r := gin.New()
		r.Use(sessions.Sessions("test", cookie.NewStore([]byte("local-test-session-key"))))
		r.Use(func(c *gin.Context) {
			if role != 0 {
				session := sessions.Default(c)
				session.Set("username", "local-test-user")
				session.Set("role", role)
				session.Set("id", 1)
				session.Set("status", common.UserStatusEnabled)
			}
			c.Next()
		})
		SetApiRouter(r)
		for _, path := range []string{"/api/upstream_accounts/1/openrouter/finance"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("New-Api-User", "1")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var response struct {
				Success bool `json:"success"`
			}
			if err := common.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Success {
				t.Fatalf("role=%d path=%s authorization failed: %s err=%v", role, path, w.Body.String(), err)
			}
		}
	}
}
