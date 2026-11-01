package controller

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/publicsuffix"
	"gorm.io/gorm"
)

const (
	imagePlaygroundDefaultModel    = "gpt-image-2"
	imagePlaygroundAgentModel      = "gpt-6-sol"
	imagePlaygroundPreferredGroup  = "福利"
	imagePlaygroundSessionDuration = 2 * time.Hour
	imagePlaygroundRefreshWindow   = 15 * time.Minute
)

type imagePlaygroundSessionResponse struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
	Model     string `json:"model"`
}

type imagePlaygroundLaunchSettings struct {
	Profiles                []imagePlaygroundLaunchProfile `json:"profiles"`
	ActiveProfileID         string                         `json:"activeProfileId"`
	DefaultImageModel       string                         `json:"defaultImageModel"`
	DefaultPlanModel        string                         `json:"defaultPlanModel"`
	SupportsEcommerce       bool                           `json:"supportsEcommerce"`
	EcommerceDisabledReason string                         `json:"ecommerceDisabledReason,omitempty"`
}

type imagePlaygroundLaunchProfile struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Provider              string `json:"provider"`
	BaseURL               string `json:"baseUrl"`
	APIKey                string `json:"apiKey"`
	Model                 string `json:"model"`
	Timeout               int    `json:"timeout"`
	APIMode               string `json:"apiMode"`
	CodexCli              bool   `json:"codexCli"`
	APIProxy              bool   `json:"apiProxy"`
	StreamImages          bool   `json:"streamImages"`
	StreamPartialImages   int    `json:"streamPartialImages"`
	ResponseFormatB64Json bool   `json:"responseFormatB64Json"`
}

func CreateImagePlaygroundSession(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		common.ApiErrorMsg(c, "未登录")
		return
	}

	now := common.GetTimestamp()
	if err := model.DeleteExpiredImagePlaygroundTokens(userId, now); err != nil {
		common.SysLog("failed to delete expired image playground tokens: " + err.Error())
	}

	var supportsEcommerce bool
	minExpiredTime := now + int64(imagePlaygroundRefreshWindow/time.Second)
	token, err := model.GetReusableImagePlaygroundToken(userId, minExpiredTime)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, err)
			return
		}
		token, supportsEcommerce, err = createImagePlaygroundToken(userId, now)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		changed, refreshed, refreshErr := refreshImagePlaygroundToken(token, userId)
		// 即便 refreshErr 非空，clearImagePlaygroundTokenModelLimits 可能已经把 ModelLimits 改掉，
		// 这里仍把可持久化的内存改动写回，避免 in-memory/DB 状态长时间漂移。
		if changed {
			if updateErr := token.Update(); updateErr != nil {
				common.ApiError(c, updateErr)
				return
			}
		}
		if refreshErr != nil {
			common.ApiError(c, refreshErr)
			return
		}
		supportsEcommerce = refreshed
	}

	origin := buildImagePlaygroundOrigin(c)
	if origin == "" {
		common.ApiErrorMsg(c, "无法识别当前站点地址，请先配置服务器地址")
		return
	}

	common.ApiSuccess(c, imagePlaygroundSessionResponse{
		URL:       buildImagePlaygroundLaunchURL(origin, token.Key, supportsEcommerce),
		ExpiresAt: token.ExpiredTime,
		Model:     imagePlaygroundDefaultModel,
	})
}

func createImagePlaygroundToken(userId int, now int64) (*model.Token, bool, error) {
	key, err := common.GenerateKey()
	if err != nil {
		return nil, false, err
	}
	group, supportsEcommerce, err := resolveImagePlaygroundTokenGroup(userId)
	if err != nil {
		return nil, false, err
	}

	token := &model.Token{
		UserId:               userId,
		Key:                  key,
		Status:               common.TokenStatusEnabled,
		Name:                 "image-playground",
		SourceType:           model.TokenSourceTypeImagePlayground,
		CreatedTime:          now,
		AccessedTime:         now,
		ExpiredTime:          now + int64(imagePlaygroundSessionDuration/time.Second),
		UnlimitedQuota:       true,
		ModelLimitsEnabled:   false,
		ModelLimits:          "",
		Group:                group,
		AutoGroupFallback:    true,
		MaxConcurrency:       2,
		WindowRequestLimit:   60,
		WindowSeconds:        60,
		PackagePeriod:        model.TokenPackagePeriodNone,
		PackagePeriodMode:    model.TokenPackagePeriodModeRelative,
		PackageNextResetTime: 0,
	}
	if err := model.ValidateTokenRuntimeLimitConfig(token); err != nil {
		return nil, false, err
	}
	if err := token.Insert(); err != nil {
		return nil, false, err
	}
	return token, supportsEcommerce, nil
}

// refreshImagePlaygroundToken 刷新已存在的 image-playground token：清空遗留的模型限制并将
// 分组重定向到最适合 image-playground 的 group。
// 返回值：
//   - changed: token 内存状态是否已被修改（caller 应据此决定是否回写 DB；即便随后返回 err，
//     已经发生的内存变更也应一并持久化以避免内存/DB 漂移）。
//   - supportsEcommerce: 选定的 group 是否同时支持 image + agent 两个模型，用于决定 UI 是否启用电商模式。
//   - err: 解析 group 时的错误，nil 表示成功。
func refreshImagePlaygroundToken(token *model.Token, userId int) (bool, bool, error) {
	changed := clearImagePlaygroundTokenModelLimits(token)
	group, supportsEcommerce, err := resolveImagePlaygroundTokenGroup(userId)
	if err != nil {
		return changed, false, err
	}
	if token != nil && strings.TrimSpace(token.Group) != group {
		token.Group = group
		changed = true
	}
	if token != nil {
		// 系统管理的生图会话使用首选组自动回退，避免旧的自定义路由覆盖它。
		if !token.AutoGroupFallback || token.CrossGroupRetry || token.RoutingConfig != "" {
			token.AutoGroupFallback = true
			token.CrossGroupRetry = false
			token.RoutingConfig = ""
			changed = true
		}
	}
	return changed, supportsEcommerce, nil
}

func clearImagePlaygroundTokenModelLimits(token *model.Token) bool {
	if token == nil || (!token.ModelLimitsEnabled && strings.TrimSpace(token.ModelLimits) == "") {
		return false
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return true
}

// resolveImagePlaygroundTokenGroup 选出最适合当前用户的 image-playground 分组并报告其能力。
// 选择顺序：
//  1. 福利分组（仅当用户有权限且分组仍有效时）；
//  2. 当前用户分组（仅当它仍是有效、可用分组时）；
//  3. AutoGroups 中按配置顺序排列的有效分组；
//  4. 其余有效、可用分组（按名称排序，保证结果稳定）；
//  5. 在上述候选中优先选择同时支持 image + agent 模型的分组，其次选择仅支持 image 的分组。
//
// 决策见 .agents/notes/implemented/bug-fix/2026-11-01-image-playground-defaults.md。
//
// 不再回退到 default 或已取消的用户分组：无可执行分组时直接返回配置错误，避免创建一个
// 看似可用、实际无法路由的临时令牌。
func resolveImagePlaygroundTokenGroup(userId int) (string, bool, error) {
	userGroup, err := model.GetUserGroup(userId, false)
	if err != nil {
		return "", false, err
	}
	userGroup = strings.TrimSpace(userGroup)

	candidates := buildImagePlaygroundGroupCandidates(userGroup)
	if len(candidates) == 0 {
		return "", false, errors.New(
			"当前用户没有可用于 AI 生图的有效分组，请检查用户可用分组和分组倍率配置",
		)
	}

	// 每个 candidate 对两个模型仅查询一次，避免分支间重复触发 model.GetChannel。
	type supportInfo struct {
		group string
		image bool
		agent bool
	}
	infos := make([]supportInfo, 0, len(candidates))
	var firstImageCheckErr error
	for _, group := range candidates {
		imageSupported, imageErr := imagePlaygroundGroupSupportsModel(group, imagePlaygroundDefaultModel)
		if imageErr != nil {
			common.SysError(fmt.Sprintf("image-playground supports check failed: group=%s model=%s err=%v",
				group, imagePlaygroundDefaultModel, imageErr))
			if firstImageCheckErr == nil {
				firstImageCheckErr = imageErr
			}
		}
		agentSupported, agentErr := imagePlaygroundGroupSupportsModel(group, imagePlaygroundAgentModel)
		if agentErr != nil {
			common.SysError(fmt.Sprintf("image-playground supports check failed: group=%s model=%s err=%v",
				group, imagePlaygroundAgentModel, agentErr))
		}
		infos = append(infos, supportInfo{group: group, image: imageSupported, agent: agentSupported})
	}

	for _, info := range infos {
		if info.image && info.agent {
			return info.group, true, nil
		}
	}
	for _, info := range infos {
		if info.image {
			return info.group, false, nil
		}
	}
	if firstImageCheckErr != nil {
		return "", false, fmt.Errorf("检查 AI 生图分组能力失败: %w", firstImageCheckErr)
	}
	return "", false, fmt.Errorf(
		"当前用户没有支持 %s 的可用分组，请检查渠道模型与分组配置",
		imagePlaygroundDefaultModel,
	)
}

func buildImagePlaygroundGroupCandidates(userGroup string) []string {
	usableGroups := service.GetUserUsableGroups(userGroup)
	activeGroups := ratio_setting.GetGroupRatioCopy()
	candidates := make([]string, 0, len(usableGroups))
	add := func(group string) {
		group = strings.TrimSpace(group)
		if group == "" || common.StringsContains(candidates, group) {
			return
		}
		if _, ok := usableGroups[group]; !ok {
			return
		}
		if _, ok := activeGroups[group]; !ok {
			return
		}
		candidates = append(candidates, group)
	}

	add(imagePlaygroundPreferredGroup)
	add(userGroup)
	for _, group := range setting.GetAutoGroups() {
		add(group)
	}

	remainingGroups := make([]string, 0, len(usableGroups))
	for group := range usableGroups {
		if _, ok := activeGroups[group]; ok {
			remainingGroups = append(remainingGroups, group)
		}
	}
	sort.Strings(remainingGroups)
	for _, group := range remainingGroups {
		add(group)
	}
	return candidates
}

func imagePlaygroundGroupSupportsModel(group string, modelName string) (bool, error) {
	channel, err := model.GetChannel(group, modelName, 0, nil, nil, func(ch *model.Channel) bool {
		return ch != nil && ch.Status == common.ChannelStatusEnabled
	})
	if err != nil {
		// 区分"通道不存在"（model.GetChannel 返回 nil,nil）与"查询失败"：
		// 前者属于正常的"不支持"语义，后者由 caller 决定是否记录/降级，避免把瞬时
		// 数据库错误误判为永久"不支持"并写脏 token.Group。
		return false, err
	}
	return channel != nil, nil
}

func buildImagePlaygroundOrigin(c *gin.Context) string {
	// 最高优先级：由运维通过环境变量强制指定 origin（如绕开 Cloudflare 100s 超时的灰云域名）。
	if origin := preferredImagePlaygroundOrigin(); origin != "" {
		return origin
	}
	if origin := buildImagePlaygroundBrowserOrigin(c); origin != "" {
		return origin
	}
	if origin := buildImagePlaygroundRequestOrigin(c); origin != "" {
		return origin
	}
	if serverAddress := strings.TrimSpace(system_setting.ServerAddress); serverAddress != "" {
		return strings.TrimRight(serverAddress, "/")
	}
	return ""
}

// preferredImagePlaygroundOrigin 读取并校验 performance_setting 中配置的优先 origin。
// 仅接受 http/https 协议，返回标准化后的 "scheme://host"，无效或未配置时返回空串。
// 配置入口：管理后台 Setting → Performance → 中转网关超时 → 「image-playground 首选 origin」。
func preferredImagePlaygroundOrigin() string {
	raw := strings.TrimSpace(common.ImagePlaygroundPreferredOrigin)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func buildImagePlaygroundBrowserOrigin(c *gin.Context) string {
	if origin := parseImagePlaygroundHeaderOrigin(c.GetHeader("Origin")); isTrustedImagePlaygroundOrigin(c, origin) {
		return origin
	}

	referer := strings.TrimSpace(c.GetHeader("Referer"))
	if referer == "" {
		return ""
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	origin := parsed.Scheme + "://" + parsed.Host
	if isTrustedImagePlaygroundOrigin(c, origin) {
		return origin
	}
	return ""
}

func parseImagePlaygroundHeaderOrigin(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func isTrustedImagePlaygroundOrigin(c *gin.Context, origin string) bool {
	parsedOrigin, err := url.Parse(origin)
	if err != nil || parsedOrigin.Scheme == "" || parsedOrigin.Host == "" {
		return false
	}
	if parsedOrigin.Scheme != "http" && parsedOrigin.Scheme != "https" {
		return false
	}

	originHost := normalizeImagePlaygroundHost(parsedOrigin.Host)
	if originHost == "" {
		return false
	}
	if trustedImagePlaygroundHost(originHost, normalizeImagePlaygroundHost(firstHeaderValue(c.GetHeader("X-Forwarded-Host")))) {
		return true
	}
	if trustedImagePlaygroundHost(originHost, normalizeImagePlaygroundHost(c.Request.Host)) {
		return true
	}
	if serverAddress := strings.TrimSpace(system_setting.ServerAddress); serverAddress != "" {
		if parsedServer, err := url.Parse(serverAddress); err == nil {
			return trustedImagePlaygroundHost(originHost, normalizeImagePlaygroundHost(parsedServer.Host))
		}
	}
	return false
}

func trustedImagePlaygroundHost(originHost string, trustedHost string) bool {
	if originHost == "" || trustedHost == "" {
		return false
	}
	if originHost == trustedHost {
		return true
	}
	originDomain, originErr := publicsuffix.EffectiveTLDPlusOne(originHost)
	trustedDomain, trustedErr := publicsuffix.EffectiveTLDPlusOne(trustedHost)
	if originErr == nil && trustedErr == nil && originDomain == trustedDomain {
		return true
	}
	return strings.HasSuffix(originHost, "."+trustedHost) || strings.HasSuffix(trustedHost, "."+originHost)
}

func normalizeImagePlaygroundHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if parsed, err := url.Parse("//" + host); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.TrimSuffix(host, ".")
}

func buildImagePlaygroundRequestOrigin(c *gin.Context) string {
	host := firstHeaderValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}

	proto := firstHeaderValue(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		if strings.EqualFold(c.GetHeader("X-Forwarded-Ssl"), "on") || c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	return proto + "://" + host
}

func firstHeaderValue(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}

func buildImagePlaygroundLaunchURL(origin string, key string, supportsEcommerce bool) string {
	u := url.URL{
		Scheme: "http",
		Host:   "localhost",
		Path:   "/image-playground/",
	}
	if parsed, err := url.Parse(origin + "/image-playground/"); err == nil {
		u = *parsed
	}

	q := u.Query()
	q.Set("appMode", "gallery")
	settings := imagePlaygroundLaunchSettings{
		Profiles: []imagePlaygroundLaunchProfile{
			{
				ID:                    "newapi-image-playground",
				Name:                  "URL 参数配置",
				Provider:              "openai",
				BaseURL:               origin + "/v1",
				APIKey:                "sk-" + key,
				Model:                 imagePlaygroundDefaultModel,
				Timeout:               600,
				APIMode:               "images",
				CodexCli:              false,
				APIProxy:              false,
				StreamImages:          false,
				StreamPartialImages:   0,
				ResponseFormatB64Json: true,
			},
		},
		ActiveProfileID:   "newapi-image-playground",
		DefaultImageModel: imagePlaygroundDefaultModel,
		DefaultPlanModel:  imagePlaygroundAgentModel,
		SupportsEcommerce: supportsEcommerce,
	}
	if !supportsEcommerce {
		settings.EcommerceDisabledReason = fmt.Sprintf(
			"当前令牌分组未同时支持 %s 与 %s，电商套图模式已禁用。",
			imagePlaygroundDefaultModel, imagePlaygroundAgentModel,
		)
	}
	if settingsJSON, err := common.Marshal(settings); err == nil {
		q.Set("settings", string(settingsJSON))
	} else {
		q.Set("apiUrl", origin+"/v1")
		q.Set("apiKey", "sk-"+key)
		q.Set("model", imagePlaygroundDefaultModel)
		q.Set("apiMode", "images")
		q.Set("streamImages", "false")
	}
	u.RawQuery = q.Encode()
	return u.String()
}
