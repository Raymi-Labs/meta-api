package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type tokenModelOption struct {
	Groups                 []string                `json:"groups"`
	Name                   string                  `json:"name"`
	SupportedEndpointTypes []constant.EndpointType `json:"supported_endpoint_types"`
}

type tokenAbilityEndpointRow struct {
	Model       string `json:"model"`
	ChannelType int    `json:"channel_type"`
}

type tokenTestAbilityChannelRow struct {
	Group     string `json:"group"`
	ChannelId int    `json:"channel_id"`
}

func shouldAcceptUnsetRatioModel(userId int) bool {
	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled
	if acceptUnsetRatioModel || userId <= 0 {
		return acceptUnsetRatioModel
	}
	userSettings, err := model.GetUserSetting(userId, false)
	if err != nil {
		return false
	}
	return userSettings.AcceptUnsetRatioModel
}

func hasConfiguredModelPrice(modelName string) bool {
	if _, _, exists := ratio_setting.GetModelRatioOrPrice(modelName); exists {
		return true
	}
	_, exists := billing_setting.GetTieredExpr(modelName)
	return exists
}

func filterVisibleModelsByRatio(models []string, acceptUnsetRatioModel bool) []string {
	filtered := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		if !acceptUnsetRatioModel {
			if !hasConfiguredModelPrice(modelName) {
				continue
			}
		}
		seen[modelName] = struct{}{}
		filtered = append(filtered, modelName)
	}
	return filtered
}

func collectGroupModels(groups []string, acceptUnsetRatioModel bool) []string {
	models := make([]string, 0)
	for _, group := range groups {
		models = append(models, model.GetGroupEnabledModels(group)...)
	}
	models = filterVisibleModelsByRatio(models, acceptUnsetRatioModel)
	sort.Strings(models)
	return models
}

func resolveRequestedTokenGroups(userId int, requestedGroup string, fallback ...bool) ([]string, error) {
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return nil, err
	}
	userGroup := userCache.Group
	group := strings.TrimSpace(requestedGroup)
	if group == "" {
		group = userGroup
	}
	if group == "auto" {
		groups := service.GetUserAutoGroup(userGroup)
		sort.Strings(groups)
		return groups, nil
	}
	if group != userGroup {
		if _, ok := service.GetUserUsableGroups(userGroup)[group]; !ok {
			return nil, fmt.Errorf("无权访问 %s 分组", group)
		}
	}
	if !ratio_setting.ContainsGroupRatio(group) && group != userGroup {
		return nil, fmt.Errorf("分组 %s 已被弃用", group)
	}
	return service.TokenRoutingGroups(userGroup, group, len(fallback) > 0 && fallback[0]), nil
}

func resolveRequestedTokenModelsRaw(userId int, requestedGroup string, fallback ...bool) ([]string, error) {
	groups, err := resolveRequestedTokenGroups(userId, requestedGroup, fallback...)
	if err != nil {
		return nil, err
	}
	return collectGroupModels(groups, shouldAcceptUnsetRatioModel(userId)), nil
}

func filterModelsVisibleToUser(userId int, models []string) ([]string, error) {
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return nil, err
	}
	return model.FilterModelsByVisibility(models, userCache.Role), nil
}

func resolveRequestedTokenModels(userId int, requestedGroup string, fallback ...bool) ([]string, error) {
	models, err := resolveRequestedTokenModelsRaw(userId, requestedGroup, fallback...)
	if err != nil {
		return nil, err
	}
	return filterModelsVisibleToUser(userId, models)
}

func resolveTokenAllowedModelsRaw(userId int, token *model.Token) ([]string, error) {
	if token == nil {
		return nil, fmt.Errorf("token is nil")
	}
	options, err := tokenRoutingCatalog(userId, token)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(options))
	for _, option := range options {
		names = append(names, option.Name)
	}
	return names, nil
}

func resolveTokenAllowedModels(userId int, token *model.Token) ([]string, error) {
	models, err := resolveTokenAllowedModelsRaw(userId, token)
	if err != nil {
		return nil, err
	}
	return filterModelsVisibleToUser(userId, models)
}

func buildTokenModelOptions(userId int, token *model.Token, requestedGroup string, fallback ...bool) ([]tokenModelOption, error) {
	var (
		models []string
		groups []string
		err    error
	)

	if token != nil {
		options, err := tokenRoutingCatalog(userId, token)
		if err != nil {
			return nil, err
		}
		user, err := model.GetUserCache(userId)
		if err != nil {
			return nil, err
		}
		visible := make([]tokenModelOption, 0, len(options))
		for _, option := range options {
			if model.IsModelVisibleToRole(option.Name, user.Role) {
				visible = append(visible, option)
			}
		}
		return visible, nil
	} else {
		models, err = resolveRequestedTokenModels(userId, requestedGroup, fallback...)
		if err != nil {
			return nil, err
		}
		groups, err = resolveRequestedTokenGroups(userId, requestedGroup, fallback...)
		if err != nil {
			return nil, err
		}
	}

	modelEndpointMap, err := collectTokenModelEndpointTypes(groups, models)
	if err != nil {
		return nil, err
	}
	options := make([]tokenModelOption, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		supportedEndpointTypes := normalizeTokenModelSupportedEndpointTypes(modelName, modelEndpointMap[modelName])
		if supportedEndpointTypes == nil {
			supportedEndpointTypes = make([]constant.EndpointType, 0)
		}
		options = append(options, tokenModelOption{
			Name:                   modelName,
			SupportedEndpointTypes: supportedEndpointTypes,
		})
	}
	return options, nil
}

func collectTokenModelEndpointTypes(groups []string, models []string) (map[string][]constant.EndpointType, error) {
	modelEndpointMap := make(map[string][]constant.EndpointType, len(models))
	if len(groups) == 0 || len(models) == 0 {
		return modelEndpointMap, nil
	}

	rows := make([]tokenAbilityEndpointRow, 0)
	query := model.DB.Table("abilities").
		Select("abilities.model, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where(clause.Eq{Column: clause.Column{Table: "abilities", Name: "enabled"}, Value: true}).
		Where(clause.IN{Column: clause.Column{Table: "abilities", Name: "group"}, Values: stringSliceToAny(groups)}).
		Where(clause.IN{Column: clause.Column{Table: "abilities", Name: "model"}, Values: stringSliceToAny(models)})
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		endpoints := modelEndpointMap[row.Model]
		for _, endpointType := range common.GetEndpointTypesByChannelType(row.ChannelType, row.Model) {
			if !containsEndpointType(endpoints, endpointType) {
				endpoints = append(endpoints, endpointType)
			}
		}
		modelEndpointMap[row.Model] = endpoints
	}

	return modelEndpointMap, nil
}

func normalizeTokenModelSupportedEndpointTypes(modelName string, supportedEndpointTypes []constant.EndpointType) []constant.EndpointType {
	requestPath, _ := resolveTestRequestPath(nil, modelName, "")
	switch requestPath {
	case "/v1/videos":
		return []constant.EndpointType{constant.EndpointTypeOpenAIVideo}
	case "/v1/messages":
		return []constant.EndpointType{constant.EndpointTypeAnthropic}
	case "/v1/responses":
		return []constant.EndpointType{constant.EndpointTypeOpenAIResponse}
	case "/v1/responses/compact":
		return []constant.EndpointType{constant.EndpointTypeOpenAIResponseCompact}
	case "/v1/embeddings":
		if len(supportedEndpointTypes) == 0 {
			return []constant.EndpointType{constant.EndpointTypeEmbeddings}
		}
	case "/v1/images/generations":
		// 生图模型的令牌测试必须走图片端点，避免错误落到 chat/completions。
		return []constant.EndpointType{constant.EndpointTypeImageGeneration}
	case "/v1/rerank":
		if len(supportedEndpointTypes) == 0 {
			return []constant.EndpointType{constant.EndpointTypeJinaRerank}
		}
	case "/v1/chat/completions":
		if len(supportedEndpointTypes) == 0 && common.IsOpenAITextModel(modelName) {
			return []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	}
	if common.IsOpenAITextModel(modelName) && containsEndpointType(supportedEndpointTypes, constant.EndpointTypeOpenAI) && !containsEndpointType(supportedEndpointTypes, constant.EndpointTypeOpenAIResponse) {
		supportedEndpointTypes = append(supportedEndpointTypes, constant.EndpointTypeOpenAIResponse)
	}
	return supportedEndpointTypes
}

func containsEndpointType(endpointTypes []constant.EndpointType, endpointType constant.EndpointType) bool {
	for _, existing := range endpointTypes {
		if existing == endpointType {
			return true
		}
	}
	return false
}

func stringSliceToAny(items []string) []interface{} {
	values := make([]interface{}, 0, len(items))
	for _, item := range items {
		values = append(values, item)
	}
	return values
}

func buildRelayExecutionTestRequest(modelName string, requestPath string, endpointType string, channel *model.Channel, isStream bool) dto.Request {
	if requestPath == "/v1/messages" {
		return &dto.ClaudeRequest{
			Model:  modelName,
			Stream: isStream,
			Messages: []dto.ClaudeMessage{
				{
					Role:    "user",
					Content: "hi",
				},
			},
			MaxTokens: 16,
		}
	}
	return buildTestRequest(modelName, endpointType, channel, isStream)
}

func prepareOwnedTokenContext(c *gin.Context, token *model.Token) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}
	userCache, err := model.GetUserCache(token.UserId)
	if err != nil {
		return err
	}
	if userCache.Status != common.UserStatusEnabled {
		return fmt.Errorf("用户已被封禁")
	}
	userCache.WriteContext(c)

	usingGroup := userCache.Group
	tokenGroup := strings.TrimSpace(token.Group)
	if tokenGroup != "" && token.RoutingConfig == "" {
		if _, ok := service.GetUserUsableGroups(userCache.Group)[tokenGroup]; !ok {
			return fmt.Errorf("无权访问 %s 分组", tokenGroup)
		}
		if tokenGroup != "auto" && !ratio_setting.ContainsGroupRatio(tokenGroup) {
			return fmt.Errorf("分组 %s 已被弃用", tokenGroup)
		}
		usingGroup = tokenGroup
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
	return middlewareSetupContextForToken(c, token)
}

func middlewareSetupContextForToken(c *gin.Context, token *model.Token) error {
	return middleware.SetupContextForToken(c, token)
}

func createInternalJSONContext(parent *gin.Context, method string, path string, body []byte, accept string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	request, _ := http.NewRequestWithContext(parent.Request.Context(), method, path, strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(accept) != "" {
		request.Header.Set("Accept", accept)
	}
	request.RemoteAddr = parent.Request.RemoteAddr
	ctx.Request = request
	return ctx, recorder
}

func tokenTestAcceptHeader(isStream bool) string {
	if isStream {
		return "text/event-stream"
	}
	return "application/json"
}

func copySelectionState(src *gin.Context, dst *gin.Context) {
	if autoGroup := common.GetContextKeyString(src, constant.ContextKeyAutoGroup); autoGroup != "" {
		common.SetContextKey(dst, constant.ContextKeyAutoGroup, autoGroup)
	}
	if usingGroup := common.GetContextKeyString(src, constant.ContextKeyUsingGroup); usingGroup != "" {
		common.SetContextKey(dst, constant.ContextKeyUsingGroup, usingGroup)
	}
}

func summarizeOpenAIStyleError(statusCode int, responseBody []byte, fallback string) string {
	message := strings.TrimSpace(detectErrorMessageFromJSONBytes(responseBody))
	if message != "" {
		return message
	}
	if fallback != "" {
		return fallback
	}
	if statusCode > 0 {
		return http.StatusText(statusCode)
	}
	return "测试失败"
}

func selectTokenTestChannelByAbility(c *gin.Context, modelName string) (*model.Channel, string, error) {
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if usingGroup == "" {
		usingGroup = userGroup
	}

	groups := service.RequestModelRoutingGroups(c, usingGroup, modelName)

	normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
	for _, group := range groups {
		channel, err := model.GetChannel(group, modelName, 0, nil, nil)
		if err != nil {
			return nil, "", err
		}
		if channel == nil && normalizedModel != "" && normalizedModel != modelName {
			channel, err = model.GetChannel(group, normalizedModel, 0, nil, nil)
			if err != nil {
				return nil, "", err
			}
		}
		if channel == nil {
			continue
		}
		if channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if usingGroup == "auto" || service.UsesPreferredGroupFallback(c, usingGroup) {
			common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
		return channel, group, nil
	}
	return nil, "", nil
}

func runTokenRelayTest(parent *gin.Context, token *model.Token, modelName string, endpointType string) (float64, string, error) {
	selectCtx, selectRecorder := createInternalJSONContext(parent, http.MethodPost, "/v1/chat/completions", nil, "application/json")
	defer common.CleanupBodyStorage(selectCtx)
	if err := prepareOwnedTokenContext(selectCtx, token); err != nil {
		return 0, "", err
	}
	releaseRuntimeLimit, err := middleware.AcquireTokenRuntimeLimit(selectCtx)
	if err != nil {
		return 0, err.Error(), err
	}
	defer releaseRuntimeLimit()
	start := time.Now()
	retryParam := &service.RetryParam{
		Ctx:             selectCtx,
		TokenGroup:      common.GetContextKeyString(selectCtx, constant.ContextKeyTokenGroup),
		ModelName:       modelName,
		AllowedChannels: middleware.GetAllowedTokenChannelIDs(selectCtx),
		Retry:           common.GetPointer(0),
	}

	var lastErr error
	lastMessage := ""
	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		channel, selectedGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
		if err != nil {
			lastErr = err
			lastMessage = err.Error()
			break
		}
		if channel == nil {
			break
		}
		if selectedGroup != "" && selectedGroup != "auto" {
			common.SetContextKey(selectCtx, constant.ContextKeyUsingGroup, selectedGroup)
		}

		if channelVideoTest(channel, modelName, endpointType) {
			preflight := inspectVideoConfiguration(channel, modelName)
			preflight.History, err = loadVideoHistory(channel.Id, modelName, token.UserId)
			preflight.HistoryUnavailable = err != nil
			parent.Set(videoPreflightKind, preflight)
			return 0, "", nil
		}

		execution := buildChannelStyleTestExecution(channel, modelName, endpointType, nil)
		execCtx, execRecorder := createInternalJSONContext(parent, http.MethodPost, execution.requestPath, nil, tokenTestAcceptHeader(execution.isStream))

		if err = prepareOwnedTokenContext(execCtx, token); err != nil {
			common.CleanupBodyStorage(execCtx)
			return 0, "", err
		}
		copySelectionState(selectCtx, execCtx)
		common.SetContextKey(execCtx, constant.ContextKeyRequestStartTime, time.Now())
		if newAPIError := middleware.SetupContextForSelectedChannel(execCtx, channel, execution.modelName); newAPIError != nil {
			common.CleanupBodyStorage(execCtx)
			lastErr = fmt.Errorf("setup selected channel failed")
			lastMessage = newAPIError.Error()
			return float64(time.Since(start).Milliseconds()) / 1000.0, lastMessage, lastErr
		}

		result := executeChannelStyleTest(execCtx, execRecorder, channel, execution.modelName, execution.endpointType, execution.request, execution.relayFormat, execution.isStream, testExecutionOptions{
			enableBilling: true,
			isChannelTest: false,
		})
		common.CleanupBodyStorage(execCtx)
		elapsed := float64(time.Since(start).Milliseconds()) / 1000.0
		if result.localErr == nil {
			return elapsed, "", nil
		}

		message := "测试失败"
		if result.responseBody != nil {
			message = summarizeOpenAIStyleError(execRecorder.Code, result.responseBody, message)
		} else if result.newAPIError != nil {
			message = result.newAPIError.Error()
		}
		lastErr = result.localErr
		lastMessage = message

		if result.newAPIError != nil && service.IsChannelModelMismatchError(result.newAPIError) {
			service.ApplyChannelFailureRetryExclusion(retryParam, channel, result.newAPIError)
			retryParam.ResetRetryNextTry()
			continue
		}
		if result.newAPIError != nil {
			service.ApplyChannelFailureRetryExclusion(retryParam, channel, result.newAPIError)
			if service.ShouldRetryChannelError(execCtx, result.newAPIError, common.RetryTimes-retryParam.GetRetry()) {
				continue
			}
		}
		return elapsed, message, result.localErr
	}

	elapsed := float64(time.Since(start).Milliseconds()) / 1000.0
	if lastErr != nil {
		return elapsed, lastMessage, lastErr
	}
	if selectCtx.IsAborted() || selectRecorder.Code >= http.StatusBadRequest {
		return elapsed, summarizeOpenAIStyleError(selectRecorder.Code, selectRecorder.Body.Bytes(), "模型分发失败"), fmt.Errorf("token test distribute failed")
	}
	return elapsed, "未选中可用渠道", fmt.Errorf("no channel selected")
}
