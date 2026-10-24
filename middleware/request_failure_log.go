package middleware

import (
	"time"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// RequestFailureLog observes the final result without buffering or reading the
// response body. Anonymous failures cannot be attributed to a user's logs.
func RequestFailureLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if c.GetString(RouteTagKey) == "relay" {
			service.RecordFinalRequestFailure(c, time.Since(start))
			service.ObserveQuotaNoticeOutcome(c)
		}
	}
}
