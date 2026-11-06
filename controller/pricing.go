package controller

import (
	"time"

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

func GetPricing(c *gin.Context) {
	role := getRequestRole(c)
	pricing := model.FilterPricingByVisibility(model.GetPricing(), role)
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}
	now := time.Now()

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            pricingVendorsForModels(pricing, model.GetVendors()),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
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
