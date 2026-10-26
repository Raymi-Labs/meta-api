package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
	"AudioDurationPrice",
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}
	for _, modelName := range model.GetEnabledModels() {
		modelNames[modelName] = struct{}{}
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func isVisiblePublicKeyOption(key string) bool {
	switch key {
	case "WaffoPancakeWebhookPublicKey", "WaffoPancakeWebhookTestKey":
		return true
	default:
		return false
	}
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		value := common.Interface2String(v)
		if common.IsPulseConfigKey(k) || strings.HasSuffix(k, "Token") ||
			strings.HasSuffix(k, "Secret") ||
			(strings.HasSuffix(k, "Key") && !isVisiblePublicKeyOption(k)) ||
			strings.HasSuffix(k, "secret") ||
			strings.HasSuffix(k, "api_key") {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	cacheCreationMeta := make(map[string]float64)
	for _, price := range model.GetPricing() {
		if price.SupportsCacheCreationTTL {
			cacheCreationMeta[price.ModelName] = ratio_setting.ClaudeCacheCreation1hMultiplier
		}
	}
	if encoded, err := common.Marshal(cacheCreationMeta); err == nil {
		options = append(options, &model.Option{Key: "CacheCreationPriceMeta", Value: string(encoded)})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
	return
}

type OptionUpdateRequest struct {
	Key       string `json:"key"`
	Value     any    `json:"value"`
	EmailSync bool   `json:"email_sync"`
}

func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	err := common.DecodeJson(c.Request.Body, &option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	switch option.Value.(type) {
	case bool:
		option.Value = common.Interface2String(option.Value.(bool))
	case float64:
		option.Value = common.Interface2String(option.Value.(float64))
	case int:
		option.Value = common.Interface2String(option.Value.(int))
	default:
		option.Value = fmt.Sprintf("%v", option.Value)
	}
	if option.Key == billing_setting.TieredBundleOptionKey {
		if err = model.UpdateTieredBillingOptions(option.Value.(string)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
		})
		return
	}
	if option.Key == model.WalletRedemptionPolicyBundleOptionKey {
		before := model.GetWalletRedemptionPolicy()
		if err = model.UpdateWalletRedemptionPolicyOptions(option.Value.(string)); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		after := model.GetWalletRedemptionPolicy()
		model.RecordLogWithAdminInfo(c.GetInt("id"), model.LogTypeManage,
			fmt.Sprintf("管理员更新兑换码策略：%+v -> %+v", before, after),
			map[string]interface{}{"admin_id": c.GetInt("id"), "admin_username": c.GetString("username")},
		)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}
	if strings.HasPrefix(option.Key, "slow_ttft_setting.") {
		if err = operation_setting.ValidateSlowTTFTOption(option.Key, option.Value.(string)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	if strings.HasPrefix(option.Key, "channel_concurrency_setting.") {
		if err = operation_setting.ValidateChannelConcurrencyOption(option.Key, option.Value.(string)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！",
			})
			return
		}
	case "discord.enabled":
		if option.Value == "true" && system_setting.GetDiscordSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！",
			})
			return
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！",
			})
			return
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！",
			})
			return
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名限制，请先填入限制的邮箱域名！",
			})
			return
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})

			return
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && common.TelegramBotToken == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Telegram OAuth，请先填入 Telegram Bot Token！",
			})
			return
		}
	case "GroupRatio":
		err = ratio_setting.CheckGroupRatio(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "TimeRatioRules":
		_, err = ratio_setting.ParseTimeRatioRules(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "时间倍率规则设置失败: " + err.Error(),
			})
			return
		}
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "图片倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频补全倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioDurationPrice":
		err = ratio_setting.ValidateAudioDurationPriceJSON(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频时长价格设置失败: " + err.Error(),
			})
			return
		}
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "缓存创建倍率设置失败: " + err.Error(),
			})
			return
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "global.image_generation_tool_call_permission":
		scope, parseErr := strconv.Atoi(option.Value.(string))
		if parseErr != nil || model.NormalizeModelPermissionScope(scope) != scope {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "图片生成工具调用权限设置无效",
			})
			return
		}
	case "AutomaticDisableStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticRetryStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "UserScopedCircuitBreakerStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "UserScopedCircuitBreakerTTLSeconds", "UserScopedCircuitBreakerFailureThreshold":
		value, parseErr := strconv.Atoi(option.Value.(string))
		if parseErr != nil || value <= 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "用户维度短熔断配置必须是大于 0 的整数",
			})
			return
		}
	case "ResponsesRequestBodyLimitMB":
		limitMB, parseErr := strconv.Atoi(strings.TrimSpace(option.Value.(string)))
		maxRequestBodyMB := constant.MaxRequestBodyMB
		if maxRequestBodyMB <= 0 {
			maxRequestBodyMB = 256
		}
		if parseErr != nil || limitMB < 0 || limitMB > maxRequestBodyMB {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("请求体业务预检上限必须是 0-%d 之间的整数，0 表示关闭业务预检", maxRequestBodyMB),
			})
			return
		}
	case "InviteBindingSettings":
		if _, parseErr := common.ParseInviteBindingSettings(option.Value.(string)); parseErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "邀请绑定设置无效：人数阈值必须是非负整数，阈值后绑定概率必须在 0% 到 100% 之间",
			})
			return
		}
	case "InviterRechargeCommissionRate", "InviterRechargeSecondLevelCommissionRate":
		rate, parseErr := strconv.ParseFloat(strings.TrimSpace(option.Value.(string)), 64)
		_, firstLevelRate, secondLevelRate := common.InviteCommissionConfigSnapshot()
		if option.Key == "InviterRechargeCommissionRate" {
			firstLevelRate = rate
		} else {
			secondLevelRate = rate
		}
		if parseErr != nil || common.ValidateInviteCommissionRates(firstLevelRate, secondLevelRate) != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "邀请返佣比例无效：一级、二级比例必须分别在 0 到 1 之间，且合计不能超过 1",
			})
			return
		}
	case "InviterCommissionDailyCap":
		dailyCap, parseErr := strconv.Atoi(strings.TrimSpace(option.Value.(string)))
		if parseErr != nil || dailyCap < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "邀请返佣单日上限必须是非负整数，0 表示不限制",
			})
			return
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.uptime_kuma_groups":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "UptimeKumaGroups")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "ClaudeToOpenAIReasoningMap":
		err = validateClaudeToOpenAIReasoningMap(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	if option.Key == model.AnnouncementOptionKey {
		queued, saveErr := model.SaveAnnouncementPublications(option.Value.(string), option.EmailSync, c.GetInt("id"), service.AnnouncementSMTPReady())
		if saveErr != nil {
			// SQL errors may contain snapshot addresses; return only validation
			// errors, never the raw database error.
			message := "公告保存失败，请检查邮件配置后重试"
			if strings.Contains(saveErr.Error(), "SMTP 未配置") || strings.HasPrefix(saveErr.Error(), "公告 ID") || strings.HasPrefix(saveErr.Error(), "单次最多同步") {
				message = saveErr.Error()
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": message})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"email_jobs_queued": queued}})
		return
	}
	err = model.UpdateOption(option.Key, option.Value.(string))
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

func validateClaudeToOpenAIReasoningMap(raw string) error {
	parsed := make(map[string]string)
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return fmt.Errorf("推理映射必须是 JSON 对象: %w", err)
	}

	requiredLevels := []string{"low", "medium", "high", "max"}
	allowedEfforts := map[string]struct{}{
		"minimal": {},
		"low":     {},
		"medium":  {},
		"high":    {},
		"xhigh":   {},
	}
	for _, level := range requiredLevels {
		value := strings.TrimSpace(parsed[level])
		if value == "" {
			return fmt.Errorf("缺少 %s 档位", level)
		}
		if _, ok := allowedEfforts[value]; !ok {
			return fmt.Errorf("%s 档位仅支持 minimal、low、medium、high、xhigh", level)
		}
	}
	return nil
}
