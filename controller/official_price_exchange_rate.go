package controller

import (
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func GetOfficialPriceExchangeRate(c *gin.Context) {
	quote, available := service.GetOfficialPriceExchangeRate()
	c.Header("Cache-Control", "no-store")
	if !available {
		c.JSON(http.StatusOK, gin.H{"success": false, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": quote})
}
