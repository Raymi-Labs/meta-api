package middleware

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	server.Use(gin.LoggerWithConfig(gin.LoggerConfig{Output: common.LogWriter, Formatter: func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			sanitizeAccessLogPath(param.Path),
		)
	}}))
}

// sanitizeAccessLogPath only changes the log value, never the request URL used
// by authentication or the relay. Invalid queries are omitted, not logged raw.
func sanitizeAccessLogPath(path string) string {
	base, raw, hasQuery := strings.Cut(path, "?")
	if !hasQuery {
		return path
	}
	if strings.HasPrefix(base, "/api/oauth/") || strings.HasPrefix(base, "/oauth/") {
		return base
	}
	query, err := url.ParseQuery(raw)
	if err != nil {
		return base + "?redacted=invalid-query"
	}
	for key := range query {
		if sensitiveAccessQueryKey(key) {
			query.Set(key, "[REDACTED]")
		}
	}
	return base + "?" + query.Encode()
}

func sensitiveAccessQueryKey(key string) bool {
	key = strings.ToLower(key)
	if strings.Contains(key, "token") || strings.Contains(key, "secret") ||
		strings.Contains(key, "signature") || strings.Contains(key, "password") ||
		strings.Contains(key, "credential") {
		return true
	}
	switch key {
	case "key", "api_key", "api-key", "apikey", "x-api-key", "authorization", "auth", "passwd", "sig", "awsaccesskeyid":
		return true
	default:
		return false
	}
}
