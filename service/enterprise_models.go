package service

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// EnterprisePlatformModels 返回平台对企业计费用户组开放的全部模型（对普通用户可见的），
// 是企业"继承平台"时的范围，也是企业管理员挑选"指定模型"时的候选清单。
func EnterprisePlatformModels(billingGroup string) []string {
	seen := map[string]struct{}{}
	for group := range GetUserUsableGroups(billingGroup) {
		if group == "auto" {
			continue
		}
		for _, name := range model.GetGroupEnabledModels(group) {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	names = model.FilterModelsByVisibility(names, common.RoleCommonUser)
	sort.Strings(names)
	return names
}

// EnterpriseModelPolicy 计算企业与成员两级"指定模型"的交集。
// restricted 为假表示两级都继承平台，不额外限制；为真时 allowed 是允许的模型集合，
// 空集合即全部禁止。
func EnterpriseModelPolicy(ent *model.Enterprise, member *model.EnterpriseMember) (allowed map[string]bool, restricted bool) {
	entCustom := ent.ModelMode == model.EnterpriseModelModeCustom
	memberCustom := member.ModelMode == model.EnterpriseModelModeCustom
	if !entCustom && !memberCustom {
		return nil, false
	}
	allowed = map[string]bool{}
	switch {
	case entCustom && memberCustom:
		entSet := make(map[string]struct{}, len(ent.Models))
		for _, name := range ent.Models {
			entSet[name] = struct{}{}
		}
		for _, name := range member.Models {
			if _, ok := entSet[name]; ok {
				allowed[name] = true
			}
		}
	case entCustom:
		for _, name := range ent.Models {
			allowed[name] = true
		}
	default:
		for _, name := range member.Models {
			allowed[name] = true
		}
	}
	return allowed, true
}

// EnterpriseMemberModels 返回成员当前实际可用的模型清单（平台开放范围与企业、成员限制的交集）。
func EnterpriseMemberModels(ent *model.Enterprise, member *model.EnterpriseMember) []string {
	platform := EnterprisePlatformModels(ent.BillingGroup)
	allowed, restricted := EnterpriseModelPolicy(ent, member)
	if !restricted {
		return platform
	}
	out := make([]string, 0, len(platform))
	for _, name := range platform {
		if allowed[name] {
			out = append(out, name)
		}
	}
	return out
}
