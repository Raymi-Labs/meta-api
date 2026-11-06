package router

import (
	"embed"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCanonicalLoginNavigationDoesNotRedirectAPIsOrCallbacks(t *testing.T) {
	r := gin.New()
	// API routes are registered before web middleware in SetRouter.
	r.POST("/api/desktop-auth/exchange", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.POST("/v1/responses", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	SetWebRouter(r, embed.FS{}, []byte("<html>login fixture</html>"))
	for _, tc := range []struct {
		method, path, host string
		status             int
	}{
		{"GET", "/login?code=secret", "meta-api.vip", http.StatusOK},
		{"HEAD", "/register", "cn.meta-api.vip", http.StatusOK},
		{"GET", "/desktop-login", "nan.meta-api.vip", http.StatusOK},
		{"GET", "/login", "meta-api.com", http.StatusOK},
		{"POST", "/login", "meta-api.vip", http.StatusOK},
		{"POST", "/api/desktop-auth/exchange", "meta-api.vip", http.StatusNoContent},
		{"GET", "/oauth/google?code=secret", "meta-api.vip", http.StatusOK},
		{"POST", "/v1/responses", "cn.meta-api.vip", http.StatusNoContent},
		{"GET", "/login", "attacker.test", http.StatusOK},
	} {
		req := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
		req.Header.Set("X-Forwarded-Host", "attacker.test")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code)
		require.Empty(t, w.Header().Get("Location"))
	}
}
