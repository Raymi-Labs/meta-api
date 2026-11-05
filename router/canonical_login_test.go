package router

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCanonicalLoginNavigationDoesNotRedirectAPIsOrCallbacks(t *testing.T) {
	r := gin.New()
	r.Use(canonicalLoginNavigation())
	r.NoRoute(func(c *gin.Context) { c.String(200, "unchanged") })
	for _, tc := range []struct {
		method, path, host string
		redirect           bool
	}{
		{"GET", "/login?code=secret", "meta-api.vip", true},
		{"HEAD", "/register", "cn.meta-api.vip", true},
		{"GET", "/desktop-login", "nan.meta-api.vip", true},
		{"GET", "/login", "meta-api.com", false},
		{"POST", "/login", "meta-api.vip", false},
		{"POST", "/api/desktop-auth/exchange", "meta-api.vip", false},
		{"GET", "/oauth/google?code=secret", "meta-api.vip", false},
		{"POST", "/v1/responses", "cn.meta-api.vip", false},
		{"GET", "/login", "attacker.test", false},
	} {
		req := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
		req.Header.Set("X-Forwarded-Host", "attacker.test")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if tc.redirect {
			require.Equal(t, http.StatusFound, w.Code)
			require.Contains(t, w.Header().Get("Location"), "https://meta-api.com/")
			require.NotContains(t, w.Header().Get("Location"), "secret")
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		} else {
			require.Equal(t, 200, w.Code)
			require.Empty(t, w.Header().Get("Location"))
		}
	}
}
