package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetMyWalletSummary(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	id := c.GetInt("id")
	if id <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false})
		return
	}
	// Same live quota path as the wallet's /user/self, including Redis reservations.
	quota, err := model.GetUserQuota(id, false)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false})
		return
	}
	now := time.Now()
	usage, _ := model.GetWalletUsage(c.Request.Context(), id, now)
	if c.Request.Context().Err() != nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user_id": id, "balance": quota, "as_of": now.Unix(), "usage": usage}})
}
