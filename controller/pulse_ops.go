package controller

import (
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	pulseAdminRole       = "admin"
	pulseOpsOverviewPath = "/v1/internal/admin/operations/overview"
)

// GetPulseOperationsOverview proxies Pulse's read-only operations projection to
// the new-api admin console.
//
// The console cannot call Pulse directly: that route requires an HMAC signature
// under the admin role, and putting the admin secret in a browser would hand
// every visitor the privilege. new-api already authenticates administrators via
// session, so it signs here instead and the secret stays server-side.
func GetPulseOperationsOverview(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问"})
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writePulseBFFBadRequest(c, "运营概览不支持查询参数")
		return
	}
	proxyPulseSignedReadProjected(c, pulseAdminRole, pulseOpsConfig, pulseOpsOverviewPath, nil, pulseBFFHTTPClient, projectPulseOperationsOverview)
}

func projectPulseOperationsOverview(body []byte) ([]byte, error) {
	var upstream struct {
		ObservedAt string `json:"observed_at"`
		Snapshot   *struct {
			SettlementRetryCount int64 `json:"settlement_retry_count"`
			SettlementDeadCount  int64 `json:"settlement_dead_count"`
		} `json:"snapshot"`
	}
	if err := common.Unmarshal(body, &upstream); err != nil {
		return nil, err
	}
	projected := map[string]any{"observed_at": upstream.ObservedAt}
	if upstream.Snapshot != nil {
		projected["snapshot"] = map[string]int64{
			"settlement_retry_count": upstream.Snapshot.SettlementRetryCount,
			"settlement_dead_count":  upstream.Snapshot.SettlementDeadCount,
		}
	}
	return common.Marshal(projected)
}

func pulseOpsConfig() (*url.URL, string, error) {
	return pulseInternalConfig("PulseAdminHMACSecret", "PULSE_ADMIN_HMAC_SECRET")
}

// GeneratePulseSecret returns a fresh HMAC secret for the admin console to
// display once. It deliberately does not persist anything: the same value has
// to be copied into Pulse's own configuration, and an operator who saves it
// here before Pulse knows it would break signing until both sides match.
func GeneratePulseSecret(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
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
