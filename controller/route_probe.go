package controller

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

var routeProbeNonce = regexp.MustCompile(`^[a-f0-9]{32}$`)

// No model, credentials, database queries or usage accounting belong on this path.
func prepareRouteProbe(c *gin.Context) (string, bool) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("CDN-Cache-Control", "no-store")
	c.Header("Cloudflare-CDN-Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	values := c.Request.URL.Query()["nonce"]
	if len(values) != 1 || !routeProbeNonce.MatchString(values[0]) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_nonce"})
		return "", false
	}
	return values[0], true
}

func RouteProbe(c *gin.Context) {
	nonce, ok := prepareRouteProbe(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": 1, "service": "yuanheng-api", "nonce": nonce})
}

// Synthetic SSE checks relay buffering without starting a paid generation.
func RouteProbeStream(c *gin.Context) {
	nonce, ok := prepareRouteProbe(c)
	if !ok {
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	for sequence := 0; sequence < 3; sequence++ {
		if sequence > 0 {
			timer := time.NewTimer(350 * time.Millisecond)
			select {
			case <-c.Request.Context().Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		data, err := common.Marshal(gin.H{"version": 1, "service": "yuanheng-api", "nonce": nonce, "sequence": sequence})
		if err != nil {
			return
		}
		if _, err = fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			return
		}
		c.Writer.Flush()
	}
}
