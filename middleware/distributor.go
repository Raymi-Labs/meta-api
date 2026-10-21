package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type ModelRequest struct {
	Model              string          `json:"model"`
	Group              string          `json:"group,omitempty"`
	Tools              json.RawMessage `json:"tools,omitempty"`
	ToolChoice         json.RawMessage `json:"tool_choice,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
}

func (r *ModelRequest) HasImageGenerationTool() bool {
	if r == nil {
		return false
	}
	request := dto.OpenAIResponsesRequest{
		Tools:      r.Tools,
		ToolChoice: r.ToolChoice,
	}
	return request.HasImageGenerationTool()
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		// Detect client tool type from request headers
		clientID := service.DetectClient(c)
		common.SetContextKey(c, constant.ContextKeyClientID, clientID)
		var selectedChannel *model.Channel

		var specificChannelID *int
		channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) {
				abortWithRequestBodyTooLarge(c)
				return
			}
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
			return
		}
		c.Set("original_model", modelRequest.Model)
		responsesImageGenerationTool := isResponsesCreateRequest(c) && modelRequest.HasImageGenerationTool()
		if shouldSelectChannel && strings.TrimSpace(modelRequest.Model) != "" {
			role := getModelPermissionRole(c)
			permissionModel := ratio_setting.FormatMatchingModelName(modelRequest.Model)
			if !model.IsModelCallableByRole(permissionModel, role) {
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("无权调用模型 %s", modelRequest.Model), types.ErrorCodeAccessDenied)
				return
			}
			if responsesImageGenerationTool {
				if !isImageGenerationToolCallableByRole(role) {
					abortWithOpenAiMessage(c, http.StatusForbidden, "无权调用图片生成工具 image_generation", types.ErrorCodeAccessDenied)
					return
				}
			}
		}
		if ok {
			id, err := strconv.Atoi(channelId.(string))
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
				return
			}
			specificChannelID = common.GetPointer(id)
			if !IsTokenChannelAllowed(c, id) {
				abortWithOpenAiMessage(c, http.StatusForbidden, "当前令牌不允许使用该渠道", types.ErrorCodeAccessDenied)
				return
			}
		}
		// Select a channel for the user
		// check token model mapping
		modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
		if modelLimitEnable {
			s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
			if !ok {
				// token model limit is empty, all models are not allowed
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
				return
			}
			var tokenModelLimit map[string]bool
			tokenModelLimit, ok = s.(map[string]bool)
			if !ok {
				tokenModelLimit = map[string]bool{}
			}
			matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model) // match gpts & thinking-*
			if _, ok := tokenModelLimit[matchName]; !ok {
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": modelRequest.Model}))
				return
			}
		}

		if shouldSelectChannel {
			if modelRequest.Model == "" {
				abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorModelNameRequired))
				return
			}
			usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
			// check path is /pg/chat/completions
			if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
				playgroundRequest := &dto.PlayGroundRequest{}
				err = common.UnmarshalBodyReusable(c, playgroundRequest)
				if err != nil {
					abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidPlayground, map[string]any{"Error": err.Error()}))
					return
				}
				if playgroundRequest.Group != "" {
					if !service.GroupInUserUsableGroups(usingGroup, playgroundRequest.Group) && playgroundRequest.Group != usingGroup {
						abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorGroupAccessDenied))
						return
					}
					usingGroup = playgroundRequest.Group
					common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
				}
			}
			service.PrepareSlowTTFTRequest(
				c,
				modelRequest.Model,
				usingGroup,
				strings.TrimSpace(modelRequest.PreviousResponseID) != "",
				responsesImageGenerationTool,
			)

			channel, selectGroup, selectErr := selectChannelForRequest(c, modelRequest.Model, usingGroup, clientID, specificChannelID)
			if selectErr != nil {
				recordUnavailableGroupHealth(c, modelRequest.Model, usingGroup, selectGroup, grouphealth.OutcomeForError(selectErr))
				abortWithOpenAiMessage(c, selectErr.StatusCode, selectErr.Error(), selectErr.GetErrorCode())
				return
			}
			if channel == nil {
				if !hasConfiguredRequestModel(c, modelRequest.Model, usingGroup) {
					abortWithOpenAiMessage(c, http.StatusNotFound, i18n.T(c, i18n.MsgDistributorModelNotFound, map[string]any{"Model": modelRequest.Model}), types.ErrorCodeModelNotFound)
					return
				}
				recordUnavailableGroupHealth(c, modelRequest.Model, usingGroup, selectGroup, grouphealth.OutcomeServiceFailure)
				showGroup := usingGroup
				if usingGroup == "auto" && selectGroup != "" {
					showGroup = fmt.Sprintf("auto(%s)", selectGroup)
				}
				abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgDistributorNoAvailableChannel, map[string]any{"Group": showGroup, "Model": modelRequest.Model}), types.ErrorCodeNoAvailableChannel)
				return
			}
			selectedChannel = channel
		}
		c.Next()
		if selectedChannel != nil && c.Writer != nil && c.Writer.Status() < http.StatusBadRequest {
			// relay 层可能因容量压力改选了其它渠道，应记录最终实际使用的渠道，
			// 不能继续使用进入 controller 前预选的 selectedChannel。
			actualChannelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
			if actualChannelID <= 0 {
				actualChannelID = selectedChannel.Id
			}
			service.RecordChannelAffinity(c, actualChannelID)
		}
	}
}

func isResponsesCreateRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	path := c.Request.URL.Path
	return strings.HasPrefix(path, "/v1/responses") && !strings.HasPrefix(path, "/v1/responses/compact")
}

func isImageGenerationToolCallableByRole(role int) bool {
	scope := model_setting.GetGlobalSettings().ImageGenerationToolCallPermission
	return model.IsModelPermissionScopeCallableByRole(scope, role)
}

func getModelPermissionRole(c *gin.Context) int {
	if role := common.GetContextKeyInt(c, constant.ContextKeyUserRole); role != common.RoleGuestUser {
		return role
	}
	if role := c.GetInt("role"); role != common.RoleGuestUser {
		return role
	}
	if userId := c.GetInt("id"); userId > 0 {
		if userCache, err := model.GetUserCache(userId); err == nil {
			return userCache.Role
		}
	}
	return common.RoleGuestUser
}

func appendUniqueChannelID(ids []int, channelID int) []int {
	for _, id := range ids {
		if id == channelID {
			return ids
		}
	}
	return append(ids, channelID)
}

func tryAffinityChannel(c *gin.Context, modelName string, usingGroup string, clientID string, excludeChannels []int) (*model.Channel, string, bool) {
	preferredChannelID, found := service.GetPreferredChannelByAffinity(c, modelName, usingGroup)
	if !found {
		return nil, "", false
	}
	for _, channelID := range excludeChannels {
		if channelID == preferredChannelID {
			return nil, "", false
		}
	}
	preferred, err := model.CacheGetChannel(preferredChannelID)
	if err != nil || preferred == nil {
		return nil, "", false
	}
	if service.IsCodexAutoReviewRequestModel(modelName) && !constant.IsCodexAutoReviewCompatibleChannelType(preferred.Type) {
		return nil, "", false
	}
	if !IsTokenChannelAllowed(c, preferred.Id) || !service.IsChannelAllowedForClient(preferred, clientID) {
		return nil, "", false
	}
	if service.IsChannelUnavailableForRequestContext(c, preferred) {
		return nil, "", false
	}
	if service.IsChannelModelCircuitOpen(preferred, modelName) {
		return nil, "", false
	}
	if usingGroup == "auto" {
		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		autoGroups := service.GetUserAutoGroup(userGroup)
		for _, group := range autoGroups {
			if model.IsChannelEnabledForGroupModel(group, modelName, preferred.Id) {
				return preferred, group, true
			}
		}
		return nil, "", false
	}
	if model.IsChannelEnabledForGroupModel(usingGroup, modelName, preferred.Id) {
		return preferred, usingGroup, true
	}
	return nil, "", false
}

// ResolveAffinityChannelForRelay 返回经【完整校验】的亲和绑定渠道及其生效分组，供 relay 层的
// 渠道亲和粘性复用——与 distributor 的亲和选择共用同一套校验（可用性/熔断、token 与客户端权限、
// codex-auto-review 兼容、分组内该模型是否启用、auto 分组展开），保证不遗漏、单一事实来源。
// 返回 (channel, selectGroup, true) 表示可安全粘定；false 表示无绑定或亲和渠道不可用/无权限。
func ResolveAffinityChannelForRelay(c *gin.Context, modelName string, usingGroup string, excludeChannels []int) (*model.Channel, string, bool) {
	clientID := service.DetectClient(c)
	return tryAffinityChannel(c, modelName, usingGroup, clientID, excludeChannels)
}

func selectChannelForRequest(c *gin.Context, modelName string, usingGroup string, clientID string, specificChannelID *int) (*model.Channel, string, *types.NewAPIError) {
	allowedChannels := GetAllowedTokenChannelIDs(c)
	excludeChannels := make([]int, 0, 4)

	if specificChannelID != nil {
		channel, err := model.GetChannelById(*specificChannelID, true)
		if err != nil {
			return nil, usingGroup, types.NewErrorWithStatusCode(errors.New(i18n.T(c, i18n.MsgDistributorInvalidChannelId)), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if !service.IsChannelAllowedForClient(channel, clientID) {
			return nil, usingGroup, types.NewErrorWithStatusCode(errors.New("当前客户端工具不允许使用该渠道"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		}
		if service.IsCodexAutoReviewRequestModel(modelName) && !constant.IsCodexAutoReviewCompatibleChannelType(channel.Type) {
			return nil, usingGroup, types.NewErrorWithStatusCode(errors.New("codex-auto-review only supports codex or openai channel"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		}
		if service.IsChannelUnavailableForRequest(channel) || service.IsChannelModelCircuitOpen(channel, modelName) {
			excludeChannels = appendUniqueChannelID(excludeChannels, channel.Id)
			common.DeleteContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		} else {
			setupErr := SetupContextForSelectedChannel(c, channel, modelName)
			if setupErr == nil {
				return channel, usingGroup, nil
			}
			if !service.ShouldFallbackAfterSetupError(setupErr) {
				return nil, usingGroup, setupErr
			}
			excludeChannels = appendUniqueChannelID(excludeChannels, channel.Id)
			common.DeleteContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		}
	}

	if preferred, selectGroup, ok := tryAffinityChannel(c, modelName, usingGroup, clientID, excludeChannels); ok {
		setupErr := SetupContextForSelectedChannel(c, preferred, modelName)
		if setupErr == nil {
			service.MarkChannelAffinityUsed(c, selectGroup, preferred.Id)
			return preferred, selectGroup, nil
		}
		if !service.ShouldFallbackAfterSetupError(setupErr) {
			return nil, selectGroup, setupErr
		}
		excludeChannels = appendUniqueChannelID(excludeChannels, preferred.Id)
	}

	retryParam := &service.RetryParam{
		Ctx:             c,
		ModelName:       modelName,
		TokenGroup:      usingGroup,
		AllowedChannels: allowedChannels,
		ExcludeChannels: excludeChannels,
		Retry:           common.GetPointer(0),
	}

	for {
		channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
		if err != nil {
			showGroup := usingGroup
			if usingGroup == "auto" && selectGroup != "" {
				showGroup = fmt.Sprintf("auto(%s)", selectGroup)
			}
			return nil, selectGroup, types.NewErrorWithStatusCode(
				errors.New(i18n.T(c, i18n.MsgDistributorGetChannelFailed, map[string]any{"Group": showGroup, "Model": modelName, "Error": err.Error()})),
				types.ErrorCodeGetChannelFailed,
				http.StatusServiceUnavailable,
				types.ErrOptionWithSkipRetry(),
			)
		}
		if channel == nil {
			return nil, selectGroup, nil
		}
		setupErr := SetupContextForSelectedChannel(c, channel, modelName)
		if setupErr == nil {
			return channel, selectGroup, nil
		}
		if !service.ShouldFallbackAfterSetupError(setupErr) {
			return nil, selectGroup, setupErr
		}
		retryParam.ExcludeChannels = appendUniqueChannelID(retryParam.ExcludeChannels, channel.Id)
	}
}

// getModelFromRequest 从请求中读取模型信息
// 根据 Content-Type 自动处理：
// - application/json
// - application/x-www-form-urlencoded
// - multipart/form-data
func getModelFromRequest(c *gin.Context) (*ModelRequest, error) {
	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) {
			return nil, err
		}
		return nil, errors.New(i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
	}
	return &modelRequest, nil
}

func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	if strings.Contains(c.Request.URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
			midjourneyRequest := dto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, errors.New(i18n.T(c, i18n.MsgDistributorInvalidMidjourney, map[string]any{"Error": err.Error()}))
			}
			midjourneyModel, mjErr, success := service.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, fmt.Errorf("%s", mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("%s", i18n.T(c, i18n.MsgDistributorInvalidParseModel))
				} else {
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		relayMode := relayconstant.RelayModeVideoSubmit
		c.Set("relay_mode", relayMode)
		shouldSelectChannel = false
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos") {
		//curl https://api.openai.com/v1/videos \
		//  -H "Authorization: Bearer $OPENAI_API_KEY" \
		//  -F "model=sora-2" \
		//  -F "prompt=A calico cat playing a piano on stage"
		//	-F input_reference="@image.jpg"
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			relayMode = relayconstant.RelayModeVideoSubmit
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			if req != nil {
				modelRequest.Model = req.Model
			}
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
			modelRequest.Model = getTaskOriginModelName(c)
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/video/generations") {
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			modelRequest.Model = req.Model
			relayMode = relayconstant.RelayModeVideoSubmit
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
			modelRequest.Model = getTaskOriginModelName(c)
		}
		if _, ok := c.Get("relay_mode"); !ok {
			c.Set("relay_mode", relayMode)
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models/") || strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
		// Gemini API 路径处理: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request.URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest = *req
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits") {
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			if err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {

			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
		// playground chat completions
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
		modelRequest.Group = req.Group
		common.SetContextKey(c, constant.ContextKeyTokenGroup, modelRequest.Group)
	}

	if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") && modelRequest.Model != "" {
		modelRequest.Model = ratio_setting.WithCompactModelSuffix(modelRequest.Model)
	}
	return &modelRequest, shouldSelectChannel, nil
}

func getTaskOriginModelName(c *gin.Context) string {
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return ""
	}

	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	if taskId == "" {
		return ""
	}

	userId := c.GetInt("id")
	task, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil || !exist || task == nil {
		return ""
	}
	return task.Properties.OriginModelName
}

func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) *types.NewAPIError {
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	tokenId := c.GetInt("token_id")
	key, index, newAPIError := channel.GetNextEnabledKeyForRequest(tokenId, modelName)
	if newAPIError != nil {
		return newAPIError
	}
	return SetupContextForSelectedChannelKey(c, channel, modelName, key, index)
}

func SetupContextForSelectedChannelKey(c *gin.Context, channel *model.Channel, modelName string, key string, index int) *types.NewAPIError {
	c.Set("original_model", modelName) // for retry
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelTag, channel.GetTag())
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, channel.GetParamOverride())
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, channel.GetHeaderOverride())
	if nil != channel.OpenAIOrganization && *channel.OpenAIOrganization != "" {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *channel.OpenAIOrganization)
	}
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channel.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, channel.GetStatusCodeMapping())
	if channel.ChannelInfo.IsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
	} else {
		// 必须设置为 false，否则在重试到单个 key 的时候会导致日志显示错误
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	}
	// c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)

	// TODO: api_version统一
	switch channel.Type {
	case constant.ChannelTypeAzure:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeVertexAi:
		c.Set("region", channel.Other)
	case constant.ChannelTypeXunfei:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeGemini:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeAli:
		c.Set("plugin", channel.Other)
	case constant.ChannelCloudflare:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeMokaAI:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeCoze:
		c.Set("bot_id", channel.Other)
	}
	return nil
}

// extractModelNameFromGeminiPath 从 Gemini API URL 路径中提取模型名
// 输入格式: /v1beta/models/gemini-2.0-flash:generateContent
// 输出: gemini-2.0-flash
func extractModelNameFromGeminiPath(path string) string {
	// 查找 "/models/" 的位置
	modelsPrefix := "/models/"
	modelsIndex := strings.Index(path, modelsPrefix)
	if modelsIndex == -1 {
		return ""
	}

	// 从 "/models/" 之后开始提取
	startIndex := modelsIndex + len(modelsPrefix)
	if startIndex >= len(path) {
		return ""
	}

	// 查找 ":" 的位置，模型名在 ":" 之前
	colonIndex := strings.Index(path[startIndex:], ":")
	if colonIndex == -1 {
		// 如果没有找到 ":"，返回从 "/models/" 到路径结尾的部分
		return path[startIndex:]
	}

	// 返回模型名部分
	return path[startIndex : startIndex+colonIndex]
}

// Selection outages are samples only for configured models after authentication
// and model/group permission checks. Invalid requests and unknown model names do
// not manufacture failures or unbounded metric cardinality.
func hasConfiguredRequestModel(c *gin.Context, modelName, usingGroup string) bool {
	groups := []string{usingGroup}
	if usingGroup == "auto" {
		groups = service.GetUserAutoGroup(common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	}
	exists, err := model.HasConfiguredModelInGroups(groups, modelName, GetAllowedTokenChannelIDs(c))
	// Database failures are service failures, not proof that a model is absent.
	return err != nil || exists
}

func recordUnavailableGroupHealth(c *gin.Context, modelName, usingGroup, selectedGroup string, outcome grouphealth.Outcome) {
	if !grouphealth.SupportsRequest(c.Request.Method, c.Request.URL.Path) {
		return
	}
	for _, group := range unavailableHealthGroups(c, modelName, usingGroup, selectedGroup) {
		grouphealth.Record(grouphealth.Sample{Group: group, Model: modelName, Outcome: outcome})
	}
}

func unavailableHealthGroups(c *gin.Context, modelName, usingGroup, selectedGroup string) []string {
	candidates := []string{usingGroup}
	if usingGroup == "auto" {
		candidates = []string{selectedGroup}
		if selectedGroup == "" || selectedGroup == "auto" {
			// An exhausted auto chain has no selected group. Include only its
			// configured, permitted model routes, never unsupported candidates.
			candidates = service.GetUserAutoGroup(common.GetContextKeyString(c, constant.ContextKeyUserGroup))
		}
	}
	groups := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	allowed := GetAllowedTokenChannelIDs(c)
	for _, group := range candidates {
		if !seen[group] && (model.HasConfiguredGroupModel(group, modelName, allowed) ||
			model.HasConfiguredGroupModel(group, ratio_setting.FormatMatchingModelName(modelName), allowed)) {
			groups = append(groups, group)
			seen[group] = true
		}
	}
	return groups
}
