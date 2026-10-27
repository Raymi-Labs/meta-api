package controller

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

func validateTokenRoutingForSave(userID int, token *model.Token) error {
	userGroup, err := model.GetUserGroup(userID, false)
	if err != nil {
		return err
	}
	return service.ValidateTokenRoutingConfig(userGroup, token)
}

// Query parameters describe an unsaved editor draft; ownership is still checked
// by the calling handler when token_id is supplied.
func tokenRoutingDraft(c *gin.Context, token *model.Token) error {
	if raw, present := c.GetQuery("routing_config"); present {
		token.RoutingConfig = raw
	}
	if token.RoutingConfig == "" {
		return nil
	}
	return validateTokenRoutingForSave(c.GetInt("id"), token)
}

func tokenCatalogGroups(userID int, token *model.Token) ([]string, error) {
	if token.RoutingConfig == "" {
		return resolveRequestedTokenGroups(userID, token.Group, token.AllowsGroupFallback())
	}
	user, err := model.GetUserCache(userID)
	if err != nil {
		return nil, err
	}
	return service.TokenConfiguredGroups(user.Group, token, "")
}

type tokenCatalogAbility struct {
	Model       string
	Group       string
	ChannelId   int
	ChannelType int
}

// Build the catalog from real (model, group, channel) tuples. A union of models
// followed by a union of groups would leak models through unrelated mappings.
func tokenRoutingCatalog(userID int, token *model.Token) ([]tokenModelOption, error) {
	groups, err := tokenCatalogGroups(userID, token)
	if err != nil {
		return nil, err
	}
	config, err := model.ParseTokenRoutingConfig(token.RoutingConfig)
	if err != nil {
		return nil, err
	}
	options := make([]tokenModelOption, 0)
	if len(groups) == 0 {
		return options, nil
	}
	rows := []tokenCatalogAbility{}
	query := model.DB.Table("abilities").Select("abilities.*, channels.type as channel_type").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where(clause.Eq{Column: clause.Column{Table: "abilities", Name: "enabled"}, Value: true}).
		Where("channels.status = ?", common.ChannelStatusEnabled).
		Where(clause.IN{Column: clause.Column{Table: "abilities", Name: "group"}, Values: stringSliceToAny(groups)})
	if token.ChannelLimitsEnabled {
		ids := token.GetChannelLimitIDs()
		if len(ids) == 0 {
			return options, nil
		}
		query = query.Where("abilities.channel_id IN ?", ids)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	byName := map[string]*tokenModelOption{}
	limits := token.GetModelLimitsMap()
	acceptUnset := shouldAcceptUnsetRatioModel(userID)
	for _, row := range rows {
		if token.ModelLimitsEnabled && !limits[row.Model] && !limits[ratio_setting.FormatMatchingModelName(row.Model)] {
			continue
		}
		if !acceptUnset && !hasConfiguredModelPrice(row.Model) {
			continue
		}
		if config != nil && !common.StringsContains(config.GroupsForModel(row.Model), row.Group) {
			continue
		}
		option := byName[row.Model]
		if option == nil {
			option = &tokenModelOption{Name: row.Model, Groups: []string{}, SupportedEndpointTypes: []constant.EndpointType{}}
			byName[row.Model] = option
		}
		if !common.StringsContains(option.Groups, row.Group) {
			option.Groups = append(option.Groups, row.Group)
		}
		for _, endpoint := range common.GetEndpointTypesByChannelType(row.ChannelType, row.Model) {
			if !containsEndpointType(option.SupportedEndpointTypes, endpoint) {
				option.SupportedEndpointTypes = append(option.SupportedEndpointTypes, endpoint)
			}
		}
	}
	for _, option := range byName {
		sort.Strings(option.Groups)
		option.SupportedEndpointTypes = normalizeTokenModelSupportedEndpointTypes(option.Name, option.SupportedEndpointTypes)
		options = append(options, *option)
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Name < options[j].Name })
	return options, nil
}

// A test group is a narrowing operation, never a grant. Keep the same token ID,
// key, quota and channel limits so normal billing still applies.
func scopedTokenForTest(userID int, token *model.Token, requestedGroup string) (*model.Token, error) {
	group := strings.TrimSpace(requestedGroup)
	if group == "" && token.RoutingConfig == "" && token.Group != "auto" {
		group = token.Group
	}
	if group == "" || group == "*" {
		return token, nil
	}
	groups, err := tokenCatalogGroups(userID, token)
	if err != nil {
		return nil, err
	}
	if !common.StringsContains(groups, group) {
		return nil, fmt.Errorf("无权使用该令牌测试 %s 分组", group)
	}
	options, err := tokenRoutingCatalog(userID, token)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, option := range options {
		if common.StringsContains(option.Groups, group) {
			names = append(names, option.Name)
		}
	}
	scoped := *token
	scoped.Group, scoped.AutoGroupFallback, scoped.CrossGroupRetry = group, false, false
	// Retain an explicit one-group boundary for affinity and specified channels.
	config, err := common.Marshal(model.TokenRoutingConfig{Groups: []string{group}})
	if err != nil {
		return nil, err
	}
	scoped.RoutingConfig = string(config)
	scoped.ModelLimitsEnabled, scoped.ModelLimits = true, strings.Join(names, ",")
	return &scoped, nil
}
