package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDesktopFeedbackRateLimitUsesAccount(t *testing.T) {
	old := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = old })
	r := gin.New()
	r.Use(func(c *gin.Context) { id, _ := strconv.Atoi(c.GetHeader("Test-User")); c.Set("id", id) })
	r.POST("/", DesktopFeedbackRateLimit(), func(c *gin.Context) { c.Status(204) })
	request := func(id string, n int) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Test-User", id)
		req.RemoteAddr = "192.0.2." + strconv.Itoa(n) + ":4000"
		r.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 10; i++ {
		require.Equal(t, 204, request("927451", i+1))
	}
	require.Equal(t, 429, request("927451", 20))
	require.Equal(t, 204, request("927452", 20))
	require.Equal(t, 401, request("", 20))
}
