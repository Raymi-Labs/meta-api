package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func ValidateTokenRoutingConfig(userGroup string, token *model.Token) error {
	config, err := model.ParseTokenRoutingConfig(token.RoutingConfig)
	if err != nil || config == nil {
		return err
	}
	if token.SourceType == model.TokenSourceTypeSellableToken {
		return fmt.Errorf("可售令牌不支持自定义模型分组")
	}
	if token.Group != config.Groups[0] {
		return fmt.Errorf("首选分组必须是可用分组中的第一项")
	}
	usable := GetUserUsableGroups(userGroup)
	for _, group := range config.Groups {
		if _, ok := usable[group]; !ok || !ratio_setting.ContainsGroupRatio(group) {
			return fmt.Errorf("无权访问分组 %s 或分组已被弃用", group)
		}
	}
	if token.ModelLimitsEnabled {
		limits := token.GetModelLimitsMap()
		for name := range config.ModelGroups {
			if !limits[name] && !limits[ratio_setting.FormatMatchingModelName(name)] {
				return fmt.Errorf("模型 %s 不在令牌模型限制列表中", name)
			}
		}
	}
	return nil
}

func TokenConfiguredGroups(userGroup string, token *model.Token, modelName string) ([]string, error) {
	config, err := model.ParseTokenRoutingConfig(token.RoutingConfig)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return TokenRoutingGroups(userGroup, token.Group, token.AllowsGroupFallback()), nil
	}
	groups := config.Groups
	if modelName != "" {
		groups = config.GroupsForModel(modelName)
	}
	// Permission changes take effect without rewriting tokens. Never expand a
	// revoked/empty scope to the legacy auto catalog.
	usable := GetUserUsableGroups(userGroup)
	allowed := make([]string, 0, len(groups))
	for _, group := range groups {
		if _, ok := usable[group]; ok && ratio_setting.ContainsGroupRatio(group) {
			allowed = append(allowed, group)
		}
	}
	return allowed, nil
}

func HasTokenRoutingConfig(c *gin.Context) bool {
	return common.GetContextKeyString(c, constant.ContextKeyTokenRoutingConfig) != ""
}

func RequestModelRoutingGroups(c *gin.Context, group, name string) []string {
	if HasTokenRoutingConfig(c) {
		groups, err := TokenConfiguredGroups(common.GetContextKeyString(c, constant.ContextKeyUserGroup),
			&model.Token{RoutingConfig: common.GetContextKeyString(c, constant.ContextKeyTokenRoutingConfig)}, name)
		if err != nil {
			return []string{}
		}
		return groups
	}
	return TokenRoutingGroups(common.GetContextKeyString(c, constant.ContextKeyUserGroup), group,
		common.GetContextKeyBool(c, constant.ContextKeyTokenAutoGroupFallback))
}
