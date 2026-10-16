package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRouteProbePublicUncompressedAndLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previous })
	engine := gin.New()
	SetApiRouter(engine)
	nonce := strings.Repeat("a", 32)
	for _, path := range []string{"/api/route-probe", "/api/route-probe/stream"} {
		req := httptest.NewRequest(http.MethodGet, path+"?nonce="+nonce, nil)
		req.RemoteAddr = "203.0.113.99:12345"
		req.Header.Set("Accept-Encoding", "gzip")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Empty(t, recorder.Header().Get("Content-Encoding"))
		require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
		require.Contains(t, recorder.Body.String(), nonce)
	}
	// Invalid requests consume the dedicated quota too; no model/auth setup is needed.
	for i := 0; i < 120; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/route-probe?nonce=invalid", nil)
		req.RemoteAddr = "203.0.113.99:12345"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if i == 119 {
			require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		}
	}
}
