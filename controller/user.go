package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type accountBindRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

var invalidateManagedUserCaches = model.InvalidateUserAndTokenCaches

func resolveInviterIDByAffCode(affCode string) int {
	affCode = strings.TrimSpace(affCode)
	if affCode == "" {
		return 0
	}
	inviterId, err := model.GetUserIdByAffCode(affCode)
	if err != nil {
		return 0
	}
	return inviterId
}

func Login(c *gin.Context) {
	if !common.PasswordLoginEnabled {
		common.ApiErrorI18n(c, i18n.MsgUserPasswordLoginDisabled)
		return
	}
	var loginRequest LoginRequest
	err := common.DecodeJson(c.Request.Body, &loginRequest)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	username := loginRequest.Username
	password := loginRequest.Password
	if username == "" || password == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	user := model.User{
		Username: username,
		Password: password,
	}
	err = user.ValidateAndFill()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}

	// 检查是否启用2FA；查询失败时按失败处理，避免数据库异常绕过二次验证。
	twoFAEnabled, err := model.IsTwoFAEnabled(user.Id)
	if err != nil {
		common.SysLog(fmt.Sprintf("Login failed to load 2FA status for user %d: %v", user.Id, err))
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	if twoFAEnabled {
		// 设置pending session，等待2FA验证
		session := sessions.Default(c)
		session.Set("pending_username", user.Username)
		session.Set("pending_user_id", user.Id)
		err := session.Save()
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgUserSessionSaveFailed)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": i18n.T(c, i18n.MsgUserRequire2FA),
			"success": true,
			"data": map[string]interface{}{
				"require_2fa": true,
			},
		})
		return
	}

	setupLogin(&user, c)
}

// setup session & cookies and then return user info
func setupLogin(user *model.User, c *gin.Context) {
	model.UpdateUserLastLoginAt(user.Id)
	session := sessions.Default(c)
	session.Set("id", user.Id)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	session.Set("status", user.Status)
	session.Set("group", user.Group)
	err := session.Save()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserSessionSaveFailed)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
		"data": map[string]any{
			"id":           user.Id,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"role":         user.Role,
			"status":       user.Status,
			"group":        user.Group,
		},
	})
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	err := session.Save()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
	})
}

func Register(c *gin.Context) {
	if !common.RegisterEnabled {
		common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
		return
	}
	if !common.PasswordRegisterEnabled {
		common.ApiErrorI18n(c, i18n.MsgUserPasswordRegisterDisabled)
		return
	}
	var user model.User
	err := common.DecodeJson(c.Request.Body, &user)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	user.Email = common.NormalizeEmailAddress(user.Email)
	if err := common.Validate.Struct(&user); err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserInputInvalid, map[string]any{"Error": common.TranslateValidationErrors(err)})
		return
	}
	if common.EmailVerificationEnabled {
		if user.Email == "" || user.VerificationCode == "" {
			common.ApiErrorI18n(c, i18n.MsgUserEmailVerificationRequired)
			return
		}
	}
	conflict, err := model.CheckUserRegisterConflict(user.Username, user.Email)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		common.SysLog(fmt.Sprintf("CheckUserRegisterConflict error: %v", err))
		return
	}
	switch conflict {
	case model.UserRegisterConflictUsername:
		common.ApiErrorI18n(c, i18n.MsgUserUsernameExists)
		return
	case model.UserRegisterConflictEmail:
		common.ApiErrorI18n(c, i18n.MsgUserEmailExists)
		return
	case model.UserRegisterConflictBoth:
		common.ApiErrorI18n(c, i18n.MsgUserUsernameAndEmailExists)
		return
	}
	if err := enforceUserRegisterRisk(c, &user); err != nil {
		if errors.Is(err, errRegisterRiskBlocked) {
			common.ApiError(c, err)
			return
		}
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		common.SysLog(fmt.Sprintf("enforceUserRegisterRisk error: %v", err))
		return
	}
	if common.EmailVerificationEnabled {
		if !common.VerifyCodeWithKey(user.Email, user.VerificationCode, common.EmailVerificationPurpose) {
			common.ApiErrorI18n(c, i18n.MsgUserVerificationCodeError)
			return
		}
		common.DeleteKey(user.Email, common.EmailVerificationPurpose)
	}
	affCode := user.AffCode // this code is the inviter's code, not the user's own code
	inviterId := resolveInviterIDByAffCode(affCode)
	cleanUser := model.User{
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.Username,
		InviterId:   inviterId,
		Role:        common.RoleCommonUser, // 明确设置角色为普通用户
	}
	applyUserRegisterAudit(c, &cleanUser, model.UserRegisterSourcePassword)
	if common.EmailVerificationEnabled {
		cleanUser.Email = user.Email
	}
	if err := cleanUser.Insert(inviterId); err != nil {
		common.ApiError(c, err)
		return
	}

	// 获取插入后的用户ID
	var insertedUser model.User
	if err := model.DB.Where("username = ?", cleanUser.Username).First(&insertedUser).Error; err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserRegisterFailed)
		return
	}
	// 生成默认令牌
	if constant.GenerateDefaultToken {
		key, err := common.GenerateKey()
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgUserDefaultTokenFailed)
			common.SysLog("failed to generate token key: " + err.Error())
			return
		}
		// 生成默认令牌
		token := model.Token{
			UserId:             insertedUser.Id, // 使用插入后的用户ID
			Name:               cleanUser.Username + "的初始令牌",
			Key:                key,
			CreatedTime:        common.GetTimestamp(),
			AccessedTime:       common.GetTimestamp(),
			ExpiredTime:        -1,     // 永不过期
			RemainQuota:        500000, // 示例额度
			UnlimitedQuota:     true,
			ModelLimitsEnabled: false,
		}
		if setting.DefaultUseAutoGroup {
			token.AutoGroupFallback = true
		}
		if err := token.Insert(); err != nil {
			common.ApiErrorI18n(c, i18n.MsgCreateDefaultTokenErr)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func GetAllUsers(c *gin.Context) {
	sortBy, sortOrder, idSortOrder, balanceSortOrder, usedQuotaSortOrder, err := parseUserSortQuery(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo := common.GetPageQuery(c)
	users, total, err := model.GetAllUsers(pageInfo, sortBy, sortOrder, idSortOrder, balanceSortOrder, usedQuotaSortOrder)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)

	common.ApiSuccess(c, pageInfo)
	return
}

func SearchUsers(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	group := strings.TrimSpace(c.Query("group"))
	// 这里将可选筛选参数统一解析为“指针语义”：
	// nil 代表“未传该条件”，非 nil 代表“显式过滤”，避免把 0/false 与“未传”混淆。
	role, err := parseOptionalIntQuery(c.Query("role"), "role")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status, err := parseOptionalIntQuery(c.Query("status"), "status")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	inviterID, err := parseOptionalIntQuery(c.Query("inviter_id"), "inviter_id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	inviteeUserID, err := parseOptionalIntQuery(c.Query("invitee_user_id"), "invitee_user_id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	hasInviter, err := parseOptionalBoolQuery(c.Query("has_inviter"), "has_inviter")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	hasInvitees, err := parseOptionalBoolQuery(c.Query("has_invitees"), "has_invitees")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	hasActiveSubscription, err := parseOptionalBoolQuery(c.Query("has_active_subscription"), "has_active_subscription")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	hasSellableToken, err := parseOptionalBoolQuery(c.Query("has_sellable_token"), "has_sellable_token")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	walletMinRaw := strings.TrimSpace(c.Query("wallet_min"))
	if walletMinRaw == "" {
		walletMinRaw = strings.TrimSpace(c.Query("balance_min"))
	}
	walletMin, err := parseOptionalIntQuery(walletMinRaw, "wallet_min")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	walletMaxRaw := strings.TrimSpace(c.Query("wallet_max"))
	if walletMaxRaw == "" {
		walletMaxRaw = strings.TrimSpace(c.Query("balance_max"))
	}
	walletMax, err := parseOptionalIntQuery(walletMaxRaw, "wallet_max")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if walletMin != nil && *walletMin < 0 {
		common.ApiError(c, errors.New("参数 wallet_min 不能小于 0"))
		return
	}
	if walletMax != nil && *walletMax < 0 {
		common.ApiError(c, errors.New("参数 wallet_max 不能小于 0"))
		return
	}
	// 对范围条件做前置校验，避免出现“最小值大于最大值”导致结果不可预期。
	if walletMin != nil && walletMax != nil && *walletMin > *walletMax {
		common.ApiError(c, errors.New("参数 wallet_min 不能大于 wallet_max"))
		return
	}
	usedBalanceMin, err := parseOptionalIntQuery(c.Query("used_balance_min"), "used_balance_min")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usedBalanceMax, err := parseOptionalIntQuery(c.Query("used_balance_max"), "used_balance_max")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if usedBalanceMin != nil && *usedBalanceMin < 0 {
		common.ApiError(c, errors.New("参数 used_balance_min 不能小于 0"))
		return
	}
	if usedBalanceMax != nil && *usedBalanceMax < 0 {
		common.ApiError(c, errors.New("参数 used_balance_max 不能小于 0"))
		return
	}
	if usedBalanceMin != nil && usedBalanceMax != nil && *usedBalanceMin > *usedBalanceMax {
		common.ApiError(c, errors.New("参数 used_balance_min 不能大于 used_balance_max"))
		return
	}
	registerSource := strings.TrimSpace(c.Query("register_source"))
	registerIP := strings.TrimSpace(c.Query("register_ip"))
	contentSafetyStatus, err := parseContentSafetyStatus(c.Query("content_safety_status"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	contentSafetyCodes, err := parseContentSafetyCodes(c.Query("content_safety_codes"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	contentSafetySortOrder, err := parseOptionalSortOrderQuery(c.Query("content_safety_sort_order"), "content_safety_sort_order")
	if err != nil {
		common.ApiError(c, err)
		return
	}

	sortBy, sortOrder, idSortOrder, balanceSortOrder, usedQuotaSortOrder, err := parseUserSortQuery(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo := common.GetPageQuery(c)
	users, total, err := model.SearchUsersWithParams(model.UserSearchParams{
		Keyword:                keyword,
		Group:                  group,
		Role:                   role,
		Status:                 status,
		InviterID:              inviterID,
		InviteeUserID:          inviteeUserID,
		HasInviter:             hasInviter,
		HasInvitees:            hasInvitees,
		HasActiveSubscription:  hasActiveSubscription,
		HasSellableToken:       hasSellableToken,
		WalletMin:              walletMin,
		WalletMax:              walletMax,
		UsedBalanceMin:         usedBalanceMin,
		UsedBalanceMax:         usedBalanceMax,
		RegisterSource:         registerSource,
		RegisterIP:             registerIP,
		ContentSafetyStatus:    contentSafetyStatus,
		ContentSafetyCodes:     contentSafetyCodes,
		ContentSafetySortOrder: contentSafetySortOrder,
		SortBy:                 sortBy,
		SortOrder:              sortOrder,
		IdSortOrder:            idSortOrder,
		WalletSortOrder:        balanceSortOrder,
		UsedQuotaSortOrder:     usedQuotaSortOrder,
		StartIdx:               pageInfo.GetStartIdx(),
		PageSize:               pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
	return
}

func parseContentSafetyStatus(raw string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	allowed := map[string]struct{}{
		"": {}, model.ContentSafetyLevelNormal: {}, model.ContentSafetyLevelTriggered: {}, model.ContentSafetyLevelWhitelisted: {},
		model.ContentSafetyLevelWarning1: {}, model.ContentSafetyLevelWarning2: {},
		model.ContentSafetyLevelCoolingOff: {}, model.ContentSafetyLevelObserved: {},
		model.ContentSafetyLevelFocus: {}, model.ContentSafetyLevelReviewPending: {},
		model.ContentSafetyLevelAdminDisabled: {}, model.ContentSafetyLevelLegacyDisabled: {},
	}
	if _, ok := allowed[status]; !ok {
		return "", errors.New("参数 content_safety_status 无效")
	}
	return status, nil
}

func parseContentSafetyCodes(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	codes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		code := strings.ToLower(strings.TrimSpace(item))
		if code == "" {
			continue
		}
		if !service.IsContentSafetyPolicyCode(code) {
			return nil, fmt.Errorf("参数 content_safety_codes 包含无效错误码: %s", code)
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes, nil
}

func GetUserInviteRelations(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, errors.New("无效的 id"))
		return
	}

	pageInfo := common.GetPageQuery(c)
	user, inviter, invitees, total, incomeSummary, err := model.GetUserInviteRelations(id, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 关系详情里被邀请人列表沿用统一分页结构，前端可直接复用表格分页组件。
	inviteesPage := &common.PageInfo{
		Page:     pageInfo.GetPage(),
		PageSize: pageInfo.GetPageSize(),
	}
	inviteesPage.SetTotal(int(total))
	inviteesPage.SetItems(invitees)

	common.ApiSuccess(c, gin.H{
		"user":                  user,
		"inviter":               inviter,
		"invitees":              inviteesPage,
		"invite_income_summary": incomeSummary,
	})
}

func GetSelfInviteRechargeCommissions(c *gin.Context) {
	id := c.GetInt("id")
	if id <= 0 {
		common.ApiError(c, errors.New("用户不存在"))
		return
	}

	pageInfo := common.GetPageQuery(c)
	items, total, summary, err := model.GetUserInviteRechargeCommissions(id, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	inviteesPage := &common.PageInfo{
		Page:     pageInfo.GetPage(),
		PageSize: pageInfo.GetPageSize(),
	}
	inviteesPage.SetTotal(int(total))
	inviteesPage.SetItems(items)

	common.ApiSuccess(c, gin.H{
		"invitees": inviteesPage,
		"summary":  summary,
	})
}

type RebuildAffCountRequest struct {
	UserID *int `json:"user_id"`
}

func RebuildAffCount(c *gin.Context) {
	req := RebuildAffCountRequest{}
	// 允许通过 JSON body 传 user_id；为空则表示全量修复。
	// 示例：
	// - 全量：POST /api/user/rebuild-aff-count
	// - 单用户：POST /api/user/rebuild-aff-count {"user_id":123}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
	}
	// 兼容 query 传参，便于后台或脚本直接调用。
	if rawUserID := strings.TrimSpace(c.Query("user_id")); rawUserID != "" {
		parsedUserID, err := strconv.Atoi(rawUserID)
		if err != nil || parsedUserID <= 0 {
			common.ApiError(c, errors.New("无效的 user_id"))
			return
		}
		req.UserID = &parsedUserID
	}

	result, err := model.RebuildAffCount(req.UserID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"target_user_id": req.UserID,
		"result":         result,
	})
}

func parseOptionalIntQuery(raw string, name string) (*int, error) {
	// 可选整型参数解析：
	// - 空字符串：视为未传
	// - 非空：必须是合法整数，否则直接返回参数错误，避免下游查询含糊处理
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("参数 %s 不是有效整数", name)
	}
	return &value, nil
}

func parseOptionalBoolQuery(raw string, name string) (*bool, error) {
	// 可选布尔参数解析，支持 true/false（大小写不敏感）。
	// 这里不接受其它文本，避免出现“看起来像开启筛选但实际未生效”的隐性问题。
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf("参数 %s 不是有效布尔值", name)
	}
	return &value, nil
}

func parseOptionalSortOrderQuery(raw string, name string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "asc", "desc":
		return value, nil
	default:
		return "", fmt.Errorf("参数 %s 仅支持 asc 或 desc", name)
	}
}

func parseUserSortQuery(c *gin.Context) (string, string, string, string, string, error) {
	sortBy := strings.ToLower(strings.TrimSpace(c.Query("sort_by")))
	if sortBy == "" {
		sortBy = "id"
	}
	switch sortBy {
	case "id", "quota":
	default:
		return "", "", "", "", "", errors.New("参数 sort_by 仅支持 id 或 quota")
	}

	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sort_order")))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	switch sortOrder {
	case "asc", "desc":
	default:
		return "", "", "", "", "", errors.New("参数 sort_order 仅支持 asc 或 desc")
	}

	// 新增组合排序参数：
	// - id_sort_order: 控制 ID 的升/降序
	// - wallet_sort_order: 控制钱包额度(quota)的升/降序
	// 两者可同时传入，例如 id desc + quota asc。
	idSortOrder := strings.ToLower(strings.TrimSpace(c.Query("id_sort_order")))
	switch idSortOrder {
	case "", "asc", "desc":
	default:
		return "", "", "", "", "", errors.New("参数 id_sort_order 仅支持 asc 或 desc")
	}

	walletSortOrder := strings.ToLower(strings.TrimSpace(c.Query("wallet_sort_order")))
	if walletSortOrder == "" {
		walletSortOrder = strings.ToLower(strings.TrimSpace(c.Query("balance_sort_order")))
	}
	switch walletSortOrder {
	case "", "asc", "desc":
	default:
		return "", "", "", "", "", errors.New("参数 wallet_sort_order 仅支持 asc 或 desc")
	}

	usedQuotaSortOrder := strings.ToLower(strings.TrimSpace(c.Query("used_quota_sort_order")))
	switch usedQuotaSortOrder {
	case "", "asc", "desc":
	default:
		return "", "", "", "", "", errors.New("参数 used_quota_sort_order 仅支持 asc 或 desc")
	}

	return sortBy, sortOrder, idSortOrder, walletSortOrder, usedQuotaSortOrder, nil
}

func GetUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 补充套餐和令牌元数据，确保管理页面刷新单行或详情时显示一致。
	_ = model.AttachUserSubscriptionMetadata(model.DB, []*model.User{user})
	_ = model.AttachUserSellableTokenMetadata(model.DB, []*model.User{user})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user,
	})
	return
}

func GenerateAccessToken(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// get rand int 28-32
	randI := common.GetRandomInt(4)
	key, err := common.GenerateRandomKey(29 + randI)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgGenerateFailed)
		common.SysLog("failed to generate key: " + err.Error())
		return
	}
	user.SetAccessToken(key)

	if model.DB.Where("access_token = ?", user.AccessToken).First(user).RowsAffected != 0 {
		common.ApiErrorI18n(c, i18n.MsgUuidDuplicate)
		return
	}

	if err := user.Update(false); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.AccessToken,
	})
	return
}

type TransferAffQuotaRequest struct {
	Quota int `json:"quota" binding:"required"`
}

func TransferAffQuota(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tran := TransferAffQuotaRequest{}
	if err := c.ShouldBindJSON(&tran); err != nil {
		common.ApiError(c, err)
		return
	}
	err = user.TransferAffQuotaToQuota(tran.Quota)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserTransferFailed, map[string]any{"Error": err.Error()})
		return
	}
	common.ApiSuccessI18n(c, i18n.MsgUserTransferSuccess, nil)
}

func GetAffCode(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user.AffCode == "" {
		affCode, err := model.GenerateUniqueAffCode()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		user.AffCode = affCode
		if err := user.Update(false); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.AffCode,
	})
	return
}

// SetAffCode 设置当前用户的自定义短邀请码
func SetAffCode(c *gin.Context) {
	id := c.GetInt("id")
	var req struct {
		AffCode string `json:"aff_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateUserAffCode(id, req.AffCode); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    strings.TrimSpace(req.AffCode),
	})
}

func GetSelf(c *gin.Context) {
	id := c.GetInt("id")
	userRole := c.GetInt("role")
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Hide admin remarks: set to empty to trigger omitempty tag, ensuring the remark field is not included in JSON returned to regular users
	user.Remark = ""

	// 计算用户权限信息
	permissions := calculateUserPermissions(userRole)

	// 获取用户设置并提取sidebar_modules
	userSetting := user.GetSetting()
	currentQuota := user.Quota
	if quota, quotaErr := model.GetUserQuota(id, false); quotaErr == nil {
		currentQuota = quota
	}

	// 构建响应数据，包含用户信息和权限
	responseData := map[string]interface{}{
		"id":           user.Id,
		"username":     user.Username,
		"display_name": user.DisplayName,
		"role":         user.Role,
		"status":       user.Status,
		"email":        user.Email,
		"github_id":    user.GitHubId,
		"discord_id":   user.DiscordId,
		"oidc_id":      user.OidcId,
		"wechat_id":    user.WeChatId,
		"telegram_id":  user.TelegramId,
		"group":        user.Group,
		"quota":        currentQuota,
		"transferable_quota": model.EffectiveTransferableQuota(
			currentQuota,
			user.TransferableQuota,
		),
		"used_quota":        user.UsedQuota,
		"request_count":     user.RequestCount,
		"aff_code":          user.AffCode,
		"aff_count":         user.AffCount,
		"aff_quota":         user.AffQuota,
		"aff_history_quota": user.AffHistoryQuota,
		"inviter_id":        user.InviterId,
		"linux_do_id":       user.LinuxDOId,
		"setting":           user.Setting,
		"stripe_customer":   user.StripeCustomer,
		"sidebar_modules":   userSetting.SidebarModules, // 正确提取sidebar_modules字段
		"permissions":       permissions,                // 新增权限字段
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responseData,
	})
	return
}

// 计算用户权限的辅助函数
func calculateUserPermissions(userRole int) map[string]interface{} {
	permissions := map[string]interface{}{}

	// 根据用户角色计算权限
	if userRole == common.RoleRootUser {
		// 超级管理员不需要边栏设置功能
		permissions["sidebar_settings"] = false
		permissions["sidebar_modules"] = map[string]interface{}{}
	} else if userRole == common.RoleAdminUser {
		// 管理员可以设置边栏，但不包含系统设置功能
		permissions["sidebar_settings"] = true
		permissions["sidebar_modules"] = map[string]interface{}{
			"admin": map[string]interface{}{
				"setting": false, // 管理员不能访问系统设置
			},
		}
	} else {
		// 普通用户只能设置个人功能，不包含管理员区域
		permissions["sidebar_settings"] = true
		permissions["sidebar_modules"] = map[string]interface{}{
			"admin": false, // 普通用户不能访问管理员区域
		}
	}

	return permissions
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfig(userRole int) string {
	defaultConfig := map[string]interface{}{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]interface{}{
		"enabled":    true,
		"playground": true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]interface{}{
		"enabled":    true,
		"detail":     true,
		"token":      true,
		"log":        true,
		"midjourney": true,
		"task":       true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]interface{}{
		"enabled":  true,
		"topup":    true,
		"pulse":    true,
		"personal": true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":    true,
			"channel":    true,
			"models":     true,
			"redemption": true,
			"user":       true,
			"setting":    false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":    true,
			"channel":    true,
			"models":     true,
			"redemption": true,
			"user":       true,
			"setting":    true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := common.Marshal(defaultConfig)
	if err != nil {
		common.SysLog("生成默认边栏配置失败: " + err.Error())
		return ""
	}

	return string(configBytes)
}

func GetUserModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		id = c.GetInt("id")
	}
	user, err := model.GetUserCache(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groups := service.GetUserUsableGroups(user.Group)
	requestedGroup := strings.TrimSpace(c.Query("group"))
	if requestedGroup != "" {
		if _, ok := groups[requestedGroup]; !ok {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "",
				"data":    []string{},
			})
			return
		}

		models := make([]string, 0)
		if requestedGroup == "auto" {
			for _, group := range service.GetUserAutoGroup(user.Group) {
				for _, modelName := range model.GetGroupEnabledModels(group) {
					if !common.StringsContains(models, modelName) {
						models = append(models, modelName)
					}
				}
			}
		} else {
			models = model.GetGroupEnabledModels(requestedGroup)
		}
		models = model.FilterModelsByVisibility(models, user.Role)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    models,
		})
		return
	}

	models := make([]string, 0)
	for group := range groups {
		for _, g := range model.GetGroupEnabledModels(group) {
			if !common.StringsContains(models, g) {
				models = append(models, g)
			}
		}
	}
	models = model.FilterModelsByVisibility(models, user.Role)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    models,
	})
	return
}

func UpdateUser(c *gin.Context) {
	var updatedUser model.User
	err := common.DecodeJson(c.Request.Body, &updatedUser)
	if err != nil || updatedUser.Id == 0 || !common.IsValidateRole(updatedUser.Role) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if updatedUser.Password == "" {
		updatedUser.Password = "$I_LOVE_U" // make Validator happy :)
	}
	if err := common.Validate.Struct(&updatedUser); err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserInputInvalid, map[string]any{"Error": common.TranslateValidationErrors(err)})
		return
	}
	originUser, err := model.GetUserById(updatedUser.Id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	myRole := c.GetInt("role")
	if myRole <= originUser.Role && myRole != common.RoleRootUser {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionHigherLevel)
		return
	}
	if myRole <= updatedUser.Role && myRole != common.RoleRootUser {
		common.ApiErrorI18n(c, i18n.MsgUserCannotCreateHigherLevel)
		return
	}
	if updatedUser.Password == "$I_LOVE_U" {
		updatedUser.Password = "" // rollback to what it should be
	}
	updatePassword := updatedUser.Password != ""
	if err := updatedUser.Edit(updatePassword); err != nil {
		common.ApiError(c, err)
		return
	}
	if originUser.Quota != updatedUser.Quota {
		model.RecordLogWithAdminInfo(originUser.Id, model.LogTypeManage,
			fmt.Sprintf("管理员将用户额度从 %s修改为 %s", logger.LogQuota(originUser.Quota), logger.LogQuota(updatedUser.Quota)),
			map[string]interface{}{
				"admin_id":       c.GetInt("id"),
				"admin_username": c.GetString("username"),
			},
		)
	}
	if originUser.Status == common.UserStatusDisabled && updatedUser.Status == common.UserStatusEnabled {
		if state, stateErr := model.GetUserContentSafetyState(updatedUser.Id); stateErr == nil && state.WindowCount > 0 {
			model.RecordLogWithAdminInfo(updatedUser.Id, model.LogTypeManage,
				fmt.Sprintf("管理员重新启用了存在内容安全历史的用户；30 天记录 %d 次、冷静期 %d 次，历史记录继续累计", state.WindowCount, state.CooldownCount),
				map[string]interface{}{"admin_id": c.GetInt("id"), "admin_username": c.GetString("username")},
			)
		}
	}

	// 补充套餐和令牌元数据，确保管理页面执行更新用户操作后，返回的对象带有完整的状态，防止表格因数据覆盖而显示异常。
	_ = model.AttachUserSubscriptionMetadata(model.DB, []*model.User{&updatedUser})
	_ = model.AttachUserSellableTokenMetadata(model.DB, []*model.User{&updatedUser})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    updatedUser,
	})
	return
}

func AdminClearUserBinding(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	bindingType := strings.ToLower(strings.TrimSpace(c.Param("binding_type")))
	if bindingType == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if myRole <= user.Role && myRole != common.RoleRootUser {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return
	}

	if err := user.ClearBinding(bindingType); err != nil {
		common.ApiError(c, err)
		return
	}

	model.RecordLog(user.Id, model.LogTypeManage, fmt.Sprintf("admin cleared %s binding for user %s", bindingType, user.Username))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "success",
	})
}

func UpdateSelf(c *gin.Context) {
	var requestData map[string]interface{}
	err := common.DecodeJson(c.Request.Body, &requestData)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// 检查是否是用户设置更新请求 (sidebar_modules 或 language)
	if sidebarModules, sidebarExists := requestData["sidebar_modules"]; sidebarExists {
		userId := c.GetInt("id")
		user, err := model.GetUserById(userId, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}

		// 获取当前用户设置
		currentSetting := user.GetSetting()

		// 更新sidebar_modules字段
		if sidebarModulesStr, ok := sidebarModules.(string); ok {
			currentSetting.SidebarModules = sidebarModulesStr
		}

		// 保存更新后的设置
		user.SetSetting(currentSetting)
		if err := user.Update(false); err != nil {
			common.ApiErrorI18n(c, i18n.MsgUpdateFailed)
			return
		}

		common.ApiSuccessI18n(c, i18n.MsgUpdateSuccess, nil)
		return
	}

	// 检查是否是语言偏好更新请求
	if language, langExists := requestData["language"]; langExists {
		userId := c.GetInt("id")
		user, err := model.GetUserById(userId, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}

		// 获取当前用户设置
		currentSetting := user.GetSetting()

		// 更新language字段
		if langStr, ok := language.(string); ok {
			currentSetting.Language = langStr
		}

		// 保存更新后的设置
		user.SetSetting(currentSetting)
		if err := user.Update(false); err != nil {
			common.ApiErrorI18n(c, i18n.MsgUpdateFailed)
			return
		}

		common.ApiSuccessI18n(c, i18n.MsgUpdateSuccess, nil)
		return
	}

	// 原有的用户信息更新逻辑
	var user model.User
	requestDataBytes, err := common.Marshal(requestData)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	err = common.Unmarshal(requestDataBytes, &user)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	if user.Password == "" {
		user.Password = "$I_LOVE_U" // make Validator happy :)
	}
	if err := common.Validate.Struct(&user); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidInput)
		return
	}

	cleanUser := model.User{
		Id:          c.GetInt("id"),
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
	}
	if user.Password == "$I_LOVE_U" {
		user.Password = "" // rollback to what it should be
		cleanUser.Password = ""
	}
	updatePassword, err := checkUpdatePassword(user.OriginalPassword, user.Password, cleanUser.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := cleanUser.Update(updatePassword); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func checkUpdatePassword(originalPassword string, newPassword string, userId int) (updatePassword bool, err error) {
	var currentUser *model.User
	currentUser, err = model.GetUserById(userId, true)
	if err != nil {
		return
	}

	// 密码不为空,需要验证原密码
	// 支持第一次账号绑定时原密码为空的情况
	if !common.ValidatePasswordAndHash(originalPassword, currentUser.Password) && currentUser.Password != "" {
		err = fmt.Errorf("原密码错误")
		return
	}
	if newPassword == "" {
		return
	}
	updatePassword = true
	return
}

func DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	originUser, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	myRole := c.GetInt("role")
	if myRole <= originUser.Role {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionHigherLevel)
		return
	}
	err = model.HardDeleteUserById(id)
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

func DeleteSelf(c *gin.Context) {
	id := c.GetInt("id")
	user, _ := model.GetUserById(id, false)

	if user.Role == common.RoleRootUser {
		common.ApiErrorI18n(c, i18n.MsgUserCannotDeleteRootUser)
		return
	}

	err := model.DeleteUserById(id)
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

func CreateUser(c *gin.Context) {
	var user model.User
	err := common.DecodeJson(c.Request.Body, &user)
	user.Username = strings.TrimSpace(user.Username)
	if err != nil || user.Username == "" || user.Password == "" || !common.IsValidateRole(user.Role) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := common.Validate.Struct(&user); err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserInputInvalid, map[string]any{"Error": common.TranslateValidationErrors(err)})
		return
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Username
	}
	myRole := c.GetInt("role")
	if user.Role >= myRole {
		common.ApiErrorI18n(c, i18n.MsgUserCannotCreateHigherLevel)
		return
	}
	// Even for admin users, we cannot fully trust them!
	cleanUser := model.User{
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
		Remark:      user.Remark,
		Role:        user.Role, // 保持管理员设置的角色
	}
	applyUserRegisterAudit(c, &cleanUser, model.UserRegisterSourceAdmin)
	if err := cleanUser.Insert(0); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

type ManageRequest struct {
	Id     int    `json:"id"`
	Action string `json:"action"`
}

type ManageBatchRequest struct {
	Ids    []int  `json:"ids"`
	Action string `json:"action"`
}

// applyManageAction 处理单个用户的管理动作。
// 单用户和批量接口都会走这里，避免两边规则不一致。
func applyManageAction(user *model.User, action string, myRole int) error {
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	switch normalizedAction {
	case "disable":
		// root 用户是系统最高权限账户，禁止被禁用，避免后台完全失控。
		if user.Role == common.RoleRootUser {
			return errors.New("不能禁用 root 用户")
		}
		user.Status = common.UserStatusDisabled
		return user.Update(false)
	case "enable":
		// 启用仅修改状态位，不触及其他字段。
		user.Status = common.UserStatusEnabled
		return user.Update(false)
	case "delete":
		// root 用户同样禁止删除，防止系统不可恢复。
		if user.Role == common.RoleRootUser {
			return errors.New("不能删除 root 用户")
		}
		// 这里走软删除（Delete），保留审计/追溯能力。
		return user.Delete()
	case "promote":
		// 提升管理员属于高风险操作，仅 root 可执行。
		if myRole != common.RoleRootUser {
			return errors.New("仅 root 用户可提升管理员")
		}
		if user.Role >= common.RoleAdminUser {
			return errors.New("用户已是管理员")
		}
		user.Role = common.RoleAdminUser
		return user.Update(false)
	case "demote":
		// root 用户不可降级；普通用户已是最低角色无需重复降级。
		if user.Role == common.RoleRootUser {
			return errors.New("不能降级 root 用户")
		}
		if user.Role == common.RoleCommonUser {
			return errors.New("用户已是普通用户")
		}
		user.Role = common.RoleCommonUser
		return user.Update(false)
	default:
		return errors.New("不支持的操作类型")
	}
}

func shouldInvalidateManagedUserCaches(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "disable", "delete", "promote", "demote":
		return true
	default:
		return false
	}
}

// ManageUser Only admin user can do this
func ManageUser(c *gin.Context) {
	var req ManageRequest
	err := common.DecodeJson(c.Request.Body, &req)

	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	user := model.User{
		Id: req.Id,
	}
	// Fill attributes
	model.DB.Unscoped().Where(&user).First(&user)
	if user.Id == 0 {
		common.ApiErrorI18n(c, i18n.MsgUserNotExists)
		return
	}
	myRole := c.GetInt("role")
	if myRole <= user.Role && myRole != common.RoleRootUser {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionHigherLevel)
		return
	}
	if err := applyManageAction(&user, req.Action, myRole); err != nil {
		common.ApiError(c, err)
		return
	}
	if shouldInvalidateManagedUserCaches(req.Action) {
		if err := invalidateManagedUserCaches(user.Id); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	// 补充套餐和令牌元数据，确保管理页面执行单条禁用/启用等操作后，该行的套餐状态不会因数据覆盖而显示为“无套餐”。
	_ = model.AttachUserSubscriptionMetadata(model.DB, []*model.User{&user})
	_ = model.AttachUserSellableTokenMetadata(model.DB, []*model.User{&user})

	clearUser := model.User{
		Id:                               user.Id,
		Role:                             user.Role,
		Status:                           user.Status,
		HasActiveSubscription:            user.HasActiveSubscription,
		ActiveSubscriptionCount:          user.ActiveSubscriptionCount,
		PendingSubscriptionIssuanceCount: user.PendingSubscriptionIssuanceCount,
		HasSellableToken:                 user.HasSellableToken,
		ActiveSellableTokenCount:         user.ActiveSellableTokenCount,
		PendingSellableIssuanceCount:     user.PendingSellableIssuanceCount,
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    clearUser,
	})
	return
}

// ManageUserBatch 批量执行用户管理动作。
// 每个用户单独校验并返回结果，方便前端直接展示成功和失败明细。
func ManageUserBatch(c *gin.Context) {
	req := ManageBatchRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	myRole := c.GetInt("role")
	seen := make(map[int]struct{}, len(req.Ids))
	updatedUsers := make([]*model.User, 0, len(req.Ids))
	failed := make([]gin.H, 0)

	for _, id := range req.Ids {
		// ... (previous logic for ID validation, deduplication, and user fetching)
		if id <= 0 {
			failed = append(failed, gin.H{
				"id":      id,
				"message": "用户 ID 无效",
			})
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		user := model.User{Id: id}
		model.DB.Unscoped().Where(&user).First(&user)
		if user.Id == 0 {
			failed = append(failed, gin.H{
				"id":      id,
				"message": "用户不存在",
			})
			continue
		}
		if myRole <= user.Role && myRole != common.RoleRootUser {
			failed = append(failed, gin.H{
				"id":      id,
				"message": "权限不足",
			})
			continue
		}

		if err := applyManageAction(&user, action, myRole); err != nil {
			failed = append(failed, gin.H{
				"id":      id,
				"message": err.Error(),
			})
			continue
		}
		if shouldInvalidateManagedUserCaches(action) {
			if err := invalidateManagedUserCaches(user.Id); err != nil {
				failed = append(failed, gin.H{
					"id":      id,
					"message": err.Error(),
				})
				continue
			}
		}

		updatedUsers = append(updatedUsers, &user)
	}

	// 批量补充套餐和令牌元数据，确保批量操作（如批量禁用）后前端表格状态同步。
	if len(updatedUsers) > 0 {
		_ = model.AttachUserSubscriptionMetadata(model.DB, updatedUsers)
		_ = model.AttachUserSellableTokenMetadata(model.DB, updatedUsers)
	}

	updated := make([]gin.H, 0, len(updatedUsers))
	for _, u := range updatedUsers {
		updated = append(updated, gin.H{
			"id":                                  u.Id,
			"role":                                u.Role,
			"status":                              u.Status,
			"has_active_subscription":             u.HasActiveSubscription,
			"active_subscription_count":           u.ActiveSubscriptionCount,
			"pending_subscription_issuance_count": u.PendingSubscriptionIssuanceCount,
			"has_sellable_token":                  u.HasSellableToken,
			"active_sellable_token_count":         u.ActiveSellableTokenCount,
			"pending_sellable_issuance_count":     u.PendingSellableIssuanceCount,
		})
	}

	common.ApiSuccess(c, gin.H{
		"success_count": len(updated),
		"failed_count":  len(failed),
		"updated":       updated,
		"failed":        failed,
	})
}

func EmailBind(c *gin.Context) {
	email := c.Query("email")
	code := c.Query("code")
	if c.Request.Method == http.MethodPost {
		var req accountBindRequest
		if err := common.DecodeJson(c.Request.Body, &req); err != nil {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		email = req.Email
		code = req.Code
	}
	email = common.NormalizeEmailAddress(email)
	if !common.VerifyCodeWithKey(email, code, common.EmailVerificationPurpose) {
		common.ApiErrorI18n(c, i18n.MsgUserVerificationCodeError)
		return
	}
	session := sessions.Default(c)
	id := session.Get("id")
	user := model.User{
		Id: id.(int),
	}
	err := user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user.Email = email
	// no need to check if this email already taken, because we have used verification code to check it
	err = user.Update(false)
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

type topUpRequest struct {
	Key                       string `json:"key"`
	RenewTargetSubscriptionId int    `json:"renew_target_subscription_id"`
	PurchaseMode              string `json:"purchase_mode"`
}

var topUpLocks sync.Map
var topUpCreateLock sync.Mutex

type topUpTryLock struct {
	ch chan struct{}
}

func newTopUpTryLock() *topUpTryLock {
	return &topUpTryLock{ch: make(chan struct{}, 1)}
}

func (l *topUpTryLock) TryLock() bool {
	select {
	case l.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *topUpTryLock) Unlock() {
	select {
	case <-l.ch:
	default:
	}
}

func getTopUpLock(userID int) *topUpTryLock {
	if v, ok := topUpLocks.Load(userID); ok {
		return v.(*topUpTryLock)
	}
	topUpCreateLock.Lock()
	defer topUpCreateLock.Unlock()
	if v, ok := topUpLocks.Load(userID); ok {
		return v.(*topUpTryLock)
	}
	l := newTopUpTryLock()
	topUpLocks.Store(userID, l)
	return l
}

func TopUp(c *gin.Context) {
	id := c.GetInt("id")
	lock := getTopUpLock(id)
	if !lock.TryLock() {
		common.ApiErrorI18n(c, i18n.MsgUserTopUpProcessing)
		return
	}
	defer lock.Unlock()
	req := topUpRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quota, err := model.Redeem(req.Key, id)
	if err != nil {
		if errors.Is(err, model.ErrRedemptionAlreadyUsed) {
			common.ApiErrorMsg(c, "此兑换码已有人使用")
			return
		}
		if errors.Is(err, model.ErrRedemptionDisabled) {
			common.ApiErrorMsg(c, "该兑换码已禁用")
			return
		}
		if errors.Is(err, model.ErrRedemptionExpired) {
			common.ApiErrorMsg(c, "该兑换码已过期")
			return
		}
		if errors.Is(err, model.ErrLegacyWalletRedemptionRestricted) {
			common.ApiErrorMsg(c, "该历史钱包兑换码只能由创建者本人兑换并取回额度")
			return
		}
		if errors.Is(err, model.ErrRedeemFailed) {
			common.ApiErrorI18n(c, i18n.MsgRedeemFailed)
			return
		}
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    quota,
	})
}

// Redeem 提供统一兑换入口，兼容余额码与套餐码两种权益。
func Redeem(c *gin.Context) {
	id := c.GetInt("id")

	req := topUpRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.RecordRedemptionAttempt(c, false)
		common.ApiError(c, err)
		return
	}

	if !middleware.CheckRedemptionRateLimit(c) {
		return
	}

	lock := getTopUpLock(id)
	if !lock.TryLock() {
		common.ApiErrorI18n(c, i18n.MsgUserTopUpProcessing)
		return
	}
	defer lock.Unlock()

	result, err := model.RedeemWithOptions(req.Key, id, req.RenewTargetSubscriptionId, req.PurchaseMode)
	if err != nil {
		if shouldCountAsRedeemFailure(err) {
			middleware.RecordRedemptionAttempt(c, false)
		}
		if needTargetErr, ok := err.(*model.RedeemNeedRenewTargetError); ok {
			// 多条可续费订阅时，由前端弹出选择器让用户自己决定续到哪一条。
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": needTargetErr.Error(),
				"data": gin.H{
					"code":       "redeem_select_renew_target",
					"plan_id":    needTargetErr.PlanId,
					"plan_title": needTargetErr.PlanTitle,
					"options":    needTargetErr.Options,
				},
			})
			return
		}
		if selectModeErr, ok := err.(*model.RedeemNeedSelectPurchaseModeError); ok {
			// 套餐码未指定兑换方式时，返回特殊响应让前端弹出选择框。
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": selectModeErr.Error(),
				"data": gin.H{
					"code":       "redeem_select_purchase_mode",
					"plan_id":    selectModeErr.PlanId,
					"plan_title": selectModeErr.PlanTitle,
				},
			})
			return
		}
		if errors.Is(err, model.ErrRedemptionAlreadyUsed) {
			common.ApiErrorMsg(c, "此兑换码已有人使用")
			return
		}
		if errors.Is(err, model.ErrRedemptionDisabled) {
			common.ApiErrorMsg(c, "该兑换码已禁用")
			return
		}
		if errors.Is(err, model.ErrRedemptionExpired) {
			common.ApiErrorMsg(c, "该兑换码已过期")
			return
		}
		if errors.Is(err, model.ErrLegacyWalletRedemptionRestricted) {
			common.ApiErrorMsg(c, "该历史钱包兑换码只能由创建者本人兑换并取回额度")
			return
		}
		if errors.Is(err, model.ErrRedeemFailed) {
			// 对外统一返回兑换失败文案，避免把内部细节直接暴露给前端。
			common.ApiErrorI18n(c, i18n.MsgRedeemFailed)
			return
		}
		common.ApiError(c, err)
		return
	}

	// 统一兑换接口直接返回结构化结果，让前端根据权益类型自行处理展示和刷新。
	middleware.RecordRedemptionAttempt(c, true)
	common.ApiSuccess(c, result)
}

// shouldCountAsRedeemFailure 用于区分"真正失败"和"尚未裁决"的兑换错误。
// "尚未裁决"（需要前端进一步选择）的错误不应计入失败次数。
func shouldCountAsRedeemFailure(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*model.RedeemNeedRenewTargetError); ok {
		return false
	}
	if _, ok := err.(*model.RedeemNeedSelectPurchaseModeError); ok {
		return false
	}
	return true
}

type UpdateUserSettingRequest struct {
	QuotaBlockedNotifyEnabled        *bool   `json:"quota_blocked_notify_enabled"`
	QuotaWarningType                 string  `json:"notify_type"`
	QuotaWarningThreshold            float64 `json:"quota_warning_threshold"`
	WebhookUrl                       string  `json:"webhook_url,omitempty"`
	WebhookSecret                    string  `json:"webhook_secret,omitempty"`
	NotificationEmail                string  `json:"notification_email,omitempty"`
	BarkUrl                          string  `json:"bark_url,omitempty"`
	GotifyUrl                        string  `json:"gotify_url,omitempty"`
	GotifyToken                      string  `json:"gotify_token,omitempty"`
	GotifyPriority                   int     `json:"gotify_priority,omitempty"`
	UpstreamModelUpdateNotifyEnabled *bool   `json:"upstream_model_update_notify_enabled,omitempty"`
	AcceptUnsetModelRatioModel       bool    `json:"accept_unset_model_ratio_model"`
	RecordIpLog                      bool    `json:"record_ip_log"`
}

func UpdateUserSetting(c *gin.Context) {
	var req UpdateUserSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// 验证预警类型
	if req.QuotaWarningType != dto.NotifyTypeEmail && req.QuotaWarningType != dto.NotifyTypeWebhook && req.QuotaWarningType != dto.NotifyTypeBark && req.QuotaWarningType != dto.NotifyTypeGotify {
		common.ApiErrorI18n(c, i18n.MsgSettingInvalidType)
		return
	}

	// 验证预警阈值
	if req.QuotaWarningThreshold <= 0 {
		common.ApiErrorI18n(c, i18n.MsgQuotaThresholdGtZero)
		return
	}

	// 如果是webhook类型,验证webhook地址
	if req.QuotaWarningType == dto.NotifyTypeWebhook {
		if req.WebhookUrl == "" {
			common.ApiErrorI18n(c, i18n.MsgSettingWebhookEmpty)
			return
		}
		// 验证URL格式
		if _, err := url.ParseRequestURI(req.WebhookUrl); err != nil {
			common.ApiErrorI18n(c, i18n.MsgSettingWebhookInvalid)
			return
		}
	}

	// 如果是邮件类型，验证邮箱地址
	if req.QuotaWarningType == dto.NotifyTypeEmail && req.NotificationEmail != "" {
		// 验证邮箱格式
		if !strings.Contains(req.NotificationEmail, "@") {
			common.ApiErrorI18n(c, i18n.MsgSettingEmailInvalid)
			return
		}
	}

	// 如果是Bark类型，验证Bark URL
	if req.QuotaWarningType == dto.NotifyTypeBark {
		if req.BarkUrl == "" {
			common.ApiErrorI18n(c, i18n.MsgSettingBarkUrlEmpty)
			return
		}
		// 验证URL格式
		if _, err := url.ParseRequestURI(req.BarkUrl); err != nil {
			common.ApiErrorI18n(c, i18n.MsgSettingBarkUrlInvalid)
			return
		}
		// 检查是否是HTTP或HTTPS
		if !strings.HasPrefix(req.BarkUrl, "https://") && !strings.HasPrefix(req.BarkUrl, "http://") {
			common.ApiErrorI18n(c, i18n.MsgSettingUrlMustHttp)
			return
		}
	}

	// 如果是Gotify类型，验证Gotify URL和Token
	if req.QuotaWarningType == dto.NotifyTypeGotify {
		if req.GotifyUrl == "" {
			common.ApiErrorI18n(c, i18n.MsgSettingGotifyUrlEmpty)
			return
		}
		if req.GotifyToken == "" {
			common.ApiErrorI18n(c, i18n.MsgSettingGotifyTokenEmpty)
			return
		}
		// 验证URL格式
		if _, err := url.ParseRequestURI(req.GotifyUrl); err != nil {
			common.ApiErrorI18n(c, i18n.MsgSettingGotifyUrlInvalid)
			return
		}
		// 检查是否是HTTP或HTTPS
		if !strings.HasPrefix(req.GotifyUrl, "https://") && !strings.HasPrefix(req.GotifyUrl, "http://") {
			common.ApiErrorI18n(c, i18n.MsgSettingUrlMustHttp)
			return
		}
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	existingSettings := user.GetSetting()
	upstreamModelUpdateNotifyEnabled := existingSettings.UpstreamModelUpdateNotifyEnabled
	if user.Role >= common.RoleAdminUser && req.UpstreamModelUpdateNotifyEnabled != nil {
		upstreamModelUpdateNotifyEnabled = *req.UpstreamModelUpdateNotifyEnabled
	}

	settings := existingSettings
	if req.QuotaBlockedNotifyEnabled != nil {
		settings.QuotaBlockedNotifyEnabled = req.QuotaBlockedNotifyEnabled
	}
	settings.NotifyType = req.QuotaWarningType
	settings.QuotaWarningThreshold = req.QuotaWarningThreshold
	settings.UpstreamModelUpdateNotifyEnabled = upstreamModelUpdateNotifyEnabled
	settings.AcceptUnsetRatioModel = req.AcceptUnsetModelRatioModel
	settings.RecordIpLog = req.RecordIpLog
	settings.WebhookUrl = ""
	settings.NotificationEmail = ""
	settings.BarkUrl = ""
	settings.GotifyUrl = ""
	settings.GotifyToken = ""
	settings.GotifyPriority = 0

	// 如果是webhook类型,添加webhook相关设置
	if req.QuotaWarningType == dto.NotifyTypeWebhook {
		settings.WebhookUrl = req.WebhookUrl
		if req.WebhookSecret != "" {
			settings.WebhookSecret = req.WebhookSecret
		}
	}

	// 如果提供了通知邮箱，添加到设置中
	if req.QuotaWarningType == dto.NotifyTypeEmail && req.NotificationEmail != "" {
		settings.NotificationEmail = req.NotificationEmail
	}

	// 如果是Bark类型，添加Bark URL到设置中
	if req.QuotaWarningType == dto.NotifyTypeBark {
		settings.BarkUrl = req.BarkUrl
	}

	// 如果是Gotify类型，添加Gotify配置到设置中
	if req.QuotaWarningType == dto.NotifyTypeGotify {
		settings.GotifyUrl = req.GotifyUrl
		settings.GotifyToken = req.GotifyToken
		// Gotify优先级范围0-10，超出范围则使用默认值5
		if req.GotifyPriority < 0 || req.GotifyPriority > 10 {
			settings.GotifyPriority = 5
		} else {
			settings.GotifyPriority = req.GotifyPriority
		}
	}

	// 更新用户设置
	user.SetSetting(settings)
	if err := user.Update(false); err != nil {
		common.ApiErrorI18n(c, i18n.MsgUpdateFailed)
		return
	}

	common.ApiSuccessI18n(c, i18n.MsgSettingSaved, nil)
}
