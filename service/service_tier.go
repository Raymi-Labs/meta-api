package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const ManagedServiceTierKey = "managed_service_tier"
const RequestedServiceTierKey = "requested_service_tier"

func ChannelAllowsRequestedServiceTier(c *gin.Context, ch *model.Channel, name string) bool {
	if c == nil || !c.GetBool(ManagedServiceTierKey) {
		return true
	}
	tier := c.GetString(RequestedServiceTierKey)
	if tier == "" || tier == "default" {
		return true
	}
	return model.ChannelSupportsPaidServiceTier(ch, name, tier)
}
