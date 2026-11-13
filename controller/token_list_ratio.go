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
	// Payer 标明这把令牌由谁付款：personal（个人钱包）或 enterprise（企业额度）。
	Payer                string `json:"payer"`
	EnterpriseName       string `json:"enterprise_name,omitempty"`
	EnterpriseLimitQuota int64  `json:"enterprise_limit_quota"`
	EnterpriseUsedQuota  int64  `json:"enterprise_used_quota"`
}

func buildTokenListItems(userID int, tokens []*model.Token) []tokenListItem {
	items := make([]tokenListItem, 0, len(tokens))
	if len(tokens) == 0 {
		return items
	}

	// Resolve once per page, not once per token. Failure must not hide the list
	// or manufacture a default ratio for an unknown user's special pricing.
	personalGroup, personalErr := model.GetUserGroup(userID, false)
	enterprises := map[int]*model.Enterprise{}
	enterpriseTokenIDs := make([]int, 0)
	for _, token := range tokens {
		if token == nil || token.EnterpriseId <= 0 || token.UserId != userID {
			continue
		}
		enterpriseTokenIDs = append(enterpriseTokenIDs, token.Id)
		if _, seen := enterprises[token.EnterpriseId]; !seen {
			// 读取失败时保留 nil：该令牌显示为倍率不可用，绝不回退成个人用户组的倍率。
			ent, _ := model.GetEnterpriseByID(token.EnterpriseId)
			enterprises[token.EnterpriseId] = ent
		}
	}
	budgets, _ := model.GetEnterpriseTokenBudgets(enterpriseTokenIDs)

	for _, token := range tokens {
		item := tokenListItem{
			Token:            buildMaskedTokenResponse(token),
			GroupRatioStatus: "unavailable",
			Payer:            tokenPayer(token),
		}
		userGroup, err := personalGroup, personalErr
		if token != nil && token.EnterpriseId > 0 {
			userGroup, err = "", nil
			if ent := enterprises[token.EnterpriseId]; ent != nil {
				userGroup = ent.BillingGroup
				item.EnterpriseName = ent.Name
			}
			budget := budgets[token.Id]
			item.EnterpriseLimitQuota, item.EnterpriseUsedQuota = budget.LimitQuota, budget.UsedQuota
		}
		if token != nil && err == nil && userGroup != "" && token.UserId == userID {
			group := service.ResolveTokenGroup(userGroup, token.Group)
			if token.RoutingConfig != "" {
				groups, scopeErr := service.TokenConfiguredGroups(userGroup, token, "")
				if scopeErr == nil && len(groups) > 0 {
					item.GroupRatioStatus = "auto"
				}
				items = append(items, item)
				continue
			}
			if _, allowed := service.GetUserUsableGroups(userGroup)[group]; allowed {
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
