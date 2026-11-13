package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func RelayMidjourneyImage(c *gin.Context) {
	taskId := c.Param("id")
	midjourneyTask := model.GetMJTaskForSubject(mjTaskSubject(c), taskId)
	if midjourneyTask == nil {
		c.JSON(400, gin.H{
			"error": "midjourney_task_not_found",
		})
		return
	}
	fetch := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(midjourneyTask.ImageUrl, fetch.EnableSSRFProtection, fetch.AllowPrivateIp, fetch.DomainFilterMode, fetch.IpFilterMode, fetch.DomainList, fetch.IpList, fetch.AllowedPorts, fetch.ApplyIPFilterForDomain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_url_not_allowed"})
		return
	}
	var httpClient *http.Client
	if channel, err := model.CacheGetChannel(midjourneyTask.ChannelId); err == nil {
		proxy := channel.GetSetting().Proxy
		if proxy != "" {
			if httpClient, err = service.NewProxyHttpClient(proxy); err != nil {
				c.JSON(400, gin.H{
					"error": "proxy_url_invalid",
				})
				return
			}
		}
	}
	if httpClient == nil {
		httpClient = service.GetHttpClient()
	}
	resp, err := httpClient.Get(midjourneyTask.ImageUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "http_get_image_failed",
		})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		c.JSON(resp.StatusCode, gin.H{
			"error": string(responseBody),
		})
		return
	}
	// 从Content-Type头获取MIME类型
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		// 如果无法确定内容类型，则默认为jpeg
		contentType = "image/jpeg"
	}
	// 设置响应的内容类型
	c.Writer.Header().Set("Content-Type", contentType)
	// 将图片流式传输到响应体
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		log.Println("Failed to stream image:", err)
	}
	return
}

func RelayMidjourneyNotify(c *gin.Context) *dto.MidjourneyResponse {
	var midjRequest dto.MidjourneyDto
	err := common.UnmarshalBodyReusable(c, &midjRequest)
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "bind_request_body_failed",
			Properties:  nil,
			Result:      "",
		}
	}
	midjourneyTask := model.GetMJTaskForSubject(mjTaskSubject(c), midjRequest.MjId)
	if midjourneyTask == nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "midjourney_task_not_found",
			Properties:  nil,
			Result:      "",
		}
	}
	// A user API token is not proof of an upstream outcome. Enterprise callbacks
	// are scoped acknowledgements only; the authenticated channel poll confirms status
	// before funds can move. Personal callbacks retain their historical semantics.
	if midjourneyTask.EnterpriseId > 0 {
		return nil
	}
	midjourneyTask.Progress = midjRequest.Progress
	midjourneyTask.PromptEn = midjRequest.PromptEn
	midjourneyTask.State = midjRequest.State
	midjourneyTask.SubmitTime = midjRequest.SubmitTime
	midjourneyTask.StartTime = midjRequest.StartTime
	midjourneyTask.FinishTime = midjRequest.FinishTime
	midjourneyTask.ImageUrl = midjRequest.ImageUrl
	midjourneyTask.VideoUrl = midjRequest.VideoUrl
	videoUrlsStr, _ := common.Marshal(midjRequest.VideoUrls)
	midjourneyTask.VideoUrls = string(videoUrlsStr)
	midjourneyTask.Status = midjRequest.Status
	midjourneyTask.FailReason = midjRequest.FailReason
	err = midjourneyTask.Update()
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "update_midjourney_task_failed",
		}
	}

	return nil
}

func applyMidjourneyRateLimitMetadata(c *gin.Context, result *dto.MidjourneyResponseWithStatusCode) {
	if c == nil || result == nil || result.UpstreamStatusCode != http.StatusTooManyRequests {
		return
	}
	description := strings.TrimSpace(result.Response.Description)
	if description == "" {
		description = "midjourney upstream rate limited"
	}
	// controller 通过 Code=30 统一映射为 429；即使上游响应体使用其它错误码，
	// 也必须以真实 HTTP 429 为准。
	result.Response.Code = 30
	apiErr := types.NewOpenAIError(
		fmt.Errorf("%s", description),
		types.ErrorCodeBadResponseStatusCode,
		result.StatusCode,
	)
	apiErr.UpstreamStatusCode = result.UpstreamStatusCode
	apiErr.RetryAfter = result.RetryAfter

	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	if channelID > 0 {
		if channel, err := model.CacheGetChannel(channelID); err == nil && channel != nil {
			service.RecordChannelRateLimitCooldown(c, channel, apiErr)
		}
	}
	result.RetryAfter = apiErr.RetryAfter
	if result.RetryAfter > 0 {
		seconds := int((result.RetryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(seconds))
	}
}

func coverMidjourneyTaskDto(c *gin.Context, originTask *model.Midjourney) (midjourneyTask dto.MidjourneyDto) {
	midjourneyTask.MjId = originTask.MjId
	midjourneyTask.Progress = originTask.Progress
	midjourneyTask.PromptEn = originTask.PromptEn
	midjourneyTask.State = originTask.State
	midjourneyTask.SubmitTime = originTask.SubmitTime
	midjourneyTask.StartTime = originTask.StartTime
	midjourneyTask.FinishTime = originTask.FinishTime
	midjourneyTask.ImageUrl = ""
	if originTask.ImageUrl != "" && setting.MjForwardUrlEnabled {
		midjourneyTask.ImageUrl = system_setting.ServerAddress + "/mj/image/" + originTask.MjId
		if originTask.Status != "SUCCESS" {
			midjourneyTask.ImageUrl += "?rand=" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
	} else {
		midjourneyTask.ImageUrl = originTask.ImageUrl
	}
	if originTask.VideoUrl != "" {
		midjourneyTask.VideoUrl = originTask.VideoUrl
	}
	midjourneyTask.Status = originTask.Status
	midjourneyTask.FailReason = originTask.FailReason
	midjourneyTask.Action = originTask.Action
	midjourneyTask.Description = originTask.Description
	midjourneyTask.Prompt = originTask.Prompt
	if originTask.Buttons != "" {
		var buttons []dto.ActionButton
		err := common.Unmarshal([]byte(originTask.Buttons), &buttons)
		if err == nil {
			midjourneyTask.Buttons = buttons
		}
	}
	if originTask.VideoUrls != "" {
		var videoUrls []dto.ImgUrls
		err := common.Unmarshal([]byte(originTask.VideoUrls), &videoUrls)
		if err == nil {
			midjourneyTask.VideoUrls = videoUrls
		}
	}
	if originTask.Properties != "" {
		var properties dto.Properties
		err := common.Unmarshal([]byte(originTask.Properties), &properties)
		if err == nil {
			midjourneyTask.Properties = &properties
		}
	}
	return
}

func RelaySwapFace(c *gin.Context, info *relaycommon.RelayInfo) *dto.MidjourneyResponse {
	var swapFaceRequest dto.SwapFaceRequest
	err := common.UnmarshalBodyReusable(c, &swapFaceRequest)
	if err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "bind_request_body_failed")
	}

	info.InitChannelMeta(c)

	if swapFaceRequest.SourceBase64 == "" || swapFaceRequest.TargetBase64 == "" {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "sour_base64_and_target_base64_is_required")
	}
	modelName := service.CovertMjpActionToModelName(constant.MjActionSwapFace)

	priceData := helper.ModelPriceHelperPerCall(c, info)
	if info.EnterpriseId > 0 {
		return relayEnterpriseMJSubmit(c, info, constant.MjActionSwapFace, "InsightFace", true)
	}

	userQuota, err := model.GetUserQuota(info.UserId, false)
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: err.Error(),
		}
	}

	if userQuota-priceData.Quota < 0 {
		service.MarkRequestFailure(c, types.NewErrorWithStatusCode(&types.QuotaRejection{Source: service.BillingSourceWallet, Available: int64(userQuota), Required: int64(priceData.Quota), Known: true, Message: "quota_not_enough"}, types.ErrorCodeInsufficientUserQuota, http.StatusBadRequest))
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "quota_not_enough",
		}
	}
	requestURL := getMjRequestPath(c.Request.URL.String())
	baseURL := c.GetString("base_url")
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, requestURL)
	mjResp, _, err := service.DoMidjourneyHttpRequest(c, time.Second*60, fullRequestURL)
	applyMidjourneyRateLimitMetadata(c, mjResp)
	if err != nil {
		return &mjResp.Response
	}
	defer func() {
		if mjResp.StatusCode == 200 && mjResp.Response.Code == 1 {
			err := service.PostConsumeQuota(info, priceData.Quota, 0, true)
			if err != nil {
				common.SysLog("error consuming token remain quota: " + err.Error())
			}

			tokenName := c.GetString("token_name")
			logContent := fmt.Sprintf("模型固定价格 %.2f，分组倍率 %.2f，操作 %s", priceData.ModelPrice, priceData.GroupRatioInfo.GroupRatio, constant.MjActionSwapFace)
			if timeRatioContent := service.FormatTimeRatioContent(priceData); timeRatioContent != "" {
				logContent += "，" + timeRatioContent
			}
			other := service.GenerateMjOtherInfo(info, priceData)
			model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
				ChannelId: info.ChannelId,
				ModelName: modelName,
				TokenName: tokenName,
				Quota:     priceData.Quota,
				Content:   logContent,
				TokenId:   info.TokenId,
				Group:     info.UsingGroup,
				Other:     other,
			})
			if info.BillingSource == service.BillingSourceToken {
				model.UpdateUserRequestCount(info.UserId)
			} else {
				model.UpdateUserUsedQuotaAndRequestCount(info.UserId, priceData.Quota)
			}
			model.UpdateChannelUsedQuota(info.ChannelId, priceData.Quota)
		}
	}()
	midjResponse := &mjResp.Response
	midjourneyTask := &model.Midjourney{
		PollingKey:     strings.TrimPrefix(common.GetContextKeyString(c, constant.ContextKeyChannelKey), "Bearer "),
		PollingBaseURL: c.GetString("base_url"),
		UserId:         info.UserId,
		Code:           midjResponse.Code,
		Action:         constant.MjActionSwapFace,
		MjId:           midjResponse.Result,
		Prompt:         "InsightFace",
		PromptEn:       "",
		Description:    midjResponse.Description,
		State:          "",
		SubmitTime:     info.StartTime.UnixNano() / int64(time.Millisecond),
		StartTime:      time.Now().UnixNano() / int64(time.Millisecond),
		FinishTime:     0,
		ImageUrl:       "",
		Status:         "",
		Progress:       "0%",
		FailReason:     "",
		ChannelId:      c.GetInt("channel_id"),
		Quota:          priceData.Quota,
	}
	err = midjourneyTask.Insert()
	if err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "insert_midjourney_task_failed")
	}
	c.Writer.WriteHeader(mjResp.StatusCode)
	respBody, err := common.Marshal(midjResponse)
	if err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "unmarshal_response_body_failed")
	}
	_, err = io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "copy_response_body_failed")
	}
	return nil
}

func RelayMidjourneyTaskImageSeed(c *gin.Context) *dto.MidjourneyResponse {
	taskId := c.Param("id")
	originTask := model.GetMJTaskForSubject(mjTaskSubject(c), taskId)
	if originTask == nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "task_no_found")
	}
	channel, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "get_channel_info_failed")
	}
	if channel.Status != common.ChannelStatusEnabled {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "该任务所属渠道已被禁用")
	}
	if mjErr := prepareMJOriginChannel(c, channel, c.GetString("original_model")); mjErr != nil {
		return mjErr
	}

	requestURL := getMjRequestPath(c.Request.URL.String())
	fullRequestURL := fmt.Sprintf("%s%s", channel.GetBaseURL(), requestURL)
	midjResponseWithStatus, _, err := service.DoMidjourneyHttpRequest(c, time.Second*30, fullRequestURL)
	applyMidjourneyRateLimitMetadata(c, midjResponseWithStatus)
	if err != nil {
		return &midjResponseWithStatus.Response
	}
	midjResponse := &midjResponseWithStatus.Response
	c.Writer.WriteHeader(midjResponseWithStatus.StatusCode)
	respBody, err := common.Marshal(midjResponse)
	if err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "unmarshal_response_body_failed")
	}
	service.IOCopyBytesGracefully(c, nil, respBody)
	return nil
}

func RelayMidjourneyTask(c *gin.Context, relayMode int) *dto.MidjourneyResponse {
	var err error
	var respBody []byte
	switch relayMode {
	case relayconstant.RelayModeMidjourneyTaskFetch:
		taskId := c.Param("id")
		originTask := model.GetMJTaskForSubject(mjTaskSubject(c), taskId)
		if originTask == nil {
			return &dto.MidjourneyResponse{
				Code:        4,
				Description: "task_no_found",
			}
		}
		midjourneyTask := coverMidjourneyTaskDto(c, originTask)
		respBody, err = common.Marshal(midjourneyTask)
		if err != nil {
			return &dto.MidjourneyResponse{
				Code:        4,
				Description: "unmarshal_response_body_failed",
			}
		}
	case relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		var condition = struct {
			IDs []string `json:"ids"`
		}{}
		err = c.BindJSON(&condition)
		if err != nil {
			return &dto.MidjourneyResponse{
				Code:        4,
				Description: "do_request_failed",
			}
		}
		var tasks []dto.MidjourneyDto
		if len(condition.IDs) != 0 {
			originTasks := model.GetMJTasksForSubject(mjTaskSubject(c), condition.IDs)
			for _, originTask := range originTasks {
				midjourneyTask := coverMidjourneyTaskDto(c, originTask)
				tasks = append(tasks, midjourneyTask)
			}
		}
		if tasks == nil {
			tasks = make([]dto.MidjourneyDto, 0)
		}
		respBody, err = common.Marshal(tasks)
		if err != nil {
			return &dto.MidjourneyResponse{
				Code:        4,
				Description: "unmarshal_response_body_failed",
			}
		}
	}

	c.Writer.Header().Set("Content-Type", "application/json")

	_, err = io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "copy_response_body_failed",
		}
	}
	return nil
}

func RelayMidjourneySubmit(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.MidjourneyResponse {
	consumeQuota := true
	var midjRequest dto.MidjourneyRequest
	err := common.UnmarshalBodyReusable(c, &midjRequest)
	if err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "bind_request_body_failed")
	}

	relayInfo.InitChannelMeta(c)
	if !unambiguousMJReferences(c) {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "ambiguous_task_reference")
	}

	originTask, originErr := ResolveMidjourneyOriginTask(c, relayInfo.RelayMode, &midjRequest)
	if originErr != nil {
		return originErr
	}
	if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyAction { // midjourney plus，需要从customId中获取任务信息
		mjErr := service.CoverPlusActionToNormalAction(&midjRequest)
		if mjErr != nil {
			return mjErr
		}
		relayInfo.RelayMode = relayconstant.RelayModeMidjourneyChange
	}
	if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyVideo {
		midjRequest.Action = constant.MjActionVideo
	}

	if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyImagine { //绘画任务，此类任务可重复
		if midjRequest.Prompt == "" {
			return service.MidjourneyErrorWrapper(constant.MjRequestError, "prompt_is_required")
		}
		midjRequest.Action = constant.MjActionImagine
	} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyDescribe { //按图生文任务，此类任务可重复
		midjRequest.Action = constant.MjActionDescribe
	} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyEdits { //编辑任务，此类任务可重复
		midjRequest.Action = constant.MjActionEdits
	} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyShorten { //缩短任务，此类任务可重复，plus only
		midjRequest.Action = constant.MjActionShorten
	} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyBlend { //绘画任务，此类任务可重复
		midjRequest.Action = constant.MjActionBlend
	} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyUpload { //绘画任务，此类任务可重复
		midjRequest.Action = constant.MjActionUpload
	} else if midjRequest.TaskId != "" { //放大、变换任务，此类任务，如果重复且已有结果，远端api会直接返回最终结果
		mjId := ""
		if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyChange {
			if midjRequest.TaskId == "" {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "task_id_is_required")
			} else if midjRequest.Action == "" {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "action_is_required")
			} else if midjRequest.Index == 0 {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "index_is_required")
			}
			//action = midjRequest.Action
			mjId = midjRequest.TaskId
		} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneySimpleChange {
			if midjRequest.Content == "" {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "content_is_required")
			}
			params := parseMJSimpleChange(midjRequest.Content)
			if params == nil {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "content_parse_failed")
			}
			mjId = params.TaskId
			midjRequest.Action = params.Action
		} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyModal {
			//if midjRequest.MaskBase64 == "" {
			//	return service.MidjourneyErrorWrapper(constant.MjRequestError, "mask_base64_is_required")
			//}
			mjId = midjRequest.TaskId
			midjRequest.Action = constant.MjActionModal
		} else if relayInfo.RelayMode == relayconstant.RelayModeMidjourneyVideo {
			midjRequest.Action = constant.MjActionVideo
			if midjRequest.TaskId == "" {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "task_id_is_required")
			} else if midjRequest.Action == "" {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "action_is_required")
			}
			mjId = midjRequest.TaskId
		}

		if originTask == nil || originTask.MjId != mjId {
			originTask = model.GetMJTaskForSubject(mjTaskSubject(c), mjId)
		}
		if originTask == nil {
			return service.MidjourneyErrorWrapper(constant.MjRequestError, "task_not_found")
		} else { //原任务的Status=SUCCESS，则可以做放大UPSCALE、变换VARIATION等动作，此时必须使用原来的请求地址才能正确处理
			if setting.MjActionCheckSuccessEnabled {
				if originTask.Status != "SUCCESS" && relayInfo.RelayMode != relayconstant.RelayModeMidjourneyModal {
					return service.MidjourneyErrorWrapper(constant.MjRequestError, "task_status_not_success")
				}
			}
			channel, err := model.GetChannelById(originTask.ChannelId, true)
			if err != nil {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "get_channel_info_failed")
			}
			if channel.Status != common.ChannelStatusEnabled {
				return service.MidjourneyErrorWrapper(constant.MjRequestError, "该任务所属渠道已被禁用")
			}
			if mjErr := prepareMJOriginChannel(c, channel, relayInfo.OriginModelName); mjErr != nil {
				return mjErr
			}
			relayInfo.InitChannelMeta(c)
			log.Printf("检测到此操作为放大、变换、重绘，获取原channel信息: %s,%s", strconv.Itoa(originTask.ChannelId), channel.GetBaseURL())
		}
		midjRequest.Prompt = originTask.Prompt

		//if channelType == common.ChannelTypeMidjourneyPlus {
		//	// plus
		//} else {
		//	// 普通版渠道
		//
		//}
	}

	if midjRequest.Action == constant.MjActionInPaint || midjRequest.Action == constant.MjActionCustomZoom {
		consumeQuota = false
	}

	//baseURL := common.ChannelBaseURLs[channelType]
	requestURL := getMjRequestPath(c.Request.URL.String())

	baseURL := c.GetString("base_url")

	//midjRequest.NotifyHook = "http://127.0.0.1:3000/mj/notify"

	fullRequestURL := fmt.Sprintf("%s%s", baseURL, requestURL)

	modelName := service.CovertMjpActionToModelName(midjRequest.Action)

	priceData := helper.ModelPriceHelperPerCall(c, relayInfo)
	if relayInfo.EnterpriseId > 0 {
		return relayEnterpriseMJSubmit(c, relayInfo, midjRequest.Action, midjRequest.Prompt, consumeQuota)
	}

	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: err.Error(),
		}
	}

	if consumeQuota && userQuota-priceData.Quota < 0 {
		service.MarkRequestFailure(c, types.NewErrorWithStatusCode(&types.QuotaRejection{Source: service.BillingSourceWallet, Available: int64(userQuota), Required: int64(priceData.Quota), Known: true, Message: "quota_not_enough"}, types.ErrorCodeInsufficientUserQuota, http.StatusBadRequest))
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "quota_not_enough",
		}
	}

	midjResponseWithStatus, responseBody, err := service.DoMidjourneyHttpRequest(c, time.Second*60, fullRequestURL)
	applyMidjourneyRateLimitMetadata(c, midjResponseWithStatus)
	if err != nil {
		return &midjResponseWithStatus.Response
	}
	midjResponse := &midjResponseWithStatus.Response

	defer func() {
		if consumeQuota && midjResponseWithStatus.StatusCode == 200 {
			err := service.PostConsumeQuota(relayInfo, priceData.Quota, 0, true)
			if err != nil {
				common.SysLog("error consuming token remain quota: " + err.Error())
			}
			tokenName := c.GetString("token_name")
			logContent := fmt.Sprintf("模型固定价格 %.2f，分组倍率 %.2f，操作 %s，ID %s", priceData.ModelPrice, priceData.GroupRatioInfo.GroupRatio, midjRequest.Action, midjResponse.Result)
			if timeRatioContent := service.FormatTimeRatioContent(priceData); timeRatioContent != "" {
				logContent += "，" + timeRatioContent
			}
			other := service.GenerateMjOtherInfo(relayInfo, priceData)
			model.RecordConsumeLog(c, relayInfo.UserId, model.RecordConsumeLogParams{
				ChannelId: relayInfo.ChannelId,
				ModelName: modelName,
				TokenName: tokenName,
				Quota:     priceData.Quota,
				Content:   logContent,
				TokenId:   relayInfo.TokenId,
				Group:     relayInfo.UsingGroup,
				Other:     other,
			})
			if relayInfo.BillingSource == service.BillingSourceToken {
				model.UpdateUserRequestCount(relayInfo.UserId)
			} else {
				model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, priceData.Quota)
			}
			model.UpdateChannelUsedQuota(relayInfo.ChannelId, priceData.Quota)
		}
	}()

	// 文档：https://github.com/novicezk/midjourney-proxy/blob/main/docs/api.md
	//1-提交成功
	// 21-任务已存在（处理中或者有结果了） {"code":21,"description":"任务已存在","result":"0741798445574458","properties":{"status":"SUCCESS","imageUrl":"https://xxxx"}}
	// 22-排队中 {"code":22,"description":"排队中，前面还有1个任务","result":"0741798445574458","properties":{"numberOfQueues":1,"discordInstanceId":"1118138338562560102"}}
	// 23-队列已满，请稍后再试 {"code":23,"description":"队列已满，请稍后尝试","result":"14001929738841620","properties":{"discordInstanceId":"1118138338562560102"}}
	// 24-prompt包含敏感词 {"code":24,"description":"可能包含敏感词","properties":{"promptEn":"nude body","bannedWord":"nude"}}
	// other: 提交错误，description为错误描述
	midjourneyTask := &model.Midjourney{
		PollingKey:     strings.TrimPrefix(common.GetContextKeyString(c, constant.ContextKeyChannelKey), "Bearer "),
		PollingBaseURL: c.GetString("base_url"),
		UserId:         relayInfo.UserId,
		Code:           midjResponse.Code,
		Action:         midjRequest.Action,
		MjId:           midjResponse.Result,
		Prompt:         midjRequest.Prompt,
		PromptEn:       "",
		Description:    midjResponse.Description,
		State:          "",
		SubmitTime:     time.Now().UnixNano() / int64(time.Millisecond),
		StartTime:      0,
		FinishTime:     0,
		ImageUrl:       "",
		Status:         "",
		Progress:       "0%",
		FailReason:     "",
		ChannelId:      c.GetInt("channel_id"),
		Quota:          priceData.Quota,
	}
	if midjResponse.Code == 3 {
		//无实例账号自动禁用渠道（No available account instance）
		channel, err := model.GetChannelById(midjourneyTask.ChannelId, true)
		if err != nil {
			common.SysLog("get_channel_null: " + err.Error())
		}
		if channel != nil && channel.GetAutoBan() && common.AutomaticDisableChannelEnabled {
			model.UpdateChannelStatus(midjourneyTask.ChannelId, "", 2, "No available account instance")
		}
	}
	if midjResponse.Code != 1 && midjResponse.Code != 21 && midjResponse.Code != 22 {
		//非1-提交成功,21-任务已存在和22-排队中，则记录错误原因
		midjourneyTask.FailReason = midjResponse.Description
		consumeQuota = false
	}

	if midjResponse.Code == 21 { //21-任务已存在（处理中或者有结果了）
		// 将 properties 转换为一个 map
		properties, ok := midjResponse.Properties.(map[string]interface{})
		if ok {
			imageUrl, ok1 := properties["imageUrl"].(string)
			status, ok2 := properties["status"].(string)
			if ok1 && ok2 {
				midjourneyTask.ImageUrl = imageUrl
				midjourneyTask.Status = status
				if status == "SUCCESS" {
					midjourneyTask.Progress = "100%"
					midjourneyTask.StartTime = time.Now().UnixNano() / int64(time.Millisecond)
					midjourneyTask.FinishTime = time.Now().UnixNano() / int64(time.Millisecond)
					midjResponse.Code = 1
				}
			}
		}
		//修改返回值
		if midjRequest.Action != constant.MjActionInPaint && midjRequest.Action != constant.MjActionCustomZoom {
			newBody := strings.Replace(string(responseBody), `"code":21`, `"code":1`, -1)
			responseBody = []byte(newBody)
		}
	}
	if midjResponse.Code == 1 && midjRequest.Action == "UPLOAD" {
		midjourneyTask.Progress = "100%"
		midjourneyTask.Status = "SUCCESS"
	}
	err = midjourneyTask.Insert()
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "insert_midjourney_task_failed",
		}
	}

	if midjResponse.Code == 22 { //22-排队中，说明任务已存在
		//修改返回值
		newBody := strings.Replace(string(responseBody), `"code":22`, `"code":1`, -1)
		responseBody = []byte(newBody)
	}
	//resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
	bodyReader := io.NopCloser(bytes.NewBuffer(responseBody))

	//for k, v := range resp.Header {
	//	c.Writer.Header().Set(k, v[0])
	//}
	c.Writer.WriteHeader(midjResponseWithStatus.StatusCode)

	_, err = io.Copy(c.Writer, bodyReader)
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "copy_response_body_failed",
		}
	}
	err = bodyReader.Close()
	if err != nil {
		return &dto.MidjourneyResponse{
			Code:        4,
			Description: "close_response_body_failed",
		}
	}
	return nil
}

type taskChangeParams struct {
	ID     string
	Action string
	Index  int
}

func getMjRequestPath(path string) string {
	requestURL := path
	if strings.Contains(requestURL, "/mj-") {
		urls := strings.Split(requestURL, "/mj/")
		if len(urls) < 2 {
			return requestURL
		}
		requestURL = "/mj/" + urls[1]
	}
	return requestURL
}

func mjTaskSubject(c *gin.Context) model.MJTaskSubject {
	return model.MJTaskSubject{UserId: c.GetInt("id"), EnterpriseId: common.GetContextKeyInt(c, constant.ContextKeyEnterpriseId), MemberId: common.GetContextKeyInt(c, constant.ContextKeyEnterpriseMemberId)}
}

func relayEnterpriseMJSubmit(c *gin.Context, info *relaycommon.RelayInfo, action, prompt string, charge bool) *dto.MidjourneyResponse {
	quota := info.PriceData.Quota
	if !charge {
		quota = 0
	}
	task := &model.Midjourney{UserId: info.UserId, EnterpriseId: info.EnterpriseId, EnterpriseMemberId: info.EnterpriseMemberId,
		BillingSource: service.BillingSourceEnterprise, BillingRequestId: info.RequestId, BillingTokenId: info.TokenId,
		Action: action, Prompt: prompt, ChannelId: info.ChannelId, Quota: quota, Progress: "0%", SubmitTime: time.Now().UnixMilli()}
	task.PollingKey = strings.TrimPrefix(common.GetContextKeyString(c, constant.ContextKeyChannelKey), "Bearer ")
	task.PollingBaseURL = c.GetString("base_url")
	if task.PollingKey == "" || task.PollingBaseURL == "" {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "missing_midjourney_polling_snapshot")
	}
	if err := model.CreateEnterpriseMJIntent(task); err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "persist_midjourney_billing_failed")
	}
	if apiErr := service.PreConsumeBilling(c, quota, info); apiErr != nil {
		service.MarkRequestFailure(c, apiErr)
		return service.MidjourneyErrorWrapper(constant.MjRequestError, apiErr.Error())
	}
	fullURL := c.GetString("base_url") + getMjRequestPath(c.Request.URL.String())
	response, body, err := service.DoMidjourneyHttpRequest(c, time.Second*60, fullURL)
	applyMidjourneyRateLimitMetadata(c, response)
	if err != nil {
		if response != nil {
			return &response.Response
		}
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "midjourney_result_unknown")
	}
	if response == nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "midjourney_result_unknown")
	}
	// Parse the actual upstream body. A synthesized gateway error or an invalid
	// response is not a confirmed rejection; keep its reservation for review.
	var result dto.MidjourneyResponse
	valid := common.Unmarshal(body, &result) == nil && result.Code != 0
	uploadComplete := false
	if action == constant.MjActionUpload {
		var upload dto.MidjourneyUploadResponse
		if common.Unmarshal(body, &upload) == nil && upload.Code != 0 {
			result.Code, result.Description = upload.Code, upload.Description
			valid = true
			uploadComplete = upload.Code == 1 && len(upload.Result) > 0
		}
	}
	if response.StatusCode != http.StatusOK && response.Response.Code != 0 {
		// Preserve upstream rate-limit/error semantics without treating it as proof
		// that the job did not start. The persisted reservation remains unknown.
		return &response.Response
	}
	if !valid || response.StatusCode != http.StatusOK {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "midjourney_result_unknown")
	}
	accepted := result.Code == 1 || result.Code == 21 || result.Code == 22
	if accepted && result.Result == "" && !uploadComplete {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "midjourney_result_unknown")
	}
	task.Code, task.MjId, task.Description = result.Code, result.Result, result.Description
	if !accepted {
		task.FailReason = result.Description
	}
	if props, ok := result.Properties.(map[string]interface{}); ok {
		if image, ok := props["imageUrl"].(string); ok {
			task.ImageUrl = image
		}
		if status, ok := props["status"].(string); ok {
			task.Status = status
		}
		if data, err := common.Marshal(props); err == nil {
			task.Properties = string(data)
		}
	}
	if uploadComplete {
		task.Status = "SUCCESS"
	}
	if task.Status == "SUCCESS" || task.Status == "FAILURE" {
		task.Progress = "100%"
		task.FinishTime = time.Now().UnixMilli()
	}
	if err := model.SaveEnterpriseMJSubmission(task, accepted); err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "persist_midjourney_result_failed")
	}
	if err := service.SettleBilling(c, info, task.Quota); err != nil {
		// The accepted response is durable, so maintenance can finish settlement.
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "midjourney_settlement_pending")
	}
	if err := model.SettleEnterpriseMJSubmission(task.Id, task.EnterpriseInitialQuota); err != nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "midjourney_settlement_pending")
	}
	if task.Status == "SUCCESS" || task.Status == "FAILURE" {
		if _, err := model.FinalizeEnterpriseMJ(task.Id); err != nil {
			common.SysLog("midjourney correction pending: " + err.Error())
		}
	}
	if accepted && task.EnterpriseBillingState == model.MJEnterpriseAccepted {
		other := service.GenerateMjOtherInfo(info, info.PriceData)
		other["billing_source"] = service.BillingSourceEnterprise
		other["enterprise_id"] = info.EnterpriseId
		other["enterprise_member_id"] = info.EnterpriseMemberId
		other["task_id"] = result.Result
		model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{ChannelId: info.ChannelId, ModelName: service.CovertMjpActionToModelName(action), TokenName: c.GetString("token_name"), Quota: task.Quota, Content: action, TokenId: info.TokenId, Group: info.UsingGroup, Other: other})
		model.UpdateUserRequestCount(info.UserId)
		model.UpdateChannelUsedQuota(info.ChannelId, task.Quota)
	}
	// Preserve protocol bodies (especially upload's array result) and the existing
	// accepted/queued compatibility normalization.
	if result.Code == 22 || (result.Code == 21 && action != constant.MjActionInPaint && action != constant.MjActionCustomZoom) {
		var payload map[string]interface{}
		if common.Unmarshal(body, &payload) == nil {
			payload["code"] = 1
			if encoded, err := common.Marshal(payload); err == nil {
				body = encoded
			}
		}
	}
	c.Data(response.StatusCode, "application/json", body)
	return nil
}

// ResolveMidjourneyOriginTask is shared by the submission handler and the
// controller's fixed-channel capacity admission. It resolves indirect references
// within the same user/payer before a channel credential can be selected.
func ResolveMidjourneyOriginTask(c *gin.Context, mode int, request *dto.MidjourneyRequest) (*model.Midjourney, *dto.MidjourneyResponse) {
	denied := func() (*model.Midjourney, *dto.MidjourneyResponse) {
		return nil, service.MidjourneyErrorWrapper(constant.MjRequestError, "task_not_found")
	}
	if request == nil {
		return denied()
	}
	if mode == relayconstant.RelayModeMidjourneySimpleChange {
		params := parseMJSimpleChange(request.Content)
		if params == nil {
			return nil, service.MidjourneyErrorWrapper(constant.MjRequestError, "content_parse_failed")
		}
		if request.TaskId != "" && request.TaskId != params.TaskId {
			return denied()
		}
		request.TaskId = params.TaskId
	}
	if mode == relayconstant.RelayModeMidjourneyAction {
		if !validMJCustomID(request.CustomId) {
			return nil, service.MidjourneyErrorWrapper(constant.MjRequestError, "invalid_custom_id")
		}
		var task *model.Midjourney
		if request.TaskId != "" {
			task = model.GetMJTaskForSubject(mjTaskSubject(c), request.TaskId)
		} else {
			task = model.GetMJTaskForCustomID(mjTaskSubject(c), request.CustomId)
		}
		if task == nil || !task.HasCustomID(request.CustomId) {
			return denied()
		}
		request.TaskId = task.MjId
		return task, nil
	}
	if request.TaskId != "" {
		task := model.GetMJTaskForSubject(mjTaskSubject(c), request.TaskId)
		if task == nil {
			return denied()
		}
		return task, nil
	}
	switch mode {
	case relayconstant.RelayModeMidjourneyChange, relayconstant.RelayModeMidjourneySimpleChange, relayconstant.RelayModeMidjourneyModal:
		return nil, service.MidjourneyErrorWrapper(constant.MjRequestError, "task_id_is_required")
	}
	return nil, nil
}

func validMJCustomID(id string) bool {
	parts := strings.Split(id, "::")
	if len(parts) < 2 {
		return false
	}
	action := parts[1]
	if action == "JOB" {
		if len(parts) < 3 {
			return false
		}
		action = parts[2]
	}
	if strings.Contains(action, "upsample") || action == "variation" {
		return len(parts) >= 4
	}
	return action != ""
}

func prepareMJOriginChannel(c *gin.Context, channel *model.Channel, modelName string) *dto.MidjourneyResponse {
	// Retain the exact key selected by capacity admission, including multi-key
	// channels. Never replace it with the channel's entire stored key collection.
	if common.GetContextKeyInt(c, constant.ContextKeyChannelId) != channel.Id || common.GetContextKeyString(c, constant.ContextKeyChannelKey) == "" {
		if apiErr := middleware.SetupContextForSelectedChannel(c, channel, modelName); apiErr != nil {
			service.MarkRequestFailure(c, apiErr)
			return service.MidjourneyErrorWrapper(constant.MjRequestError, "get_channel_info_failed")
		}
	}
	c.Set("base_url", channel.GetBaseURL())
	return nil
}

// The forwarding helper re-encodes a JSON map. Reject case aliases that a struct
// decoder and the map's reordered output could resolve to different task IDs.
func unambiguousMJReferences(c *gin.Context) bool {
	var body map[string]interface{}
	if common.UnmarshalBodyReusable(c, &body) != nil {
		return false
	}
	seen := map[string]bool{}
	for key := range body {
		normalized := strings.ToLower(key)
		switch normalized {
		case "taskid", "customid", "content", "action", "index":
			if seen[normalized] {
				return false
			}
			seen[normalized] = true
		}
	}
	return true
}

func parseMJSimpleChange(content string) *dto.MidjourneyRequest {
	parts := strings.Split(content, " ")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	if strings.ToLower(parts[1]) != "r" && len(parts[1]) < 2 {
		return nil
	}
	return service.ConvertSimpleChangeParams(content)
}

// FetchMidjourneyPollingBatch uses the same selected key as submission. A channel
// remains the capacity/cooldown authority; its current key/base URL cannot replace
// an enterprise task snapshot. No response body or credential is logged on error.
func FetchMidjourneyPollingBatch(ctx context.Context, channel *model.Channel, batch model.MJPollingBatch) ([]dto.MidjourneyDto, bool, error) {
	baseURL, key, err := batch.PollingTransport(channel)
	if err != nil {
		return nil, false, err
	}
	if len(batch.TaskIDs) == 0 || service.IsChannelRateLimitCoolingDown(channel) {
		return nil, false, nil
	}
	admission := middleware.AcquireChannelCapacityForChannel(channel, fmt.Sprintf("mj-poll:%d:%d", channel.Id, time.Now().UnixNano()))
	if !admission.Acquired {
		return nil, false, nil
	}
	if admission.Release != nil {
		defer admission.Release()
	}
	body, err := common.Marshal(map[string]any{"ids": batch.TaskIDs})
	if err != nil {
		return nil, false, fmt.Errorf("encode midjourney polling request failed")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/mj/task/list-by-condition", bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("create midjourney polling request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("mj-api-secret", key)
	// Never follow redirects with a stored credential to a different endpoint.
	client := *service.GetHttpClient()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("midjourney polling transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			apiErr := types.NewOpenAIError(fmt.Errorf("midjourney polling upstream rate limited"), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
			apiErr.UpstreamStatusCode = resp.StatusCode
			apiErr.RetryAfter = service.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			service.RecordChannelRateLimitCooldown(nil, channel, apiErr)
		}
		return nil, true, fmt.Errorf("midjourney polling upstream status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("read midjourney polling response failed")
	}
	var items []dto.MidjourneyDto
	if common.Unmarshal(data, &items) != nil {
		return nil, true, fmt.Errorf("invalid midjourney polling response")
	}
	// The provider cannot inject a different ID or contradictory duplicate items.
	selected := make([]dto.MidjourneyDto, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		if _, exists := batch.TasksByID[item.MjId]; !exists {
			continue
		}
		if seen[item.MjId] {
			return nil, true, fmt.Errorf("duplicate midjourney polling response ID")
		}
		seen[item.MjId] = true
		selected = append(selected, item)
	}
	return selected, true, nil
}
