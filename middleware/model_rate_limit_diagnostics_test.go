package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryRateLimitRejectionsAreDiagnosable(t *testing.T) {
	for _, tc := range []struct {
		name           string
		total, success int
		message        string
	}{
		{"total", 1, 0, "总请求数限制"}, {"success", 0, 1, "请求数限制"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			var rejected *gin.Context
			r.Use(func(c *gin.Context) {
				c.Set("id", 999)
				c.Set(common.RequestIdKey, "diagnostic-test-id")
				c.Next()
				rejected = c
			})
			r.Use(memoryRateLimitHandler("diagnostic-"+tc.name+"-"+common.GetUUID(), 60, tc.total, tc.success))
			r.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusOK) })
			for _, expected := range []int{200, 429} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", nil))
				require.Equal(t, expected, w.Code)
				if expected == 429 {
					var body struct {
						Error struct{ Message, Type string }
					}
					require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
					require.Contains(t, body.Error.Message, tc.message)
					require.Contains(t, body.Error.Message, "diagnostic-test-id")
					require.Equal(t, "new_api_error", body.Error.Type)
					require.True(t, rejected.IsAborted())
					require.False(t, service.RequestSucceeded(rejected))
					_, marked := rejected.Get("final_request_failure")
					require.True(t, marked)
				}
			}
		})
	}
}
