package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type tokenChannelOption struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Tag           string   `json:"tag"`
	Type          int      `json:"type"`
	Status        int      `json:"status"`
	BaseURL       string   `json:"base_url"`
	Available     bool     `json:"available"`
	MatchedGroups []string `json:"matched_groups"`
	MatchedModels []string `json:"matched_models"`
}

type tokenChannelCandidateRow struct {
	ChannelID int     `json:"channel_id"`
	Name      string  `json:"name"`
	Tag       *string `json:"tag"`
	Type      int     `json:"type"`
	Status    int     `json:"status"`
	BaseURL   *string `json:"base_url"`
	GroupName string  `json:"ability_group" gorm:"column:ability_group"`
	Model     string  `json:"model"`
}

type tokenChannelMetaRow struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Tag     *string `json:"tag"`
	Type    int     `json:"type"`
	Status  int     `json:"status"`
	BaseURL *string `json:"base_url"`
}

func normalizeChannelLimitIDs(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return []int{}
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func serializeChannelLimitIDs(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		parts = append(parts, strconv.Itoa(id))
	}
	return strings.Join(parts, ",")
}

func resolveRequestedTokenChannelModels(userID int, requestedGroup string, modelLimits string, modelLimitsEnabled bool, fallback ...bool) ([]string, error) {
	models, err := resolveRequestedTokenModels(userID, requestedGroup, fallback...)
	if err != nil {
		return nil, err
	}
	if !modelLimitsEnabled || strings.TrimSpace(modelLimits) == "" {
		return models, nil
	}

	limitSet := make(map[string]bool)
	for _, item := range strings.Split(modelLimits, ",") {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		limitSet[name] = true
		normalized := ratio_setting.FormatMatchingModelName(name)
		if normalized != "" {
			limitSet[normalized] = true
		}
	}

	filtered := make([]string, 0, len(models))
	for _, modelName := range models {
		normalized := ratio_setting.FormatMatchingModelName(modelName)
		if limitSet[modelName] || (normalized != "" && limitSet[normalized]) {
			filtered = append(filtered, modelName)
		}
	}
	return filtered, nil
}

func buildTokenChannelOptions(userID int, requestedGroup string, modelLimits string, modelLimitsEnabled bool, fallback ...bool) ([]tokenChannelOption, error) {
	groups, err := resolveRequestedTokenGroups(userID, requestedGroup, fallback...)
	if err != nil {
		return nil, err
	}
	models, err := resolveRequestedTokenChannelModels(userID, requestedGroup, modelLimits, modelLimitsEnabled, fallback...)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 || len(models) == 0 {
		return []tokenChannelOption{}, nil
	}

	rows := make([]tokenChannelCandidateRow, 0)
	groupSelect := "abilities.`group` as ability_group"
	if common.UsingPostgreSQL {
		groupSelect = `abilities."group" as ability_group`
	}
	query := model.DB.Table("abilities").
		Select("channels.id as channel_id, channels.name, channels.tag, channels.type, channels.status, channels.base_url, " + groupSelect + ", abilities.model").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where(clause.Eq{Column: clause.Column{Table: "abilities", Name: "enabled"}, Value: true}).
		Where(clause.Eq{Column: clause.Column{Table: "channels", Name: "status"}, Value: common.ChannelStatusEnabled}).
		Where(clause.IN{Column: clause.Column{Table: "abilities", Name: "group"}, Values: stringSliceToAny(groups)}).
		Where(clause.IN{Column: clause.Column{Table: "abilities", Name: "model"}, Values: stringSliceToAny(models)}).
		Order("channels.id desc")
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	optionsMap := make(map[int]*tokenChannelOption)
	groupSetByChannel := make(map[int]map[string]struct{})
	modelSetByChannel := make(map[int]map[string]struct{})

	for _, row := range rows {
		option, ok := optionsMap[row.ChannelID]
		if !ok {
			baseURL := ""
			if row.BaseURL != nil {
				baseURL = *row.BaseURL
			}
			tag := ""
			if row.Tag != nil {
				tag = strings.TrimSpace(*row.Tag)
			}
			option = &tokenChannelOption{
				ID:        row.ChannelID,
				Name:      row.Name,
				Tag:       tag,
				Type:      row.Type,
				Status:    row.Status,
				BaseURL:   baseURL,
				Available: true,
			}
			optionsMap[row.ChannelID] = option
			groupSetByChannel[row.ChannelID] = make(map[string]struct{})
			modelSetByChannel[row.ChannelID] = make(map[string]struct{})
		}
		groupSetByChannel[row.ChannelID][row.GroupName] = struct{}{}
		modelSetByChannel[row.ChannelID][row.Model] = struct{}{}
	}

	options := make([]tokenChannelOption, 0, len(optionsMap))
	for channelID, option := range optionsMap {
		option.MatchedGroups = sortedStringKeys(groupSetByChannel[channelID])
		option.MatchedModels = sortedStringKeys(modelSetByChannel[channelID])
		options = append(options, *option)
	}

	sort.Slice(options, func(i, j int) bool {
		if options[i].Name == options[j].Name {
			return options[i].ID > options[j].ID
		}
		return options[i].Name < options[j].Name
	})
	return options, nil
}

func sortedStringKeys(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

func mergePersistedTokenChannelOptions(options []tokenChannelOption, channelLimitIDs []int) ([]tokenChannelOption, error) {
	if len(channelLimitIDs) == 0 {
		return options, nil
	}

	optionMap := make(map[int]tokenChannelOption, len(options))
	for _, option := range options {
		optionMap[option.ID] = option
	}

	missingIDs := make([]int, 0, len(channelLimitIDs))
	for _, channelID := range channelLimitIDs {
		if _, ok := optionMap[channelID]; ok {
			continue
		}
		missingIDs = append(missingIDs, channelID)
	}
	if len(missingIDs) == 0 {
		return options, nil
	}

	rows := make([]tokenChannelMetaRow, 0, len(missingIDs))
	if err := model.DB.Table("channels").
		Select("id, name, tag, type, status, base_url").
		Where("id IN ?", missingIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		tag := ""
		if row.Tag != nil {
			tag = strings.TrimSpace(*row.Tag)
		}
		baseURL := ""
		if row.BaseURL != nil {
			baseURL = *row.BaseURL
		}
		optionMap[row.ID] = tokenChannelOption{
			ID:        row.ID,
			Name:      row.Name,
			Tag:       tag,
			Type:      row.Type,
			Status:    row.Status,
			BaseURL:   baseURL,
			Available: false,
		}
	}

	merged := make([]tokenChannelOption, 0, len(optionMap))
	for _, option := range optionMap {
		merged = append(merged, option)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Name == merged[j].Name {
			return merged[i].ID > merged[j].ID
		}
		return merged[i].Name < merged[j].Name
	})
	return merged, nil
}

func normalizeTokenChannelLimitsForSave(userID int, role int, group string, modelLimits string, modelLimitsEnabled bool, channelLimits string, channelLimitsEnabled bool, existingChannelLimits string, fallback ...bool) (bool, string, error) {
	normalizedIDs := normalizeChannelLimitIDs(channelLimits)
	if role < common.RoleAdminUser {
		if channelLimitsEnabled || len(normalizedIDs) > 0 || strings.TrimSpace(channelLimits) != "" {
			return false, "", fmt.Errorf("仅管理员可配置渠道限制")
		}
		return false, "", nil
	}
	if len(normalizedIDs) == 0 {
		return false, "", nil
	}

	options, err := buildTokenChannelOptions(userID, group, modelLimits, modelLimitsEnabled, fallback...)
	if err != nil {
		return false, "", err
	}
	allowedSet := make(map[int]struct{}, len(options))
	for _, option := range options {
		allowedSet[option.ID] = struct{}{}
	}
	existingSet := make(map[int]struct{})
	for _, channelID := range normalizeChannelLimitIDs(existingChannelLimits) {
		existingSet[channelID] = struct{}{}
	}
	for _, channelID := range normalizedIDs {
		if _, ok := allowedSet[channelID]; ok {
			continue
		}
		if _, ok := existingSet[channelID]; ok {
			continue
		}
		if _, ok := allowedSet[channelID]; !ok {
			return false, "", fmt.Errorf("渠道 %d 与当前令牌分组或模型限制不匹配", channelID)
		}
	}
	return true, serializeChannelLimitIDs(normalizedIDs), nil
}

func GetTokenChannels(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		common.ApiError(c, fmt.Errorf("无权进行此操作，权限不足"))
		return
	}

	userID := c.GetInt("id")
	group := strings.TrimSpace(c.Query("group"))
	modelLimits := strings.TrimSpace(c.Query("model_limits"))
	modelLimitsEnabled := modelLimits != ""
	fallbackRaw, fallbackSet := c.GetQuery("auto_group_fallback")
	fallback, _ := strconv.ParseBool(fallbackRaw)

	tokenIDRaw := strings.TrimSpace(c.Query("token_id"))
	tokenChannelLimitIDs := []int{}
	if tokenIDRaw != "" {
		tokenID, err := strconv.Atoi(tokenIDRaw)
		if err != nil || tokenID <= 0 {
			common.ApiError(c, fmt.Errorf("token_id 无效"))
			return
		}
		token, err := model.GetTokenByIds(tokenID, userID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if group == "" {
			group = token.Group
		}
		if modelLimits == "" {
			modelLimits = token.ModelLimits
			modelLimitsEnabled = token.ModelLimitsEnabled
		}
		if !fallbackSet {
			fallback = token.AllowsGroupFallback()
		}
		if token.SourceType == model.TokenSourceTypeSellableToken {
			fallback = false
		}
		tokenChannelLimitIDs = token.GetChannelLimitIDs()
	}

	options, err := buildTokenChannelOptions(userID, group, modelLimits, modelLimitsEnabled, fallback)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	options, err = mergePersistedTokenChannelOptions(options, tokenChannelLimitIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}
