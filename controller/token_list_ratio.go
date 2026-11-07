package controller

import (
	"math"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// Display-only snapshot. Never persist these fields or use them for billing.
type tokenListItem struct {
	*model.Token
	GroupRatio       *float64 `json:"group_ratio"`
	GroupRatioStatus string   `json:"group_ratio_status"`
}

func buildTokenListItems(userID int, tokens []*model.Token) []tokenListItem {
	items := make([]tokenListItem, 0, len(tokens))
	if len(tokens) == 0 {
		return items
	}

	// Resolve once per page, not once per token. Failure must not hide the list
	// or manufacture a default ratio for an unknown user's special pricing.
	userGroup, err := model.GetUserGroup(userID, false)
	usableGroups := service.GetUserUsableGroups(userGroup)
	for _, token := range tokens {
		item := tokenListItem{
			Token:            buildMaskedTokenResponse(token),
			GroupRatioStatus: "unavailable",
		}
		if token != nil && err == nil && userGroup != "" && token.UserId == userID {
			group := token.Group
			if group == "" {
				group = userGroup
			}
			if token.RoutingConfig != "" {
				groups, scopeErr := service.TokenConfiguredGroups(userGroup, token, "")
				if scopeErr == nil && len(groups) > 0 {
					item.GroupRatioStatus = "auto"
				}
				items = append(items, item)
				continue
			}
			if _, allowed := usableGroups[group]; allowed {
				if group == "auto" {
					item.GroupRatioStatus = "auto"
				} else if ratio_setting.ContainsGroupRatio(group) {
					ratio := service.GetUserGroupRatio(userGroup, group)
					if ratio >= 0 && !math.IsNaN(ratio) && !math.IsInf(ratio, 0) {
						item.GroupRatio = &ratio
						item.GroupRatioStatus = "fixed"
					}
				}
			}
		}
		items = append(items, item)
	}
	return items
}
