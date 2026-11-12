package controller

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const updateCheckCacheTTL = 10 * time.Minute

var (
	updateCheckCacheMutex sync.Mutex
	updateCheckCacheData  *updateCheckData
	updateCheckCacheUntil time.Time
	versionSHARegexp      = regexp.MustCompile(`(?i)([0-9a-f]{7,40})`)
)

type updateCheckData struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	CurrentCommit   string `json:"current_commit"`
	LatestCommit    string `json:"latest_commit"`
	Repository      string `json:"repository"`
	Branch          string `json:"branch"`
	ReleaseURL      string `json:"release_url"`
	CompareURL      string `json:"compare_url"`
	UpdateAvailable bool   `json:"update_available"`
	Mode            string `json:"mode"`
	Message         string `json:"message"`
	Body            string `json:"body"`
	CheckedAt       int64  `json:"checked_at"`
}

type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Message string `json:"message"`
}

type githubCommitResponse struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Message string `json:"message"`
}

type githubAtomFeed struct {
	Entries []githubAtomEntry `xml:"entry"`
}

type githubAtomEntry struct {
	ID      string            `xml:"id"`
	Title   string            `xml:"title"`
	Updated string            `xml:"updated"`
	Links   []githubAtomLink  `xml:"link"`
	Content githubAtomContent `xml:"content"`
}

type githubAtomLink struct {
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

type githubAtomContent struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

func TestStatus(c *gin.Context) {
	err := model.PingDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "数据库连接失败",
		})
		return
	}
	// 获取HTTP统计信息
	httpStats := middleware.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Server is running",
		"http_stats": httpStats,
	})
	return
}

func GetStatus(c *gin.Context) {

	cs := console_setting.GetConsoleSetting()
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	passkeySetting := system_setting.GetPasskeySettings()
	legalSetting := system_setting.GetLegalSettings()

	data := gin.H{
		"version":                     common.Version,
		"build_commit":                common.BuildCommit,
		"build_repository":            common.BuildRepository,
		"build_branch":                common.BuildBranch,
		"start_time":                  common.StartTime,
		"email_verification":          common.EmailVerificationEnabled,
		"github_oauth":                common.GitHubOAuthEnabled,
		"github_client_id":            common.GitHubClientId,
		"discord_oauth":               system_setting.GetDiscordSettings().Enabled,
		"discord_client_id":           system_setting.GetDiscordSettings().ClientId,
		"linuxdo_oauth":               common.LinuxDOOAuthEnabled,
		"linuxdo_client_id":           common.LinuxDOClientId,
		"linuxdo_minimum_trust_level": common.LinuxDOMinimumTrustLevel,
		"telegram_oauth":              common.TelegramOAuthEnabled,
		"telegram_bot_name":           common.TelegramBotName,
		"system_name":                 common.SystemName,
		"enterprise_enabled":          common.EnterpriseEnabled,
		"logo":                        common.Logo,
		"footer_html":                 common.Footer,
		"wechat_qrcode":               common.WeChatAccountQRCodeImageURL,
		"wechat_login":                common.WeChatAuthEnabled,
		"server_address":              system_setting.ServerAddress,
		"turnstile_check":             common.TurnstileCheckEnabled,
		"turnstile_site_key":          common.TurnstileSiteKey,
		"top_up_link":                 common.TopUpLink,
		"docs_link":                   operation_setting.GetGeneralSetting().DocsLink,
		"quota_per_unit":              common.QuotaPerUnit,
		"invoice_service_fee_rate":    common.InvoiceServiceFeeRate,
		// 兼容旧前端：保留 display_in_currency，同时提供新的 quota_display_type
		"display_in_currency":           operation_setting.IsCurrencyDisplay(),
		"quota_display_type":            operation_setting.GetQuotaDisplayType(),
		"custom_currency_symbol":        operation_setting.GetGeneralSetting().CustomCurrencySymbol,
		"custom_currency_exchange_rate": operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
		"payment_currency_symbol":       operation_setting.GetGeneralSetting().PaymentCurrencySymbol,
		"enable_batch_update":           common.BatchUpdateEnabled,
		"enable_drawing":                common.DrawingEnabled,
		"enable_task":                   common.TaskEnabled,
		"enable_data_export":            common.DataExportEnabled,
		"data_export_default_time":      common.DataExportDefaultTime,
		"default_collapse_sidebar":      common.DefaultCollapseSidebar,
		"mj_notify_enabled":             setting.MjNotifyEnabled,
		"chats":                         setting.GetChatsCopy(),
		"demo_site_enabled":             operation_setting.DemoSiteEnabled,
		"self_use_mode_enabled":         operation_setting.SelfUseModeEnabled,
		"default_use_auto_group":        setting.DefaultUseAutoGroup,

		"usd_exchange_rate": operation_setting.USDExchangeRate,
		"price":             operation_setting.Price,
		"stripe_unit_price": setting.StripeUnitPrice,

		// 面板启用开关
		"api_info_enabled":      cs.ApiInfoEnabled,
		"uptime_kuma_enabled":   cs.UptimeKumaEnabled,
		"announcements_enabled": cs.AnnouncementsEnabled,
		"faq_enabled":           cs.FAQEnabled,

		// 模块管理配置
		"HeaderNavModules":    common.OptionMap["HeaderNavModules"],
		"SidebarModulesAdmin": common.OptionMap["SidebarModulesAdmin"],

		"oidc_enabled":                system_setting.GetOIDCSettings().Enabled,
		"oidc_client_id":              system_setting.GetOIDCSettings().ClientId,
		"oidc_authorization_endpoint": system_setting.GetOIDCSettings().AuthorizationEndpoint,
		"passkey_login":               passkeySetting.Enabled,
		"passkey_display_name":        passkeySetting.RPDisplayName,
		"passkey_rp_id":               passkeySetting.RPID,
		"passkey_origins":             passkeySetting.Origins,
		"passkey_allow_insecure":      passkeySetting.AllowInsecureOrigin,
		"passkey_user_verification":   passkeySetting.UserVerification,
		"passkey_attachment":          passkeySetting.AttachmentPreference,
		"setup":                       constant.Setup,
		"user_agreement_enabled":      legalSetting.UserAgreement != "",
		"privacy_policy_enabled":      legalSetting.PrivacyPolicy != "",
		"checkin_enabled":             operation_setting.GetCheckinSetting().Enabled,
		"_qn":                         "new-api",
	}

	// 根据启用状态注入可选内容
	if cs.ApiInfoEnabled {
		data["api_info"] = console_setting.GetApiInfo()
	}
	if cs.AnnouncementsEnabled {
		data["announcements"] = console_setting.GetAnnouncements()
	}
	if cs.FAQEnabled {
		data["faq"] = console_setting.GetFAQ()
	}

	// Add enabled custom OAuth providers
	customProviders := oauth.GetEnabledCustomProviders()
	if len(customProviders) > 0 {
		type CustomOAuthInfo struct {
			Id                    int    `json:"id"`
			Name                  string `json:"name"`
			Slug                  string `json:"slug"`
			Icon                  string `json:"icon"`
			ClientId              string `json:"client_id"`
			AuthorizationEndpoint string `json:"authorization_endpoint"`
			Scopes                string `json:"scopes"`
		}
		providersInfo := make([]CustomOAuthInfo, 0, len(customProviders))
		for _, p := range customProviders {
			config := p.GetConfig()
			providersInfo = append(providersInfo, CustomOAuthInfo{
				Id:                    config.Id,
				Name:                  config.Name,
				Slug:                  config.Slug,
				Icon:                  config.Icon,
				ClientId:              config.ClientId,
				AuthorizationEndpoint: config.AuthorizationEndpoint,
				Scopes:                config.Scopes,
			})
		}
		data["custom_oauth_providers"] = providersInfo
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

func CheckUpdate(c *gin.Context) {
	data, err := getUpdateCheckData(c.Request.Context())
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, data)
}

func getUpdateCheckData(parentCtx context.Context) (*updateCheckData, error) {
	updateCheckCacheMutex.Lock()
	if updateCheckCacheData != nil && time.Now().Before(updateCheckCacheUntil) {
		cached := *updateCheckCacheData
		updateCheckCacheMutex.Unlock()
		return &cached, nil
	}
	updateCheckCacheMutex.Unlock()

	repository := normalizeGitHubRepository(firstNonEmpty(os.Getenv("UPDATE_CHECK_REPOSITORY"), common.BuildRepository, "QuantumNous/new-api"))
	branch := strings.TrimSpace(firstNonEmpty(os.Getenv("UPDATE_CHECK_BRANCH"), common.BuildBranch, "main"))
	currentVersion := strings.TrimSpace(common.Version)
	currentCommit := strings.TrimSpace(firstNonEmpty(os.Getenv("UPDATE_CHECK_CURRENT_COMMIT"), common.BuildCommit, extractVersionSHA(currentVersion)))

	ctx, cancel := context.WithTimeout(parentCtx, 8*time.Second)
	defer cancel()

	var data *updateCheckData
	var err error
	if currentCommit != "" {
		data, err = getGitHubCommitUpdate(ctx, repository, branch, currentVersion, currentCommit)
	} else {
		data, err = getGitHubReleaseUpdate(ctx, repository, currentVersion)
	}
	if err != nil {
		return nil, err
	}

	updateCheckCacheMutex.Lock()
	updateCheckCacheData = data
	updateCheckCacheUntil = time.Now().Add(updateCheckCacheTTL)
	updateCheckCacheMutex.Unlock()

	return data, nil
}

func getGitHubCommitUpdate(ctx context.Context, repository string, branch string, currentVersion string, currentCommit string) (*updateCheckData, error) {
	latestCommit, commitURL, err := fetchGitHubLatestCommit(ctx, repository, branch)
	if err != nil {
		return nil, err
	}
	if latestCommit == "" {
		return nil, fmt.Errorf("GitHub 未返回最新提交信息")
	}

	latestShort := shortSHA(latestCommit)
	currentShort := shortSHA(currentCommit)
	updateAvailable := latestShort != "" && currentShort != "" && latestShort != currentShort
	compareURL := ""
	if currentCommit != "" {
		compareURL = fmt.Sprintf("https://github.com/%s/compare/%s...%s", repository, currentCommit, latestCommit)
	}
	message := fmt.Sprintf("当前已是最新提交：%s", latestShort)
	body := fmt.Sprintf("当前部署提交：`%s`\n\n最新主分支提交：`%s`", currentShort, latestShort)
	if updateAvailable {
		message = fmt.Sprintf("发现新提交：%s", latestShort)
		body = fmt.Sprintf("当前部署提交：`%s`\n\n最新主分支提交：`%s`\n\n建议确认构建流水线是否已基于最新代码发布。", currentShort, latestShort)
	}

	return &updateCheckData{
		CurrentVersion:  currentVersion,
		LatestVersion:   fmt.Sprintf("%s-%s", branch, latestShort),
		CurrentCommit:   currentCommit,
		LatestCommit:    latestCommit,
		Repository:      repository,
		Branch:          branch,
		ReleaseURL:      firstNonEmpty(commitURL, fmt.Sprintf("https://github.com/%s/commit/%s", repository, latestCommit)),
		CompareURL:      compareURL,
		UpdateAvailable: updateAvailable,
		Mode:            "commit",
		Message:         message,
		Body:            body,
		CheckedAt:       time.Now().Unix(),
	}, nil
}

func getGitHubReleaseUpdate(ctx context.Context, repository string, currentVersion string) (*updateCheckData, error) {
	latestVersion, releaseURL, body, err := fetchGitHubLatestRelease(ctx, repository)
	if err != nil {
		return nil, err
	}
	if latestVersion == "" {
		return nil, fmt.Errorf("GitHub 未返回最新版本信息")
	}

	currentComparable := currentVersion != "" && currentVersion != "v0.0.0" && currentVersion != "dev"
	updateAvailable := !currentComparable || latestVersion != currentVersion
	message := fmt.Sprintf("已是最新版本：%s", latestVersion)
	if updateAvailable {
		message = fmt.Sprintf("发现新版本：%s", latestVersion)
		if !currentComparable {
			message = fmt.Sprintf("当前构建版本为空或无效，最新版本为：%s", latestVersion)
		}
	}

	return &updateCheckData{
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		Repository:      repository,
		ReleaseURL:      firstNonEmpty(releaseURL, fmt.Sprintf("https://github.com/%s/releases/tag/%s", repository, latestVersion)),
		UpdateAvailable: updateAvailable,
		Mode:            "release",
		Message:         message,
		Body:            firstNonEmpty(body, message),
		CheckedAt:       time.Now().Unix(),
	}, nil
}

func fetchGitHubLatestCommit(ctx context.Context, repository string, branch string) (string, string, error) {
	atomURL := fmt.Sprintf("https://github.com/%s/commits/%s.atom", repository, branch)
	feed, err := fetchGitHubAtom(ctx, atomURL)
	if err == nil && len(feed.Entries) > 0 {
		entry := feed.Entries[0]
		commitURL := atomEntryAlternateLink(entry)
		sha := extractCommitSHA(firstNonEmpty(entry.ID, commitURL))
		if sha != "" {
			return sha, commitURL, nil
		}
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repository, branch)
	var resp githubCommitResponse
	if apiErr := fetchGitHubJSON(ctx, apiURL, &resp); apiErr != nil {
		if err != nil {
			return "", "", fmt.Errorf("%v；API 兜底也失败：%w", err, apiErr)
		}
		return "", "", apiErr
	}
	return resp.SHA, resp.HTMLURL, nil
}

func fetchGitHubLatestRelease(ctx context.Context, repository string) (string, string, string, error) {
	atomURL := fmt.Sprintf("https://github.com/%s/releases.atom", repository)
	feed, err := fetchGitHubAtom(ctx, atomURL)
	if err == nil && len(feed.Entries) > 0 {
		entry := feed.Entries[0]
		releaseURL := atomEntryAlternateLink(entry)
		tagName := extractReleaseTag(releaseURL)
		if tagName != "" {
			return tagName, releaseURL, strings.TrimSpace(entry.Content.Text), nil
		}
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repository)
	var resp githubReleaseResponse
	if apiErr := fetchGitHubJSON(ctx, apiURL, &resp); apiErr != nil {
		if err != nil {
			return "", "", "", fmt.Errorf("%v；API 兜底也失败：%w", err, apiErr)
		}
		return "", "", "", apiErr
	}
	return resp.TagName, resp.HTMLURL, resp.Body, nil
}

func fetchGitHubAtom(ctx context.Context, atomURL string) (*githubAtomFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, atomURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/atom+xml, application/xml")
	req.Header.Set("User-Agent", "new-api-update-checker")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 GitHub Atom 失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("GitHub Atom 返回 HTTP %d", resp.StatusCode)
	}

	var feed githubAtomFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("解析 GitHub Atom 失败：%w", err)
	}
	return &feed, nil
}

func fetchGitHubJSON(ctx context.Context, apiURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "new-api-update-checker")
	if token := strings.TrimSpace(firstNonEmpty(os.Getenv("UPDATE_CHECK_GITHUB_TOKEN"), os.Getenv("GITHUB_TOKEN"))); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("连接 GitHub 失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var errResp githubReleaseResponse
		_ = common.DecodeJson(resp.Body, &errResp)
		if resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(errResp.Message), "rate limit") {
			return fmt.Errorf("GitHub API 已限流，请稍后再试，或配置 UPDATE_CHECK_GITHUB_TOKEN")
		}
		if errResp.Message != "" {
			return fmt.Errorf("GitHub 返回错误：%s", errResp.Message)
		}
		return fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}

	if err := common.DecodeJson(resp.Body, target); err != nil {
		return fmt.Errorf("解析 GitHub 响应失败：%w", err)
	}
	return nil
}

func normalizeGitHubRepository(repository string) string {
	repository = strings.TrimSpace(repository)
	repository = strings.TrimPrefix(repository, "https://github.com/")
	repository = strings.TrimPrefix(repository, "http://github.com/")
	repository = strings.TrimSuffix(repository, ".git")
	repository = strings.Trim(repository, "/")
	if matched, _ := regexp.MatchString(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`, repository); matched {
		return repository
	}
	return "QuantumNous/new-api"
}

func extractVersionSHA(version string) string {
	return extractCommitSHA(version)
}

func extractCommitSHA(value string) string {
	matches := versionSHARegexp.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

func extractReleaseTag(releaseURL string) string {
	parts := strings.Split(strings.TrimSpace(releaseURL), "/releases/tag/")
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func atomEntryAlternateLink(entry githubAtomEntry) string {
	for _, link := range entry.Links {
		if link.Rel == "alternate" || link.Type == "text/html" {
			return strings.TrimSpace(link.Href)
		}
	}
	if len(entry.Links) > 0 {
		return strings.TrimSpace(entry.Links[0].Href)
	}
	return ""
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func GetNotice(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Notice"],
	})
	return
}

func GetAbout(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["About"],
	})
	return
}

func GetUserAgreement(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    system_setting.GetLegalSettings().UserAgreement,
	})
	return
}

func GetPrivacyPolicy(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    system_setting.GetLegalSettings().PrivacyPolicy,
	})
	return
}

func GetMidjourney(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Midjourney"],
	})
	return
}

func GetHomePageContent(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["HomePageContent"],
	})
	return
}

func SendEmailVerification(c *gin.Context) {
	email := common.NormalizeEmailAddress(c.Query("email"))
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的邮箱地址",
		})
		return
	}
	localPart := parts[0]
	domainPart := parts[1]
	if common.EmailDomainRestrictionEnabled {
		if !common.EmailDomainInWhitelist(domainPart, common.EmailDomainWhitelist) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "The administrator has enabled the email domain name whitelist, and your email address is not allowed due to special symbols or it's not in the whitelist.",
			})
			return
		}
	}
	if common.EmailAliasRestrictionEnabled {
		containsSpecialSymbols := strings.Contains(localPart, "+") || strings.Contains(localPart, ".")
		if containsSpecialSymbols {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员已启用邮箱地址别名限制，您的邮箱地址由于包含特殊符号而被拒绝。",
			})
			return
		}
	}

	if model.IsEmailAlreadyTaken(email) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "邮箱地址已被占用",
		})
		return
	}
	code := common.GenerateVerificationCode(6)
	common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)
	subject := fmt.Sprintf("%s邮箱验证邮件", common.SystemName)
	content := fmt.Sprintf("<p>您好，你正在进行%s邮箱验证。</p>"+
		"<p>您的验证码为: <strong>%s</strong></p>"+
		"<p>验证码 %d 分钟内有效，如果不是本人操作，请忽略。</p>", common.SystemName, code, common.VerificationValidMinutes)
	err := common.SendEmail(subject, email, content)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.SysLog(fmt.Sprintf("verification email accepted by SMTP: %s", common.MaskEmail(email)))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func SendPasswordResetEmail(c *gin.Context) {
	email := common.NormalizeEmailAddress(c.Query("email"))
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if !model.IsEmailAlreadyTaken(email) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该邮箱地址未注册",
		})
		return
	}
	code := common.GenerateVerificationCode(0)
	common.RegisterVerificationCodeWithKey(email, code, common.PasswordResetPurpose)
	link := fmt.Sprintf("%s/user/reset?email=%s&token=%s", system_setting.ServerAddress, email, code)
	subject := fmt.Sprintf("%s密码重置", common.SystemName)
	content := fmt.Sprintf("<p>您好，你正在进行%s密码重置。</p>"+
		"<p>点击 <a href='%s'>此处</a> 进行密码重置。</p>"+
		"<p>如果链接无法点击，请尝试点击下面的链接或将其复制到浏览器中打开：<br> %s </p>"+
		"<p>重置链接 %d 分钟内有效，如果不是本人操作，请忽略。</p>", common.SystemName, link, link, common.VerificationValidMinutes)
	err := common.SendEmail(subject, email, content)
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

type PasswordResetRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

func ResetPassword(c *gin.Context) {
	var req PasswordResetRequest
	err := common.DecodeJson(c.Request.Body, &req)
	req.Email = common.NormalizeEmailAddress(req.Email)
	if req.Email == "" || req.Token == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if !common.VerifyCodeWithKey(req.Email, req.Token, common.PasswordResetPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "重置链接非法或已过期",
		})
		return
	}
	password := common.GenerateVerificationCode(12)
	err = model.ResetUserPasswordByEmail(req.Email, password)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.DeleteKey(req.Email, common.PasswordResetPurpose)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    password,
	})
	return
}
