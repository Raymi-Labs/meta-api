package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetMyActivity(c *gin.Context) {
	id := c.GetInt("id")
	if id <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "用户未认证"})
		return
	}
	zone := c.DefaultQuery("timezone", "UTC")
	if len(zone) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid timezone"})
		return
	}
	if _, err := time.LoadLocation(zone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid timezone"})
		return
	}
	result, err := model.GetUserActivity(c.Request.Context(), id, zone, time.Now())
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("activity calendar unavailable for user %d: %v", id, err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "activity temporarily unavailable"})
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
