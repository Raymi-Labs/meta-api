package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInvitePromotionRoutesRequireOwnerOrAdmin(t *testing.T) {
	old := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = old })
	for _, role := range []int{0, common.RoleCommonUser} {
		r := gin.New()
		r.Use(sessions.Sessions("test", cookie.NewStore([]byte("local-invite-test-session"))))
		r.Use(func(c *gin.Context) {
			if role != 0 {
				s := sessions.Default(c)
				s.Set("username", "fixture")
				s.Set("id", 7)
				s.Set("role", role)
				s.Set("status", common.UserStatusEnabled)
			}
			c.Next()
		})
		SetApiRouter(r)
		paths := []string{"/api/user/8/invite-promotion"}
		if role == 0 {
			paths = append(paths, "/api/user/invite-promotion/self")
		}
		for _, path := range paths {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("New-Api-User", "7")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var body struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
			require.False(t, body.Success)
		}
	}
}
