package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/performance_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"gorm.io/gorm"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

// inviteCommissionRateOptionMutex serializes the two economic rate settings.
// They are persisted as separate legacy options, but must be validated and
// published as one logical configuration inside a process.
var inviteCommissionRateOptionMutex sync.Mutex

// tieredBillingOptionMutex keeps persistence and runtime publication ordered.
// Without it, two concurrent saves can commit in one order and publish in the
// opposite order, leaving the process with a snapshot different from the DB.
var tieredBillingOptionMutex sync.Mutex

func AllOption() ([]*Option, error) {
	var options []*Option
	var err error
	err = DB.Find(&options).Error
	return options, err
}

func InitOptionMap() {
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)

	// 添加原有的系统配置
	common.OptionMap["FileUploadPermission"] = strconv.Itoa(common.FileUploadPermission)
	common.OptionMap["FileDownloadPermission"] = strconv.Itoa(common.FileDownloadPermission)
	common.OptionMap["ImageUploadPermission"] = strconv.Itoa(common.ImageUploadPermission)
	common.OptionMap["ImageDownloadPermission"] = strconv.Itoa(common.ImageDownloadPermission)
	common.OptionMap["PasswordLoginEnabled"] = strconv.FormatBool(common.PasswordLoginEnabled)
	common.OptionMap["PasswordRegisterEnabled"] = strconv.FormatBool(common.PasswordRegisterEnabled)
	common.OptionMap["EmailVerificationEnabled"] = strconv.FormatBool(common.EmailVerificationEnabled)
	common.OptionMap["GitHubOAuthEnabled"] = strconv.FormatBool(common.GitHubOAuthEnabled)
	common.OptionMap["LinuxDOOAuthEnabled"] = strconv.FormatBool(common.LinuxDOOAuthEnabled)
	common.OptionMap["TelegramOAuthEnabled"] = strconv.FormatBool(common.TelegramOAuthEnabled)
	common.OptionMap["WeChatAuthEnabled"] = strconv.FormatBool(common.WeChatAuthEnabled)
	common.OptionMap["TurnstileCheckEnabled"] = strconv.FormatBool(common.TurnstileCheckEnabled)
	common.OptionMap["RegisterEnabled"] = strconv.FormatBool(common.RegisterEnabled)
	common.OptionMap["AutomaticDisableChannelEnabled"] = strconv.FormatBool(common.AutomaticDisableChannelEnabled)
	common.OptionMap["AutomaticEnableChannelEnabled"] = strconv.FormatBool(common.AutomaticEnableChannelEnabled)
	common.OptionMap["LogConsumeEnabled"] = strconv.FormatBool(common.LogConsumeEnabled)
	common.OptionMap["ErrorLogEnabled"] = strconv.FormatBool(constant.ErrorLogEnabled)
	common.OptionMap["DisplayInCurrencyEnabled"] = strconv.FormatBool(common.DisplayInCurrencyEnabled)
	common.OptionMap["DisplayTokenStatEnabled"] = strconv.FormatBool(common.DisplayTokenStatEnabled)
	common.OptionMap["DrawingEnabled"] = strconv.FormatBool(common.DrawingEnabled)
	common.OptionMap["TaskEnabled"] = strconv.FormatBool(common.TaskEnabled)
	common.OptionMap["DataExportEnabled"] = strconv.FormatBool(common.DataExportEnabled)
	common.OptionMap["ChannelDisableThreshold"] = strconv.FormatFloat(common.ChannelDisableThreshold, 'f', -1, 64)
	common.OptionMap["EmailDomainRestrictionEnabled"] = strconv.FormatBool(common.EmailDomainRestrictionEnabled)
	common.OptionMap["EmailAliasRestrictionEnabled"] = strconv.FormatBool(common.EmailAliasRestrictionEnabled)
	common.OptionMap["EmailDomainWhitelist"] = strings.Join(common.EmailDomainWhitelist, ",")
	common.OptionMap["SMTPServer"] = ""
	common.OptionMap["SMTPFrom"] = ""
	common.OptionMap["SMTPPort"] = strconv.Itoa(common.SMTPPort)
	common.OptionMap["SMTPAccount"] = ""
	common.OptionMap["SMTPToken"] = ""
	common.OptionMap["SMTPSSLEnabled"] = strconv.FormatBool(common.SMTPSSLEnabled)
	common.OptionMap["SMTPForceAuthLogin"] = strconv.FormatBool(common.SMTPForceAuthLogin)
	common.OptionMap["Notice"] = ""
	common.OptionMap["About"] = ""
	common.OptionMap["HomePageContent"] = ""
	common.OptionMap["Footer"] = common.Footer
	common.OptionMap["SystemName"] = common.SystemName
	common.OptionMap["Logo"] = common.Logo
	common.OptionMap["ServerAddress"] = ""
	common.OptionMap["WorkerUrl"] = system_setting.WorkerUrl
	common.OptionMap["WorkerValidKey"] = system_setting.WorkerValidKey
	common.OptionMap["WorkerAllowHttpImageRequestEnabled"] = strconv.FormatBool(system_setting.WorkerAllowHttpImageRequestEnabled)
	common.OptionMap["PayAddress"] = ""
	common.OptionMap["CustomCallbackAddress"] = ""
	common.OptionMap["EpayId"] = ""
	common.OptionMap["EpayKey"] = ""
	common.OptionMap["Price"] = strconv.FormatFloat(operation_setting.Price, 'f', -1, 64)
	common.OptionMap["USDExchangeRate"] = strconv.FormatFloat(operation_setting.USDExchangeRate, 'f', -1, 64)
	common.OptionMap["MinTopUp"] = strconv.Itoa(operation_setting.MinTopUp)
	common.OptionMap["StripeMinTopUp"] = strconv.Itoa(setting.StripeMinTopUp)
	common.OptionMap["StripeApiSecret"] = setting.StripeApiSecret
	common.OptionMap["StripeWebhookSecret"] = setting.StripeWebhookSecret
	common.OptionMap["StripePriceId"] = setting.StripePriceId
	common.OptionMap["StripeUnitPrice"] = strconv.FormatFloat(setting.StripeUnitPrice, 'f', -1, 64)
	common.OptionMap["StripePromotionCodesEnabled"] = strconv.FormatBool(setting.StripePromotionCodesEnabled)
	common.OptionMap["StripeDisplayName"] = setting.StripeDisplayName
	common.OptionMap["StripeDisplayIcon"] = setting.StripeDisplayIcon
	common.OptionMap["CreemApiKey"] = setting.CreemApiKey
	common.OptionMap["CreemProducts"] = setting.CreemProducts
	common.OptionMap["CreemTestMode"] = strconv.FormatBool(setting.CreemTestMode)
	common.OptionMap["CreemWebhookSecret"] = setting.CreemWebhookSecret
	common.OptionMap["WaffoEnabled"] = strconv.FormatBool(setting.WaffoEnabled)
	common.OptionMap["WaffoApiKey"] = setting.WaffoApiKey
	common.OptionMap["WaffoPrivateKey"] = setting.WaffoPrivateKey
	common.OptionMap["WaffoPublicCert"] = setting.WaffoPublicCert
	common.OptionMap["WaffoSandboxPublicCert"] = setting.WaffoSandboxPublicCert
	common.OptionMap["WaffoSandboxApiKey"] = setting.WaffoSandboxApiKey
	common.OptionMap["WaffoSandboxPrivateKey"] = setting.WaffoSandboxPrivateKey
	common.OptionMap["WaffoSandbox"] = strconv.FormatBool(setting.WaffoSandbox)
	common.OptionMap["WaffoMerchantId"] = setting.WaffoMerchantId
	common.OptionMap["WaffoNotifyUrl"] = setting.WaffoNotifyUrl
	common.OptionMap["WaffoReturnUrl"] = setting.WaffoReturnUrl
	common.OptionMap["WaffoSubscriptionReturnUrl"] = setting.WaffoSubscriptionReturnUrl
	common.OptionMap["WaffoCurrency"] = setting.WaffoCurrency
	common.OptionMap["WaffoUnitPrice"] = strconv.FormatFloat(setting.WaffoUnitPrice, 'f', -1, 64)
	common.OptionMap["WaffoMinTopUp"] = strconv.Itoa(setting.WaffoMinTopUp)
	common.OptionMap["WaffoPayMethods"] = setting.WaffoPayMethods2JsonString()
	common.OptionMap["WaffoPancakeEnabled"] = strconv.FormatBool(setting.WaffoPancakeEnabled)
	common.OptionMap["WaffoPancakeSandbox"] = strconv.FormatBool(setting.WaffoPancakeSandbox)
	common.OptionMap["WaffoPancakeMerchantID"] = setting.WaffoPancakeMerchantID
	common.OptionMap["WaffoPancakePrivateKey"] = setting.WaffoPancakePrivateKey
	common.OptionMap["WaffoPancakeWebhookPublicKey"] = setting.WaffoPancakeWebhookPublicKey
	common.OptionMap["WaffoPancakeWebhookTestKey"] = setting.WaffoPancakeWebhookTestKey
	common.OptionMap["WaffoPancakeStoreID"] = setting.WaffoPancakeStoreID
	common.OptionMap["WaffoPancakeProductID"] = setting.WaffoPancakeProductID
	common.OptionMap["WaffoPancakeReturnURL"] = setting.WaffoPancakeReturnURL
	common.OptionMap["WaffoPancakeCurrency"] = setting.WaffoPancakeCurrency
	common.OptionMap["WaffoPancakeUnitPrice"] = strconv.FormatFloat(setting.WaffoPancakeUnitPrice, 'f', -1, 64)
	common.OptionMap["WaffoPancakeMinTopUp"] = strconv.Itoa(setting.WaffoPancakeMinTopUp)
	common.OptionMap["TopupGroupRatio"] = common.TopupGroupRatio2JSONString()
	common.OptionMap["Chats"] = setting.Chats2JsonString()
	common.OptionMap["AutoGroups"] = setting.AutoGroups2JsonString()
	common.OptionMap["DefaultUseAutoGroup"] = strconv.FormatBool(setting.DefaultUseAutoGroup)
	common.OptionMap["PayMethods"] = operation_setting.PayMethods2JsonString()
	common.OptionMap["GitHubClientId"] = ""
	common.OptionMap["GitHubClientSecret"] = ""
	common.OptionMap["TelegramBotToken"] = ""
	common.OptionMap["TelegramBotName"] = ""
	common.OptionMap["WeChatServerAddress"] = ""
	common.OptionMap["WeChatServerToken"] = ""
	common.OptionMap["WeChatAccountQRCodeImageURL"] = ""
	common.OptionMap["TurnstileSiteKey"] = ""
	common.OptionMap["TurnstileSecretKey"] = ""
	common.OptionMap["QuotaForNewUser"] = strconv.Itoa(common.QuotaForNewUser)
	common.OptionMap["QuotaForInviter"] = strconv.Itoa(common.QuotaForInviter)
	common.OptionMap["QuotaForInvitee"] = strconv.Itoa(common.QuotaForInvitee)
	common.OptionMap["InviteBindingSettings"] = common.InviteBindingSettings2JSONString()
	// 邀请充值返佣配置：
	// - InviterCommissionEnabled: 开关
	// - InviterRechargeCommissionRate: 一级比例（如 0.1 = 10%）
	// - InviterRechargeSecondLevelCommissionRate: 二级比例（如 0.05 = 5%）
	// - InviterCommissionDailyCap: 单日上限（0 表示不限）
	common.OptionMap["InviterCommissionEnabled"] = strconv.FormatBool(common.InviterCommissionEnabled)
	common.OptionMap["InviterRechargeCommissionRate"] = strconv.FormatFloat(common.InviterRechargeCommissionRate, 'f', -1, 64)
	common.OptionMap["InviterRechargeSecondLevelCommissionRate"] = strconv.FormatFloat(common.InviterRechargeSecondLevelCommissionRate, 'f', -1, 64)
	common.OptionMap["InviterCommissionDailyCap"] = strconv.Itoa(common.InviterCommissionDailyCap)
	common.OptionMap["WalletRedemptionDailyCreateLimit"] = strconv.Itoa(common.WalletRedemptionDailyCreateLimit)
	common.OptionMap["WalletRedemptionMinimumQuota"] = strconv.Itoa(common.WalletRedemptionMinimumQuota)
	common.OptionMap["WalletRedemptionActiveLimit"] = strconv.Itoa(common.WalletRedemptionActiveLimit)
	common.OptionMap["WalletRedemptionDailyQuotaLimit"] = strconv.Itoa(common.WalletRedemptionDailyQuotaLimit)
	common.OptionMap["WalletRedemptionReviewDistinctCreatorThreshold"] = strconv.Itoa(common.WalletRedemptionReviewDistinctCreatorThreshold)
	common.OptionMap["WalletRedemptionReviewSmallQuotaLimit"] = strconv.Itoa(common.WalletRedemptionReviewSmallQuotaLimit)
	common.OptionMap["WalletRedemptionAutoBindMaxAgeHours"] = strconv.Itoa(common.WalletRedemptionAutoBindMaxAgeHours)
	common.OptionMap["InvoiceServiceFeeRate"] = strconv.FormatFloat(common.InvoiceServiceFeeRate, 'f', -1, 64)
	common.OptionMap["QuotaRemindThreshold"] = strconv.Itoa(common.QuotaRemindThreshold)
	common.OptionMap["PreConsumedQuota"] = strconv.Itoa(common.PreConsumedQuota)
	common.OptionMap["QuotaStabilityEnabled"] = strconv.FormatBool(common.QuotaStabilityEnabled)
	common.OptionMap["QuotaInsufficientDBRecheckEnabled"] = strconv.FormatBool(common.QuotaInsufficientDBRecheckEnabled)
	common.OptionMap["MultiKeySameChannelFailoverOnce"] = strconv.FormatBool(common.MultiKeySameChannelFailoverOnce)
	common.OptionMap["SubscriptionRenewRespectPurchaseLimit"] = strconv.FormatBool(common.SubscriptionRenewRespectPurchaseLimit)
	common.OptionMap["MultiKeyCooldownSeconds"] = strconv.Itoa(common.MultiKeyCooldownSeconds)
	common.OptionMap["MultiKeyStickySeconds"] = strconv.Itoa(common.MultiKeyStickySeconds)
	common.OptionMap["ModelRequestRateLimitCount"] = strconv.Itoa(setting.ModelRequestRateLimitCount)
	common.OptionMap["ModelRequestRateLimitDurationMinutes"] = strconv.Itoa(setting.ModelRequestRateLimitDurationMinutes)
	common.OptionMap["ModelRequestRateLimitSuccessCount"] = strconv.Itoa(setting.ModelRequestRateLimitSuccessCount)
	common.OptionMap["ModelRequestRateLimitGroup"] = setting.ModelRequestRateLimitGroup2JSONString()
	common.OptionMap["RedemptionRateLimitDurationSeconds"] = strconv.Itoa(setting.RedemptionRateLimitDurationSeconds)
	common.OptionMap["RedemptionRateLimitSuccessCount"] = strconv.Itoa(setting.RedemptionRateLimitSuccessCount)
	common.OptionMap["RedemptionRateLimitFailureCount"] = strconv.Itoa(setting.RedemptionRateLimitFailureCount)
	common.OptionMap["ModelRatio"] = ratio_setting.ModelRatio2JSONString()
	common.OptionMap["ModelPrice"] = ratio_setting.ModelPrice2JSONString()
	common.OptionMap["CacheRatio"] = ratio_setting.CacheRatio2JSONString()
	common.OptionMap["CreateCacheRatio"] = ratio_setting.CreateCacheRatio2JSONString()
	common.OptionMap["GroupRatio"] = ratio_setting.GroupRatio2JSONString()
	common.OptionMap["GroupGroupRatio"] = ratio_setting.GroupGroupRatio2JSONString()
	common.OptionMap["TimeRatioRules"] = ratio_setting.TimeRatioRules2JSONString()
	common.OptionMap["UserUsableGroups"] = setting.UserUsableGroups2JSONString()
	common.OptionMap["CompletionRatio"] = ratio_setting.CompletionRatio2JSONString()
	common.OptionMap["ImageRatio"] = ratio_setting.ImageRatio2JSONString()
	common.OptionMap["AudioRatio"] = ratio_setting.AudioRatio2JSONString()
	common.OptionMap["AudioCompletionRatio"] = ratio_setting.AudioCompletionRatio2JSONString()
	common.OptionMap["AudioDurationPrice"] = ratio_setting.AudioDurationPrice2JSONString()
	common.OptionMap["TopUpLink"] = common.TopUpLink
	//common.OptionMap["ChatLink"] = common.ChatLink
	//common.OptionMap["ChatLink2"] = common.ChatLink2
	common.OptionMap["QuotaPerUnit"] = strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64)
	common.OptionMap["RetryTimes"] = strconv.Itoa(common.RetryTimes)
	common.OptionMap["ResponsesRequestBodyLimitMB"] = strconv.Itoa(constant.ResponsesRequestBodyLimitMB)
	common.OptionMap["DataExportInterval"] = strconv.Itoa(common.DataExportInterval)
	common.OptionMap["DataExportDefaultTime"] = common.DataExportDefaultTime
	common.OptionMap["DefaultCollapseSidebar"] = strconv.FormatBool(common.DefaultCollapseSidebar)
	common.OptionMap["MjNotifyEnabled"] = strconv.FormatBool(setting.MjNotifyEnabled)
	common.OptionMap["MjAccountFilterEnabled"] = strconv.FormatBool(setting.MjAccountFilterEnabled)
	common.OptionMap["MjModeClearEnabled"] = strconv.FormatBool(setting.MjModeClearEnabled)
	common.OptionMap["MjForwardUrlEnabled"] = strconv.FormatBool(setting.MjForwardUrlEnabled)
	common.OptionMap["MjActionCheckSuccessEnabled"] = strconv.FormatBool(setting.MjActionCheckSuccessEnabled)
	common.OptionMap["CheckSensitiveEnabled"] = strconv.FormatBool(setting.CheckSensitiveEnabled)
	common.OptionMap["DemoSiteEnabled"] = strconv.FormatBool(operation_setting.DemoSiteEnabled)
	common.OptionMap["SelfUseModeEnabled"] = strconv.FormatBool(operation_setting.SelfUseModeEnabled)
	common.OptionMap["ModelRequestRateLimitEnabled"] = strconv.FormatBool(setting.ModelRequestRateLimitEnabled)
	common.OptionMap["RedemptionRateLimitEnabled"] = strconv.FormatBool(setting.RedemptionRateLimitEnabled)
	common.OptionMap["CheckSensitiveOnPromptEnabled"] = strconv.FormatBool(setting.CheckSensitiveOnPromptEnabled)
	common.OptionMap["StopOnSensitiveEnabled"] = strconv.FormatBool(setting.StopOnSensitiveEnabled)
	common.OptionMap["SensitiveWords"] = setting.SensitiveWordsToString()
	common.OptionMap["StreamCacheQueueLength"] = strconv.Itoa(setting.StreamCacheQueueLength)
	common.OptionMap["AutomaticDisableKeywords"] = operation_setting.AutomaticDisableKeywordsToString()
	common.OptionMap["AutomaticDisableStatusCodes"] = operation_setting.AutomaticDisableStatusCodesToString()
	common.OptionMap["AutomaticRetryStatusCodes"] = operation_setting.AutomaticRetryStatusCodesToString()
	common.OptionMap["UserScopedCircuitBreakerEnabled"] = strconv.FormatBool(operation_setting.UserScopedCircuitBreakerEnabled)
	common.OptionMap["UserScopedCircuitBreakerStatusCodes"] = operation_setting.UserScopedCircuitBreakerStatusCodesToString()
	common.OptionMap["UserScopedCircuitBreakerTTLSeconds"] = strconv.Itoa(operation_setting.UserScopedCircuitBreakerTTLSeconds)
	common.OptionMap["UserScopedCircuitBreakerFailureThreshold"] = strconv.Itoa(operation_setting.UserScopedCircuitBreakerFailureThreshold)
	common.OptionMap["ExposeRatioEnabled"] = strconv.FormatBool(ratio_setting.IsExposeRatioEnabled())

	// 自动添加所有注册的模型配置
	modelConfigs := config.GlobalConfig.ExportAllConfigs()
	for k, v := range modelConfigs {
		common.OptionMap[k] = v
	}

	common.OptionMapRWMutex.Unlock()
	loadOptionsFromDatabase()
}

func loadOptionsFromDatabase() {
	pulseConfigOptionMutex.Lock()
	defer pulseConfigOptionMutex.Unlock()
	tieredBillingOptionMutex.Lock()
	options, err := AllOption()
	if err != nil {
		tieredBillingOptionMutex.Unlock()
		common.SysLog("failed to load options from database; retaining current configuration")
		return
	}
	loadPulseConfigOptions(options)
	loadTieredBillingOptions(options)
	tieredBillingOptionMutex.Unlock()
	for _, option := range options {
		if common.IsPulseConfigKey(option.Key) || option.Key == "billing_setting.billing_mode" || option.Key == "billing_setting.billing_expr" {
			continue
		}
		err := updateOptionMap(option.Key, option.Value)
		if err != nil {
			common.SysLog("failed to update option map: " + err.Error())
		}
	}
}

func loadTieredBillingOptions(options []*Option) {
	modeBytes, err := common.Marshal(billing_setting.GetBillingModeCopy())
	if err != nil {
		common.SysLog("failed to serialize current tiered billing modes: " + err.Error())
		return
	}
	exprBytes, err := common.Marshal(billing_setting.GetBillingExprCopy())
	if err != nil {
		common.SysLog("failed to serialize current tiered billing expressions: " + err.Error())
		return
	}
	modeRaw := string(modeBytes)
	exprRaw := string(exprBytes)
	found := false
	for _, option := range options {
		switch option.Key {
		case "billing_setting.billing_mode":
			modeRaw = option.Value
			found = true
		case "billing_setting.billing_expr":
			exprRaw = option.Value
			found = true
		}
	}
	if !found {
		return
	}
	bundle, err := billing_setting.ParseAndValidateFields(modeRaw, exprRaw)
	if err != nil {
		common.SysLog("failed to load tiered billing options: " + err.Error())
		return
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["billing_setting.billing_mode"] = modeRaw
	common.OptionMap["billing_setting.billing_expr"] = exprRaw
	common.OptionMapRWMutex.Unlock()
	billing_setting.ReplaceBundle(bundle)
}

func SyncOptions(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing options from database")
		loadOptionsFromDatabase()
	}
}

func validatePulseUsageLogOption(key string, value string) error {
	if key == "LogConsumeEnabled" && value != "true" && common.PulseUsageLogsRequired() {
		return errors.New("Meta Pulse 已启用，不能关闭消费日志（Pulse 消费日志保护已开启）")
	}
	return nil
}

func UpdateOption(key string, value string) error {
	if err := setting.ValidateStripeDisplayOption(key, value); err != nil {
		return err
	}
	if common.IsPulseConfigKey(key) {
		return errors.New("请通过 Meta Pulse 专用设置一次性保存配置")
	}
	if key == "LogConsumeEnabled" {
		return updatePulseConsumeLogOption(value)
	}
	if err := validatePulseUsageLogOption(key, value); err != nil {
		return err
	}
	isTieredBillingField := key == "billing_setting.billing_mode" || key == "billing_setting.billing_expr"
	if isTieredBillingField {
		tieredBillingOptionMutex.Lock()
		defer tieredBillingOptionMutex.Unlock()
	}
	isWalletRedemptionPolicy := isWalletRedemptionPolicyField(key)
	if isWalletRedemptionPolicy {
		walletRedemptionPolicyMutex.Lock()
		defer walletRedemptionPolicyMutex.Unlock()
	}
	isInviteCommissionRate := isInviteCommissionRateOption(key)
	if isInviteCommissionRate {
		inviteCommissionRateOptionMutex.Lock()
		defer inviteCommissionRateOptionMutex.Unlock()
	}
	if err := validateInviteCommissionRateOption(key, value); err != nil {
		return err
	}
	if err := validateInviteCommissionDailyCapOption(key, value); err != nil {
		return err
	}
	if err := validateWalletRedemptionPolicyOption(key, value); err != nil {
		return err
	}
	if strings.HasPrefix(key, "billing_setting.") {
		field := strings.TrimPrefix(key, "billing_setting.")
		if field == billing_setting.BillingModeField || field == billing_setting.BillingExprField {
			if err := billing_setting.ValidateFieldUpdate(field, value); err != nil {
				return err
			}
		}
	}
	// Save to database first
	option := Option{
		Key: key,
	}
	// https://gorm.io/docs/update.html#Save-All-Fields
	if err := DB.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
		return err
	}
	option.Value = value
	// Save is a combination function.
	// If save value does not contain primary key, it will execute Create,
	// otherwise it will execute Update (with all fields).
	if err := DB.Save(&option).Error; err != nil {
		return err
	}
	// Update OptionMap
	if isInviteCommissionRate || isWalletRedemptionPolicy {
		return updateOptionMapUnlocked(key, value)
	}
	return updateOptionMap(key, value)
}

func UpdateTieredBillingOptions(raw string) error {
	tieredBillingOptionMutex.Lock()
	defer tieredBillingOptionMutex.Unlock()

	bundle, err := billing_setting.ParseAndValidateBundle(raw)
	if err != nil {
		return err
	}
	modeBytes, err := common.Marshal(bundle.BillingMode)
	if err != nil {
		return err
	}
	exprBytes, err := common.Marshal(bundle.BillingExpr)
	if err != nil {
		return err
	}
	modeRaw := string(modeBytes)
	exprRaw := string(exprBytes)

	err = DB.Transaction(func(tx *gorm.DB) error {
		for key, value := range map[string]string{
			"billing_setting.billing_mode": modeRaw,
			"billing_setting.billing_expr": exprRaw,
		} {
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = value
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["billing_setting.billing_mode"] = modeRaw
	common.OptionMap["billing_setting.billing_expr"] = exprRaw
	common.OptionMapRWMutex.Unlock()
	billing_setting.ReplaceBundle(bundle)
	return nil
}

func updateOptionMap(key string, value string) (err error) {
	if isWalletRedemptionPolicyField(key) {
		walletRedemptionPolicyMutex.Lock()
		defer walletRedemptionPolicyMutex.Unlock()
	}
	if isInviteCommissionRateOption(key) {
		inviteCommissionRateOptionMutex.Lock()
		defer inviteCommissionRateOptionMutex.Unlock()
	}
	return updateOptionMapUnlocked(key, value)
}

func updateOptionMapUnlocked(key string, value string) (err error) {
	if err = setting.ValidateStripeDisplayOption(key, value); err != nil {
		return err
	}
	if err = validatePulseUsageLogOption(key, value); err != nil {
		return err
	}
	// Validate the combined settings before publishing either snapshot. This
	// prevents a malformed persisted value from replacing the last good value.
	if key == "InviteBindingSettings" {
		if _, err = common.ParseInviteBindingSettings(value); err != nil {
			return err
		}
	}
	if err = validateInviteCommissionRateOption(key, value); err != nil {
		return err
	}
	if err = validateInviteCommissionDailyCapOption(key, value); err != nil {
		return err
	}
	if err = validateWalletRedemptionPolicyOption(key, value); err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	common.OptionMap[key] = value

	// 检查是否是模型配置 - 使用更规范的方式处理
	if handled, configErr := handleConfigUpdate(key, value); handled {
		if configErr != nil {
			return configErr
		}
		return nil // 已由配置系统处理
	}

	// 处理传统配置项...
	if strings.HasSuffix(key, "Permission") {
		intValue, _ := strconv.Atoi(value)
		switch key {
		case "FileUploadPermission":
			common.FileUploadPermission = intValue
		case "FileDownloadPermission":
			common.FileDownloadPermission = intValue
		case "ImageUploadPermission":
			common.ImageUploadPermission = intValue
		case "ImageDownloadPermission":
			common.ImageDownloadPermission = intValue
		}
	}
	if strings.HasSuffix(key, "Enabled") || key == "DefaultCollapseSidebar" || key == "DefaultUseAutoGroup" || key == "SMTPForceAuthLogin" {
		boolValue := value == "true"
		switch key {
		case "PasswordRegisterEnabled":
			common.PasswordRegisterEnabled = boolValue
		case "PasswordLoginEnabled":
			common.PasswordLoginEnabled = boolValue
		case "EmailVerificationEnabled":
			common.EmailVerificationEnabled = boolValue
		case "GitHubOAuthEnabled":
			common.GitHubOAuthEnabled = boolValue
		case "LinuxDOOAuthEnabled":
			common.LinuxDOOAuthEnabled = boolValue
		case "WeChatAuthEnabled":
			common.WeChatAuthEnabled = boolValue
		case "TelegramOAuthEnabled":
			common.TelegramOAuthEnabled = boolValue
		case "TurnstileCheckEnabled":
			common.TurnstileCheckEnabled = boolValue
		case "RegisterEnabled":
			common.RegisterEnabled = boolValue
		case "EmailDomainRestrictionEnabled":
			common.EmailDomainRestrictionEnabled = boolValue
		case "EmailAliasRestrictionEnabled":
			common.EmailAliasRestrictionEnabled = boolValue
		case "AutomaticDisableChannelEnabled":
			common.AutomaticDisableChannelEnabled = boolValue
		case "AutomaticEnableChannelEnabled":
			common.AutomaticEnableChannelEnabled = boolValue
		case "LogConsumeEnabled":
			common.LogConsumeEnabled = boolValue
		case "ErrorLogEnabled":
			constant.ErrorLogEnabled = boolValue
		case "DisplayInCurrencyEnabled":
			// 兼容旧字段：同步到新配置 general_setting.quota_display_type（运行时生效）
			// true -> USD, false -> TOKENS
			newVal := "USD"
			if !boolValue {
				newVal = "TOKENS"
			}
			if cfg := config.GlobalConfig.Get("general_setting"); cfg != nil {
				_ = config.UpdateConfigFromMap(cfg, map[string]string{"quota_display_type": newVal})
			}
		case "DisplayTokenStatEnabled":
			common.DisplayTokenStatEnabled = boolValue
		case "DrawingEnabled":
			common.DrawingEnabled = boolValue
		case "TaskEnabled":
			common.TaskEnabled = boolValue
		case "DataExportEnabled":
			common.DataExportEnabled = boolValue
		case "DefaultCollapseSidebar":
			common.DefaultCollapseSidebar = boolValue
		case "MjNotifyEnabled":
			setting.MjNotifyEnabled = boolValue
		case "MjAccountFilterEnabled":
			setting.MjAccountFilterEnabled = boolValue
		case "MjModeClearEnabled":
			setting.MjModeClearEnabled = boolValue
		case "MjForwardUrlEnabled":
			setting.MjForwardUrlEnabled = boolValue
		case "MjActionCheckSuccessEnabled":
			setting.MjActionCheckSuccessEnabled = boolValue
		case "CheckSensitiveEnabled":
			setting.CheckSensitiveEnabled = boolValue
		case "DemoSiteEnabled":
			operation_setting.DemoSiteEnabled = boolValue
		case "SelfUseModeEnabled":
			operation_setting.SelfUseModeEnabled = boolValue
		case "UserScopedCircuitBreakerEnabled":
			operation_setting.UserScopedCircuitBreakerEnabled = boolValue
		case "CheckSensitiveOnPromptEnabled":
			setting.CheckSensitiveOnPromptEnabled = boolValue
		case "ModelRequestRateLimitEnabled":
			setting.ModelRequestRateLimitEnabled = boolValue
		case "RedemptionRateLimitEnabled":
			setting.RedemptionRateLimitEnabled = boolValue
		case "StopOnSensitiveEnabled":
			setting.StopOnSensitiveEnabled = boolValue
		case "SMTPSSLEnabled":
			common.SMTPSSLEnabled = boolValue
		case "SMTPForceAuthLogin":
			common.SMTPForceAuthLogin = boolValue
		case "WorkerAllowHttpImageRequestEnabled":
			system_setting.WorkerAllowHttpImageRequestEnabled = boolValue
		case "DefaultUseAutoGroup":
			setting.DefaultUseAutoGroup = boolValue
		case "ExposeRatioEnabled":
			ratio_setting.SetExposeRatioEnabled(boolValue)
		case "InviterCommissionEnabled":
			common.InviterCommissionEnabled = boolValue
		case "QuotaStabilityEnabled":
			common.QuotaStabilityEnabled = boolValue
		case "QuotaInsufficientDBRecheckEnabled":
			common.QuotaInsufficientDBRecheckEnabled = boolValue
		case "MultiKeySameChannelFailoverOnce":
			common.MultiKeySameChannelFailoverOnce = boolValue
		case "SubscriptionRenewRespectPurchaseLimit":
			common.SubscriptionRenewRespectPurchaseLimit = boolValue
		}
	}
	switch key {
	case "EmailDomainWhitelist":
		common.EmailDomainWhitelist = strings.Split(value, ",")
	case "SMTPServer":
		common.SMTPServer = value
	case "SMTPPort":
		intValue, _ := strconv.Atoi(value)
		common.SMTPPort = intValue
	case "SMTPAccount":
		common.SMTPAccount = value
	case "SMTPFrom":
		common.SMTPFrom = value
	case "SMTPToken":
		common.SMTPToken = value
	case "ServerAddress":
		system_setting.ServerAddress = value
	case "WorkerUrl":
		system_setting.WorkerUrl = value
	case "WorkerValidKey":
		system_setting.WorkerValidKey = value
	case "PayAddress":
		operation_setting.PayAddress = value
	case "Chats":
		err = setting.UpdateChatsByJsonString(value)
	case "AutoGroups":
		err = setting.UpdateAutoGroupsByJsonString(value)
	case "CustomCallbackAddress":
		operation_setting.CustomCallbackAddress = value
	case "EpayId":
		operation_setting.EpayId = value
	case "EpayKey":
		operation_setting.EpayKey = value
	case "Price":
		operation_setting.Price, _ = strconv.ParseFloat(value, 64)
	case "USDExchangeRate":
		operation_setting.USDExchangeRate, _ = strconv.ParseFloat(value, 64)
	case "MinTopUp":
		operation_setting.MinTopUp, _ = strconv.Atoi(value)
	case "StripeApiSecret":
		setting.StripeApiSecret = value
	case "StripeWebhookSecret":
		setting.StripeWebhookSecret = value
	case "StripePriceId":
		setting.StripePriceId = value
	case "StripeUnitPrice":
		setting.StripeUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "StripeMinTopUp":
		setting.StripeMinTopUp, _ = strconv.Atoi(value)
	case "StripePromotionCodesEnabled":
		setting.StripePromotionCodesEnabled = value == "true"
	case "StripeDisplayName":
		setting.StripeDisplayName = value
	case "StripeDisplayIcon":
		setting.StripeDisplayIcon = value
	case "CreemApiKey":
		setting.CreemApiKey = value
	case "CreemProducts":
		setting.CreemProducts = value
	case "CreemTestMode":
		setting.CreemTestMode = value == "true"
	case "CreemWebhookSecret":
		setting.CreemWebhookSecret = value
	case "WaffoEnabled":
		setting.WaffoEnabled = value == "true"
	case "WaffoApiKey":
		setting.WaffoApiKey = value
	case "WaffoPrivateKey":
		setting.WaffoPrivateKey = value
	case "WaffoPublicCert":
		setting.WaffoPublicCert = value
	case "WaffoSandboxPublicCert":
		setting.WaffoSandboxPublicCert = value
	case "WaffoSandboxApiKey":
		setting.WaffoSandboxApiKey = value
	case "WaffoSandboxPrivateKey":
		setting.WaffoSandboxPrivateKey = value
	case "WaffoSandbox":
		setting.WaffoSandbox = value == "true"
	case "WaffoMerchantId":
		setting.WaffoMerchantId = value
	case "WaffoNotifyUrl":
		setting.WaffoNotifyUrl = value
	case "WaffoReturnUrl":
		setting.WaffoReturnUrl = value
	case "WaffoSubscriptionReturnUrl":
		setting.WaffoSubscriptionReturnUrl = value
	case "WaffoCurrency":
		setting.WaffoCurrency = value
	case "WaffoUnitPrice":
		setting.WaffoUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "WaffoMinTopUp":
		setting.WaffoMinTopUp, _ = strconv.Atoi(value)
	case "WaffoPayMethods":
		var methods []constant.WaffoPayMethod
		if err = common.UnmarshalJsonStr(value, &methods); err == nil {
			err = setting.SetWaffoPayMethods(methods)
		}
	case "WaffoPancakeEnabled":
		setting.WaffoPancakeEnabled = value == "true"
	case "WaffoPancakeSandbox":
		setting.WaffoPancakeSandbox = value == "true"
	case "WaffoPancakeMerchantID":
		setting.WaffoPancakeMerchantID = value
	case "WaffoPancakePrivateKey":
		setting.WaffoPancakePrivateKey = value
	case "WaffoPancakeWebhookPublicKey":
		setting.WaffoPancakeWebhookPublicKey = value
	case "WaffoPancakeWebhookTestKey":
		setting.WaffoPancakeWebhookTestKey = value
	case "WaffoPancakeStoreID":
		setting.WaffoPancakeStoreID = value
	case "WaffoPancakeProductID":
		setting.WaffoPancakeProductID = value
	case "WaffoPancakeReturnURL":
		setting.WaffoPancakeReturnURL = value
	case "WaffoPancakeCurrency":
		setting.WaffoPancakeCurrency = value
	case "WaffoPancakeUnitPrice":
		setting.WaffoPancakeUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "WaffoPancakeMinTopUp":
		setting.WaffoPancakeMinTopUp, _ = strconv.Atoi(value)
	case "TopupGroupRatio":
		err = common.UpdateTopupGroupRatioByJSONString(value)
	case "GitHubClientId":
		common.GitHubClientId = value
	case "GitHubClientSecret":
		common.GitHubClientSecret = value
	case "LinuxDOClientId":
		common.LinuxDOClientId = value
	case "LinuxDOClientSecret":
		common.LinuxDOClientSecret = value
	case "LinuxDOMinimumTrustLevel":
		common.LinuxDOMinimumTrustLevel, _ = strconv.Atoi(value)
	case "Footer":
		common.Footer = value
	case "SystemName":
		common.SystemName = value
	case "Logo":
		common.Logo = value
	case "WeChatServerAddress":
		common.WeChatServerAddress = value
	case "WeChatServerToken":
		common.WeChatServerToken = value
	case "WeChatAccountQRCodeImageURL":
		common.WeChatAccountQRCodeImageURL = value
	case "TelegramBotToken":
		common.TelegramBotToken = value
	case "TelegramBotName":
		common.TelegramBotName = value
	case "TurnstileSiteKey":
		common.TurnstileSiteKey = value
	case "TurnstileSecretKey":
		common.TurnstileSecretKey = value
	case "QuotaForNewUser":
		common.QuotaForNewUser, _ = strconv.Atoi(value)
	case "QuotaForInviter":
		common.QuotaForInviter, _ = strconv.Atoi(value)
	case "QuotaForInvitee":
		common.QuotaForInvitee, _ = strconv.Atoi(value)
	case "InviteBindingSettings":
		err = common.UpdateInviteBindingSettingsByJSONString(value)
	case "InviterRechargeCommissionRate":
		// 比例配置使用小数表达（例如 0.1 表示 10%）。
		common.InviterRechargeCommissionRate, _ = strconv.ParseFloat(value, 64)
	case "InviterRechargeSecondLevelCommissionRate":
		// 二级比例仅用于下级的下级，邀请关系沿已绑定链路自动继承。
		common.InviterRechargeSecondLevelCommissionRate, _ = strconv.ParseFloat(value, 64)
	case "InviterCommissionDailyCap":
		// 单日上限单位为额度，0 表示不限制。
		common.InviterCommissionDailyCap, _ = strconv.Atoi(value)
	case "WalletRedemptionDailyCreateLimit":
		common.WalletRedemptionDailyCreateLimit, _ = strconv.Atoi(value)
	case "WalletRedemptionMinimumQuota":
		common.WalletRedemptionMinimumQuota, _ = strconv.Atoi(value)
	case "WalletRedemptionActiveLimit":
		common.WalletRedemptionActiveLimit, _ = strconv.Atoi(value)
	case "WalletRedemptionDailyQuotaLimit":
		common.WalletRedemptionDailyQuotaLimit, _ = strconv.Atoi(value)
	case "WalletRedemptionReviewDistinctCreatorThreshold":
		common.WalletRedemptionReviewDistinctCreatorThreshold, _ = strconv.Atoi(value)
	case "WalletRedemptionReviewSmallQuotaLimit":
		common.WalletRedemptionReviewSmallQuotaLimit, _ = strconv.Atoi(value)
	case "WalletRedemptionAutoBindMaxAgeHours":
		common.WalletRedemptionAutoBindMaxAgeHours, _ = strconv.Atoi(value)
	case "InvoiceServiceFeeRate":
		// 发票手续费费率，使用小数表达（例如 0.01 表示 1%）。
		common.InvoiceServiceFeeRate, _ = strconv.ParseFloat(value, 64)
	case "QuotaRemindThreshold":
		common.QuotaRemindThreshold, _ = strconv.Atoi(value)
	case "PreConsumedQuota":
		common.PreConsumedQuota, _ = strconv.Atoi(value)
	case "MultiKeyCooldownSeconds":
		common.MultiKeyCooldownSeconds, _ = strconv.Atoi(value)
	case "MultiKeyStickySeconds":
		common.MultiKeyStickySeconds, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitCount":
		setting.ModelRequestRateLimitCount, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitDurationMinutes":
		setting.ModelRequestRateLimitDurationMinutes, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitSuccessCount":
		setting.ModelRequestRateLimitSuccessCount, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitGroup":
		err = setting.UpdateModelRequestRateLimitGroupByJSONString(value)
	case "RedemptionRateLimitDurationSeconds":
		setting.RedemptionRateLimitDurationSeconds, _ = strconv.Atoi(value)
	case "RedemptionRateLimitSuccessCount":
		setting.RedemptionRateLimitSuccessCount, _ = strconv.Atoi(value)
	case "RedemptionRateLimitFailureCount":
		setting.RedemptionRateLimitFailureCount, _ = strconv.Atoi(value)
	case "RetryTimes":
		common.RetryTimes, _ = strconv.Atoi(value)
	case "ResponsesRequestBodyLimitMB":
		constant.ResponsesRequestBodyLimitMB, _ = strconv.Atoi(value)
	case "DataExportInterval":
		common.DataExportInterval, _ = strconv.Atoi(value)
	case "DataExportDefaultTime":
		common.DataExportDefaultTime = value
	case "ModelRatio":
		err = ratio_setting.UpdateModelRatioByJSONString(value)
	case "GroupRatio":
		err = ratio_setting.UpdateGroupRatioByJSONString(value)
	case "GroupGroupRatio":
		err = ratio_setting.UpdateGroupGroupRatioByJSONString(value)
	case "TimeRatioRules":
		err = ratio_setting.UpdateTimeRatioRulesByJSONString(value)
	case "UserUsableGroups":
		err = setting.UpdateUserUsableGroupsByJSONString(value)
	case "CompletionRatio":
		err = ratio_setting.UpdateCompletionRatioByJSONString(value)
	case "ModelPrice":
		err = ratio_setting.UpdateModelPriceByJSONString(value)
	case "CacheRatio":
		err = ratio_setting.UpdateCacheRatioByJSONString(value)
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(value)
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(value)
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(value)
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(value)
	case "AudioDurationPrice":
		err = ratio_setting.UpdateAudioDurationPriceByJSONString(value)
	case "TopUpLink":
		common.TopUpLink = value
	//case "ChatLink":
	//	common.ChatLink = value
	//case "ChatLink2":
	//	common.ChatLink2 = value
	case "ChannelDisableThreshold":
		common.ChannelDisableThreshold, _ = strconv.ParseFloat(value, 64)
	case "QuotaPerUnit":
		common.QuotaPerUnit, _ = strconv.ParseFloat(value, 64)
	case "SensitiveWords":
		setting.SensitiveWordsFromString(value)
	case "AutomaticDisableKeywords":
		operation_setting.AutomaticDisableKeywordsFromString(value)
	case "AutomaticDisableStatusCodes":
		err = operation_setting.AutomaticDisableStatusCodesFromString(value)
	case "AutomaticRetryStatusCodes":
		err = operation_setting.AutomaticRetryStatusCodesFromString(value)
	case "UserScopedCircuitBreakerStatusCodes":
		err = operation_setting.UserScopedCircuitBreakerStatusCodesFromString(value)
	case "UserScopedCircuitBreakerTTLSeconds":
		seconds, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			err = parseErr
		} else {
			operation_setting.UserScopedCircuitBreakerTTLSeconds = operation_setting.NormalizeUserScopedCircuitBreakerTTLSeconds(seconds)
		}
	case "UserScopedCircuitBreakerFailureThreshold":
		threshold, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			err = parseErr
		} else {
			operation_setting.UserScopedCircuitBreakerFailureThreshold = operation_setting.NormalizeUserScopedCircuitBreakerFailureThreshold(threshold)
		}
	case "StreamCacheQueueLength":
		setting.StreamCacheQueueLength, _ = strconv.Atoi(value)
	case "PayMethods":
		err = operation_setting.UpdatePayMethodsByJsonString(value)
	}
	return err
}

func validateInviteCommissionRateOption(key string, value string) error {
	if !isInviteCommissionRateOption(key) {
		return nil
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fmt.Errorf("invalid invite commission rate: %w", err)
	}
	_, firstLevelRate, secondLevelRate := common.InviteCommissionConfigSnapshot()
	if key == "InviterRechargeCommissionRate" {
		firstLevelRate = rate
	} else {
		secondLevelRate = rate
	}
	return common.ValidateInviteCommissionRates(firstLevelRate, secondLevelRate)
}

func isInviteCommissionRateOption(key string) bool {
	return key == "InviterRechargeCommissionRate" || key == "InviterRechargeSecondLevelCommissionRate"
}

func validateInviteCommissionDailyCapOption(key string, value string) error {
	if key != "InviterCommissionDailyCap" {
		return nil
	}
	dailyCap, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || dailyCap < 0 {
		return errors.New("invite commission daily cap must be a non-negative integer")
	}
	return nil
}

func validateWalletRedemptionPolicyOption(key string, value string) error {
	if key == "WalletRedemptionAutoBindMaxAgeHours" {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 0 {
			return errors.New("WalletRedemptionAutoBindMaxAgeHours must be a non-negative integer")
		}
		return nil
	}
	return validateWalletRedemptionPolicyField(key, value)
}

// handleConfigUpdate 处理分层配置更新，返回是否已处理
func handleConfigUpdate(key, value string) (bool, error) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return false, nil // 不是分层配置
	}

	configName := parts[0]
	configKey := parts[1]

	// 获取配置对象
	cfg := config.GlobalConfig.Get(configName)
	if cfg == nil {
		return false, nil // 未注册的配置
	}
	if configName == "billing_setting" {
		return true, billing_setting.UpdateField(configKey, value)
	}

	// 更新配置
	configMap := map[string]string{
		configKey: value,
	}
	config.UpdateConfigFromMap(cfg, configMap)

	// 特定配置的后处理
	if configName == "performance_setting" {
		performance_setting.UpdateAndSync()
	} else if configName == "tool_price_setting" {
		operation_setting.RebuildToolPriceIndex()
	}

	return true, nil // 已处理
}
