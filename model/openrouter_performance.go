package model

import "github.com/QuantumNous/new-api/constant"

// OpenRouterPerformanceRoute is a read-only projection of enabled, exact routes.
// Callers must first resolve the user's current model and group permissions.
type OpenRouterPerformanceRoute struct {
	ChannelID int
	ModelName string
	GroupName string
}

func GetOpenRouterPerformanceRoutes(groups, names []string) ([]OpenRouterPerformanceRoute, error) {
	routes := []OpenRouterPerformanceRoute{}
	if len(groups) == 0 || len(names) == 0 {
		return routes, nil
	}
	err := DB.Table("abilities AS a").Select("a.channel_id, a.model AS model_name, a."+commonGroupCol+" AS group_name").
		Joins("JOIN channels AS c ON c.id = a.channel_id").
		Where("a.enabled = ? AND c.status = ? AND c.type = ?", true, 1, constant.ChannelTypeOpenRouter).
		Where("a."+commonGroupCol+" IN ? AND a.model IN ?", groups, names).
		Order("a.priority DESC, a.channel_id ASC").Limit(10000).Scan(&routes).Error
	return routes, err
}
