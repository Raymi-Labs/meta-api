package service

import (
	"sort"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// EnterprisePlatformModels 返回指定账号授权组和平台角色可见的配置候选。
// userGroup 不能是企业计费组；企业角色不等于平台角色。
func EnterprisePlatformModels(userGroup string, role int) []string {
	return enterprisePlatformModels(userGroup, role)
}

func enterprisePlatformModels(userGroup string, role int) []string {
	seen := map[string]struct{}{}
	for group := range GetUserUsableGroups(userGroup) {
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
	names = model.FilterModelsByVisibility(names, role)
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
	if ent == nil || member == nil || member.EnterpriseId != ent.Id {
		return nil
	}
	user, err := model.GetUserCache(member.UserId)
	if err != nil {
		return nil
	}
	platform := enterprisePlatformModels(user.Group, user.Role)
	allowed, restricted := EnterpriseModelPolicy(ent, member)
	if !restricted {
		return platform
	}
	out := make([]string, 0, len(platform))
	for _, name := range platform {
		if allowed[name] || allowed[ratio_setting.FormatMatchingModelName(name)] {
			out = append(out, name)
		}
	}
	return out
}
