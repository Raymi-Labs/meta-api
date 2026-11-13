package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	// Payer and permissions must already be established before Distribute. Fail
	// closed if a route forgot the middleware; rebuilding here would be too late.
	if !common.GetContextKeyBool(c, constant.ContextKeyPlaygroundPayerPrepared) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
			"message": "体验区付款方尚未验证", "type": "access_denied",
		}})
		return
	}
	Relay(c, types.RelayFormatOpenAI)
}
