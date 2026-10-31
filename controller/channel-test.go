package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

type testResult struct {
	preflight    *videoPreflight
	context      *gin.Context
	localErr     error
	newAPIError  *types.NewAPIError
	info         *relaycommon.RelayInfo
	usage        *dto.Usage
	priceData    types.PriceData
	responseBody []byte
}

type testExecutionOptions struct {
	enableBilling bool
	isChannelTest bool
}

type channelStyleTestExecution struct {
	modelName    string
	requestPath  string
	endpointType string
	relayFormat  types.RelayFormat
	request      dto.Request
	isStream     bool
}

func normalizeChannelTestEndpoint(channel *model.Channel, modelName, endpointType string) string {
	normalized := strings.TrimSpace(endpointType)
	if normalized != "" {
		return normalized
	}
	lowerModelName := strings.ToLower(strings.TrimSpace(modelName))
	channelType := 0
	if channel != nil {
		channelType = channel.Type
	}
	if common.IsVideoGenerationModelForChannel(channelType, lowerModelName) {
		return string(constant.EndpointTypeOpenAIVideo)
	}
	if common.IsImageGenerationModel(lowerModelName) ||
		(channel != nil && channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(lowerModelName, "seedream")) {
		return string(constant.EndpointTypeImageGeneration)
	}
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return string(constant.EndpointTypeOpenAIResponseCompact)
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	return normalized
}

func resolveChannelTestUserID(c *gin.Context) (int, error) {
	if c != nil {
		if userID := c.GetInt("id"); userID > 0 {
			return userID, nil
		}
	}

	var rootUser model.User
	if err := model.DB.Select("id").Where("role = ?", common.RoleRootUser).First(&rootUser).Error; err != nil {
		return 0, fmt.Errorf("failed to resolve channel test user: %w", err)
	}
	if rootUser.Id == 0 {
		return 0, errors.New("failed to resolve channel test user")
	}
	return rootUser.Id, nil
}

func shouldUseAnthropicMessagesPath(channel *model.Channel, modelName string) bool {
	if !strings.Contains(strings.ToLower(modelName), "claude") || channel == nil {
		return false
	}
	switch channel.Type {
	case constant.ChannelTypeAnthropic, constant.ChannelTypeAws:
		return true
	default:
		return false
	}
}

func resolveTestRequestPath(channel *model.Channel, modelName, endpointType string) (string, string) {
	endpointType = normalizeChannelTestEndpoint(channel, modelName, endpointType)
	requestPath := "/v1/chat/completions"

	if endpointType != "" {
		if endpointInfo, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpointType)); ok {
			return endpointInfo.Path, endpointType
		}
		return requestPath, endpointType
	}

	if channel != nil && common.IsVideoGenerationModelForChannel(channel.Type, modelName) {
		return "/v1/videos", string(constant.EndpointTypeOpenAIVideo)
	}

	if strings.Contains(strings.ToLower(modelName), "rerank") {
		requestPath = "/v1/rerank"
	}

	if strings.Contains(strings.ToLower(modelName), "embedding") ||
		strings.HasPrefix(modelName, "m3e") ||
		strings.Contains(modelName, "bge-") ||
		strings.Contains(modelName, "embed") ||
		(channel != nil && channel.Type == constant.ChannelTypeMokaAI) {
		requestPath = "/v1/embeddings"
	}

	if shouldUseAnthropicMessagesPath(channel, modelName) {
		requestPath = "/v1/messages"
	}

	if strings.Contains(strings.ToLower(modelName), "codex") {
		requestPath = "/v1/responses"
	}

	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		requestPath = "/v1/responses/compact"
	}

	return requestPath, endpointType
}

func shouldUseResponsesPathForTest(channel *model.Channel, modelName, endpointType string) bool {
	if channel == nil || strings.TrimSpace(endpointType) != "" {
		return false
	}
	if !common.IsOpenAITextModel(modelName) {
		return false
	}
	return service.ShouldChatCompletionsUseResponsesWithChannelSetting(channel.GetSetting(), channel.Id, channel.Type, modelName)
}

func shouldAutoStreamChannelTest(modelName string, requestPath string, endpointType string) bool {
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeAnthropic, constant.EndpointTypeOpenAIResponse:
			return true
		default:
			return false
		}
	}

	switch requestPath {
	case "/v1/messages", "/v1/responses":
		return true
	case "/v1/videos":
		return false
	case "/v1/chat/completions":
		return common.IsOpenAITextModel(modelName)
	default:
		return false
	}
}

func buildChannelStyleTestExecution(channel *model.Channel, modelName, endpointType string, streamOverride *bool) channelStyleTestExecution {
	requestPath, resolvedEndpointType := resolveTestRequestPath(channel, modelName, endpointType)
	if requestPath == "/v1/chat/completions" && shouldUseResponsesPathForTest(channel, modelName, resolvedEndpointType) {
		requestPath = "/v1/responses"
		resolvedEndpointType = string(constant.EndpointTypeOpenAIResponse)
	}

	isStream := shouldAutoStreamChannelTest(modelName, requestPath, resolvedEndpointType)
	if streamOverride != nil {
		isStream = *streamOverride
	}
	if forceNonStreamMiMoTTSTest(channel, modelName) {
		isStream = false
	}
	if strings.HasPrefix(requestPath, "/v1/responses/compact") {
		modelName = ratio_setting.WithCompactModelSuffix(modelName)
	}

	return channelStyleTestExecution{
		modelName:    modelName,
		requestPath:  requestPath,
		endpointType: resolvedEndpointType,
		relayFormat:  resolveRelayFormatForTest(requestPath, resolvedEndpointType),
		request:      buildRelayExecutionTestRequest(modelName, requestPath, resolvedEndpointType, channel, isStream),
		isStream:     isStream,
	}
}

func resolveRelayFormatForTest(requestPath string, endpointType string) types.RelayFormat {
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeOpenAI:
			return types.RelayFormatOpenAI
		case constant.EndpointTypeOpenAIResponse:
			return types.RelayFormatOpenAIResponses
		case constant.EndpointTypeOpenAIResponseCompact:
			return types.RelayFormatOpenAIResponsesCompaction
		case constant.EndpointTypeAnthropic:
			return types.RelayFormatClaude
		case constant.EndpointTypeGemini:
			return types.RelayFormatGemini
		case constant.EndpointTypeJinaRerank:
			return types.RelayFormatRerank
		case constant.EndpointTypeImageGeneration:
			return types.RelayFormatOpenAIImage
		case constant.EndpointTypeOpenAIVideo:
			return types.RelayFormatTask
		case constant.EndpointTypeEmbeddings:
			return types.RelayFormatEmbedding
		default:
			return types.RelayFormatOpenAI
		}
	}

	switch {
	case requestPath == "/v1/embeddings":
		return types.RelayFormatEmbedding
	case requestPath == "/v1/images/generations":
		return types.RelayFormatOpenAIImage
	case requestPath == "/v1/videos":
		return types.RelayFormatTask
	case requestPath == "/v1/messages":
		return types.RelayFormatClaude
	case strings.Contains(requestPath, "/v1beta/models"):
		return types.RelayFormatGemini
	case requestPath == "/v1/rerank" || requestPath == "/rerank":
		return types.RelayFormatRerank
	case requestPath == "/v1/responses":
		return types.RelayFormatOpenAIResponses
	case strings.HasPrefix(requestPath, "/v1/responses/compact"):
		return types.RelayFormatOpenAIResponsesCompaction
	default:
		return types.RelayFormatOpenAI
	}
}

func executeChannelStyleTest(
	c *gin.Context,
	recorder *httptest.ResponseRecorder,
	channel *model.Channel,
	testModel string,
	endpointType string,
	request dto.Request,
	relayFormat types.RelayFormat,
	isStream bool,
	options testExecutionOptions,
) (result testResult) {
	// A dashboard test must never submit a paid asynchronous video job.
	if channelVideoTest(channel, testModel, endpointType) {
		err := errors.New("视频测试仅支持本地预检，请通过正式视频接口进行真实生成")
		return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())}
	}
	var relayInfo *relaycommon.RelayInfo
	billingStarted := false
	defer func() {
		if options.enableBilling && billingStarted && result.localErr != nil && relayInfo != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)
	if err != nil {
		result.context = c
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}
	relayInfo = info
	relayInfo.IsChannelTest = options.isChannelTest
	relayInfo.InitChannelMeta(c)

	// 渠道测试使用的是内部构造请求，HTTP Body 可能为空。
	// 在模型映射前保存原始请求，供阶梯计费表达式读取 param()。
	err = attachTestBillingRequestInput(relayInfo, request)
	if err != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
		return
	}

	err = helper.ModelMappedHelper(c, relayInfo, request)
	if err != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeChannelModelMappedError)
		return
	}

	testModel = relayInfo.UpstreamModelName
	if channelVideoTest(channel, testModel, endpointType) || relayInfo.RelayMode == relayconstant.RelayModeVideoSubmit {
		err := errors.New("视频测试仅支持本地预检，请通过正式视频接口进行真实生成")
		return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())}
	}
	request.SetModelName(testModel)

	meta := request.GetTokenCountMeta()
	promptTokens := 0
	if options.enableBilling {
		promptTokens, err = service.EstimateRequestToken(c, meta, relayInfo)
		if err != nil {
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
			return
		}
		relayInfo.SetEstimatePromptTokens(promptTokens)
	}

	priceData, err := helper.ModelPriceHelper(c, relayInfo, promptTokens, meta)
	if err != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
		return
	}

	if options.enableBilling && !priceData.FreeModel {
		apiErr := service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if apiErr != nil {
			result.context = c
			result.info = relayInfo
			result.localErr = apiErr.Err
			result.newAPIError = apiErr
			return
		}
		billingStarted = true
	}

	apiType, _ := common.ChannelType2APIType(channel.Type)
	adaptor := relay.GetAdaptor(apiType)
	if relayInfo.RelayMode == relayconstant.RelayModeResponsesCompact &&
		apiType != constant.APITypeOpenAI &&
		apiType != constant.APITypeCodex {
		err = fmt.Errorf("responses compaction test only supports openai/codex channels, got api type %d", apiType)
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeInvalidApiType)
		return
	}
	if adaptor == nil {
		err = fmt.Errorf("invalid api type: %d, adaptor is nil", apiType)
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeInvalidApiType)
		return
	}

	common.SysLog(fmt.Sprintf("testing channel %d with model %s , info %+v ", channel.Id, testModel, relayInfo.ToString()))
	if adaptor != nil {
		adaptor.Init(relayInfo)
	}

	var convertedRequest any
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		if embeddingReq, ok := request.(*dto.EmbeddingRequest); ok {
			convertedRequest, err = adaptor.ConvertEmbeddingRequest(c, relayInfo, *embeddingReq)
		} else {
			err = errors.New("invalid embedding request type")
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
			return
		}
	case relayconstant.RelayModeImagesGenerations:
		if imageReq, ok := request.(*dto.ImageRequest); ok {
			convertedRequest, err = adaptor.ConvertImageRequest(c, relayInfo, *imageReq)
		} else {
			err = errors.New("invalid image request type")
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
			return
		}
	case relayconstant.RelayModeRerank:
		if rerankReq, ok := request.(*dto.RerankRequest); ok {
			convertedRequest, err = adaptor.ConvertRerankRequest(c, relayInfo.RelayMode, *rerankReq)
		} else {
			err = errors.New("invalid rerank request type")
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
			return
		}
	case relayconstant.RelayModeResponses:
		if responseReq, ok := request.(*dto.OpenAIResponsesRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, relayInfo, *responseReq)
		} else {
			err = errors.New("invalid response request type")
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
			return
		}
	case relayconstant.RelayModeResponsesCompact:
		switch req := request.(type) {
		case *dto.OpenAIResponsesCompactionRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, relayInfo, dto.OpenAIResponsesRequest{
				Model:              req.Model,
				Input:              req.Input,
				Instructions:       req.Instructions,
				PreviousResponseID: req.PreviousResponseID,
			})
		case *dto.OpenAIResponsesRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, relayInfo, *req)
		default:
			err = errors.New("invalid response compaction request type")
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
			return
		}
	default:
		if generalReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIRequest(c, relayInfo, generalReq)
		} else if claudeReq, ok := request.(*dto.ClaudeRequest); ok {
			convertedRequest, err = adaptor.ConvertClaudeRequest(c, relayInfo, claudeReq)
		} else {
			err = errors.New("invalid general request type")
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
			return
		}
	}

	if err != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
		return
	}

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewError(err, types.ErrorCodeJsonMarshalFailed)
		return
	}

	if len(relayInfo.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverride(jsonData, relayInfo.ParamOverride, relaycommon.BuildParamOverrideContext(relayInfo))
		if err != nil {
			result.context = c
			result.info = relayInfo
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid)
			return
		}
	}

	if channelVideoTest(channel, gjson.GetBytes(jsonData, "model").String(), endpointType) {
		err := errors.New("视频测试仅支持本地预检，请通过正式视频接口进行真实生成")
		return testResult{context: c, localErr: err, newAPIError: types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())}
	}

	requestBody := bytes.NewBuffer(jsonData)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	// Only actual channel probes reach this point. Local preflight/configuration
	// errors and billed playground requests must not become availability samples.
	if options.isChannelTest {
		targets, groupErr := model.ChannelHealthProbeTargets(c.Request.Context(), channel.Id)
		if groupErr != nil {
			common.SysError("failed to resolve channel health probe groups: " + groupErr.Error())
		} else {
			defer func() {
				outcome := grouphealth.OutcomeSuccess
				if result.localErr != nil || result.newAPIError != nil {
					outcome = grouphealth.OutcomeServiceFailure
				}
				for _, target := range targets {
					grouphealth.Record(grouphealth.Sample{Group: target.Group, Model: target.Model, Probe: true, Outcome: outcome})
				}
			}()
		}
	}
	resp, err := adaptor.DoRequest(c, relayInfo, requestBody)
	if err != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = err
		result.newAPIError = types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		return
	}

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		validStatus := httpResp.StatusCode == http.StatusOK
		if !validStatus {
			responseBody, readErr := io.ReadAll(httpResp.Body)
			if readErr != nil {
				result.context = c
				result.info = relayInfo
				result.localErr = readErr
				result.newAPIError = types.NewOpenAIError(readErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
				return
			}
			_ = httpResp.Body.Close()
			result.responseBody = responseBody
			httpResp.Body = io.NopCloser(bytes.NewReader(responseBody))
			apiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, true)
			common.SysError(fmt.Sprintf(
				"channel test bad response: channel_id=%d name=%s type=%d model=%s endpoint_type=%s status=%d err=%v",
				channel.Id,
				channel.Name,
				channel.Type,
				testModel,
				endpointType,
				httpResp.StatusCode,
				apiErr,
			))
			result.context = c
			result.info = relayInfo
			if apiErr != nil {
				result.localErr = apiErr.Err
				result.newAPIError = apiErr
			} else {
				err = fmt.Errorf("bad response status code %d", httpResp.StatusCode)
				result.localErr = err
				result.newAPIError = types.NewOpenAIError(err, types.ErrorCodeBadResponseStatusCode, httpResp.StatusCode)
			}
			return
		}
	}

	usageAny, respErr := adaptor.DoResponse(c, httpResp, relayInfo)
	if respErr != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = respErr
		result.newAPIError = respErr
		return
	}

	usage, usageErr := coerceTestUsage(usageAny, isStream, relayInfo.GetEstimatePromptTokens())
	if usageErr != nil {
		result.context = c
		result.info = relayInfo
		result.localErr = usageErr
		result.newAPIError = types.NewOpenAIError(usageErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		return
	}

	respBody, err := readTestResponseBody(recorder.Result().Body, isStream)
	if err != nil {
		result.context = c
		result.info = relayInfo
		result.usage = usage
		result.priceData = priceData
		result.localErr = err
		result.newAPIError = types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
		return
	}
	if bodyErr := validateTestResponseBody(respBody, isStream); bodyErr != nil {
		result.context = c
		result.info = relayInfo
		result.usage = usage
		result.priceData = priceData
		result.responseBody = respBody
		result.localErr = bodyErr
		result.newAPIError = types.NewOpenAIError(bodyErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		return
	}

	if usage.PromptTokens > 0 {
		relayInfo.SetEstimatePromptTokens(usage.PromptTokens)
	}

	if options.enableBilling {
		if err = relay.FinalizeTestConsumeQuota(c, relayInfo, usage); err != nil {
			result.context = c
			result.info = relayInfo
			result.usage = usage
			result.priceData = priceData
			result.responseBody = respBody
			result.localErr = err
			result.newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
			return
		}
	}

	result.context = c
	result.info = relayInfo
	result.usage = usage
	result.priceData = priceData
	result.responseBody = respBody
	return
}

func testChannel(channel *model.Channel, testUserID int, testModel string, endpointType string, streamOverride *bool, forcedKeyIndex *int) testResult {
	tik := time.Now()
	testModel = channelTestModel(channel, testModel)
	if channelVideoTest(channel, testModel, endpointType) {
		return testResult{preflight: inspectVideoConfiguration(channel, testModel)}
	}
	var unsupportedTestChannelTypes = []int{
		constant.ChannelTypeMidjourney,
		constant.ChannelTypeMidjourneyPlus,
		constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling,
		constant.ChannelTypeJimeng,
		constant.ChannelTypeDoubaoVideo,
		constant.ChannelTypeVidu,
	}
	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	execution := buildChannelStyleTestExecution(channel, testModel, endpointType, streamOverride)

	c.Request = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: execution.requestPath}, // 使用动态路径
		Body:   nil,
		Header: make(http.Header),
	}

	cache, err := model.GetUserCache(testUserID)
	if err != nil {
		return testResult{
			localErr:    err,
			newAPIError: nil,
		}
	}
	cache.WriteContext(c)
	c.Set("id", testUserID)

	//c.Request.Header.Set("Authorization", "Bearer "+channel.Key)
	c.Request.Header.Set("Content-Type", "application/json")
	if execution.isStream {
		c.Request.Header.Set("Accept", "text/event-stream")
	}
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(testUserID, false)
	c.Set("group", group)

	var newAPIError *types.NewAPIError
	if forcedKeyIndex != nil {
		key, keyErr := channel.GetKeyByIndex(*forcedKeyIndex)
		if keyErr != nil {
			newAPIError = types.NewError(keyErr, types.ErrorCodeChannelNoAvailableKey, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = middleware.SetupContextForSelectedChannelKey(c, channel, execution.modelName, key, *forcedKeyIndex)
		}
	} else {
		newAPIError = middleware.SetupContextForSelectedChannel(c, channel, execution.modelName)
	}
	if newAPIError != nil {
		return testResult{
			context:     c,
			localErr:    newAPIError,
			newAPIError: newAPIError,
		}
	}
	execResult := executeChannelStyleTest(c, w, channel, execution.modelName, execution.endpointType, execution.request, execution.relayFormat, execution.isStream, testExecutionOptions{
		enableBilling: false,
		isChannelTest: true,
	})
	if execResult.localErr != nil || execResult.preflight != nil {
		return execResult
	}

	info := execResult.info
	usage := execResult.usage
	if usage == nil {
		// Keep settlement total for adapters that omit optional usage.
		usage = &dto.Usage{}
	}
	priceData := execResult.priceData
	respBody := execResult.responseBody
	quota, tieredResult := settleTestQuota(info, priceData, usage)

	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	other := buildTestLogOther(c, info, priceData, usage, tieredResult)
	model.RecordConsumeLog(c, testUserID, model.RecordConsumeLogParams{
		ChannelId:        channel.Id,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        info.OriginModelName,
		TokenName:        "模型测试",
		Quota:            quota,
		Content:          "模型测试",
		UseTimeSeconds:   int(consumedTime),
		IsStream:         info.IsStream,
		Group:            info.UsingGroup,
		Other:            other,
	})
	common.SysLog(fmt.Sprintf("testing channel #%d, response: \n%s", channel.Id, channelTestResponseForLog(testModel, respBody)))
	return testResult{
		context:      c,
		localErr:     nil,
		newAPIError:  nil,
		info:         info,
		usage:        usage,
		priceData:    priceData,
		responseBody: respBody,
	}
}

func attachTestBillingRequestInput(info *relaycommon.RelayInfo, request dto.Request) error {
	if info == nil {
		return nil
	}

	input, err := helper.BuildBillingExprRequestInputFromRequest(request, info.RequestHeaders)
	if err != nil {
		return err
	}
	info.BillingRequestInput = &input
	return nil
}

func settleTestQuota(info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage) (int, *billingexpr.TieredResult) {
	if usage == nil {
		return priceData.Quota, nil
	}
	if usage != nil && info != nil && info.TieredBillingSnapshot != nil {
		isClaudeUsageSemantic := usage.UsageSemantic == "anthropic" || info.GetFinalRequestRelayFormat() == types.RelayFormatClaude
		usedVars := billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString)
		if ok, quota, result := service.TryTieredSettle(info, service.BuildTieredTokenParams(usage, isClaudeUsageSemantic, usedVars)); ok {
			return quota, result
		}
	}

	quota := 0
	if !priceData.UsePrice {
		quota = usage.PromptTokens + int(math.Round(float64(usage.CompletionTokens)*priceData.CompletionRatio))
		quota = int(math.Round(float64(quota) * priceData.ModelRatio))
		if priceData.ModelRatio != 0 && quota <= 0 {
			quota = 1
		}
		return quota, nil
	}

	return int(priceData.ModelPrice * common.QuotaPerUnit), nil
}

func buildTestLogOther(c *gin.Context, info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage, tieredResult *billingexpr.TieredResult) map[string]interface{} {
	other := service.GenerateTextOtherInfo(c, info, priceData.ModelRatio, priceData.GroupRatioInfo.GroupRatio, priceData.CompletionRatio,
		usage.PromptTokensDetails.CachedTokens, priceData.CacheRatio, priceData.ModelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		isClaudeUsageSemantic := usage.UsageSemantic == "anthropic" || info.GetFinalRequestRelayFormat() == types.RelayFormatClaude
		usedVars := billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString)
		tieredParams := service.BuildTieredTokenParams(usage, isClaudeUsageSemantic, usedVars)
		service.InjectTieredBillingInfo(other, info, tieredResult, &tieredParams)
	}
	return other
}

func coerceTestUsage(usageAny any, isStream bool, estimatePromptTokens int) (*dto.Usage, error) {
	switch u := usageAny.(type) {
	case *dto.Usage:
		return u, nil
	case dto.Usage:
		return &u, nil
	case nil:
		if !isStream {
			return nil, errors.New("usage is nil")
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	default:
		if !isStream {
			return nil, fmt.Errorf("invalid usage type: %T", usageAny)
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	}
}

func readTestResponseBody(body io.ReadCloser, isStream bool) ([]byte, error) {
	defer func() { _ = body.Close() }()
	const maxStreamLogBytes = 8 << 10
	if isStream {
		return io.ReadAll(io.LimitReader(body, maxStreamLogBytes))
	}
	return io.ReadAll(body)
}

func detectErrorFromTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return nil
	}
	if message := detectErrorMessageFromJSONBytes(b); message != "" {
		return fmt.Errorf("upstream error: %s", message)
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if message := detectErrorMessageFromJSONBytes(payload); message != "" {
			return fmt.Errorf("upstream error: %s", message)
		}
	}

	return nil
}

func validateStreamTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return errors.New("stream response body is empty")
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		return nil
	}

	return errors.New("stream response body does not contain a valid stream event")
}

func validateTestResponseBody(respBody []byte, isStream bool) error {
	if bodyErr := detectErrorFromTestResponseBody(respBody); bodyErr != nil {
		return bodyErr
	}
	if isStream {
		return validateStreamTestResponseBody(respBody)
	}
	return nil
}

func shouldUseStreamForAutomaticChannelTest(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex
}

func detectErrorMessageFromJSONBytes(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	if jsonBytes[0] != '{' && jsonBytes[0] != '[' {
		return ""
	}
	errVal := gjson.GetBytes(jsonBytes, "error")
	if !errVal.Exists() || errVal.Type == gjson.Null {
		return ""
	}

	message := gjson.GetBytes(jsonBytes, "error.message").String()
	if message == "" {
		message = gjson.GetBytes(jsonBytes, "error.error.message").String()
	}
	if message == "" {
		message = gjson.GetBytes(jsonBytes, "message").String()
	}
	if message == "" && errVal.Type == gjson.String {
		message = errVal.String()
	}
	if message == "" {
		message = errVal.Raw
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "upstream returned error payload"
	}
	return message
}

func resolveChannelTestFailure(result testResult) (string, int) {
	statusCode := http.StatusInternalServerError
	if result.newAPIError != nil && result.newAPIError.StatusCode > 0 {
		statusCode = result.newAPIError.StatusCode
	}
	message := ""
	if len(result.responseBody) > 0 {
		message = summarizeOpenAIStyleError(statusCode, result.responseBody, "")
	}
	if message != "" {
		return message, statusCode
	}
	if result.newAPIError != nil {
		return result.newAPIError.Error(), statusCode
	}
	if result.localErr != nil {
		return result.localErr.Error(), statusCode
	}
	return "测试失败", statusCode
}

func buildChannelTestFailureResponse(result testResult, consumedTime float64) gin.H {
	message, statusCode := resolveChannelTestFailure(result)
	resp := gin.H{
		"success":     false,
		"message":     message,
		"time":        consumedTime,
		"status_code": statusCode,
	}
	if result.newAPIError != nil {
		resp["error_code"] = string(result.newAPIError.GetErrorCode())
	}
	return resp
}

func buildTestRequest(model string, endpointType string, channel *model.Channel, isStream bool) dto.Request {
	testResponsesInput := json.RawMessage(`[{"role":"user","content":"hi"}]`)
	if request, ok := buildMiMoTTSTestRequest(model, endpointType, channel, isStream); ok {
		return request
	}

	// 根据端点类型构建不同的测试请求
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeEmbeddings:
			// 返回 EmbeddingRequest
			return &dto.EmbeddingRequest{
				Model: model,
				Input: []any{"hello world"},
			}
		case constant.EndpointTypeImageGeneration:
			// 生图测试使用单张、低质量的最小有效请求，降低测试耗时与费用。
			quality := ""
			if strings.HasPrefix(strings.ToLower(model), "gpt-image-") {
				quality = "low"
			}
			return &dto.ImageRequest{
				Model:   model,
				Prompt:  "a simple white square on a plain background",
				N:       common.GetPointer(uint(1)),
				Size:    "1024x1024",
				Quality: quality,
			}
		case constant.EndpointTypeOpenAIVideo:
			return &dto.OpenAIVideoRequest{
				Model: model,
			}
		case constant.EndpointTypeJinaRerank:
			// 返回 RerankRequest
			return &dto.RerankRequest{
				Model:     model,
				Query:     "What is Deep Learning?",
				Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
				TopN:      2,
			}
		case constant.EndpointTypeOpenAIResponse:
			// 返回 OpenAIResponsesRequest
			return &dto.OpenAIResponsesRequest{
				Model:  model,
				Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
				Stream: isStream,
			}
		case constant.EndpointTypeOpenAIResponseCompact:
			// 返回 OpenAIResponsesCompactionRequest
			return &dto.OpenAIResponsesCompactionRequest{
				Model: model,
				Input: testResponsesInput,
			}
		case constant.EndpointTypeAnthropic, constant.EndpointTypeGemini, constant.EndpointTypeOpenAI:
			// 返回 GeneralOpenAIRequest
			maxTokens := uint(16)
			if constant.EndpointType(endpointType) == constant.EndpointTypeGemini {
				maxTokens = 3000
			}
			req := &dto.GeneralOpenAIRequest{
				Model:  model,
				Stream: isStream,
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hi",
					},
				},
				MaxTokens: maxTokens,
			}
			if isStream {
				req.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
			}
			return req
		}
	}

	// 自动检测逻辑（保持原有行为）
	if strings.Contains(strings.ToLower(model), "rerank") {
		return &dto.RerankRequest{
			Model:     model,
			Query:     "What is Deep Learning?",
			Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
			TopN:      2,
		}
	}

	// 先判断是否为 Embedding 模型
	if strings.Contains(strings.ToLower(model), "embedding") ||
		strings.HasPrefix(model, "m3e") ||
		strings.Contains(model, "bge-") {
		// 返回 EmbeddingRequest
		return &dto.EmbeddingRequest{
			Model: model,
			Input: []any{"hello world"},
		}
	}

	// Responses compaction models (must use /v1/responses/compact)
	if strings.HasSuffix(model, ratio_setting.CompactModelSuffix) {
		return &dto.OpenAIResponsesCompactionRequest{
			Model: model,
			Input: testResponsesInput,
		}
	}

	// Responses-only models (e.g. codex series)
	if strings.Contains(strings.ToLower(model), "codex") {
		return &dto.OpenAIResponsesRequest{
			Model:  model,
			Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
			Stream: isStream,
		}
	}

	// Chat/Completion 请求 - 返回 GeneralOpenAIRequest
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  model,
		Stream: isStream,
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}
	if isStream {
		testRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}

	if dto.GetOpenAIChatCapabilities(model, "").UseMaxCompletionTokens {
		testRequest.MaxCompletionTokens = 16
	} else if strings.Contains(model, "thinking") {
		if !strings.Contains(model, "claude") {
			testRequest.MaxTokens = 50
		}
	} else if strings.Contains(model, "gemini") {
		testRequest.MaxTokens = 3000
	} else {
		testRequest.MaxTokens = 16
	}

	return testRequest
}

func TestChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	//defer func() {
	//	if channel.ChannelInfo.IsMultiKey {
	//		go func() { _ = channel.SaveChannelInfo() }()
	//	}
	//}()
	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	var streamOverride *bool
	if streamValue, ok := c.GetQuery("stream"); ok {
		isStream, _ := strconv.ParseBool(streamValue)
		streamOverride = common.GetPointer(isStream)
	}
	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tik := time.Now()
	result := testChannel(channel, testUserID, testModel, endpointType, streamOverride, nil)
	if result.preflight != nil {
		c.JSON(http.StatusOK, videoPreflightResponse(result.preflight))
		return
	}
	if result.localErr != nil {
		c.JSON(http.StatusOK, buildChannelTestFailureResponse(result, 0.0))
		return
	}
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0
	if result.newAPIError != nil {
		c.JSON(http.StatusOK, buildChannelTestFailureResponse(result, consumedTime))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "",
		"time":        consumedTime,
		"status_code": http.StatusOK,
	})
}

var testAllChannelsLock sync.Mutex
var testAllChannelsRunning bool = false

func testAllChannels(notify bool) error {
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return err
	}

	testAllChannelsLock.Lock()
	if testAllChannelsRunning {
		testAllChannelsLock.Unlock()
		return errors.New("测试已在运行中")
	}
	testAllChannelsRunning = true
	testAllChannelsLock.Unlock()
	channels, getChannelErr := model.GetAllChannels(0, 0, true, false)
	if getChannelErr != nil {
		return getChannelErr
	}
	var disableThreshold = int64(common.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // a impossible value
	}
	gopool.Go(func() {
		// 使用 defer 确保无论如何都会重置运行状态，防止死锁
		defer func() {
			testAllChannelsLock.Lock()
			testAllChannelsRunning = false
			testAllChannelsLock.Unlock()
		}()

		for _, channel := range channels {
			if channel.Status == common.ChannelStatusManuallyDisabled {
				continue
			}
			isChannelEnabled := channel.Status == common.ChannelStatusEnabled
			tik := time.Now()
			if channel.HasPendingDisable() || len(channel.GetPendingDisableKeyIndices()) > 0 {
				time.Sleep(common.RequestInterval)
				continue
			}
			var streamOverride *bool
			if shouldUseStreamForAutomaticChannelTest(channel) {
				streamOverride = common.GetPointer(true)
			}
			result := testChannel(channel, testUserID, "", "", streamOverride, nil)
			if result.preflight != nil {
				continue
			} // Local checks are not channel health probes.
			tok := time.Now()
			milliseconds := tok.Sub(tik).Milliseconds()

			shouldBanChannel := false
			newAPIError := result.newAPIError
			// request error disables the channel
			if newAPIError != nil {
				shouldBanChannel = service.ShouldDisableChannel(channel.Type, result.newAPIError)
			}

			// 当错误检查通过，才检查响应时间
			if common.AutomaticDisableChannelEnabled && !shouldBanChannel {
				if milliseconds > disableThreshold {
					err := fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
					newAPIError = types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusRequestTimeout)
					shouldBanChannel = true
				}
			}

			// disable channel
			if isChannelEnabled && shouldBanChannel && channel.GetAutoBan() {
				processChannelError(result.context, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
			}

			// enable channel
			if newAPIError == nil && model.IsCRSAutoDisabledChannel(channel) {
				if err := model.MarkCRSRecoveryProbeSuccess(channel.Id, common.GetTimestamp()); err != nil {
					common.SysError(fmt.Sprintf("mark CRS recovery probe failed: channel=%d err=%v", channel.Id, err))
				}
			}
			if !isChannelEnabled && service.ShouldEnableChannel(newAPIError, channel.Status) && !model.IsCRSAutoDisabledChannel(channel) {
				service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
			}

			channel.UpdateResponseTime(milliseconds)
			time.Sleep(common.RequestInterval)
		}

		if notify {
			service.NotifyRootUser(dto.NotifyTypeChannelTest, "通道测试完成", "所有通道测试已完成")
		}
	})
	return nil
}

func TestAllChannels(c *gin.Context) {
	err := testAllChannels(true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

var autoTestChannelsOnce sync.Once

func resolvePendingDisableFailureReason(result testResult) string {
	if result.newAPIError != nil {
		return result.newAPIError.ErrorWithStatusCode()
	}
	if result.localErr != nil {
		return result.localErr.Error()
	}
	return "pending disable confirmation failed"
}

func pendingDisableConfirmationTestOptions(reason string) (string, *bool) {
	if !strings.Contains(reason, service.ResponsesStreamMissingCompletedReason) {
		return "", nil
	}
	return string(constant.EndpointTypeOpenAIResponse), common.GetPointer(true)
}

func confirmSingleChannelPendingDisable(channel *model.Channel) {
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		common.SysError("resolve pending disable test user failed: " + err.Error())
		return
	}
	endpointType, streamOverride := pendingDisableConfirmationTestOptions(channel.GetPendingDisableReason())
	result := testChannel(channel, testUserID, "", endpointType, streamOverride, nil)
	if result.preflight != nil {
		return
	} // No evidence to change pending-disable state.
	if result.newAPIError == nil && result.localErr == nil {
		if err := model.ClearChannelPreDisable(channel.Id, ""); err != nil {
			common.SysError(fmt.Sprintf("clear pending disable failed after success: channel=%d err=%v", channel.Id, err))
		}
		return
	}
	service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, channel.Key, channel.GetAutoBan()), resolvePendingDisableFailureReason(result))
}

func confirmMultiKeyPendingDisable(channel *model.Channel, keyIndex int) {
	key, err := channel.GetKeyByIndex(keyIndex)
	if err != nil {
		_ = model.ClearChannelPreDisable(channel.Id, "")
		return
	}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		common.SysError("resolve pending key disable test user failed: " + err.Error())
		return
	}
	endpointType, streamOverride := pendingDisableConfirmationTestOptions(channel.GetPendingDisableKeyReason(keyIndex))
	result := testChannel(channel, testUserID, "", endpointType, streamOverride, common.GetPointer(keyIndex))
	if result.preflight != nil {
		return
	} // No evidence to change pending-disable state.
	if result.newAPIError == nil && result.localErr == nil {
		if err := model.ClearChannelPreDisable(channel.Id, key); err != nil {
			common.SysError(fmt.Sprintf("clear pending key disable failed after success: channel=%d key_index=%d err=%v", channel.Id, keyIndex, err))
		}
		return
	}
	service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, key, channel.GetAutoBan()), resolvePendingDisableFailureReason(result))
}

func processPendingDisableChannels() {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysError("load pending disable channels failed: " + err.Error())
		return
	}
	now := time.Now().Unix()
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if channel.Status == common.ChannelStatusManuallyDisabled {
			_ = model.ClearChannelPreDisable(channel.Id, "")
			continue
		}
		if channel.HasPendingDisable() && channel.GetPendingDisableUntil() <= now {
			confirmSingleChannelPendingDisable(channel)
			time.Sleep(common.RequestInterval)
			continue
		}
		for _, keyIndex := range channel.GetPendingDisableKeyIndices() {
			if channel.ChannelInfo.MultiKeyPendingDisableUntil[keyIndex] > now {
				continue
			}
			confirmMultiKeyPendingDisable(channel, keyIndex)
			time.Sleep(common.RequestInterval)
		}
	}
}

var autoPendingDisableCheckOnce sync.Once

func AutomaticallyTestChannels() {
	// 只在Master节点定时测试渠道
	if !common.IsMasterNode {
		return
	}
	autoTestChannelsOnce.Do(func() {
		for {
			if !operation_setting.GetMonitorSetting().AutoTestChannelEnabled {
				time.Sleep(1 * time.Minute)
				continue
			}
			for {
				frequency := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
				time.Sleep(time.Duration(int(math.Round(frequency))) * time.Minute)
				common.SysLog(fmt.Sprintf("automatically test channels with interval %f minutes", frequency))
				common.SysLog("automatically testing all channels")
				_ = testAllChannels(false)
				common.SysLog("automatically channel test finished")
				if !operation_setting.GetMonitorSetting().AutoTestChannelEnabled {
					break
				}
			}
		}
	})
}

func AutomaticallyCheckPendingDisableChannels() {
	if !common.IsMasterNode {
		return
	}
	autoPendingDisableCheckOnce.Do(func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			processPendingDisableChannels()
		}
	})
}
