package router

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	SetDesktopUpdateRouter(router)
	SetApiRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	SetVideoRouter(router)
	// 短邀请链接 /i/<code> → 302 到 /register?aff=<code>（必须在 SetWebRouter 之前注册）
	router.GET("/i/:code", middleware.InvitePromotionRateLimit(), controller.InviteRedirect)
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, buildFS, indexPage)
	} else {
		frontendBaseUrl = strings.TrimSuffix(frontendBaseUrl, "/")
		router.NoRoute(func(c *gin.Context) {
			c.Set(middleware.RouteTagKey, "web")
			if redirectMalformedFullwidthQuery(c) {
				return
			}
			c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("%s%s", frontendBaseUrl, c.Request.RequestURI))
		})
	}
}
