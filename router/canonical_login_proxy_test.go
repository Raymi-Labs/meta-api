package router

import (
	"embed"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the real web middleware stack through a local reverse proxy. A
// backend cannot recover the browser origin after a proxy overwrites Host.
func TestCanonicalLoginReverseProxyContract(t *testing.T) {
	for _, tc := range []struct {
		name, upstreamHost, forwardedHost, forwardedProto string
	}{
		{name: "preserved_browser_host"},
		{name: "rewritten_to_vip", upstreamHost: "meta-api.vip"},
		{name: "rewritten_to_cn", upstreamHost: "cn.meta-api.vip"},
		{name: "rewritten_to_nan", upstreamHost: "nan.meta-api.vip"},
		{name: "rewritten_with_https", upstreamHost: "meta-api.vip", forwardedProto: "https"},
		{name: "rewritten_with_forwarded_primary", upstreamHost: "meta-api.vip", forwardedHost: "meta-api.com", forwardedProto: "https"},
		{name: "forged_forwarded_alias", forwardedHost: "meta-api.vip"},
		{name: "forged_forwarded_host_chain", forwardedHost: "evil.test, meta-api.com", forwardedProto: "http, https"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := gin.New()
			backend.GET("/api/status", func(c *gin.Context) { c.String(200, "status") })
			SetWebRouter(backend, embed.FS{}, []byte("<html>login fixture</html>"))
			upstream := httptest.NewServer(backend)
			t.Cleanup(upstream.Close)
			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
			transport := &http.Transport{Proxy: nil}
			proxy.Transport = transport
			t.Cleanup(transport.CloseIdleConnections)
			director := proxy.Director
			proxy.Director = func(r *http.Request) {
				director(r)
				if tc.upstreamHost != "" {
					r.Host = tc.upstreamHost
				}
				r.Header.Set("X-Forwarded-Host", tc.forwardedHost)
				r.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			}
			edge := httptest.NewServer(proxy)
			t.Cleanup(edge.Close)
			client := edge.Client()
			client.Timeout = 3 * time.Second
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			t.Cleanup(client.CloseIdleConnections)

			for _, browserHost := range []string{"meta-api.com", "meta-api.vip", "cn.meta-api.vip", "nan.meta-api.vip", "localhost", "custom.test"} {
				for _, path := range []string{"/login", "/login/", "/register?aff=localtest", "/register/?aff=localtest", "/desktop-login?request_id=" + strings.Repeat("a", 43), "/pricing", "/api/status", "/oauth/google?code=SECRET"} {
					for _, method := range []string{http.MethodGet, http.MethodHead} {
						if method == http.MethodHead && (strings.HasPrefix(path, "/desktop-login") || path == "/api/status") {
							continue
						}
						req, err := http.NewRequest(method, edge.URL+path, nil)
						require.NoError(t, err)
						req.Host = browserHost
						resp, err := client.Do(req)
						require.NoError(t, err)
						body, err := io.ReadAll(resp.Body)
						resp.Body.Close()
						require.NoError(t, err)
						require.Equal(t, http.StatusOK, resp.StatusCode, "%s %s host=%s", method, path, browserHost)
						require.Empty(t, resp.Header.Get("Location"))
						require.NotContains(t, string(body), "SECRET")
						if strings.HasPrefix(path, "/login") || strings.HasPrefix(path, "/register") || strings.HasPrefix(path, "/desktop-login") {
							require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
							require.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
						}
						if strings.HasPrefix(path, "/desktop-login") {
							require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
							require.Equal(t, "frame-ancestors 'none'", resp.Header.Get("Content-Security-Policy"))
						}
					}
				}
			}
		})
	}
}
