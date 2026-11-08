package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// GeneratePulseSecret returns a fresh HMAC secret for the admin console to
// display once. It deliberately does not persist anything: the same value has
// to be copied into Pulse's own configuration, and an operator who saves it
// here before Pulse knows it would break signing until both sides match.
func GeneratePulseSecret(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问"})
		return
	}
	// 64 chars clears the 32-character production minimum with room to spare,
	// and the alphabet is alphanumeric so the value can be pasted into a
	// dotenv file without quoting.
	secret, err := common.GenerateRandomCharsKey(64)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "生成密钥失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": secret})
}
