package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInvitePromotionRateLimit(t *testing.T) {
	old := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = old })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/i/:code", InvitePromotionRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for i := 0; i < 61; i++ {
		req := httptest.NewRequest("GET", "/i/fixture?src=banner", nil)
		req.RemoteAddr = "192.0.2.93:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if i < 60 {
			require.Equal(t, http.StatusNoContent, w.Code)
		} else {
			require.Equal(t, http.StatusTooManyRequests, w.Code)
			require.Equal(t, "60", w.Header().Get("Retry-After"))
		}
	}
	req := httptest.NewRequest("GET", "/i/fixture", nil)
	req.RemoteAddr = "192.0.2.94:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}
