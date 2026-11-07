package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// TokenRoutingConfig is stored as TEXT and cached as a string, like the other
// token restrictions. Empty storage denotes the legacy routing contract.
type TokenRoutingConfig struct {
	Groups      []string            `json:"groups"`
	ModelGroups map[string][]string `json:"model_groups,omitempty"`
	// AutoFallback opts this multi-group token into trying the remaining
	// platform auto groups after its own groups. Absent means off.
	AutoFallback bool `json:"auto_fallback,omitempty"`
}

func ParseTokenRoutingConfig(raw string) (*TokenRoutingConfig, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 60000 {
		return nil, fmt.Errorf("令牌路由配置过大")
	}
	var config *TokenRoutingConfig
	if err := common.UnmarshalJsonStr(raw, &config); err != nil || config == nil {
		return nil, fmt.Errorf("令牌路由配置无效")
	}
	validateGroups := func(groups []string) error {
		if len(groups) == 0 || len(groups) > 100 {
			return fmt.Errorf("请选择 1 至 100 个可用分组")
		}
		seen := map[string]bool{}
		for _, group := range groups {
			if group == "" || group == "auto" || strings.TrimSpace(group) != group || seen[group] {
				return fmt.Errorf("令牌路由分组无效或重复")
			}
			seen[group] = true
		}
		return nil
	}
	if err := validateGroups(config.Groups); err != nil {
		return nil, err
	}
	if len(config.ModelGroups) > 500 {
		return nil, fmt.Errorf("模型分组规则过多")
	}
	for name, groups := range config.ModelGroups {
		if name == "" || strings.TrimSpace(name) != name || strings.Contains(name, ",") {
			return nil, fmt.Errorf("模型分组规则的模型名无效")
		}
		if err := validateGroups(groups); err != nil {
			return nil, err
		}
		for _, group := range groups {
			if !common.StringsContains(config.Groups, group) {
				return nil, fmt.Errorf("模型 %s 的分组 %s 不在令牌可用分组中", name, group)
			}
		}
	}
	return config, nil
}

// HasModelRule reports whether the model has its own group list, which is a
// deliberate narrowing: automatic group fallback never extends it.
func (config *TokenRoutingConfig) HasModelRule(name string) bool {
	if _, ok := config.ModelGroups[name]; ok {
		return true
	}
	_, ok := config.ModelGroups[ratio_setting.FormatMatchingModelName(name)]
	return ok
}

func (config *TokenRoutingConfig) GroupsForModel(name string) []string {
	if groups, ok := config.ModelGroups[name]; ok {
		return groups
	}
	if groups, ok := config.ModelGroups[ratio_setting.FormatMatchingModelName(name)]; ok {
		return groups
	}
	return config.Groups
}
