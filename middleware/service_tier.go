package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func prepareServiceTier(c *gin.Context, r *ModelRequest) *types.NewAPIError {
	if r == nil || r.Model == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	managed, err := model.PaidServiceTierManaged(ctx, r.Model)
	if err != nil {
		return types.NewErrorWithStatusCode(fmt.Errorf("service tier registry unavailable"), types.ErrorCode("service_tier_unavailable"), http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	if !managed {
		return nil
	}
	tier := r.ServiceTier
	if tier == "" {
		tier = "default"
	}
	if tier != "default" && tier != "priority" && tier != "ultrafast" {
		return types.NewErrorWithStatusCode(fmt.Errorf("use default, priority (Fast), or ultrafast"), types.ErrorCode("service_tier_invalid"), http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if tier != "default" {
		switch c.Request.URL.Path {
		case "/v1/chat/completions", "/v1/responses", "/pg/chat/completions":
		default:
			return types.NewErrorWithStatusCode(fmt.Errorf("paid service tiers require Chat Completions or Responses"), types.ErrorCode("service_tier_invalid"), http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	c.Set(service.ManagedServiceTierKey, true)
	c.Set(service.RequestedServiceTierKey, tier)
	return nil
}
