package common

import (
	"crypto/tls"
	//"os"
	//"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

var StartTime = time.Now().Unix() // unit: second
var Version = "v0.0.0"            // this hard coding will be replaced automatically when building, no need to manually change
var BuildCommit = ""
var BuildRepository = "QuantumNous/new-api"
var BuildBranch = "main"
var SystemName = "New API"
var Footer = ""
var Logo = ""
var TopUpLink = ""

// var ChatLink = ""
// var ChatLink2 = ""
var QuotaPerUnit = 500 * 1000.0 // $0.002 / 1K tokens
// 保留旧变量以兼容历史逻辑，实际展示由 general_setting.quota_display_type 控制
var DisplayInCurrencyEnabled = true
var DisplayTokenStatEnabled = true
var DrawingEnabled = true
var TaskEnabled = true
var DataExportEnabled = true
var DataExportInterval = 5         // unit: minute
var DataExportDefaultTime = "hour" // unit: minute
var DefaultCollapseSidebar = false // default value of collapse sidebar

// Any options with "Secret", "Token" in its key won't be return by GetOptions

var SessionSecret = uuid.New().String()
var CryptoSecret = uuid.New().String()

var OptionMap map[string]string
var OptionMapRWMutex sync.RWMutex

var ItemsPerPage = 10
var MaxRecentItems = 1000

var PasswordLoginEnabled = true
var PasswordRegisterEnabled = true
var EmailVerificationEnabled = false
var GitHubOAuthEnabled = false
var LinuxDOOAuthEnabled = false
var WeChatAuthEnabled = false
var TelegramOAuthEnabled = false
var TurnstileCheckEnabled = false
var RegisterEnabled = true

var EmailDomainRestrictionEnabled = false // 是否启用邮箱域名限制
var EmailAliasRestrictionEnabled = false  // 是否启用邮箱别名限制
var EmailDomainWhitelist = []string{
	"gmail.com",
	"163.com",
	"126.com",
	"qq.com",
	"outlook.com",
	"hotmail.com",
	"icloud.com",
	"yahoo.com",
	"foxmail.com",
}
var EmailLoginAuthServerList = []string{
	"smtp.sendcloud.net",
	"smtp.azurecomm.net",
}

var DebugEnabled bool
var MemoryCacheEnabled bool

var LogConsumeEnabled = true

var TLSInsecureSkipVerify bool
var InsecureTLSConfig = &tls.Config{InsecureSkipVerify: true}

var SMTPServer = ""
var SMTPPort = 587
var SMTPSSLEnabled = false
var SMTPForceAuthLogin = false
var SMTPAccount = ""
var SMTPFrom = ""
var SMTPToken = ""

var GitHubClientId = ""
var GitHubClientSecret = ""
var LinuxDOClientId = ""
var LinuxDOClientSecret = ""
var LinuxDOMinimumTrustLevel = 0

var WeChatServerAddress = ""
var WeChatServerToken = ""
var WeChatAccountQRCodeImageURL = ""

var TurnstileSiteKey = ""
var TurnstileSecretKey = ""

var TelegramBotToken = ""
var TelegramBotName = ""

var QuotaForNewUser = 0
var QuotaForInviter = 0
var QuotaForInvitee = 0

// InviterCommissionEnabled 是否启用“邀请充值返佣”功能。
// 默认 false：不开启时不会创建返佣台账，也不会触发日批结算发放。
var InviterCommissionEnabled = false

// InviterRechargeCommissionRate 邀请返佣比例，按“充值额度”计算。
// 取值建议 [0,1]：0.1 表示返佣 10%，0 表示不返佣。
var InviterRechargeCommissionRate = 0.05

// InviterRechargeSecondLevelCommissionRate 二级邀请返佣比例。
// A 邀请 B、B 邀请 C 时，C 的有效付费会按该比例向 A 返佣。
// 默认 0，保证升级后不会改变已有站点的返佣成本。
var InviterRechargeSecondLevelCommissionRate = 0.0

// InviterCommissionDailyCap 邀请人单日返佣上限。
// 0 表示不限制；大于 0 时超出上限部分会被跳过或截断（见返佣风控逻辑）。
var InviterCommissionDailyCap = 0

// WalletRedemptionDailyCreateLimit 普通用户单个自然日最多创建的钱包兑换码数量。
// 0 表示不限制。
var WalletRedemptionDailyCreateLimit = 100

// WalletRedemptionMinimumQuota 普通用户单个钱包兑换码的最低“闪电”额度。
// 0 表示不限制；管理员仍可创建更小额度。
var WalletRedemptionMinimumQuota = 10

// WalletRedemptionActiveLimit 单个用户最多同时保留的未使用钱包兑换码数量。
// 0 表示不限制。
var WalletRedemptionActiveLimit = 100

// WalletRedemptionDailyQuotaLimit 普通用户单个自然日最多转赠的“闪电”额度。
// 实际扣减时再按 QuotaPerUnit 换算，0 表示不限制。
var WalletRedemptionDailyQuotaLimit = 5000

// WalletRedemptionReviewDistinctCreatorThreshold 同一用户单日兑换小额钱包码时，
// 达到多少个不同创建人后进入人工复查。0 表示关闭复查触发。
var WalletRedemptionReviewDistinctCreatorThreshold = 3

// WalletRedemptionReviewSmallQuotaLimit 触发人工复查时“小额兑换码”的最大闪电额度。
// 0 表示关闭复查触发。
var WalletRedemptionReviewSmallQuotaLimit = 100

// WalletRedemptionAutoBindMaxAgeHours limits automatic inviter binding caused
// by wallet-funded redemption codes. A value of 0 disables this binding path;
// redemption itself remains available.
var WalletRedemptionAutoBindMaxAgeHours = 24

// InvoiceServiceFeeRate 发票申请手续费费率，按发票开票金额(元)计算。
// 取值建议 [0,1]：0.01 表示 1%，0 表示不收取手续费。
var InvoiceServiceFeeRate = 0.01
var ChannelDisableThreshold = 5.0
var AutomaticDisableChannelEnabled = false
var AutomaticEnableChannelEnabled = false
var QuotaRemindThreshold = 1000
var PreConsumedQuota = 500

// QuotaStabilityEnabled 是否开启额度防抖。
var QuotaStabilityEnabled = true

// QuotaInsufficientDBRecheckEnabled 报额度不足前，是否再查一次数据库。
var QuotaInsufficientDBRecheckEnabled = true

// MultiKeyCooldownSeconds Key 冷却时长（秒）。
var MultiKeyCooldownSeconds = 60

// MultiKeyStickySeconds Key 粘性时长（秒）。
var MultiKeyStickySeconds = 60

// MultiKeySameChannelFailoverOnce 额度错误时，是否同渠道立即重试一次。
var MultiKeySameChannelFailoverOnce = true

// SubscriptionRenewRespectPurchaseLimit 是否让“续费”也受每用户购买上限约束。
// false: 仅新购受限；true: 新购与续费都受限。
var SubscriptionRenewRespectPurchaseLimit = false

var RetryTimes = 0

//var RootUserEmail = ""

var IsMasterNode bool

var requestInterval int
var RequestInterval time.Duration

var SyncFrequency int // unit is second

var BatchUpdateEnabled = false
var BatchUpdateInterval int

var RelayTimeout int         // unit is second
var RelayIdleConnTimeout int // unit is second

var RelayResponseHeaderTimeout int // unit is second

var RelayImageResponseHeaderTimeout int // unit is second

// ImagePlaygroundPreferredOrigin 用于覆盖 image-playground 启动 URL 与 BaseURL 的 origin
// （为空表示按请求 Origin/Referer/Host/ServerAddress 推断，详见 controller.image_playground）。
// 由 performance_setting 同步写入，可通过管理后台 UI 修改。
var ImagePlaygroundPreferredOrigin string

// ReloadRelayHTTPClients 是 service 包提供的 relay HTTP 客户端重建钩子，
// 由 main.go 在启动时注册（避免 model/setting 包反向依赖 service 包）。
// 当 RelayResponseHeaderTimeout / RelayImageResponseHeaderTimeout 变更后调用即可热生效。
var ReloadRelayHTTPClients func()

var RelayMaxIdleConns int
var RelayMaxIdleConnsPerHost int

var GeminiSafetySetting string

// https://docs.cohere.com/docs/safety-modes Type; NONE/CONTEXTUAL/STRICT
var CohereSafetySetting string

const (
	RequestIdKey = "X-Oneapi-Request-Id"
)

const (
	RoleGuestUser  = 0
	RoleCommonUser = 1
	RoleAdminUser  = 10
	RoleRootUser   = 100
)

func IsValidateRole(role int) bool {
	return role == RoleGuestUser || role == RoleCommonUser || role == RoleAdminUser || role == RoleRootUser
}

var (
	FileUploadPermission    = RoleGuestUser
	FileDownloadPermission  = RoleGuestUser
	ImageUploadPermission   = RoleGuestUser
	ImageDownloadPermission = RoleGuestUser
)

// All duration's unit is seconds
// Shouldn't larger then RateLimitKeyExpirationDuration
var (
	GlobalApiRateLimitEnable   bool
	GlobalApiRateLimitNum      int
	GlobalApiRateLimitDuration int64

	GlobalWebRateLimitEnable   bool
	GlobalWebRateLimitNum      int
	GlobalWebRateLimitDuration int64

	GlobalStaticRateLimitEnable   bool
	GlobalStaticRateLimitNum      int
	GlobalStaticRateLimitDuration int64

	PublicTokenUsageRateLimitEnable   bool
	PublicTokenUsageRateLimitNum      int
	PublicTokenUsageRateLimitDuration int64

	CriticalRateLimitEnable   bool
	CriticalRateLimitNum            = 20
	CriticalRateLimitDuration int64 = 20 * 60

	// Pulse Benefit is a service-to-service endpoint. Keep its limit
	// independent from the stricter anonymous/critical API limit.
	PulseBenefitRateLimitEnable   bool
	PulseBenefitRateLimitNum            = 600
	PulseBenefitRateLimitDuration int64 = 60

	RegisterRateLimitNum            = 10
	RegisterRateLimitDuration int64 = 60 * 60 // 10 次/小时/IP

	UploadRateLimitNum            = 10
	UploadRateLimitDuration int64 = 60

	DownloadRateLimitNum            = 10
	DownloadRateLimitDuration int64 = 60

	// Per-user search rate limit (applies after authentication, keyed by user ID)
	SearchRateLimitNum            = 10
	SearchRateLimitDuration int64 = 60
)

var RateLimitKeyExpirationDuration = 20 * time.Minute

const (
	UserStatusEnabled  = 1 // don't use 0, 0 is the default value!
	UserStatusDisabled = 2 // also don't use 0
)

const (
	TokenStatusEnabled   = 1 // don't use 0, 0 is the default value!
	TokenStatusDisabled  = 2 // also don't use 0
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

const (
	RedemptionCodeStatusEnabled  = 1 // don't use 0, 0 is the default value!
	RedemptionCodeStatusDisabled = 2 // also don't use 0
	RedemptionCodeStatusUsed     = 3 // also don't use 0
)

const (
	ChannelStatusUnknown          = 0
	ChannelStatusEnabled          = 1 // don't use 0, 0 is the default value!
	ChannelStatusManuallyDisabled = 2 // also don't use 0
	ChannelStatusAutoDisabled     = 3
)

const (
	TopUpStatusPending = "pending"
	TopUpStatusSuccess = "success"
	TopUpStatusFailed  = "failed"
	TopUpStatusExpired = "expired"
)
