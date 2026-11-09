package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
)

// Only returns capabilities for this authenticated account and an allowed group.
// No channel identities, upstream names, credentials or account-wide permissions.
func GetUserServiceTiers(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	if c.GetInt("id") <= 0 {
		c.JSON(401, gin.H{"success": false})
		return
	}
	name, group := strings.TrimSpace(c.Query("model")), strings.TrimSpace(c.Query("group"))
	if name == "" || group == "" || len(name) > 255 || len(group) > 255 {
		c.JSON(400, gin.H{"success": false})
		return
	}
	user, err := model.GetUserCache(c.GetInt("id"))
	if err != nil {
		c.JSON(503, gin.H{"success": false})
		return
	}
	if _, ok := service.GetUserUsableGroups(user.Group)[group]; !ok || !model.IsModelCallableByRole(name, user.Role) {
		c.JSON(403, gin.H{"success": false})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	managed, err := model.PaidServiceTierManaged(ctx, name)
	if err != nil {
		c.JSON(503, gin.H{"success": false})
		return
	}
	tiers := []gin.H{{"id": "default", "multiplier": 1}}
	mode, expr := billing_setting.GetBillingConfig(name)
	if managed && mode == billing_setting.BillingModeTieredExpr && !strings.Contains(expr, "service_tier") && expr != "" {
		groups := []string{group}
		if group == "auto" {
			groups = service.GetUserAutoGroup(user.Group)
		}
		for _, tier := range []struct {
			id         string
			multiplier int
		}{{"priority", 2}, {"ultrafast", 6}} {
			found := false
			for _, g := range groups {
				candidates, err := model.GetSatisfiedChannelCandidates(g, name, 0, nil, nil, func(ch *model.Channel) bool {
					return model.ChannelSupportsPaidServiceTier(ch, name, tier.id) && service.IsChannelAllowedForClient(ch, service.DetectClient(c)) && !service.IsChannelUnavailableForRequest(ch) && !service.IsChannelModelCircuitOpen(ch, name)
				})
				if err != nil {
					c.JSON(503, gin.H{"success": false})
					return
				}
				if len(candidates) > 0 {
					found = true
					break
				}
			}
			if found {
				tiers = append(tiers, gin.H{"id": tier.id, "multiplier": tier.multiplier})
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"model": name, "group": group, "managed": managed, "tiers": tiers}})
}
