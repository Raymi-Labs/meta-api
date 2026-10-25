package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func SetWebRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	frontendFS := common.EmbedFolder(buildFS, "web/dist")
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit(frontendFS))
	router.Use(middleware.Cache())
	if imagePlaygroundIndex, err := buildFS.ReadFile("web/dist/image-playground/index.html"); err == nil {
		registerImagePlaygroundIndexRoutes(router, imagePlaygroundIndex)
	}
	registerPrerenderedIndexRoutes(router, indexPage)
	router.Use(static.Serve("/", frontendFS))
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if redirectMalformedFullwidthQuery(c) {
			return
		}
		if shouldReturnRelayNotFound(c.Request.RequestURI) {
			controller.RelayNotFound(c)
			return
		}
		serveWebIndex(c, indexPage)
	})
}

func shouldReturnRelayNotFound(requestURI string) bool {
	return strings.HasPrefix(requestURI, "/v1") ||
		strings.HasPrefix(requestURI, "/api") ||
		strings.HasPrefix(requestURI, "/assets") ||
		strings.HasPrefix(requestURI, "/image-playground/assets/")
}

func registerPrerenderedIndexRoutes(router *gin.Engine, indexPage []byte) {
	serve := func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		serveWebIndex(c, indexPage)
	}
	for _, path := range []string{"/login", "/login/", "/register", "/register/", "/pricing", "/pricing/", "/download", "/download/", "/about", "/about/"} {
		router.GET(path, serve)
		router.HEAD(path, serve)
	}
}

func serveWebIndex(c *gin.Context, indexPage []byte) {
	// 优先用预渲染产物（含完整 React HTML，爬虫不跑 JS 也能看到内容），
	// 没有则用 indexPage 兜底。两者都再过一次 RenderIndexWithMeta 按 path 注入
	// 差异化 title/description/canonical/og/twitter，确保 SEO 元数据与路径一致。
	template := service.GetPrerenderedHTML(c.Request.URL.Path)
	if template == nil {
		template = indexPage
	}
	html := service.RenderIndexWithMeta(common.InjectAnalyticsConfig(template), c.Request.URL.Path)
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", html)
}

func registerImagePlaygroundIndexRoutes(router *gin.Engine, indexPage []byte) {
	serve := func(c *gin.Context) {
		// image-playground 子站同样走「预渲染优先 + 模板替换兜底」二段链路，
		// 仅当 prerender.mjs 注册了该路径时 GetPrerenderedHTML 返回非 nil；
		// 当前阶段 2 仅预渲染主站公开页，子站直接用模板替换。
		template := service.GetPrerenderedHTML(c.Request.URL.Path)
		if template == nil {
			template = indexPage
		}
		html := service.RenderIndexWithMeta(template, c.Request.URL.Path)
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", html)
	}
	router.GET("/image-playground", serve)
	router.GET("/image-playground/", serve)
}

func redirectMalformedFullwidthQuery(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}

	path := c.Request.URL.Path
	index := strings.Index(path, "？")
	if index < 0 {
		return false
	}

	basePath := strings.TrimRight(path[:index], "/")
	if basePath == "" {
		basePath = "/"
	}
	embeddedQuery := strings.TrimLeft(path[index+len("？"):], "?&")
	rawQuery := c.Request.URL.RawQuery
	if embeddedQuery != "" {
		if rawQuery != "" {
			rawQuery = embeddedQuery + "&" + rawQuery
		} else {
			rawQuery = embeddedQuery
		}
	}

	target := basePath
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	c.Redirect(http.StatusFound, target)
	return true
}
