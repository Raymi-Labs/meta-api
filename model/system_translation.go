package model

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const SystemTranslationModel = "qwen-mt-flash"
const SystemTranslationTokenOption = "ModelDescriptionTranslationTokenId"

// Configuration contains an ID, never a second copy of an API credential.
func SystemTranslationTokenID() int {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[SystemTranslationTokenOption]
	common.OptionMapRWMutex.RUnlock()
	id, _ := strconv.Atoi(raw)
	return max(id, 0)
}

// ownerID is required when binding; background work revalidates the current root
// owner from the database, independently of token/user caches used by the relay.
func ValidateSystemTranslationToken(ctx context.Context, id, ownerID int) (*Token, error) {
	if id <= 0 {
		return nil, errors.New("未配置翻译专用令牌")
	}
	var token Token
	if err := DB.WithContext(ctx).First(&token, id).Error; err != nil {
		return nil, errors.New("翻译专用令牌不存在或已删除")
	}
	if ownerID > 0 && token.UserId != ownerID {
		return nil, errors.New("只能绑定当前超级管理员自己的令牌")
	}
	var owner User
	if err := DB.WithContext(ctx).Select("id", "role", "status").First(&owner, token.UserId).Error; err != nil || owner.Role != common.RoleRootUser || owner.Status != common.UserStatusEnabled {
		return nil, errors.New("翻译令牌必须属于有效的超级管理员")
	}
	if !token.ModelLimitsEnabled || strings.TrimSpace(token.ModelLimits) != SystemTranslationModel || (strings.TrimSpace(token.RoutingConfig) != "" && strings.TrimSpace(token.RoutingConfig) != "null") {
		return nil, errors.New("翻译专用令牌须仅允许 qwen-mt-flash，且不配置多模型路由")
	}
	if token.Status != common.TokenStatusEnabled || (token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix()) {
		return nil, errors.New("翻译专用令牌已停用或过期")
	}
	if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		return nil, errors.New("翻译专用令牌额度不足")
	}
	if token.Key == "" {
		return nil, errors.New("翻译专用令牌无效")
	}
	return &token, nil
}
