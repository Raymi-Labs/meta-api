package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func ResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		switch info.ApiType {
		case appconstant.APITypeOpenAI, appconstant.APITypeCodex:
		default:
			return types.NewErrorWithStatusCode(
				fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
				types.ErrorCodeInvalidRequest,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	var responsesReq *dto.OpenAIResponsesRequest
	switch req := info.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		responsesReq = req
	case *dto.OpenAIResponsesCompactionRequest:
		responsesReq = &dto.OpenAIResponsesRequest{
			Model:              req.Model,
			Input:              req.Input,
			Instructions:       req.Instructions,
			PreviousResponseID: req.PreviousResponseID,
		}
	default:
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected dto.OpenAIResponsesRequest or dto.OpenAIResponsesCompactionRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	normalizedInput, normalizedMessageIDs, err := relaycommon.NormalizeResponsesMessageIDs(responsesReq.Input)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to normalize responses message IDs: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	responsesReq.Input = normalizedInput
	if normalizedMessageIDs > 0 {
		logger.LogInfo(c, fmt.Sprintf("normalized %d CC Switch responses message IDs", normalizedMessageIDs))
	}

	request, err := common.DeepCopy(responsesReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	relaycommon.NormalizeResponsesStreamOptions(request, info.SupportsResponsesStreamOptions)

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	statusCodeMappingStr := c.GetString("status_code_mapping")
	passThrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled
	strippedStreamOptions := false
	conversationStateRecovery := responsesConversationStateRecovery{}
	var httpResp *http.Response
	requestAttempt := 0
	for {
		var requestBody io.Reader
		if passThrough {
			storage, err := common.GetBodyStorage(c)
			if err != nil {
				return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
			}
			requestBytes, err := storage.Bytes()
			if err != nil {
				return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
			}
			requestBytes, err = relaycommon.NormalizeJSONStreamOptions(requestBytes)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			if strippedStreamOptions {
				requestBytes, err = relaycommon.RemoveJSONStreamOptions(requestBytes)
				if err != nil {
					return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
				}
			}
			requestBytes, _, err = relaycommon.NormalizeResponsesMessageIDsInJSON(requestBytes)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			requestBody = bytes.NewBuffer(requestBytes)
		} else {
			requestForUpstream, err := common.DeepCopy(request)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			if strippedStreamOptions {
				requestForUpstream.StreamOptions = nil
			}

			convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *requestForUpstream)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
				}
			}
			jsonData, err = relaycommon.NormalizeJSONStreamOptions(jsonData)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			if strippedStreamOptions {
				jsonData, err = relaycommon.RemoveJSONStreamOptions(jsonData)
				if err != nil {
					return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
				}
			}
			jsonData, _, err = relaycommon.NormalizeResponsesMessageIDsInJSON(jsonData)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			service.SyncRelayReasoningEffortFromResponsesPayload(info, jsonData)
			service.CaptureFinalReasoningAdaptation(c, info, jsonData)

			if common.DebugEnabled {
				println("requestBody: ", string(jsonData))
			}
			requestBody = bytes.NewBuffer(jsonData)
		}

		if requestAttempt > 0 {
			if capacityErr := acquireAdditionalChannelRequestRpm(c, info); capacityErr != nil {
				return capacityErr
			}
		}
		requestAttempt++
		resp, err := adaptor.DoRequest(c, info, requestBody)
		if err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		if resp == nil {
			break
		}
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode == http.StatusOK {
			break
		}

		issue := classifyUpstreamCompatibilityIssue(httpResp, info.RelayMode)
		if issue == upstreamCompatIssueStreamOptions && !strippedStreamOptions {
			strippedStreamOptions = true
			logCompatFallback(c, info, string(issue))
			_ = httpResp.Body.Close()
			continue
		}

		newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		newAPIError = service.NormalizeResponsesConversationStateError(newAPIError)
		if conversationStateRecovery.prepare(c, info, request, responsesReq, passThrough, newAPIError) {
			logCompatFallback(c, info, "conversation_state_missing_reasoning_reference")
			continue
		}
		if service.IsResponsesConversationStateError(newAPIError) {
			return newAPIError
		}
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	var usage any
	if info.RelayMode == relayconstant.RelayModeResponses && info.IsStream {
		usage, newAPIError = openaichannel.OaiResponsesStreamHandlerWithOptions(
			c,
			info,
			httpResp,
			newResponsesAutoContinueOptions(c, info, responsesReq, passThrough),
		)
	} else {
		usage, newAPIError = adaptor.DoResponse(c, httpResp, info)
	}
	if newAPIError != nil {
		if partial, ok := usage.(*dto.Usage); ok && partial != nil && partial.InterruptedOutput {
			// Failed streams keep usage for diagnostics only. Leave the billing
			// session unsettled so the controller refunds the full reservation.
			service.CaptureUnbilledRequestUsage(c, partial)
			newAPIError = types.NewError(newAPIError, newAPIError.GetErrorCode(), types.ErrOptionWithSkipRetry())
		}
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usageDto := usage.(*dto.Usage)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		originModelName := info.OriginModelName
		originPriceData := info.PriceData

		_, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{})
		if err != nil {
			info.OriginModelName = originModelName
			info.PriceData = originPriceData
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
		}
		postConsumeQuota(c, info, usageDto)

		info.OriginModelName = originModelName
		info.PriceData = originPriceData
		return nil
	}

	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		service.PostAudioConsumeQuota(c, info, usageDto, "")
	} else {
		postConsumeQuota(c, info, usageDto)
	}
	return nil
}
