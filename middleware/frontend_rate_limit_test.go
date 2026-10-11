package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFrontendRateLimitSeparatesAssetAndPageBudgets(t *testing.T) {
	oldRedis, oldWeb, oldStatic := common.RedisEnabled, common.GlobalWebRateLimitEnable, common.GlobalStaticRateLimitEnable
	oldWebNum, oldWebDuration := common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration
	oldStaticNum, oldStaticDuration := common.GlobalStaticRateLimitNum, common.GlobalStaticRateLimitDuration
	oldEval := evalRedisRateLimit
	t.Cleanup(func() {
		common.RedisEnabled, common.GlobalWebRateLimitEnable, common.GlobalStaticRateLimitEnable = oldRedis, oldWeb, oldStatic
		common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration = oldWebNum, oldWebDuration
		common.GlobalStaticRateLimitNum, common.GlobalStaticRateLimitDuration = oldStaticNum, oldStaticDuration
		evalRedisRateLimit = oldEval
	})
	frontendFS := http.FS(fstest.MapFS{
		"assets/app.js":                   {Data: []byte("js")},
		"image-playground/assets/main.js": {Data: []byte("js")},
		"index.html":                      {Data: []byte("html")},
		"login/index.html":                {Data: []byte("html")},
	})
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration = 2, 180
	common.GlobalStaticRateLimitNum, common.GlobalStaticRateLimitDuration = 3, 180
	for _, redis := range []bool{false, true} {
		for _, staticEnabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("redis=%v/static=%v", redis, staticEnabled), func(t *testing.T) {
				common.RedisEnabled, common.GlobalStaticRateLimitEnable = redis, staticEnabled
				counts := map[string]int{}
				evalRedisRateLimit = func(_ context.Context, key string, _ time.Time, maxNum int, _ int64, _ time.Duration) (bool, error) {
					counts[key]++
					return counts[key] <= maxNum, nil
				}
				r := gin.New()
				r.Use(GlobalWebRateLimit(frontendFS))
				r.NoRoute(func(c *gin.Context) { c.Status(http.StatusOK) })
				ip := fmt.Sprintf("192.0.2.%d:1234", 10+len(t.Name()))
				request := func(method, target string) *httptest.ResponseRecorder {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(method, target, nil)
					req.RemoteAddr = ip
					r.ServeHTTP(rec, req)
					return rec
				}
				require.Equal(t, 200, request("GET", "/login").Code)
				for _, target := range []string{"/assets/app.js", "/assets/app.js?v=1", "/image-playground/assets/main.js"} {
					require.Equal(t, 200, request("GET", target).Code)
				}
				fourth := request("HEAD", "/assets/app.js")
				if staticEnabled {
					require.Equal(t, 429, fourth.Code)
					require.Equal(t, "GS", fourth.Header().Get("X-RateLimit-Scope"))
				} else {
					require.Equal(t, 200, fourth.Code)
				}
				require.Equal(t, 200, request("GET", "/console/channel").Code)
				for _, target := range []string{"/login", "/index.html", "/login/", "/assets/missing.js", "/assets/", "/unknown"} {
					page := request("GET", target)
					require.Equal(t, 429, page.Code, target)
					require.Equal(t, "GW", page.Header().Get("X-RateLimit-Scope"))
				}
				require.Equal(t, 429, request("POST", "/assets/app.js").Code)
			})
		}
	}
}
