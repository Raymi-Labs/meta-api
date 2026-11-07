package controller

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
)

const publicTokenStatsCacheTTL = 30 * time.Second

type publicTokenStatsSnapshot struct {
	Stat              model.Stat                          `json:"stat"`
	TokenUsage        model.TokenUsageTokens              `json:"token_usage"`
	TokenDistribution model.PublicTokenDistribution       `json:"token_distribution"`
	RequestCount      int64                               `json:"request_count"`
	ModelStats        []model.ModelStat                   `json:"model_stats"`
	PerToken          map[int]publicTokenPerTokenSnapshot `json:"per_token"`
}

type publicTokenPerTokenSnapshot struct {
	Stat              model.Stat                    `json:"stat"`
	TokenUsage        model.TokenUsageTokens        `json:"token_usage"`
	TokenDistribution model.PublicTokenDistribution `json:"token_distribution"`
	RequestCount      int64                         `json:"request_count"`
	ModelStats        []model.ModelStat             `json:"model_stats"`
}

var (
	publicTokenStatsCacheOnce sync.Once
	publicTokenStatsCache     *cachex.HybridCache[publicTokenStatsSnapshot]
)

func getPublicTokenStatsCache() *cachex.HybridCache[publicTokenStatsSnapshot] {
	publicTokenStatsCacheOnce.Do(func() {
		publicTokenStatsCache = cachex.NewHybridCache[publicTokenStatsSnapshot](cachex.HybridCacheConfig[publicTokenStatsSnapshot]{
			Namespace:  cachex.Namespace("public_token_stats"),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[publicTokenStatsSnapshot]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled
			},
			Memory: func() *hot.HotCache[string, publicTokenStatsSnapshot] {
				return hot.NewHotCache[string, publicTokenStatsSnapshot](hot.LRU, 256).Build()
			},
		})
	})
	return publicTokenStatsCache
}

func buildPublicTokenStatsCacheKey(tokenIDs []int, startTime int64, endTime int64) string {
	sorted := append([]int(nil), tokenIDs...)
	sort.Ints(sorted)
	parts := make([]string, 0, len(sorted))
	for _, tokenID := range sorted {
		parts = append(parts, strconv.Itoa(tokenID))
	}
	return fmt.Sprintf("%s:%d:%d", strings.Join(parts, ","), startTime, endTime)
}

func loadPublicTokenStatsSnapshot(tokenIDs []int, startTime int64, endTime int64) publicTokenStatsSnapshot {
	cacheKey := buildPublicTokenStatsCacheKey(tokenIDs, startTime, endTime)
	if cached, found, err := getPublicTokenStatsCache().Get(cacheKey); err == nil && found {
		return cached
	}

	snapshot := publicTokenStatsSnapshot{
		PerToken: make(map[int]publicTokenPerTokenSnapshot, len(tokenIDs)),
	}
	if stat, err := model.SumUsedQuotaByTokenIDs(tokenIDs, startTime, endTime); err == nil {
		snapshot.Stat = stat
	}
	if tokenUsage, err := model.SumUsedTokenDetailsByTokenIDs(tokenIDs, startTime, endTime); err == nil {
		snapshot.TokenUsage = tokenUsage
	}
	if tokenDistribution, err := model.SumPublicTokenDistributionByTokenIDs(tokenIDs, startTime, endTime); err == nil {
		snapshot.TokenDistribution = tokenDistribution
	}
	if requestCount, err := model.CountLogsByTokenIDs(tokenIDs, startTime, endTime); err == nil {
		snapshot.RequestCount = requestCount
	}
	if modelStats, err := model.GetModelStatsByTokenIDs(tokenIDs, startTime, endTime); err == nil {
		snapshot.ModelStats = modelStats
	}

	perTokenStats, _ := model.SumUsedQuotaByTokenIDsMap(tokenIDs, startTime, endTime)
	perTokenUsage, _ := model.SumUsedTokenDetailsByTokenIDsMap(tokenIDs, startTime, endTime)
	perTokenDistribution, _ := model.SumPublicTokenDistributionByTokenIDsMap(tokenIDs, startTime, endTime)
	perTokenCounts, _ := model.CountLogsByTokenIDsMap(tokenIDs, startTime, endTime)
	perTokenModelStats, _ := model.GetModelStatsByTokenIDsMap(tokenIDs, startTime, endTime)

	for _, tokenID := range tokenIDs {
		snapshot.PerToken[tokenID] = publicTokenPerTokenSnapshot{
			Stat:              perTokenStats[tokenID],
			TokenUsage:        perTokenUsage[tokenID],
			TokenDistribution: perTokenDistribution[tokenID],
			RequestCount:      perTokenCounts[tokenID],
			ModelStats:        perTokenModelStats[tokenID],
		}
	}

	_ = getPublicTokenStatsCache().SetWithTTL(cacheKey, snapshot, publicTokenStatsCacheTTL)
	return snapshot
}

func buildMaskedTokenResponse(token *model.Token) *model.Token {
	if token == nil {
		return nil
	}
	maskedToken := *token
	maskedToken.Key = token.GetMaskedKey()
	return &maskedToken
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	group := strings.TrimSpace(c.Query("group"))
	balanceMin, balanceMax, usedBalanceMin, usedBalanceMax, packageMode, sortBy, sortOrder, err := parseTokenListQuery(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo := common.GetPageQuery(c)
	tokens, total, err := model.GetAllUserTokens(
		userId,
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
		group,
		balanceMin,
		balanceMax,
		usedBalanceMin,
		usedBalanceMax,
		packageMode,
		sortBy,
		sortOrder,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildTokenListItems(userId, tokens))
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")
	group := strings.TrimSpace(c.Query("group"))
	balanceMin, balanceMax, usedBalanceMin, usedBalanceMax, packageMode, sortBy, sortOrder, err := parseTokenListQuery(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(
		userId,
		keyword,
		token,
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
		group,
		balanceMin,
		balanceMax,
		usedBalanceMin,
		usedBalanceMax,
		packageMode,
		sortBy,
		sortOrder,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildTokenListItems(userId, tokens))
	common.ApiSuccess(c, pageInfo)
	return
}

func parseTokenListQuery(c *gin.Context) (*int, *int, *int, *int, string, string, string, error) {
	// 统一解析令牌列表筛选：剩余余额区间 + 已使用余额区间 + 排序（白名单）。
	// 该解析同时给 /api/token 和 /api/token/search 复用，避免两处行为不一致。
	balanceMinRaw := strings.TrimSpace(c.Query("balance_min"))
	balanceMaxRaw := strings.TrimSpace(c.Query("balance_max"))
	usedBalanceMinRaw := strings.TrimSpace(c.Query("used_balance_min"))
	usedBalanceMaxRaw := strings.TrimSpace(c.Query("used_balance_max"))

	var balanceMin *int
	if balanceMinRaw != "" {
		value, err := strconv.Atoi(balanceMinRaw)
		if err != nil {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 balance_min 不是有效整数")
		}
		if value < 0 {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 balance_min 不能小于 0")
		}
		balanceMin = &value
	}

	var balanceMax *int
	if balanceMaxRaw != "" {
		value, err := strconv.Atoi(balanceMaxRaw)
		if err != nil {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 balance_max 不是有效整数")
		}
		if value < 0 {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 balance_max 不能小于 0")
		}
		balanceMax = &value
	}

	if balanceMin != nil && balanceMax != nil && *balanceMin > *balanceMax {
		return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 balance_min 不能大于 balance_max")
	}

	var usedBalanceMin *int
	if usedBalanceMinRaw != "" {
		value, err := strconv.Atoi(usedBalanceMinRaw)
		if err != nil {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 used_balance_min 不是有效整数")
		}
		if value < 0 {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 used_balance_min 不能小于 0")
		}
		usedBalanceMin = &value
	}

	var usedBalanceMax *int
	if usedBalanceMaxRaw != "" {
		value, err := strconv.Atoi(usedBalanceMaxRaw)
		if err != nil {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 used_balance_max 不是有效整数")
		}
		if value < 0 {
			return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 used_balance_max 不能小于 0")
		}
		usedBalanceMax = &value
	}

	if usedBalanceMin != nil && usedBalanceMax != nil && *usedBalanceMin > *usedBalanceMax {
		return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 used_balance_min 不能大于 used_balance_max")
	}

	packageMode := strings.ToLower(strings.TrimSpace(c.Query("package_mode")))
	switch packageMode {
	case "", "package", "standard":
	default:
		return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 package_mode 仅支持 package 或 standard")
	}

	sortBy := strings.ToLower(strings.TrimSpace(c.Query("sort_by")))
	if sortBy == "" {
		sortBy = "id"
	}
	switch sortBy {
	case "id", "remain_quota":
	default:
		return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 sort_by 仅支持 id 或 remain_quota")
	}

	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sort_order")))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	switch sortOrder {
	case "asc", "desc":
	default:
		return nil, nil, nil, nil, "", "", "", fmt.Errorf("参数 sort_order 仅支持 asc 或 desc")
	}

	return balanceMin, balanceMax, usedBalanceMin, usedBalanceMax, packageMode, sortBy, sortOrder, nil
}

func validateTokenGroupSelection(userID int, group string) error {
	group = strings.TrimSpace(group)
	if group == "" {
		return fmt.Errorf("令牌分组不能为空")
	}

	userGroup, err := model.GetUserGroup(userID, false)
	if err != nil {
		return err
	}
	if !service.GroupInUserUsableGroups(userGroup, group) && group != userGroup {
		return fmt.Errorf("无权访问 %s 分组", group)
	}
	if group != "auto" && !ratio_setting.ContainsGroupRatio(group) && group != userGroup {
		return fmt.Errorf("分组 %s 已被弃用", group)
	}
	return nil
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(token),
	})
	return
}

func GetTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	tokenID := c.GetInt("token_id")
	if tokenID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid token context",
		})
		return
	}
	token, err := model.GetTokenById(tokenID)
	if err != nil {
		common.SysError("failed to get token by id: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":                  "token_usage",
			"name":                    token.Name,
			"total_granted":           token.RemainQuota + token.UsedQuota,
			"total_used":              token.UsedQuota,
			"total_available":         token.RemainQuota,
			"unlimited_quota":         token.UnlimitedQuota,
			"model_limits":            token.GetModelLimitsMap(),
			"model_limits_enabled":    token.ModelLimitsEnabled,
			"max_concurrency":         token.MaxConcurrency,
			"window_request_limit":    token.WindowRequestLimit,
			"window_seconds":          token.WindowSeconds,
			"package_enabled":         token.PackageEnabled,
			"package_limit_quota":     token.PackageLimitQuota,
			"package_period":          token.PackagePeriod,
			"package_custom_seconds":  token.PackageCustomSeconds,
			"package_period_mode":     token.PackagePeriodMode,
			"package_used_quota":      token.PackageUsedQuota,
			"package_next_reset_time": token.PackageNextResetTime,
			"expires_at":              expiredAt,
		},
	})
}

func isPublicTokenUsageEnabled() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	raw, ok := common.OptionMap["HeaderNavModules"]
	if !ok || raw == "" {
		return false
	}
	var modules map[string]interface{}
	if err := common.Unmarshal([]byte(raw), &modules); err != nil {
		return false
	}
	val, ok := modules["usage"]
	if !ok {
		return false
	}
	enabled, ok := val.(bool)
	return ok && enabled
}

func normalizePublicQueryKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		cleaned, _ := common.ParseTokenKey(key)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	return normalized
}

func maskPublicTokenKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func getPublicTokenStatsRange(period string) (int64, int64) {
	now := time.Now()
	var startTime int64
	switch period {
	case "week":
		startTime = now.AddDate(0, 0, -7).Unix()
	case "month":
		startTime = now.AddDate(0, -1, 0).Unix()
	default:
		y, m, d := now.Date()
		startTime = time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
	}
	return startTime, now.Unix()
}

func parsePublicTokenLogPagination(page int, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func resolvePublicSingleToken(rawKey string) (*model.Token, error) {
	key, _ := common.ParseTokenKey(rawKey)
	if key == "" {
		return nil, fmt.Errorf("empty key")
	}
	token, err := model.ValidateUserToken(key)
	if err != nil {
		return nil, err
	}
	return model.NormalizeTokenPackageStateForRead(token)
}

func buildPublicTokenUsagePayload(token *model.Token) gin.H {
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	payload := gin.H{
		"token_id":                token.Id,
		"masked_key":              maskPublicTokenKey(token.Key),
		"name":                    token.Name,
		"status":                  token.Status,
		"group":                   token.Group,
		"max_concurrency":         token.MaxConcurrency,
		"window_request_limit":    token.WindowRequestLimit,
		"window_seconds":          token.WindowSeconds,
		"total_granted":           token.RemainQuota + token.UsedQuota,
		"total_used":              token.UsedQuota,
		"total_available":         token.RemainQuota,
		"unlimited_quota":         token.UnlimitedQuota,
		"model_limits_enabled":    token.ModelLimitsEnabled,
		"model_limits":            token.GetModelLimitsMap(),
		"package_enabled":         token.PackageEnabled,
		"package_limit_quota":     token.PackageLimitQuota,
		"package_period":          token.PackagePeriod,
		"package_custom_seconds":  token.PackageCustomSeconds,
		"package_period_mode":     token.PackagePeriodMode,
		"package_used_quota":      token.PackageUsedQuota,
		"package_next_reset_time": token.PackageNextResetTime,
		"expires_at":              expiredAt,
		"created_time":            token.CreatedTime,
		"accessed_time":           token.AccessedTime,
	}

	payload["runtime_status"] = queryTokenRuntimeStatus(token)
	return payload
}

func queryTokenRuntimeStatus(token *model.Token) gin.H {
	status := gin.H{}

	if token.MaxConcurrency > 0 {
		concurrency, err := middleware.QueryTokenConcurrency(token.Id)
		if err == nil {
			status["current_concurrency"] = concurrency
		}
	}

	if token.WindowRequestLimit > 0 && token.WindowSeconds > 0 {
		count, windowEndMs, serverNowMs, err := middleware.QueryTokenWindowStatus(token.Id, token.WindowSeconds)
		if err == nil {
			status["window_used"] = count
			status["window_end_ms"] = windowEndMs
			status["server_now_ms"] = serverNowMs
		}
	}

	return status
}

// GetTokensRuntimeStatus returns real-time runtime status for a batch of token IDs.
func GetTokensRuntimeStatus(c *gin.Context) {
	var req struct {
		TokenIDs []int `json:"token_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.TokenIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请提供有效的 token_ids",
		})
		return
	}

	if len(req.TokenIDs) > 100 {
		req.TokenIDs = req.TokenIDs[:100]
	}

	userID := c.GetInt("id")
	results := make(map[string]gin.H, len(req.TokenIDs))

	for _, tokenID := range req.TokenIDs {
		token, err := model.GetTokenById(tokenID)
		if err != nil || token == nil || token.UserId != userID {
			continue
		}
		results[strconv.Itoa(tokenID)] = queryTokenRuntimeStatus(token)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
	})
}

func resolvePublicQueryTokens(keys []string) ([]*model.Token, []string) {
	normalized := normalizePublicQueryKeys(keys)
	tokens := make([]*model.Token, 0, len(normalized))
	invalid := make([]string, 0)
	for _, key := range normalized {
		token, err := model.ValidateUserToken(key)
		if err != nil || token == nil {
			invalid = append(invalid, key)
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens, invalid
}

func buildBatchUsageSummary(tokens []*model.Token, invalid []string) gin.H {
	totalGranted := 0
	totalUsed := 0
	totalAvailable := 0
	for _, token := range tokens {
		totalGranted += token.RemainQuota + token.UsedQuota
		totalUsed += token.UsedQuota
		totalAvailable += token.RemainQuota
	}

	return gin.H{
		"valid_key_count":   len(tokens),
		"invalid_key_count": len(invalid),
		"total_granted":     totalGranted,
		"total_used":        totalUsed,
		"total_available":   totalAvailable,
	}
}

func GetPublicTokenUsage(c *gin.Context) {
	if !isPublicTokenUsageEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "公开令牌查询功能未启用",
		})
		return
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请提供有效的 API Key",
		})
		return
	}

	token, err := resolvePublicSingleToken(req.Key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 API Key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "ok",
		"data":    buildPublicTokenUsagePayload(token),
	})
}

func GetPublicTokenBatchUsage(c *gin.Context) {
	if !isPublicTokenUsageEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "公开令牌查询功能未启用",
		})
		return
	}

	var req struct {
		Keys []string `json:"keys"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请提供有效的 API Key",
		})
		return
	}

	tokens, invalid := resolvePublicQueryTokens(req.Keys)
	if len(tokens) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 API Key",
		})
		return
	}

	items := make([]gin.H, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, buildPublicTokenUsagePayload(token))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "ok",
		"data": gin.H{
			"summary":      buildBatchUsageSummary(tokens, invalid),
			"tokens":       items,
			"invalid_keys": invalid,
		},
	})
}

func AddToken(c *gin.Context) {
	token := model.Token{}
	err := c.ShouldBindJSON(&token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if err := validateTokenGroupSelection(c.GetInt("id"), token.Group); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateTokenRoutingForSave(c.GetInt("id"), &token); err != nil {
		common.ApiError(c, err)
		return
	}
	channelLimitsEnabled, channelLimits, err := normalizeTokenChannelLimitsForToken(c.GetInt("id"), c.GetInt("role"), &token, "")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token.ChannelLimitsEnabled = channelLimitsEnabled
	token.ChannelLimits = channelLimits
	if err := model.ValidateTokenPackageConfig(&token); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.ValidateTokenRuntimeLimitConfig(&token); err != nil {
		common.ApiError(c, err)
		return
	}
	// 非无限额度时，检查额度值是否超出有效范围
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:               c.GetInt("id"),
		Name:                 token.Name,
		Key:                  key,
		CreatedTime:          common.GetTimestamp(),
		AccessedTime:         common.GetTimestamp(),
		ExpiredTime:          token.ExpiredTime,
		RemainQuota:          token.RemainQuota,
		UnlimitedQuota:       token.UnlimitedQuota,
		ModelLimitsEnabled:   token.ModelLimitsEnabled,
		ModelLimits:          token.ModelLimits,
		ChannelLimitsEnabled: token.ChannelLimitsEnabled,
		ChannelLimits:        token.ChannelLimits,
		AllowIps:             token.AllowIps,
		Group:                token.Group,
		CrossGroupRetry:      token.CrossGroupRetry,
		AutoGroupFallback:    token.AutoGroupFallback,
		RoutingConfig:        token.RoutingConfig,
		MaxConcurrency:       token.MaxConcurrency,
		WindowRequestLimit:   token.WindowRequestLimit,
		WindowSeconds:        token.WindowSeconds,
		PackageEnabled:       token.PackageEnabled,
		PackageLimitQuota:    token.PackageLimitQuota,
		PackagePeriod:        token.PackagePeriod,
		PackageCustomSeconds: token.PackageCustomSeconds,
		PackagePeriodMode:    token.PackagePeriodMode,
		PackageUsedQuota:     token.PackageUsedQuota,
		PackageNextResetTime: token.PackageNextResetTime,
	}
	if err := model.ValidateTokenQuotaPackageRelation(&cleanToken); err != nil {
		common.ApiError(c, err)
		return
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	payload := struct {
		model.Token
		RoutingConfig *string `json:"routing_config"`
	}{}
	err := c.ShouldBindJSON(&payload)
	token := payload.Token
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	cleanToken, err := model.GetTokenByIds(token.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		token.RoutingConfig = cleanToken.RoutingConfig
		if payload.RoutingConfig != nil {
			token.RoutingConfig = *payload.RoutingConfig
		}
		token.SourceType = cleanToken.SourceType
		if cleanToken.SourceType == model.TokenSourceTypeSellableToken {
			token.RoutingConfig = cleanToken.RoutingConfig
		}
		if err := validateTokenRoutingForSave(userId, &token); err != nil {
			common.ApiError(c, err)
			return
		}
		if err := validateTokenGroupSelection(userId, token.Group); err != nil {
			common.ApiError(c, err)
			return
		}
		channelLimitsEnabled, channelLimits, err := normalizeTokenChannelLimitsForToken(userId, c.GetInt("role"), &token, cleanToken.ChannelLimits)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		token.ChannelLimitsEnabled = channelLimitsEnabled
		token.ChannelLimits = channelLimits
	}
	if err := model.ValidateTokenPackageConfig(&token); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.ValidateTokenRuntimeLimitConfig(&token); err != nil {
		common.ApiError(c, err)
		return
	}
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	if token.Status == common.TokenStatusEnabled {
		cleanToken, err = model.ValidateTokenCanEnable(cleanToken)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrTokenCannotEnableExpired):
				common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			case errors.Is(err, model.ErrTokenCannotEnableExhausted):
				common.ApiErrorI18n(c, i18n.MsgTokenExhaustedCannotEable)
			default:
				common.ApiErrorMsg(c, err.Error())
			}
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		cleanToken.Name = token.Name
		cleanToken.Group = token.Group
		if cleanToken.SourceType == model.TokenSourceTypeSellableToken {
			// 可售令牌仅允许修改名称和分组，其余额度/限制字段保持商品下发时的只读状态。
			if len(cleanToken.Name) > 50 {
				common.ApiErrorMsg(c, "令牌名称不能超过 50 个字符")
				return
			}
		} else {
			// If you add more fields, please also update token.Update()
			cleanToken.ExpiredTime = token.ExpiredTime
			cleanToken.RemainQuota = token.RemainQuota
			cleanToken.UnlimitedQuota = token.UnlimitedQuota
			cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
			cleanToken.ModelLimits = token.ModelLimits
			cleanToken.ChannelLimitsEnabled = token.ChannelLimitsEnabled
			cleanToken.ChannelLimits = token.ChannelLimits
			cleanToken.AllowIps = token.AllowIps
			cleanToken.CrossGroupRetry = token.CrossGroupRetry
			cleanToken.AutoGroupFallback = token.AutoGroupFallback
			cleanToken.RoutingConfig = token.RoutingConfig
			cleanToken.MaxConcurrency = token.MaxConcurrency
			cleanToken.WindowRequestLimit = token.WindowRequestLimit
			cleanToken.WindowSeconds = token.WindowSeconds
			cleanToken.PackageEnabled = token.PackageEnabled
			cleanToken.PackageLimitQuota = token.PackageLimitQuota
			cleanToken.PackagePeriod = token.PackagePeriod
			cleanToken.PackageCustomSeconds = token.PackageCustomSeconds
			cleanToken.PackagePeriodMode = token.PackagePeriodMode
			// 允许管理员/用户在编辑时手动归零该周期已用额度，便于紧急解锁
			cleanToken.PackageUsedQuota = token.PackageUsedQuota
			cleanToken.PackageNextResetTime = token.PackageNextResetTime
			if err := model.ValidateTokenQuotaPackageRelation(cleanToken); err != nil {
				common.ApiError(c, err)
				return
			}
		}
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	})
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

type TokenBatchManageRequest struct {
	Ids    []int  `json:"ids"`
	Action string `json:"action"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

// ManageTokenBatch 处理令牌批量启用、禁用和删除。
// 这里按“部分成功”返回结果，单条失败不会中断整批请求。
func ManageTokenBatch(c *gin.Context) {
	req := TokenBatchManageRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "enable", "disable", "delete":
	default:
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	userId := c.GetInt("id")
	seen := make(map[int]struct{}, len(req.Ids))
	updated := make([]gin.H, 0, len(req.Ids))
	failed := make([]gin.H, 0)

	for _, id := range req.Ids {
		// 输入兜底：非法 ID 直接记失败，避免无效查询。
		if id <= 0 {
			failed = append(failed, gin.H{
				"id":      id,
				"message": "令牌 ID 无效",
			})
			continue
		}
		// 同一请求内去重，确保一个令牌最多处理一次。
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		// 只处理当前用户自己的令牌，避免跨用户操作。
		token, err := model.GetTokenByIds(id, userId)
		if err != nil || token == nil || token.Id == 0 {
			failed = append(failed, gin.H{
				"id":      id,
				"message": "令牌不存在",
			})
			continue
		}

		switch action {
		case "enable":
			token, err = model.ValidateTokenCanEnable(token)
			if err != nil {
				failed = append(failed, gin.H{
					"id":      id,
					"message": err.Error(),
				})
				continue
			}
			token.Status = common.TokenStatusEnabled
			if err := token.Update(); err != nil {
				failed = append(failed, gin.H{
					"id":      id,
					"message": err.Error(),
				})
				continue
			}
		case "disable":
			token.Status = common.TokenStatusDisabled
			if err := token.Update(); err != nil {
				failed = append(failed, gin.H{
					"id":      id,
					"message": err.Error(),
				})
				continue
			}
		case "delete":
			if err := token.Delete(); err != nil {
				failed = append(failed, gin.H{
					"id":      id,
					"message": err.Error(),
				})
				continue
			}
		}

		// 成功项返回最小必要字段，便于前端做局部刷新或统计。
		updated = append(updated, gin.H{
			"id":     token.Id,
			"status": token.Status,
		})
	}

	common.ApiSuccess(c, gin.H{
		"success_count": len(updated),
		"failed_count":  len(failed),
		"updated":       updated,
		"failed":        failed,
	})
}

func GetPublicTokenStats(c *gin.Context) {
	if !isPublicTokenUsageEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "公开令牌查询功能未启用",
		})
		return
	}

	var req struct {
		Key    string `json:"key"`
		Period string `json:"period"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请提供有效的 API Key",
		})
		return
	}

	token, err := resolvePublicSingleToken(req.Key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 API Key",
		})
		return
	}

	startTime, endTime := getPublicTokenStatsRange(req.Period)
	tokenIDs := []int{token.Id}
	snapshot := loadPublicTokenStatsSnapshot(tokenIDs, startTime, endTime)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"period_quota":         snapshot.Stat.Quota,
			"period_request_count": snapshot.RequestCount,
			"period_token_count":   snapshot.TokenUsage.PromptTokens + snapshot.TokenUsage.CompletionTokens,
			"prompt_tokens":        snapshot.TokenUsage.PromptTokens,
			"completion_tokens":    snapshot.TokenUsage.CompletionTokens,
			"total_tokens":         snapshot.TokenUsage.PromptTokens + snapshot.TokenUsage.CompletionTokens,
			"token_distribution":   snapshot.TokenDistribution,
			"rpm":                  snapshot.Stat.Rpm,
			"tpm":                  snapshot.Stat.Tpm,
			"model_stats":          snapshot.ModelStats,
		},
	})
}

func GetPublicTokenBatchStats(c *gin.Context) {
	if !isPublicTokenUsageEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "公开令牌查询功能未启用",
		})
		return
	}

	var req struct {
		Keys   []string `json:"keys"`
		Period string   `json:"period"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请提供有效的 API Key",
		})
		return
	}

	tokens, invalid := resolvePublicQueryTokens(req.Keys)
	if len(tokens) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 API Key",
		})
		return
	}

	startTime, endTime := getPublicTokenStatsRange(req.Period)
	tokenIDs := make([]int, 0, len(tokens))
	for _, token := range tokens {
		tokenIDs = append(tokenIDs, token.Id)
	}

	snapshot := loadPublicTokenStatsSnapshot(tokenIDs, startTime, endTime)
	batchSummary := buildBatchUsageSummary(tokens, invalid)

	keyStats := make([]gin.H, 0, len(tokens))
	for _, token := range tokens {
		keySnapshot := snapshot.PerToken[token.Id]
		item := buildPublicTokenUsagePayload(token)
		item["period_quota"] = keySnapshot.Stat.Quota
		item["period_request_count"] = keySnapshot.RequestCount
		item["period_token_count"] = keySnapshot.TokenUsage.PromptTokens + keySnapshot.TokenUsage.CompletionTokens
		item["prompt_tokens"] = keySnapshot.TokenUsage.PromptTokens
		item["completion_tokens"] = keySnapshot.TokenUsage.CompletionTokens
		item["total_tokens"] = keySnapshot.TokenUsage.PromptTokens + keySnapshot.TokenUsage.CompletionTokens
		item["token_distribution"] = keySnapshot.TokenDistribution
		item["model_stats"] = keySnapshot.ModelStats
		keyStats = append(keyStats, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"summary": gin.H{
				"valid_key_count":      len(tokens),
				"invalid_key_count":    len(invalid),
				"total_granted":        batchSummary["total_granted"],
				"total_used":           batchSummary["total_used"],
				"total_available":      batchSummary["total_available"],
				"period_quota":         snapshot.Stat.Quota,
				"period_request_count": snapshot.RequestCount,
				"prompt_tokens":        snapshot.TokenUsage.PromptTokens,
				"completion_tokens":    snapshot.TokenUsage.CompletionTokens,
				"total_tokens":         snapshot.TokenUsage.PromptTokens + snapshot.TokenUsage.CompletionTokens,
				"token_distribution":   snapshot.TokenDistribution,
				"rpm":                  snapshot.Stat.Rpm,
				"tpm":                  snapshot.Stat.Tpm,
			},
			"token_distribution": snapshot.TokenDistribution,
			"model_stats":        snapshot.ModelStats,
			"key_stats":          keyStats,
			"invalid_keys":       invalid,
		},
	})
}

func GetPublicTokenLogs(c *gin.Context) {
	if !isPublicTokenUsageEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "公开令牌查询功能未启用",
		})
		return
	}

	var req struct {
		Key      string `json:"key"`
		Period   string `json:"period"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Key) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请提供有效的 API Key",
		})
		return
	}

	token, err := resolvePublicSingleToken(req.Key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 API Key",
		})
		return
	}

	page, pageSize := parsePublicTokenLogPagination(req.Page, req.PageSize)
	startTime, endTime := getPublicTokenStatsRange(req.Period)
	items, total, err := model.GetPublicTokenLogsByTokenIDs([]int{token.Id}, startTime, endTime, (page-1)*pageSize, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}
