package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UserNameMaxLength = 20
const UserRegisterSourceMaxLength = 64
const UserAffCodeLength = 12
const userAffCodeGenerateMaxRetry = 16

type UserRegisterConflict string

const (
	UserRegisterSourceUnknown           = "unknown"
	UserRegisterSourcePassword          = "password"
	UserRegisterSourceAdmin             = "admin"
	UserRegisterSourceGitHub            = "github"
	UserRegisterSourceDiscord           = "discord"
	UserRegisterSourceOIDC              = "oidc"
	UserRegisterSourceLinuxDO           = "linuxdo"
	UserRegisterSourceWeChat            = "wechat"
	UserRegisterSourceCustomOAuthPrefix = "custom_oauth:"
)

const (
	UserRegisterConflictNone     UserRegisterConflict = ""
	UserRegisterConflictUsername UserRegisterConflict = "username"
	UserRegisterConflictEmail    UserRegisterConflict = "email"
	UserRegisterConflictBoth     UserRegisterConflict = "both"
)

var generateAffCodeCandidate = func() (string, error) {
	return common.GenerateRandomCharsKey(UserAffCodeLength)
}

func NormalizeUserRegisterSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return UserRegisterSourceUnknown
	}
	runes := []rune(source)
	if len(runes) > UserRegisterSourceMaxLength {
		source = string(runes[:UserRegisterSourceMaxLength])
	}
	return source
}

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id                   int            `json:"id"`
	Username             string         `json:"username" gorm:"unique;index" validate:"max=20"`
	Password             string         `json:"password" gorm:"not null;" validate:"min=8,max=20"`
	OriginalPassword     string         `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName          string         `json:"display_name" gorm:"index" validate:"max=20"`
	Role                 int            `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status               int            `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email                string         `json:"email" gorm:"index" validate:"max=50"`
	GitHubId             string         `json:"github_id" gorm:"column:github_id;index"`
	DiscordId            string         `json:"discord_id" gorm:"column:discord_id;index"`
	OidcId               string         `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId             string         `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId           string         `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode     string         `json:"verification_code" gorm:"-:all"`                         // this field is only for Email verification, don't save it to database!
	AccessToken          *string        `json:"-" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	Quota                int            `json:"quota" gorm:"type:int;default:0"`
	PulsePaidQuota       int64          `json:"-" gorm:"type:bigint;not null;default:0"`
	PulseRewardHold      bool           `json:"-" gorm:"not null;default:false"`
	PulseFundingEpoch    uint64         `json:"-" gorm:"type:bigint;not null;default:0"`
	TransferableQuota    int            `json:"transferable_quota" gorm:"type:int;not null;default:0;column:transferable_quota"`
	UsedQuota            int            `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	RequestCount         int            `json:"request_count" gorm:"type:int;default:0;"`               // request number
	Group                string         `json:"group" gorm:"type:varchar(64);default:'default'"`
	BalanceTrendDisabled bool           `json:"balance_trend_disabled" gorm:"default:false;column:balance_trend_disabled;index"`
	AffCode              string         `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount             int            `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota             int            `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`           // 邀请剩余额度
	AffHistoryQuota      int            `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"` // 邀请历史额度
	InviterId            int            `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	DeletedAt            gorm.DeletedAt `gorm:"index"`
	LinuxDOId            string         `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting              string         `json:"setting" gorm:"type:text;column:setting"`
	Remark               string         `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer       string         `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt            int64          `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt          int64          `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	RegisterSource       string         `json:"register_source" gorm:"type:varchar(64);column:register_source;index"`
	RegisterIP           string         `json:"register_ip" gorm:"type:varchar(64);column:register_ip;index"`
	RegisterUserAgent    string         `json:"register_user_agent" gorm:"type:varchar(255);column:register_user_agent"`

	InviterUsername       string                 `json:"inviter_username,omitempty" gorm:"-"`
	InviteeCount          int                    `json:"invitee_count,omitempty" gorm:"-"`
	inviteBindingDecision *inviteBindingDecision `gorm:"-"`
	registrationBaseQuota int                    `gorm:"-"`
	creationFinalized     bool                   `gorm:"-"`
	// 生效套餐元数据（仅用于用户管理列表响应，不落库）：
	// - HasActiveSubscription：当前是否存在生效套餐
	// - ActiveSubscriptionCount：当前生效套餐条数
	HasActiveSubscription            bool                    `json:"has_active_subscription" gorm:"-"`
	ActiveSubscriptionCount          int                     `json:"active_subscription_count" gorm:"-"`
	PendingSubscriptionIssuanceCount int                     `json:"pending_subscription_issuance_count" gorm:"-"`
	SubscriptionQuotaRemaining       int64                   `json:"subscription_quota_remaining" gorm:"-"`
	SubscriptionQuotaTotal           int64                   `json:"subscription_quota_total" gorm:"-"`
	SubscriptionQuotaHasUnlimited    bool                    `json:"subscription_quota_has_unlimited" gorm:"-"`
	SubscriptionQuotaItems           []SubscriptionQuotaItem `json:"subscription_quota_items,omitempty" gorm:"-"`
	// 可售令牌元数据（仅用于用户管理列表响应，不落库）
	HasSellableToken             bool `json:"has_sellable_token" gorm:"-"`
	ActiveSellableTokenCount     int  `json:"active_sellable_token_count" gorm:"-"`
	PendingSellableIssuanceCount int  `json:"pending_sellable_issuance_count" gorm:"-"`
	// 邀请关系视图的派生字段（仅用于接口响应，不落库）：
	// 1) 直接收益：按当前配置口径（QuotaForInviter）估算每个被邀请人贡献；
	// 2) 充值返佣：从返佣台账（InviteCommissionLedger）按 invitee 聚合已结算金额；
	// 3) 总收益：直接收益 + 充值返佣。
	InviteDirectIncomeQuota       int    `json:"invite_direct_income_quota" gorm:"-"`
	InviteRechargeCommissionQuota int    `json:"invite_recharge_commission_quota" gorm:"-"`
	InviteTotalIncomeQuota        int    `json:"invite_total_income_quota" gorm:"-"`
	RegisterIPBlacklisted         bool   `json:"register_ip_blacklisted" gorm:"-"`
	RegisterIPBlacklistRuleID     int    `json:"register_ip_blacklist_rule_id,omitempty" gorm:"-"`
	RegisterIPBlacklistCIDR       string `json:"register_ip_blacklist_cidr,omitempty" gorm:"-"`
	RegisterIPBlacklistReason     string `json:"register_ip_blacklist_reason,omitempty" gorm:"-"`
	ContentSafetyWhitelisted      bool   `json:"content_safety_whitelisted" gorm:"not null;default:false"`
	ContentSafetyCount            int    `json:"content_safety_count" gorm:"-"`
	ContentSafetyLevel            string `json:"content_safety_level" gorm:"-"`
	ContentSafetyLastAt           int64  `json:"content_safety_last_at" gorm:"-"`
	ContentSafetyLastCode         string `json:"content_safety_last_code" gorm:"-"`
	ContentSafetyLastModel        string `json:"content_safety_last_model" gorm:"-"`
	ContentSafetyLastChannelID    int    `json:"content_safety_last_channel_id" gorm:"-"`
	ContentSafetyLastRequestID    string `json:"content_safety_last_request_id" gorm:"-"`
	ContentSafetyBurstCount       int    `json:"content_safety_burst_count" gorm:"-"`
	ContentSafetyCooldownCount    int    `json:"content_safety_cooldown_count" gorm:"-"`
	ContentSafetyCooldownUntil    int64  `json:"content_safety_cooldown_until" gorm:"-"`
	ContentSafetyReviewCaseID     int64  `json:"content_safety_review_case_id" gorm:"-"`
	ContentSafetyLastCategory     string `json:"content_safety_last_category" gorm:"-"`
	ContentSafetyReasonSource     string `json:"content_safety_reason_source" gorm:"-"`
	ContentSafetyReasonConfidence string `json:"content_safety_reason_confidence" gorm:"-"`
	ContentSafetyReasonSummary    string `json:"content_safety_reason_summary" gorm:"-"`
}

type UserSearchParams struct {
	Keyword                string
	Group                  string
	Role                   *int
	Status                 *int
	InviterID              *int
	InviteeUserID          *int
	HasInviter             *bool
	HasInvitees            *bool
	HasActiveSubscription  *bool
	HasSellableToken       *bool
	WalletMin              *int
	WalletMax              *int
	UsedBalanceMin         *int
	UsedBalanceMax         *int
	RegisterSource         string
	RegisterIP             string
	ContentSafetyStatus    string
	ContentSafetyCodes     []string
	ContentSafetySortOrder string
	SortBy                 string
	SortOrder              string
	IdSortOrder            string
	WalletSortOrder        string
	UsedQuotaSortOrder     string
	StartIdx               int
	PageSize               int
}

type InviteIncomeSummary struct {
	DirectTotalQuota   int `json:"direct_total_quota"`
	RechargeTotalQuota int `json:"recharge_total_quota"`
	TotalQuota         int `json:"total_quota"`
}

type InviteRechargeCommissionSummary struct {
	RechargeTotalQuota int `json:"recharge_total_quota"`
}

type InviteRechargeCommissionInvitee struct {
	Alias                   string  `json:"alias"`
	RegisteredDate          string  `json:"registered_date"`
	CommissionLevel         int     `json:"commission_level"`
	RechargeTotalMoney      float64 `json:"recharge_total_money"`
	RechargeCommissionQuota int     `json:"recharge_commission_quota"`
}

func normalizeUserSortOrder(sortOrder string) string {
	normalizedSortOrder := strings.ToLower(strings.TrimSpace(sortOrder))
	switch normalizedSortOrder {
	case "asc", "desc":
		return normalizedSortOrder
	default:
		return ""
	}
}

func normalizeLegacyUserSort(sortBy string, sortOrder string) (string, string) {
	normalizedSortBy := strings.ToLower(strings.TrimSpace(sortBy))
	switch normalizedSortBy {
	case "quota":
	default:
		normalizedSortBy = "id"
	}

	normalizedSortOrder := strings.ToLower(strings.TrimSpace(sortOrder))
	switch normalizedSortOrder {
	case "asc":
	default:
		normalizedSortOrder = "desc"
	}

	return normalizedSortBy, normalizedSortOrder
}

func buildUserOrderClause(sortBy string, sortOrder string, idSortOrder string, walletSortOrder string, usedQuotaSortOrder string) string {
	orderClauses := make([]string, 0, 3)
	if normalized := normalizeUserSortOrder(idSortOrder); normalized != "" {
		orderClauses = append(orderClauses, "id "+normalized)
	}
	if normalized := normalizeUserSortOrder(walletSortOrder); normalized != "" {
		orderClauses = append(orderClauses, "quota "+normalized)
	}
	if normalized := normalizeUserSortOrder(usedQuotaSortOrder); normalized != "" {
		orderClauses = append(orderClauses, "used_quota "+normalized)
	}

	// 兼容旧参数：如果新参数都为空，则回退到 sort_by/sort_order。
	if len(orderClauses) == 0 {
		normalizedSortBy, normalizedSortOrder := normalizeLegacyUserSort(sortBy, sortOrder)
		orderClauses = append(orderClauses, normalizedSortBy+" "+normalizedSortOrder)
	}

	return strings.Join(orderClauses, ", ")
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:       user.Id,
		Group:    user.Group,
		Quota:    user.Quota,
		Status:   user.Status,
		Role:     user.Role,
		Username: user.Username,
		Setting:  user.Setting,
		Email:    user.Email,
	}
	return cache
}

func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

func UpdateUserLastLoginAt(id int) {
	if err := DB.Model(&User{}).Where("id = ?", id).Update("last_login_at", common.GetTimestamp()).Error; err != nil {
		common.SysLog("failed to update user last_login_at: " + err.Error())
	}
}

func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
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

// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	var err error
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func CheckUserRegisterConflict(username string, email string) (UserRegisterConflict, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if username == "" && email == "" {
		return UserRegisterConflictNone, nil
	}

	query := DB.Unscoped().Model(&User{})
	if email == "" {
		var count int64
		if err := query.Where("username = ?", username).Count(&count).Error; err != nil {
			return UserRegisterConflictNone, err
		}
		if count > 0 {
			return UserRegisterConflictUsername, nil
		}
		return UserRegisterConflictNone, nil
	}

	var rows []struct {
		Username string
		Email    string
	}
	if err := query.Select("username", "email").
		Where("username = ? OR email = ?", username, email).
		Find(&rows).Error; err != nil {
		return UserRegisterConflictNone, err
	}

	usernameExists := false
	emailExists := false
	for _, row := range rows {
		if row.Username == username {
			usernameExists = true
		}
		if row.Email == email {
			emailExists = true
		}
	}

	switch {
	case usernameExists && emailExists:
		return UserRegisterConflictBoth, nil
	case usernameExists:
		return UserRegisterConflictUsername, nil
	case emailExists:
		return UserRegisterConflictEmail, nil
	default:
		return UserRegisterConflictNone, nil
	}
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

func GetAllUsers(pageInfo *common.PageInfo, sortBy string, sortOrder string, idSortOrder string, balanceSortOrder string, usedQuotaSortOrder string) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = tx.Unscoped().Model(&User{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	orderClause := buildUserOrderClause(sortBy, sortOrder, idSortOrder, balanceSortOrder, usedQuotaSortOrder)
	// Get paginated users within same transaction
	err = tx.Unscoped().Order(orderClause).Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password", "access_token").Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	// 当前页批量补充"生效套餐"统计，避免前端逐行请求造成 N+1。
	if err = AttachUserSubscriptionMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = AttachUserSellableTokenMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = AttachUserIPBlacklistMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = AttachUserContentSafetyMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

type subscriptionQuotaSummary struct {
	Remaining    int64
	Total        int64
	HasUnlimited bool
}

// SubscriptionQuotaItem 是单条生效套餐的额度快照，用于用户管理列表的悬停详情。
type SubscriptionQuotaItem struct {
	PlanTitle     string `json:"plan_title"`
	Remaining     int64  `json:"remaining"`
	Total         int64  `json:"total"`
	HasUnlimited  bool   `json:"has_unlimited"`
	NextResetTime int64  `json:"next_reset_time"`
	ResetPeriod   string `json:"reset_period"`
}

func SearchUsers(keyword string, group string, startIdx int, num int) ([]*User, int64, error) {
	return SearchUsersWithParams(UserSearchParams{
		Keyword:   keyword,
		Group:     group,
		SortBy:    "id",
		SortOrder: "desc",
		StartIdx:  startIdx,
		PageSize:  num,
	})
}

func SearchUsersWithParams(params UserSearchParams) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 构建基础查询
	query := tx.Unscoped().Model(&User{})

	// 这里统一做输入清洗（去首尾空白），并且后续所有条件都使用 GORM 参数绑定写法，
	// 避免把用户输入直接拼接到 SQL 字符串里，从根源上规避 SQL 注入风险。
	keyword := strings.TrimSpace(params.Keyword)
	group := strings.TrimSpace(params.Group)
	if keyword != "" {
		likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"
		if keywordInt, parseErr := strconv.Atoi(keyword); parseErr == nil {
			likeCondition = "id = ? OR " + likeCondition
			query = query.Where(
				likeCondition,
				keywordInt, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%",
			)
		} else {
			query = query.Where(
				likeCondition,
				"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%",
			)
		}
	}

	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	if params.Role != nil {
		query = query.Where("role = ?", *params.Role)
	}
	if params.Status != nil {
		query = query.Where("status = ?", *params.Status)
	}
	if params.InviterID != nil {
		query = query.Where("inviter_id = ?", *params.InviterID)
	}
	if params.HasInviter != nil {
		if *params.HasInviter {
			query = query.Where("inviter_id > 0")
		} else {
			query = query.Where("inviter_id = 0")
		}
	}
	if params.HasInvitees != nil {
		if *params.HasInvitees {
			query = query.Where("aff_count > 0")
		} else {
			query = query.Where("aff_count = 0")
		}
	}
	if params.HasActiveSubscription != nil {
		now := common.GetTimestamp()
		// 套餐筛选统一按"生效套餐"口径：status=active 且 end_time > now。
		// 使用 EXISTS/NOT EXISTS 让三种数据库都能稳定执行，且不影响主表分页结构。
		activeSubExistsSQL := "SELECT 1 FROM user_subscriptions us WHERE us.user_id = users.id AND us.status = ? AND us.end_time > ?"
		if *params.HasActiveSubscription {
			query = query.Where("EXISTS ("+activeSubExistsSQL+")", "active", now)
		} else {
			query = query.Where("NOT EXISTS ("+activeSubExistsSQL+")", "active", now)
		}
	}
	if params.HasSellableToken != nil {
		sellableExistsSQL := "SELECT 1 FROM tokens t WHERE t.user_id = users.id AND t.source_type = 'sellable_token' AND t.status = 1 AND t.deleted_at IS NULL"
		if *params.HasSellableToken {
			query = query.Where("EXISTS (" + sellableExistsSQL + ")")
		} else {
			query = query.Where("NOT EXISTS (" + sellableExistsSQL + ")")
		}
	}
	// 钱包额度筛选只基于 users.quota，避免把套餐消耗混入钱包余额口径。
	if params.WalletMin != nil {
		query = query.Where("quota >= ?", *params.WalletMin)
	}
	if params.WalletMax != nil {
		query = query.Where("quota <= ?", *params.WalletMax)
	}
	if params.UsedBalanceMin != nil {
		query = query.Where("used_quota >= ?", *params.UsedBalanceMin)
	}
	if params.UsedBalanceMax != nil {
		query = query.Where("used_quota <= ?", *params.UsedBalanceMax)
	}
	registerSource := strings.TrimSpace(params.RegisterSource)
	if registerSource != "" {
		registerSource = NormalizeUserRegisterSource(registerSource)
		if registerSource == UserRegisterSourceUnknown {
			query = query.Where("(register_source = ? OR register_source = '' OR register_source IS NULL)", UserRegisterSourceUnknown)
		} else {
			query = query.Where("register_source = ?", registerSource)
		}
	}
	registerIP := strings.TrimSpace(params.RegisterIP)
	if registerIP != "" {
		query = query.Where("register_ip = ?", registerIP)
	}
	query = applyUserContentSafetyFilters(tx, query, params)
	if params.InviteeUserID != nil {
		// 通过子查询拿到"被邀请人 -> 邀请人ID"，然后反查邀请人用户。
		// 子查询同样走参数绑定，避免原始 SQL 拼接带来的注入风险。
		subQuery := tx.Unscoped().
			Model(&User{}).
			Select("inviter_id").
			Where("id = ?", *params.InviteeUserID).
			Limit(1)
		query = query.Where("id = (?)", subQuery)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 对分页参数做兜底，确保极端情况下不会出现负偏移或零分页大小。
	startIdx := params.StartIdx
	if startIdx < 0 {
		startIdx = 0
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}

	orderClause := buildUserOrderClause(params.SortBy, params.SortOrder, params.IdSortOrder, params.WalletSortOrder, params.UsedQuotaSortOrder)
	query, orderClause = applyUserContentSafetySort(tx, query, params.ContentSafetySortOrder, orderClause)

	// 获取分页数据
	err = query.Omit("password", "access_token").Order(orderClause).Limit(pageSize).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = AttachUserInviteMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = AttachUserSubscriptionMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = AttachUserSellableTokenMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = AttachUserIPBlacklistMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = AttachUserContentSafetyMetadata(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func AttachUserInviteMetadata(tx *gorm.DB, users []*User) error {
	if len(users) == 0 {
		return nil
	}

	inviterIDs := make([]int, 0, len(users))
	seen := make(map[int]struct{}, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		user.InviteeCount = user.AffCount
		if user.InviterId <= 0 {
			continue
		}
		if _, ok := seen[user.InviterId]; ok {
			continue
		}
		seen[user.InviterId] = struct{}{}
		inviterIDs = append(inviterIDs, user.InviterId)
	}
	if len(inviterIDs) == 0 {
		return nil
	}

	// 为当前页批量补充 inviter_username，避免前端逐行查询导致 N+1 问题。
	type inviterProjection struct {
		Id       int    `json:"id"`
		Username string `json:"username"`
	}
	var inviters []inviterProjection
	if err := tx.Unscoped().Model(&User{}).Select("id", "username").Where("id IN ?", inviterIDs).Find(&inviters).Error; err != nil {
		return err
	}
	nameByID := make(map[int]string, len(inviters))
	for _, inviter := range inviters {
		nameByID[inviter.Id] = inviter.Username
	}
	for _, user := range users {
		if user == nil || user.InviterId <= 0 {
			continue
		}
		user.InviterUsername = nameByID[user.InviterId]
	}
	return nil
}

func AttachUserIPBlacklistMetadata(tx *gorm.DB, users []*User) error {
	if len(users) == 0 {
		return nil
	}
	if !tx.Migrator().HasTable(&IPBlacklist{}) {
		return nil
	}
	for _, user := range users {
		if user == nil {
			continue
		}
		user.RegisterIPBlacklisted = false
		user.RegisterIPBlacklistRuleID = 0
		user.RegisterIPBlacklistCIDR = ""
		user.RegisterIPBlacklistReason = ""
	}

	var items []IPBlacklist
	if err := tx.Select("id", "cidr", "reason").Find(&items).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	rules := make([]CompiledIPBlacklistRule, 0, len(items))
	reasonByID := make(map[int]string, len(items))
	for _, item := range items {
		_, network, err := net.ParseCIDR(item.CIDR)
		if err != nil {
			continue
		}
		rules = append(rules, CompiledIPBlacklistRule{
			Id:      item.Id,
			CIDR:    item.CIDR,
			Network: network,
		})
		reasonByID[item.Id] = item.Reason
	}
	if len(rules) == 0 {
		return nil
	}

	for _, user := range users {
		if user == nil {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(user.RegisterIP))
		if ip == nil {
			continue
		}
		for _, rule := range rules {
			if rule.Network.Contains(ip) {
				user.RegisterIPBlacklisted = true
				user.RegisterIPBlacklistRuleID = rule.Id
				user.RegisterIPBlacklistCIDR = rule.CIDR
				user.RegisterIPBlacklistReason = reasonByID[rule.Id]
				break
			}
		}
	}
	return nil
}

func AttachUserSubscriptionMetadata(tx *gorm.DB, users []*User) error {
	if len(users) == 0 {
		return nil
	}
	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		userIDs = append(userIDs, user.Id)
		// 先写入默认值，确保"无套餐"用户也有明确响应字段。
		user.ActiveSubscriptionCount = 0
		user.HasActiveSubscription = false
		user.PendingSubscriptionIssuanceCount = 0
		user.SubscriptionQuotaRemaining = 0
		user.SubscriptionQuotaTotal = 0
		user.SubscriptionQuotaHasUnlimited = false
	}
	if len(userIDs) == 0 {
		return nil
	}

	now := common.GetTimestamp()

	var activeSubscriptions []UserSubscription
	if err := tx.Model(&UserSubscription{}).
		Where("user_id IN ? AND status = ? AND end_time > ?", userIDs, "active", now).
		Find(&activeSubscriptions).Error; err != nil {
		return err
	}
	type pendingIssuanceCount struct {
		UserId int `gorm:"column:user_id"`
		Count  int `gorm:"column:cnt"`
	}
	var pendingCounts []pendingIssuanceCount
	if tx.Migrator().HasTable(&SubscriptionIssuance{}) {
		if err := tx.Model(&SubscriptionIssuance{}).
			Select("user_id, COUNT(*) AS cnt").
			Where("user_id IN ? AND status = ?", userIDs, SubscriptionIssuanceStatusPending).
			Group("user_id").
			Scan(&pendingCounts).Error; err != nil {
			return err
		}
	}

	planIDs := make([]int, 0, len(activeSubscriptions))
	seenPlanIDs := make(map[int]struct{}, len(activeSubscriptions))
	countByUserID := make(map[int]int, len(activeSubscriptions))
	quotaSummaryByUserID := make(map[int]subscriptionQuotaSummary, len(activeSubscriptions))
	itemsByUserID := make(map[int][]SubscriptionQuotaItem, len(activeSubscriptions))
	for _, sub := range activeSubscriptions {
		countByUserID[sub.UserId]++
		if sub.PlanId > 0 {
			if _, ok := seenPlanIDs[sub.PlanId]; !ok {
				seenPlanIDs[sub.PlanId] = struct{}{}
				planIDs = append(planIDs, sub.PlanId)
			}
		}
	}
	pendingByUserID := make(map[int]int, len(pendingCounts))
	for _, row := range pendingCounts {
		pendingByUserID[row.UserId] = row.Count
	}

	planByID := make(map[int]*SubscriptionPlan, len(planIDs))
	if len(planIDs) > 0 {
		var plans []SubscriptionPlan
		if err := tx.Model(&SubscriptionPlan{}).Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
			return err
		}
		for index := range plans {
			plan := plans[index]
			planByID[plan.Id] = &plan
		}
	}
	for _, sub := range activeSubscriptions {
		summary := quotaSummaryByUserID[sub.UserId]
		plan := planByID[sub.PlanId]
		remaining, total, hasUnlimited := projectUserSubscriptionQuotaSnapshot(&sub, plan, now)

		planTitle := ""
		resetPeriod := "never"
		if plan != nil {
			planTitle = plan.Title
			resetPeriod = plan.QuotaResetPeriod
		}
		item := SubscriptionQuotaItem{
			PlanTitle:     planTitle,
			Remaining:     remaining,
			Total:         total,
			HasUnlimited:  hasUnlimited,
			NextResetTime: sub.NextResetTime,
			ResetPeriod:   resetPeriod,
		}
		itemsByUserID[sub.UserId] = append(itemsByUserID[sub.UserId], item)

		if hasUnlimited {
			summary.HasUnlimited = true
			summary.Remaining = 0
			summary.Total = 0
			quotaSummaryByUserID[sub.UserId] = summary
			continue
		}
		if !summary.HasUnlimited {
			summary.Remaining += remaining
			summary.Total += total
		}
		quotaSummaryByUserID[sub.UserId] = summary
	}
	for _, user := range users {
		if user == nil {
			continue
		}
		activeCount := countByUserID[user.Id]
		user.ActiveSubscriptionCount = activeCount
		user.PendingSubscriptionIssuanceCount = pendingByUserID[user.Id]
		user.HasActiveSubscription = activeCount > 0 || user.PendingSubscriptionIssuanceCount > 0
		quotaSummary := quotaSummaryByUserID[user.Id]
		user.SubscriptionQuotaRemaining = quotaSummary.Remaining
		user.SubscriptionQuotaTotal = quotaSummary.Total
		user.SubscriptionQuotaHasUnlimited = quotaSummary.HasUnlimited
		user.SubscriptionQuotaItems = itemsByUserID[user.Id]
	}
	return nil
}

func projectUserSubscriptionQuotaSnapshot(sub *UserSubscription, plan *SubscriptionPlan, now int64) (int64, int64, bool) {
	if sub == nil {
		return 0, 0, false
	}
	if sub.AmountTotal <= 0 {
		return 0, 0, true
	}
	used := projectUserSubscriptionUsedAtTime(sub, plan, now)
	if used < 0 {
		used = 0
	}
	remaining := sub.AmountTotal - used
	if remaining < 0 {
		remaining = 0
	}
	return remaining, sub.AmountTotal, false
}

func projectUserSubscriptionUsedAtTime(sub *UserSubscription, plan *SubscriptionPlan, now int64) int64 {
	if sub == nil {
		return 0
	}
	used := sub.AmountUsed
	if used < 0 {
		used = 0
	}
	if plan == nil || NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return used
	}
	if sub.NextResetTime > 0 && sub.NextResetTime > now {
		return used
	}
	baseUnix := sub.LastResetTime
	if baseUnix <= 0 {
		baseUnix = sub.StartTime
	}
	if baseUnix <= 0 {
		return used
	}
	base := time.Unix(baseUnix, 0)
	next := calcNextResetTime(base, plan, sub.EndTime)
	advanced := false
	for next > 0 && next <= now {
		advanced = true
		base = time.Unix(next, 0)
		next = calcNextResetTime(base, plan, sub.EndTime)
	}
	if advanced {
		return 0
	}
	return used
}

func AttachUserSellableTokenMetadata(tx *gorm.DB, users []*User) error {
	if len(users) == 0 {
		return nil
	}
	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		userIDs = append(userIDs, user.Id)
		user.HasSellableToken = false
		user.ActiveSellableTokenCount = 0
		user.PendingSellableIssuanceCount = 0
	}
	if len(userIDs) == 0 {
		return nil
	}

	type countRow struct {
		UserId int `gorm:"column:user_id"`
		Count  int `gorm:"column:cnt"`
	}
	var tokenCounts []countRow
	if tx.Migrator().HasTable(&Token{}) && tx.Migrator().HasColumn(&Token{}, "source_type") {
		if err := tx.Model(&Token{}).
			Select("user_id, COUNT(*) AS cnt").
			Where("user_id IN ? AND source_type = ?", userIDs, TokenSourceTypeSellableToken).
			Group("user_id").
			Scan(&tokenCounts).Error; err != nil {
			return err
		}
	}
	var pendingCounts []countRow
	if tx.Migrator().HasTable(&SellableTokenIssuance{}) {
		if err := tx.Model(&SellableTokenIssuance{}).
			Select("user_id, COUNT(*) AS cnt").
			Where("user_id IN ? AND status = ?", userIDs, SellableTokenIssuanceStatusPending).
			Group("user_id").
			Scan(&pendingCounts).Error; err != nil {
			return err
		}
	}

	tokenCountByUserID := make(map[int]int, len(tokenCounts))
	for _, row := range tokenCounts {
		tokenCountByUserID[row.UserId] = row.Count
	}
	pendingCountByUserID := make(map[int]int, len(pendingCounts))
	for _, row := range pendingCounts {
		pendingCountByUserID[row.UserId] = row.Count
	}
	for _, user := range users {
		if user == nil {
			continue
		}
		user.ActiveSellableTokenCount = tokenCountByUserID[user.Id]
		user.PendingSellableIssuanceCount = pendingCountByUserID[user.Id]
		user.HasSellableToken = user.ActiveSellableTokenCount > 0 || user.PendingSellableIssuanceCount > 0
	}
	return nil
}

func GetUserInviteRelations(userID int, startIdx int, pageSize int) (*User, *User, []*User, int64, *InviteIncomeSummary, error) {
	if userID <= 0 {
		return nil, nil, nil, 0, nil, errors.New("id 为空！")
	}
	if startIdx < 0 {
		startIdx = 0
	}
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}
	summary := &InviteIncomeSummary{}

	// 先查主用户，再查邀请人与被邀请人分页列表，方便前端一次渲染完整关系视图。
	user := &User{}
	if err := DB.Unscoped().Omit("password", "access_token").First(user, "id = ?", userID).Error; err != nil {
		return nil, nil, nil, 0, nil, err
	}
	user.InviteeCount = user.AffCount

	var inviter *User
	if user.InviterId > 0 {
		inviter = &User{}
		if err := DB.Unscoped().Omit("password", "access_token").First(inviter, "id = ?", user.InviterId).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil, nil, 0, nil, err
			}
			inviter = nil
		}
	}

	query := DB.Unscoped().Model(&User{}).Where("inviter_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, nil, nil, 0, nil, err
	}

	// 直接收益采用"当前配置口径"：
	// - 每个被邀请用户按 QuotaForInviter 计一次；
	// - 该口径实现简单、开销稳定，但不追溯历史配置变更。
	if common.QuotaForInviter > 0 {
		summary.DirectTotalQuota = int(total) * common.QuotaForInviter
	}
	// 充值返佣按台账汇总（仅统计 settled）：
	// - pending/skipped 不计入已到账收益；
	// - 这里按 inviter_user_id 聚合，得到"该用户作为邀请人"的总返佣。
	if err := DB.Model(&InviteCommissionLedger{}).
		Select("COALESCE(SUM(settled_quota), 0)").
		Where("inviter_user_id = ? AND status = ?", userID, InviteCommissionStatusSettled).
		Scan(&summary.RechargeTotalQuota).Error; err != nil {
		return nil, nil, nil, 0, nil, err
	}
	summary.TotalQuota = summary.DirectTotalQuota + summary.RechargeTotalQuota

	var invitees []*User
	if err := query.
		Omit("password", "access_token").
		Order("id desc").
		Limit(pageSize).
		Offset(startIdx).
		Find(&invitees).Error; err != nil {
		return nil, nil, nil, 0, nil, err
	}

	rechargeByInvitee := map[int]int{}
	inviteeIDs := make([]int, 0, len(invitees))
	for _, invitee := range invitees {
		if invitee == nil || invitee.Id <= 0 {
			continue
		}
		inviteeIDs = append(inviteeIDs, invitee.Id)
	}
	if len(inviteeIDs) > 0 {
		type inviteeRechargeRow struct {
			DirectInviteeUserID int `gorm:"column:direct_invitee_user_id"`
			RechargeTotalQuota  int `gorm:"column:recharge_total_quota"`
		}
		var rows []inviteeRechargeRow
		// 为当前页直接下级按邀请分支聚合返佣：
		// - 一级返佣归到付款用户本人；
		// - 二级返佣归到 A 的直接下级 B，避免 A 的汇总与分支明细不一致。
		branchExpression := "CASE WHEN direct_invitee_user_id > 0 THEN direct_invitee_user_id ELSE invitee_user_id END"
		if err := DB.Model(&InviteCommissionLedger{}).
			Select(branchExpression+" AS direct_invitee_user_id, COALESCE(SUM(settled_quota), 0) AS recharge_total_quota").
			Where("inviter_user_id = ? AND status = ?", userID, InviteCommissionStatusSettled).
			Where("(direct_invitee_user_id IN ? OR (direct_invitee_user_id = 0 AND invitee_user_id IN ?))", inviteeIDs, inviteeIDs).
			Group(branchExpression).
			Find(&rows).Error; err != nil {
			return nil, nil, nil, 0, nil, err
		}
		for _, row := range rows {
			rechargeByInvitee[row.DirectInviteeUserID] = row.RechargeTotalQuota
		}
	}
	for _, invitee := range invitees {
		if invitee != nil {
			invitee.InviteeCount = invitee.AffCount
			// 每个被邀请人对应一份"直接收益"（当前配置口径）。
			directIncome := 0
			if common.QuotaForInviter > 0 {
				directIncome = common.QuotaForInviter
			}
			invitee.InviteDirectIncomeQuota = directIncome
			// 充值返佣是按台账聚合后的真实已结算值。
			invitee.InviteRechargeCommissionQuota = rechargeByInvitee[invitee.Id]
			// 总收益用于前端主展示，拆分项用于 tooltip/明细展示。
			invitee.InviteTotalIncomeQuota = invitee.InviteDirectIncomeQuota + invitee.InviteRechargeCommissionQuota
		}
	}

	return user, inviter, invitees, total, summary, nil
}

func GetUserInviteRechargeCommissions(userID int, startIdx int, pageSize int) ([]*InviteRechargeCommissionInvitee, int64, *InviteRechargeCommissionSummary, error) {
	if userID <= 0 {
		return nil, 0, nil, errors.New("id 为空！")
	}
	if startIdx < 0 {
		startIdx = 0
	}
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}

	// 一级按实际付款用户汇总；二级按当前受益人的直接下级分支汇总。
	// 这样 A 只能看到自己已知的直接下级 B 及该分支产生的返佣，
	// 不会新增暴露 B 的下级 C 的逐人注册日期和充值行为。
	sourceUserExpression := "CASE WHEN ledgers.commission_level = 2 AND ledgers.direct_invitee_user_id > 0 THEN ledgers.direct_invitee_user_id ELSE ledgers.invitee_user_id END"
	commissionLevelExpression := "CASE WHEN ledgers.commission_level = 2 THEN 2 ELSE 1 END"
	groupedSources := DB.Table("invite_commission_ledgers AS ledgers").
		Select(sourceUserExpression+" AS source_user_id, "+commissionLevelExpression+" AS commission_level").
		Where("ledgers.inviter_user_id = ? AND ledgers.status = ?", userID, InviteCommissionStatusSettled).
		Group(sourceUserExpression + ", " + commissionLevelExpression)
	var total int64
	if err := DB.Table("(?) AS commission_sources", groupedSources).Count(&total).Error; err != nil {
		return nil, 0, nil, err
	}

	summary := &InviteRechargeCommissionSummary{}
	if err := DB.Model(&InviteCommissionLedger{}).
		Select("COALESCE(SUM(settled_quota), 0)").
		Where("inviter_user_id = ? AND status = ?", userID, InviteCommissionStatusSettled).
		Scan(&summary.RechargeTotalQuota).Error; err != nil {
		return nil, 0, nil, err
	}

	type inviteeRechargeRow struct {
		SourceUserID       int     `gorm:"column:source_user_id"`
		RegisteredAt       int64   `gorm:"column:registered_at"`
		CommissionLevel    int     `gorm:"column:commission_level"`
		RechargeTotalQuota int     `gorm:"column:recharge_total_quota"`
		RechargeTotalMoney float64 `gorm:"column:recharge_total_money"`
	}
	var rows []inviteeRechargeRow
	cnyPaymentMoneyExpr := topUpCNYPaymentMoneyExpr()
	if err := DB.Table("invite_commission_ledgers AS ledgers").
		Select(sourceUserExpression+` AS source_user_id,
			users.created_at AS registered_at,
			`+commissionLevelExpression+` AS commission_level,
			COALESCE(SUM(ledgers.settled_quota), 0) AS recharge_total_quota,
				COALESCE(SUM(`+cnyPaymentMoneyExpr+`), 0) AS recharge_total_money`).
		Joins("LEFT JOIN users ON users.id = "+sourceUserExpression).
		Joins("LEFT JOIN top_ups ON top_ups.trade_no = ledgers.topup_trade_no").
		Where("ledgers.inviter_user_id = ? AND ledgers.status = ?", userID, InviteCommissionStatusSettled).
		Group(sourceUserExpression + ", users.created_at, " + commissionLevelExpression).
		Order("source_user_id asc, commission_level asc").
		Limit(pageSize).
		Offset(startIdx).
		Find(&rows).Error; err != nil {
		return nil, 0, nil, err
	}

	items := make([]*InviteRechargeCommissionInvitee, 0, len(rows))
	for index, row := range rows {
		registeredDate := ""
		if row.RegisteredAt > 0 {
			registeredDate = time.Unix(row.RegisteredAt, 0).Format("2006-01-02")
		}
		items = append(items, &InviteRechargeCommissionInvitee{
			Alias:                   fmt.Sprintf("用户%d", startIdx+index+1),
			RegisteredDate:          registeredDate,
			CommissionLevel:         row.CommissionLevel,
			RechargeTotalMoney:      row.RechargeTotalMoney,
			RechargeCommissionQuota: row.RechargeTotalQuota,
		})
	}

	return items, total, summary, nil
}

type RebuildAffCountResult struct {
	UpdatedInviters int64 `json:"updated_inviters"`
	ResetRows       int64 `json:"reset_rows"`
}

func RebuildAffCount(targetUserID *int) (*RebuildAffCountResult, error) {
	// 该能力用于修复 aff_count 与真实邀请关系(inviter_id)不一致的问题。
	// 支持两种模式：
	// 1) 传入 targetUserID：仅重算单个邀请人的 aff_count。
	// 2) targetUserID 为空：全量重算所有用户的 aff_count。
	result := &RebuildAffCountResult{}
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if targetUserID != nil {
		if *targetUserID <= 0 {
			tx.Rollback()
			return nil, errors.New("user_id 无效")
		}

		var inviteeCount int64
		if err := tx.Unscoped().Model(&User{}).Where("inviter_id = ?", *targetUserID).Count(&inviteeCount).Error; err != nil {
			tx.Rollback()
			return nil, err
		}

		updateResult := tx.Unscoped().
			Model(&User{}).
			Where("id = ?", *targetUserID).
			Update("aff_count", int(inviteeCount))
		if updateResult.Error != nil {
			tx.Rollback()
			return nil, updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			tx.Rollback()
			return nil, gorm.ErrRecordNotFound
		}
		result.UpdatedInviters = 1
		if err := tx.Commit().Error; err != nil {
			return nil, err
		}
		return result, nil
	}

	// 全量模式：先按 inviter_id 聚合真实邀请数量，再回写 aff_count。
	// 这里全部使用 GORM 构造器与参数绑定，保证 SQLite/MySQL/PostgreSQL 兼容。
	type inviterCountRow struct {
		InviterID    int   `gorm:"column:inviter_id"`
		InviteeCount int64 `gorm:"column:invitee_count"`
	}
	var inviterCountRows []inviterCountRow
	if err := tx.Unscoped().
		Model(&User{}).
		Select("inviter_id, COUNT(*) AS invitee_count").
		Where("inviter_id > 0").
		Group("inviter_id").
		Find(&inviterCountRows).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 为了避免旧脏数据残留，先把非 0 的 aff_count 统一归零，再按聚合结果回填。
	resetResult := tx.Unscoped().Model(&User{}).Where("aff_count <> 0").Update("aff_count", 0)
	if resetResult.Error != nil {
		tx.Rollback()
		return nil, resetResult.Error
	}
	result.ResetRows = resetResult.RowsAffected

	var updatedInviters int64 = 0
	for _, row := range inviterCountRows {
		if row.InviterID <= 0 {
			continue
		}
		updateResult := tx.Unscoped().
			Model(&User{}).
			Where("id = ?", row.InviterID).
			Update("aff_count", int(row.InviteeCount))
		if updateResult.Error != nil {
			tx.Rollback()
			return nil, updateResult.Error
		}
		if updateResult.RowsAffected > 0 {
			updatedInviters += updateResult.RowsAffected
		}
	}
	result.UpdatedInviters = updatedInviters

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return result, nil
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password", "access_token").First(&user, "id = ?", id).Error
	}
	return &user, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	return GetUserIdByAffCodeWithContext(context.Background(), affCode)
}

func GetUserIdByAffCodeWithContext(ctx context.Context, affCode string) (int, error) {
	affCode = strings.TrimSpace(affCode)
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.WithContext(ctx).Select("id").First(&user, "aff_code = ? AND status = ?", affCode, common.UserStatusEnabled).Error
	return user.Id, err
}

func GenerateUniqueAffCode() (string, error) {
	return generateUniqueAffCode(DB)
}

func generateUniqueAffCode(db *gorm.DB) (string, error) {
	if db == nil {
		return "", errors.New("db is nil")
	}
	for i := 0; i < userAffCodeGenerateMaxRetry; i++ {
		code, err := generateAffCodeCandidate()
		if err != nil {
			return "", err
		}
		var count int64
		if err = db.Unscoped().Model(&User{}).Where("aff_code = ?", code).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", errors.New("生成邀请码失败，请重试")
}

// ---- 自定义短邀请码（单站邀请绑定）----

const customAffCodeMinLength = 4
const customAffCodeMaxLength = 32 // aff_code 字段为 varchar(32)

// affCodeReservedWords 保留字：避免与前端路由 / 短链 /i/ 段冲突或冒充系统页面
var affCodeReservedWords = map[string]bool{
	"admin": true, "register": true, "login": true, "api": true, "i": true,
	"console": true, "oauth": true, "pricing": true, "about": true, "docs": true,
}

// ValidateCustomAffCode 校验自定义邀请码：4~32 位、仅 [0-9a-zA-Z]、非保留字
func ValidateCustomAffCode(code string) error {
	code = strings.TrimSpace(code)
	if len(code) < customAffCodeMinLength || len(code) > customAffCodeMaxLength {
		return errors.New("邀请码长度需在 4~32 个字符之间")
	}
	for _, r := range code {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return errors.New("邀请码只能包含字母和数字")
		}
	}
	if affCodeReservedWords[strings.ToLower(code)] {
		return errors.New("该邀请码为保留字，请换一个")
	}
	return nil
}

// UpdateUserAffCode 设置用户自定义邀请码（全局唯一）。
// 返佣全链路只认 inviter_id（GetUserIdByAffCode 精确匹配 aff_code），与码长度/格式无关。
func UpdateUserAffCode(id int, code string) error {
	code = strings.TrimSpace(code)
	if err := ValidateCustomAffCode(code); err != nil {
		return err
	}
	// 唯一性预检：Unscoped 含软删用户、排除自己；并由 aff_code 唯一索引兜底
	var count int64
	if err := DB.Unscoped().Model(&User{}).Where("aff_code = ? AND id <> ?", code, id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该邀请码已被占用，请换一个")
	}
	return DB.Model(&User{}).Where("id = ?", id).Update("aff_code", code).Error
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.HardDelete()
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	// 检查quota是否小于最小额度
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为%s！", logger.LogQuota(int(common.QuotaPerUnit)))
	}

	// 开始数据库事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback() // 确保在函数退出时事务能回滚

	// 加锁查询用户以确保数据一致性；SQLite 由写事务串行化。
	query := tx
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(user, user.Id).Error
	if err != nil {
		return err
	}

	// 再次检查用户的AffQuota是否足够
	if user.AffQuota < quota {
		return errors.New("邀请额度不足！")
	}

	// 更新用户额度
	user.AffQuota -= quota
	user.TransferableQuota = EffectiveTransferableQuota(user.Quota, user.TransferableQuota)
	user.Quota += quota
	user.TransferableQuota += quota

	// 保存用户状态
	if err := tx.Save(user).Error; err != nil {
		return err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return err
	}
	// Redis 中可能包含尚未批量落库的并发扣费，只原子增加本次划转额度，
	// 避免用数据库快照覆盖缓存后吞掉这部分余额变化。
	if cacheErr := cacheIncrUserQuota(user.Id, int64(quota)); cacheErr != nil {
		common.SysLog("failed to increment user quota cache after affiliate transfer: " + cacheErr.Error())
	}
	return nil
}

func (user *User) Insert(inviterId int) error {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, inviterId)
	}); err != nil {
		return err
	}
	user.finalizeUserCreation()
	return nil
}

// InsertWithTx inserts a new user within an existing transaction.
// Invitation binding, counters and fixed rewards are committed atomically with the user.
func (user *User) InsertWithTx(tx *gorm.DB, inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.registrationBaseQuota = common.QuotaForNewUser
	user.Quota = user.registrationBaseQuota
	if strings.TrimSpace(user.AffCode) == "" {
		user.AffCode, err = generateUniqueAffCode(tx)
		if err != nil {
			return err
		}
	}
	decision, err := decideInviteBindingWithTx(tx, inviterId)
	if err != nil {
		return err
	}
	user.inviteBindingDecision = &decision
	user.InviterId = decision.EffectiveInviterID
	if user.InviterId != 0 {
		user.Quota += decision.InviteeRewardQuota
	}
	user.TransferableQuota = user.Quota
	user.RegisterSource = NormalizeUserRegisterSource(user.RegisterSource)

	// 初始化用户设置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		user.SetSetting(defaultSetting)
	}

	result := tx.Create(user)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// FinalizeOAuthUserCreation performs post-transaction tasks for OAuth user creation.
// This should be called after the transaction commits successfully.
func (user *User) FinalizeOAuthUserCreation(_ int) {
	user.finalizeUserCreation()
}

func (user *User) finalizeUserCreation() {
	if user.creationFinalized {
		return
	}
	user.creationFinalized = true

	// 用户创建成功后，根据角色初始化边栏配置
	var createdUser User
	if err := DB.Where("id = ?", user.Id).First(&createdUser).Error; err == nil {
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
		}
	}

	if user.registrationBaseQuota > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(user.registrationBaseQuota)))
	}
	decision := user.inviteBindingDecision
	if user.InviterId != 0 && decision != nil {
		if decision.InviteeRewardQuota > 0 {
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(decision.InviteeRewardQuota)))
		}
		if decision.InviterRewardQuota > 0 {
			RecordLog(user.InviterId, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", logger.LogQuota(decision.InviterRewardQuota)))
		}
	}
	if decision == nil {
		return
	}
	switch decision.Outcome {
	case inviteBindingOutcomeProbabilitySkipped:
		common.SysLog(fmt.Sprintf(
			"invite binding skipped: inviter_id=%d user_id=%d threshold=%d rate=%d aff_count=%d",
			decision.RequestedInviterID, user.Id, decision.Threshold, decision.RateAfterThreshold, decision.AffCountBefore,
		))
	case inviteBindingOutcomeRandomErrorSkipped:
		common.SysError(fmt.Sprintf(
			"invite binding skipped after random failure: inviter_id=%d user_id=%d error=%v",
			decision.RequestedInviterID, user.Id, decision.RandomError,
		))
	}
}

func (user *User) Update(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	newUser := *user
	currentUser := &User{}
	if err = DB.Select("id", "quota", "transferable_quota").First(currentUser, user.Id).Error; err != nil {
		return err
	}
	newUser.TransferableQuota = EffectiveTransferableQuota(currentUser.Quota, currentUser.TransferableQuota)
	if newUser.TransferableQuota > newUser.Quota {
		newUser.TransferableQuota = newUser.Quota
	}
	if newUser.TransferableQuota < 0 {
		newUser.TransferableQuota = 0
	}
	if err = DB.Transaction(func(tx *gorm.DB) error {
		if err := InvalidatePulsePaidFundingTx(tx, user.Id, "admin_user_update"); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", user.Id).Omit("pulse_paid_quota", "pulse_funding_epoch", "pulse_reward_hold", "content_safety_whitelisted").Updates(newUser).Error
	}); err != nil {
		return err
	}

	// Update cache
	return syncUserCacheByID(user.Id)
}

func (user *User) Edit(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}

	newUser := *user
	updates := map[string]interface{}{
		"username":     newUser.Username,
		"display_name": newUser.DisplayName,
		"group":        newUser.Group,
		"quota":        newUser.Quota,
		"remark":       newUser.Remark,
	}
	if updatePassword {
		updates["password"] = newUser.Password
	}

	currentUser := &User{}
	if err = DB.Select("id", "quota", "transferable_quota").First(currentUser, user.Id).Error; err != nil {
		return err
	}
	transferableQuota := EffectiveTransferableQuota(currentUser.Quota, currentUser.TransferableQuota)
	if newUser.Quota > currentUser.Quota {
		transferableQuota += newUser.Quota - currentUser.Quota
	}
	if transferableQuota > newUser.Quota {
		transferableQuota = newUser.Quota
	}
	if transferableQuota < 0 {
		transferableQuota = 0
	}
	updates["transferable_quota"] = transferableQuota
	if err = DB.Transaction(func(tx *gorm.DB) error {
		if err := InvalidatePulsePaidFundingTx(tx, user.Id, "admin_user_edit"); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", user.Id).Omit("pulse_paid_quota", "pulse_funding_epoch", "pulse_reward_hold", "content_safety_whitelisted").Updates(updates).Error
	}); err != nil {
		return err
	}

	// Update cache
	return syncUserCacheByID(user.Id)
}

func (user *User) ClearBinding(bindingType string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}

	bindingColumnMap := map[string]string{
		"email":    "email",
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}

	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}

	if err := DB.Model(&User{}).Where("id = ?", user.Id).Update(column, "").Error; err != nil {
		return err
	}

	if err := DB.Where("id = ?", user.Id).First(user).Error; err != nil {
		return err
	}

	return updateUserCache(*user)
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	if err := DB.Delete(user).Error; err != nil {
		return err
	}

	// 清除缓存
	return InvalidateUserAndTokenCaches(user.Id)
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	var tokens []Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		if common.RedisEnabled {
			if err := tx.Unscoped().Select("id", commonKeyCol).Where("user_id = ?", user.Id).Find(&tokens).Error; err != nil {
				return err
			}
		}
		if err := deleteUserAuthenticationData(tx, user.Id); err != nil {
			return err
		}
		return tx.Unscoped().Delete(user).Error
	})
	if err != nil {
		return err
	}
	if err := invalidateTokensCache(tokens); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate token cache after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateUserCache(user.Id); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate user cache after hard deleting user %d: %v", user.Id, err))
	}
	return nil
}

func deleteUserAuthenticationData(tx *gorm.DB, userId int) error {
	for _, authenticationData := range []any{
		&TwoFABackupCode{},
		&TwoFA{},
		&PasskeyCredential{},
		&Token{},
	} {
		if err := tx.Unscoped().Where("user_id = ?", userId).Delete(authenticationData).Error; err != nil {
			return err
		}
	}
	return deleteUserOAuthBindingsByUserId(tx, userId)
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field's value is 0, '', false or other zero values,
	// it won't be used to build query conditions
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return errors.New("用户名或密码为空")
	}
	// find buy username or email
	DB.Where("username = ? OR email = ?", username, username).First(user)
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != common.UserStatusEnabled {
		return errors.New("用户名或密码错误，或用户已被封禁")
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return errors.New("email 为空！")
	}
	DB.Where(User{Email: user.Email}).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return errors.New("GitHub id 为空！")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

// UpdateGitHubId updates the user's GitHub ID (used for migration from login to numeric ID)
func (user *User) UpdateGitHubId(newGitHubId string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}
	return DB.Model(user).Update("github_id", newGitHubId).Error
}

func (user *User) FillUserByDiscordId() error {
	if user.DiscordId == "" {
		return errors.New("discord id 为空！")
	}
	DB.Where(User{DiscordId: user.DiscordId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return errors.New("oidc id 为空！")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return errors.New("WeChat id 为空！")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByTelegramId() error {
	if user.TelegramId == "" {
		return errors.New("Telegram id 为空！")
	}
	err := DB.Where(User{TelegramId: user.TelegramId}).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return nil
}

func IsEmailAlreadyTaken(email string) bool {
	return DB.Unscoped().Where("email = ?", email).Find(&User{}).RowsAffected == 1
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Unscoped().Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Unscoped().Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsDiscordIdAlreadyTaken(discordId string) bool {
	return DB.Unscoped().Where("discord_id = ?", discordId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsTelegramIdAlreadyTaken(telegramId string) bool {
	return DB.Unscoped().Where("telegram_id = ?", telegramId).Find(&User{}).RowsAffected == 1
}

func ResetUserPasswordByEmail(email string, password string) error {
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	err = DB.Model(&User{}).Where("email = ?", email).Update("password", hashedPassword).Error
	return err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

//// IsUserEnabled checks user status from Redis first, falls back to DB if needed
//func IsUserEnabled(id int, fromDB bool) (status bool, err error) {
//	defer func() {
//		// Update Redis cache asynchronously on successful DB read
//		if shouldUpdateRedis(fromDB, err) {
//			gopool.Go(func() {
//				if err := updateUserStatusCache(id, status); err != nil {
//					common.SysError("failed to update user status cache: " + err.Error())
//				}
//			})
//		}
//	}()
//	if !fromDB && common.RedisEnabled {
//		// Try Redis first
//		status, err := getUserStatusCache(id)
//		if err == nil {
//			return status == common.UserStatusEnabled, nil
//		}
//		// Don't return error - fall through to DB
//	}
//	fromDB = true
//	var user User
//	err = DB.Where("id = ?", id).Select("status").Find(&user).Error
//	if err != nil {
//		return false, err
//	}
//
//	return user.Status == common.UserStatusEnabled, nil
//}

func ValidateAccessToken(token string) (user *User) {
	if token == "" {
		return nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user = &User{}
	if DB.Where("access_token = ?", token).First(user).RowsAffected == 1 {
		return user
	}
	return nil
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserQuotaCache(id, quota); err != nil {
					common.SysLog("failed to update user quota cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		quota, err := getUserQuotaCache(id)
		if err == nil {
			return quota, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, err
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserGroup gets group from Redis first, falls back to DB if needed
func GetUserGroup(id int, fromDB bool) (group string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserGroupCache(id, group); err != nil {
					common.SysLog("failed to update user group cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		group, err := getUserGroupCache(id)
		if err == nil {
			return group, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error
	if err != nil {
		return "", err
	}

	return group, nil
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	var setting string
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserSettingCache(id, setting); err != nil {
					common.SysLog("failed to update user setting cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		setting, err := getUserSettingCache(id)
		if err == nil {
			return setting, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	// can be nil setting
	var safeSetting sql.NullString
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&safeSetting).Error
	if err != nil {
		return settingMap, err
	}
	if safeSetting.Valid {
		setting = safeSetting.String
	} else {
		setting = ""
	}
	userBase := &UserBase{
		Setting: setting,
	}
	return userBase.GetSetting(), nil
}

// Wallet changes commit immediately: deferred debits cannot safely coexist with
// paid-source reservations, because a later batch could spend already-certified funds.
func IncreaseUserQuota(id int, quota int, db bool) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	return adjustUnattributedWalletQuota(id, quota, "unattributed_credit")
}

func increaseUserQuota(id int, quota int) error { return IncreaseUserQuota(id, quota, true) }

func DecreaseUserQuota(id int, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	return adjustUnattributedWalletQuota(id, -quota, "unattributed_debit")
}

func decreaseUserQuota(id int, quota int) error { return DecreaseUserQuota(id, quota) }

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func GetRootUser() (user *User) {
	DB.Where("role = ?", common.RoleRootUser).First(&user)
	return user
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

func updateUserUsedQuotaAndRequestCount(id int, quota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota and request count: " + err.Error())
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func UpdateUserUsedQuota(id int, quota int) {
	if quota == 0 {
		return
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		return
	}
	updateUserUsedQuota(id, quota)
}

func updateUserUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

// UpdateUserRequestCount 仅更新用户请求计数，不更新已使用额度。
// 用于可售令牌等仅令牌计费模式，避免令牌消费被计入用户钱包已使用额度。
func UpdateUserRequestCount(id int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserRequestCount(id, 1)
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		common.SysLog("failed to update user request count: " + err.Error())
	}
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserNameCache(id, username); err != nil {
					common.SysLog("failed to update user name cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string) bool {
	var user User
	err := DB.Unscoped().Where("linux_do_id = ?", linuxDOId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId() error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ?", user.LinuxDOId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}
