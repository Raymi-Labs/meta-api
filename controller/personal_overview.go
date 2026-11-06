package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func GetMyMonthUsage(c *gin.Context) {
	id := c.GetInt("id")
	if id <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false})
		return
	}
	zone := c.DefaultQuery("timezone", "UTC")
	if len(zone) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	result, err := model.GetPersonalMonthUsage(c.Request.Context(), id, location, time.Now())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false})
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
func GetMyTokenSummary(c *gin.Context) {
	id := c.GetInt("id")
	if id <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false})
		return
	}
	result, err := model.GetPersonalTokenSummary(c.Request.Context(), id, time.Now().Unix())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false})
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
