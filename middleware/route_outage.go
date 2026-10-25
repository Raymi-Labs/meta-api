package middleware

import (
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

var routeOutageObserver = service.ObserveUnavailableRoutes

// Kept at the final all-groups-empty branch, after permissions and model
// existence checks. Restricted requests cannot establish whole-group outages.
func observeRouteOutage(c *gin.Context, modelName, group string, specificChannelID *int) {
	groups := routeOutageRequestGroups(c, group, specificChannelID)
	if len(groups) > 0 {
		routeOutageObserver(groups, modelName, c.GetInt("token_id"))
	}
}

func routeOutageRequestGroups(c *gin.Context, group string, specificChannelID *int) []string {
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost || c.GetInt("id") <= 0 || c.Request.Context().Err() != nil || specificChannelID != nil || GetAllowedTokenChannelIDs(c) != nil {
		return nil
	}
	return service.RequestRoutingGroups(c, group)
}
