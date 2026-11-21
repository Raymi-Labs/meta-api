package controller

import (
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

type pricingTimeRatioPreview struct {
	Ratio     float64 `json:"ratio"`
	Matched   bool    `json:"matched"`
	Timezone  string  `json:"timezone,omitempty"`
	MatchedAt string  `json:"matched_at,omitempty"`
}

func buildPricingTimeRatioMap(pricing []model.Pricing, usableGroup map[string]string, userGroup string, now time.Time) map[string]map[string]pricingTimeRatioPreview {
	if len(pricing) == 0 || len(usableGroup) == 0 {
		return map[string]map[string]pricingTimeRatioPreview{}
	}
	if len(ratio_setting.GetTimeRatioRulesCopy()) == 0 {
		return map[string]map[string]pricingTimeRatioPreview{}
	}

	result := make(map[string]map[string]pricingTimeRatioPreview)
	for _, item := range pricing {
		if item.NoDiscount {
			continue
		}
		modelRatios := make(map[string]pricingTimeRatioPreview)
		for group := range usableGroup {
			info := ratio_setting.ResolveTimeRatio(item.ModelName, group, userGroup, now)
			if !info.Matched() && info.EffectiveRatio() == 1 {
				continue
			}
			modelRatios[group] = pricingTimeRatioPreview{
				Ratio:     info.EffectiveRatio(),
				Matched:   info.Matched(),
				Timezone:  info.Timezone,
				MatchedAt: info.MatchedAt,
			}
		}
		if len(modelRatios) > 0 {
			result[item.ModelName] = modelRatios
		}
	}
	return result
}

// pricingVendorsForModels projects the shared catalog onto this response's
// already-authorized models. Never filter the shared slice in place: an admin
// request and an anonymous request may use the same cached vendor catalog.
func pricingVendorsForModels(pricing []model.Pricing, vendors []model.PricingVendor) []model.PricingVendor {
	used := make(map[int]bool, len(pricing))
	for _, item := range pricing {
		if item.VendorID > 0 {
			used[item.VendorID] = true
		}
	}
	result := make([]model.PricingVendor, 0)
	for _, vendor := range vendors {
		if used[vendor.ID] {
			result = append(result, vendor)
			delete(used, vendor.ID)
		}
	}
	return result
}

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		// The pricing cache is shared by all viewers.
		groups := make([]string, 0, len(item.EnableGroup))
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok || group == "all" {
				groups = append(groups, group)
			}
		}
		if len(groups) > 0 {
			item.EnableGroup = groups
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func catalogViewer(c *gin.Context) (int, string) {
	if id := c.GetInt("id"); id > 0 {
		user, err := model.GetUserById(id, false)
		if err != nil || user.Status != common.UserStatusEnabled {
			return common.RoleGuestUser, ""
		}
		return user.Role, user.Group
	}
	return getRequestRole(c), ""
}

func GetPricing(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	role, group := catalogViewer(c)
	usableGroup := service.GetUserUsableGroups(group)
	pricing := model.FilterPricingByVisibility(model.GetPricing(), role)
	pricing = pricingWithDiscountPolicy(filterPricingByUsableGroups(pricing, usableGroup))
	pricing, publicVendors := model.PublicPricingMetadata(pricing, pricingVendorsForModels(pricing, model.GetVendors()))
	groupRatio := ratio_setting.GetGroupRatioCopy()
	for name := range groupRatio {
		if _, ok := usableGroup[name]; !ok {
			delete(groupRatio, name)
		} else if group != "" {
			if ratio, ok := ratio_setting.GetGroupGroupRatio(group, name); ok {
				groupRatio[name] = ratio
			}
		}
	}
	endpoints := make(map[string]common.EndpointInfo)
	catalogEndpoints := model.GetSupportedEndpointMap()
	for _, item := range pricing {
		for _, kind := range item.SupportedEndpointTypes {
			if endpoint, ok := catalogEndpoints[string(kind)]; ok {
				endpoints[string(kind)] = endpoint
			}
		}
	}
	now := time.Now()

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            publicVendors,
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": endpoints,
		"auto_groups":        service.GetUserAutoGroup(group),
		"time_ratio":         buildPricingTimeRatioMap(pricing, usableGroup, group, now),
		"time_ratio_at":      now.Format(time.RFC3339),
		"_":                  "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}

// Copy the shared catalog before adding live, model-specific billing policy.
func pricingWithDiscountPolicy(pricing []model.Pricing) []model.Pricing {
	result := append([]model.Pricing(nil), pricing...)
	for i := range result {
		result[i].NoDiscount = ratio_setting.IsNoDiscountModel(result[i].ModelName)
	}
	return result
}
