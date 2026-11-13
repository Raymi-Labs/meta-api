package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 企业令牌（付款方为企业）在控制台里的差异集中在这里：
// 可选分组与倍率按企业计费用户组计算，模型目录叠加企业与成员的模型限制，
// 额度展示读企业账本里的令牌上限，不读令牌表的剩余额度。

func parseTokenPayer(raw string) (string, error) {
	switch payer := strings.ToLower(strings.TrimSpace(raw)); payer {
	case "", model.TokenPayerPersonal, model.TokenPayerEnterprise:
		return payer, nil
	default:
		return "", fmt.Errorf("参数 payer 仅支持 personal 或 enterprise")
	}
}

func tokenPayer(token *model.Token) string {
	if token != nil && token.EnterpriseId > 0 {
		return model.TokenPayerEnterprise
	}
	return model.TokenPayerPersonal
}

// tokenEnterpriseScope 返回企业令牌或企业付款草稿对应的企业与成员；个人令牌返回 nil。
// 这里只确认归属，不检查成员是否仍有效：已停用成员仍可查看自己的历史令牌。
func tokenEnterpriseScope(userID int, token *model.Token) (*model.Enterprise, *model.EnterpriseMember, error) {
	if token == nil || token.EnterpriseId <= 0 {
		return nil, nil, nil
	}
	return model.GetEnterpriseTokenContext(token.EnterpriseId, token.MembershipId, userID)
}

// tokenBaseGroup 是决定令牌可选分组与倍率的用户组：个人令牌取账号用户组，企业令牌取企业计费用户组。
func tokenBaseGroup(userID int, token *model.Token) (string, error) {
	ent, _, err := tokenEnterpriseScope(userID, token)
	if err != nil {
		return "", err
	}
	if ent != nil {
		return ent.BillingGroup, nil
	}
	return model.GetUserGroup(userID, false)
}

// enterprisePayerDraft 为"企业付款"的未保存草稿补上企业归属，供分组与模型目录按企业范围计算。
func enterprisePayerDraft(userID int, token *model.Token) error {
	ent, member, err := model.GetEnterprisePayer(userID)
	if err != nil {
		return err
	}
	token.EnterpriseId, token.MembershipId = ent.Id, member.Id
	return nil
}

// tokenEnterpriseModelFilter 返回企业与成员模型限制的判定函数；不受限时返回 nil。
func tokenEnterpriseModelFilter(userID int, token *model.Token) (func(name string) bool, error) {
	ent, member, err := tokenEnterpriseScope(userID, token)
	if err != nil || ent == nil {
		return nil, err
	}
	allowed, restricted := service.EnterpriseModelPolicy(ent, member)
	if !restricted {
		return nil, nil
	}
	return func(name string) bool {
		return allowed[name] || allowed[ratio_setting.FormatMatchingModelName(name)]
	}, nil
}

// normalizeEnterpriseTokenDraft 校验并规整企业令牌的可编辑字段。
// 企业令牌不使用令牌表的剩余额度与周期套餐，额度只受成员预算和企业账本里的令牌上限约束。
func normalizeEnterpriseTokenDraft(token *model.Token, limit int64) error {
	if limit < 0 {
		return fmt.Errorf("令牌上限不能小于 0")
	}
	if token.PackageEnabled {
		return fmt.Errorf("企业令牌暂不支持周期套餐")
	}
	if token.ChannelLimitsEnabled || strings.TrimSpace(token.ChannelLimits) != "" {
		return fmt.Errorf("企业令牌暂不支持渠道限制")
	}
	token.UnlimitedQuota, token.RemainQuota = true, 0
	return nil
}

// tokenQuotaTotals 返回令牌对外展示的额度：已授予、已用、可用、是否不限额。
// 企业令牌读企业账本：设了上限按上限展示，没设上限显示为不限额，不暴露成员预算。
func tokenQuotaTotals(token *model.Token) (granted, used, available int, unlimited bool) {
	if token.EnterpriseId <= 0 {
		return token.RemainQuota + token.UsedQuota, token.UsedQuota, token.RemainQuota, token.UnlimitedQuota
	}
	budgets, err := model.GetEnterpriseTokenBudgets([]int{token.Id})
	if err != nil {
		return 0, 0, 0, false
	}
	budget := budgets[token.Id]
	used = int(budget.UsedQuota)
	if budget.LimitQuota <= 0 {
		return used, used, 0, true
	}
	available = max(int(budget.LimitQuota-budget.UsedQuota), 0)
	return int(budget.LimitQuota), used, available, false
}
