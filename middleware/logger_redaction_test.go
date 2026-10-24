package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSanitizeAccessLogPath(t *testing.T) {
	for _, key := range []string{"key", "api_key", "API-KEY", "apikey", "x-api-key", "access_token", "refresh_token", "id_token", "token", "authorization", "auth", "client_secret", "password", "passwd", "signature", "sig", "AWSAccessKeyId", "X-Amz-Credential", "X-Amz-Security-Token", "X-Amz-Signature"} {
		t.Run(key, func(t *testing.T) {
			got := sanitizeAccessLogPath("/v1/models?" + url.QueryEscape(key) + "=private-one&" + url.QueryEscape(key) + "=private-two&model=test&page=2")
			if strings.Contains(got, "private-") {
				t.Fatalf("credential leaked: %s", got)
			}
			_, raw, _ := strings.Cut(got, "?")
			q, err := url.ParseQuery(raw)
			if err != nil || q.Get(key) != "[REDACTED]" || q.Get("model") != "test" || q.Get("page") != "2" {
				t.Fatalf("unexpected redaction: %s (%v)", got, err)
			}
		})
	}
	for _, tc := range []struct{ input, want string }{
		{"/api/status", "/api/status"},
		{"/api/oauth/github?code=private-code&state=private-state", "/api/oauth/github"},
		{"/oauth/callback?code=private-code", "/oauth/callback"},
		{"/v1/models?%6bey=private-key&model=a", "/v1/models?key=%5BREDACTED%5D&model=a"},
		{"/v1/models?key=private-key&bad=%XX", "/v1/models?redacted=invalid-query"},
		{"/v1/models?key=private-key;bad=1", "/v1/models?redacted=invalid-query"},
	} {
		if got := sanitizeAccessLogPath(tc.input); got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAccessLoggerPreservesAuthenticationQuery(t *testing.T) {
	var log bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &log
	t.Cleanup(func() { gin.DefaultWriter = old })
	router := gin.New()
	SetUpLogger(router)
	raw := "key=private-key&key=private-second&model=test"
	router.GET("/v1/models", func(c *gin.Context) {
		if c.Query("key") != "private-key" || c.Request.URL.RawQuery != raw {
			t.Error("logging modified the authenticated request")
		}
		c.Status(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/models?"+raw, nil))
	if w.Code != http.StatusNoContent || strings.Contains(log.String(), "private-") || !strings.Contains(log.String(), "model=test") {
		t.Fatalf("unexpected status/log: %d %s", w.Code, log.String())
	}
}
