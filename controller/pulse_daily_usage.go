package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"math"
	"net/http"
	"time"
)

func PulseYesterdayUsage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id := c.GetUint64("pulse_service_user_id")
	if id == 0 || id > math.MaxInt {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.ContentLength != 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !common.LogConsumeEnabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage_logs_disabled"})
		return
	}
	date, total, err := model.PulseYesterdayUsage(c.Request.Context(), int(id), time.Now())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"date": date, "recorded_consume_quota": total, "completeness": "unknown"})
}
