package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if hasResponsesOpenAIError(responsesResponse.GetOpenAIError()) || strings.EqualFold(strings.TrimSpace(responsesResponse.Status), "failed") {
		// Responses errors can contain only code/message (including DeepSeek).
		// HTTP 200 transports a response object, not proof of semantic success.
		apiErr := newResponsesStreamEventError(dto.ResponsesStreamResponse{Type: "response.failed", Response: &responsesResponse})
		return nil, types.NewError(apiErr, apiErr.GetErrorCode(), types.ErrOptionWithSkipRetry())
	}
	info.RecordResponsesCompletedSummary(&responsesResponse)

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_count", responsesResponse.CountImageGenerationCalls())
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	return buildResponsesUsage(c, info, &responsesResponse), nil
}

// buildResponsesUsage 从一个完整的 Responses 响应对象计算 *dto.Usage(含内置工具用量),
// 供非流式直连 handler 与聚合 handler 共用,保证两条路径计费口径完全一致。
func buildResponsesUsage(c *gin.Context, info *relaycommon.RelayInfo, responsesResponse *dto.OpenAIResponsesResponse) *dto.Usage {
	info.ObserveResponseModel(responsesResponse.Model)
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails = responsesResponse.Usage.InputTokensDetails.Clone()
		}
	}
	for _, output := range responsesResponse.Output {
		if output.Type == dto.BuildInCallWebSearchCall {
			usage.WebSearchRequests++
		}
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage
	}
	if usage.WebSearchRequests > 0 {
		if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.NormalizeBuiltInToolType(dto.BuildInToolWebSearch)]; exists && webSearchTool != nil {
			webSearchTool.CallCount = usage.WebSearchRequests
		}
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		toolType := dto.NormalizeBuiltInToolType(common.Interface2String(tool["type"]))
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[toolType]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		if dto.IsBuiltInWebSearchToolType(toolType) {
			buildToolinfo.CallCount = usage.WebSearchRequests
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage
}

// aggregateResponsesStream 消费上游的 Responses SSE 流并聚合出最终的完整响应对象。
//
// 使用场景:codex 上游被 newapi 恒定强制流式(stream=true),但客户端本意是非流式。
// 本函数复用 StreamScannerHandler 读上游(保留其 goroutine 防泄漏 / client-gone 检测 /
// 超时 / resp.Body 关闭等全套保护),但通过 SuppressStreamResponseHeaders 抑制客户端 SSE
// 响应头、DisablePing 抑制 ping,使调用方可在末尾以单个非流式 JSON body 返回。
//
// response.completed 事件里的 Response 就是与非流式 body 同构的完整对象(含 Output+Usage),
// 故聚合本质就是捕获它;拿到即停止扫描。上游异常(incomplete/failed/error)转为非流式错误返回;
// 未收到 completed 则按 client-gone / 上游异常分别返回可跳过重试或普通错误。
func aggregateResponsesStream(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.OpenAIResponsesResponse, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}
	// 抑制客户端 SSE 头 + ping:读上游 SSE,但最终以单个非流式 JSON 返回客户端。
	// 用 defer 存/恢复原值,把抑制严格收敛到本次聚合调用作用域内,避免标志残留到外层
	// 换渠道重试的后续轮次(RelayInfo 跨轮复用)造成跨渠道污染;保存原值而非硬编码 false,
	// 也避免误清其它设置方(如 gemini adaptor)已置的 DisablePing。StreamScannerHandler
	// 是同步调用(内部 goroutine 在其返回前已 wg 收敛),故 defer 复位不影响读流期间的抑制生效。
	entrySuppress := info.SuppressStreamResponseHeaders
	entryDisablePing := info.DisablePing
	info.SuppressStreamResponseHeaders = true
	info.DisablePing = true
	defer func() {
		info.SuppressStreamResponseHeaders = entrySuppress
		info.DisablePing = entryDisablePing
	}()

	var (
		finalResponse *dto.OpenAIResponsesResponse
		streamErr     *types.NewAPIError
		completed     bool
	)

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		if streamErr != nil {
			return false
		}
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "aggregate responses stream: failed to unmarshal event: "+err.Error())
			return true
		}
		if streamResponse.Response != nil {
			info.ObserveResponseModel(streamResponse.Response.Model)
		}
		switch streamResponse.Type {
		case "response.completed":
			if streamResponse.Response == nil {
				return true
			}
			if strings.EqualFold(strings.TrimSpace(streamResponse.Response.Status), "incomplete") {
				streamErr = newResponsesIncompleteError(streamResponse.Response)
				return false
			}
			finalResponse = streamResponse.Response
			completed = true
			return false // 已拿到完整响应,停止扫描
		case "error", "response.error", "response.failed", "response.incomplete":
			streamErr = newResponsesStreamEventError(streamResponse)
			return false
		}
		return true
	})

	if streamErr != nil {
		return nil, streamErr
	}
	if !completed || finalResponse == nil {
		// 客户端已断开:归类为取消,跳过重试且不污染错误率(与并发控制的 client-canceled 一致)。
		if info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone {
			return nil, types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithHideErrMsg("client canceled while aggregating codex stream"))
		}
		// 上游异常结束且无 completed 事件:返回可重试错误,交由外层换渠道重试(此时尚未向客户端写任何字节)。
		reason := "codex stream ended without a completed event"
		if info.StreamStatus != nil && info.StreamStatus.EndReason != "" {
			reason = fmt.Sprintf("%s (stream end: %s)", reason, info.StreamStatus.EndReason)
		}
		return nil, types.NewOpenAIError(fmt.Errorf("%s", reason), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	return finalResponse, nil
}

// sanitizeAggregatedResponseHeaders 把上游 SSE 响应的头改写为适合单个 JSON body 的形态。
// IOCopyBytesGracefully 会把上游 resp.Header 原样拷给客户端,而上游此时是 text/event-stream,
// 必须覆盖为 application/json,并清掉分块传输头,避免客户端把非流式 JSON 误当作 SSE。
func sanitizeAggregatedResponseHeaders(resp *http.Response) {
	if resp == nil || resp.Header == nil {
		return
	}
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Transfer-Encoding")
	// 聚合后写出的是重新序列化的未压缩 JSON,清掉上游可能残留的压缩标记,避免客户端误解码。
	resp.Header.Del("Content-Encoding")
	// 清掉上游 SSE 残留的流式语义头:非流式 JSON 是一次性返回,保留 X-Accel-Buffering:no
	// 会让 nginx 等反代关闭对该响应的缓冲;Connection/Cache-Control 也与非流式响应语义不符。
	resp.Header.Del("X-Accel-Buffering")
	resp.Header.Del("Connection")
	resp.Header.Del("Cache-Control")
}

// OaiResponsesAggregateHandler 处理「客户端要非流式的 /v1/responses、但上游被强制流式」的情况:
// 聚合上游 SSE 成完整 Responses 对象,再以单个非流式 JSON 返回客户端——客户端看到的正是它期望的
// 原生 Responses 非流式响应(无需任何格式转换)。计费与非流式直连 handler 完全一致。
func OaiResponsesAggregateHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	finalResponse, apiErr := aggregateResponsesStream(c, info, resp)
	if apiErr != nil {
		return nil, apiErr
	}

	info.RecordResponsesCompletedSummary(finalResponse)
	if finalResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_count", finalResponse.CountImageGenerationCalls())
		c.Set("image_generation_call_quality", finalResponse.GetQuality())
		c.Set("image_generation_call_size", finalResponse.GetSize())
	}

	responseBody, err := common.Marshal(finalResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	sanitizeAggregatedResponseHeaders(resp)
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return buildResponsesUsage(c, info, finalResponse), nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	return OaiResponsesStreamHandlerWithOptions(c, info, resp, nil)
}

type ResponsesStreamAutoContinueContext struct {
	Usage              *dto.Usage
	OutputText         string
	ResponseID         string
	ResponseModel      string
	ResponseCreatedAt  int
	HasEffectiveOutput bool
	EndReason          relaycommon.StreamEndReason
	EndError           error
}

type ResponsesStreamHandlerOptions struct {
	AutoContinue            func(ResponsesStreamAutoContinueContext) (*dto.Usage, bool)
	ContinuationOutputOnly  bool
	DisableAutoContinuation bool
	scheduleCooldown        func(string)
}

type bufferedResponsesStreamEvent struct {
	response dto.ResponsesStreamResponse
	data     string
}

const maxResponsesRetryPreludeBytes = 256 << 10

func OaiResponsesStreamHandlerWithOptions(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, opts *ResponsesStreamHandlerOptions) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	var billableDeltaBuilder strings.Builder
	hasUpstreamUsage := false
	completed := false
	decodeErrors := 0
	var terminalError *types.NewAPIError
	hasEffectiveOutput := false
	responseID := ""
	responseModel := ""
	responseCreatedAt := 0
	hasNonTextOutput := false
	failureForwarded := false
	preludeCommitted := false
	bufferedPreludeBytes := 0
	bufferedPrelude := make([]bufferedResponsesStreamEvent, 0, 4)
	var deliveryError error
	sendEvent := func(event dto.ResponsesStreamResponse, data string) bool {
		if deliveryError != nil {
			return false
		}
		if err := sendResponsesStreamData(c, event, data); err != nil {
			deliveryError = err
			info.StreamStatus.RecordError("responses stream write failed: " + err.Error())
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
			return false
		}
		return true
	}
	interruptedUsage := func() *dto.Usage {
		// No output means no partial-output snapshot, even if a failed prelude reported usage.
		if !hasEffectiveOutput {
			return nil
		}
		if !hasUpstreamUsage {
			usage.CompletionTokens = service.CountTextToken(billableDeltaBuilder.String(), info.UpstreamModelName)
			if usage.CompletionTokens > 0 {
				usage.PromptTokens = info.GetEstimatePromptTokens()
				usage.InputTokensEstimated = true
			}
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if usage.TotalTokens <= 0 && usage.WebSearchRequests == 0 {
			return nil
		}
		usage.InterruptedOutput = true
		return usage
	}
	flushBufferedPrelude := func() {
		if len(bufferedPrelude) == 0 {
			return
		}
		for _, event := range bufferedPrelude {
			if shouldSendResponsesStreamData(event.response, opts) {
				if !sendEvent(event.response, event.data) {
					return
				}
				preludeCommitted = true
			}
		}
		bufferedPrelude = bufferedPrelude[:0]
		bufferedPreludeBytes = 0
	}
	bufferPrelude := func(streamResponse dto.ResponsesStreamResponse, data string) bool {
		if bufferedPreludeBytes+len(data) > maxResponsesRetryPreludeBytes {
			flushBufferedPrelude()
			return false
		}
		bufferedPrelude = append(bufferedPrelude, bufferedResponsesStreamEvent{response: streamResponse, data: data})
		bufferedPreludeBytes += len(data)
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err == nil {
			if streamResponse.Response != nil {
				info.ObserveResponseModel(streamResponse.Response.Model)
			}
			explicitFailure := isResponsesStreamFailureEvent(streamResponse)
			if explicitFailure {
				data = decorateResponsesPolicyFailureData(streamResponse, data)
			}
			if explicitFailure && (streamResponse.Type == "" || streamResponse.Type == "response.completed") {
				data = normalizeResponsesStreamFailureEvent(&streamResponse, data)
			}
			delayCompletedEvent := streamResponse.Type == "response.completed" && !explicitFailure
			effectiveOutput := isEffectiveResponsesStreamOutput(streamResponse)
			nonTextOutput := isNonTextResponsesStreamOutput(streamResponse)
			if effectiveOutput {
				info.SetFirstEffectiveOutputTime()
			}
			if streamResponse.Response != nil {
				if reported := streamResponse.Response.Usage; reported != nil {
					// Terminal failed/incomplete events can carry authoritative usage.
					// Snapshots replace counts; never add cumulative event usage.
					captured := buildResponsesUsage(c, info, streamResponse.Response)
					if captured.TotalTokens == 0 {
						captured.TotalTokens = captured.PromptTokens + captured.CompletionTokens
					}
					if service.ValidUsage(captured) {
						captured.WebSearchRequests = max(captured.WebSearchRequests, usage.WebSearchRequests)
						usage = captured
						hasUpstreamUsage = true
					}
				}
				if streamResponse.Response.ID != "" {
					responseID = streamResponse.Response.ID
				}
				if streamResponse.Response.Model != "" {
					responseModel = streamResponse.Response.Model
				}
				if streamResponse.Response.CreatedAt != 0 {
					responseCreatedAt = streamResponse.Response.CreatedAt
				}
			}
			if delayCompletedEvent && effectiveOutput {
				hasEffectiveOutput = true
			}
			if delayCompletedEvent && nonTextOutput {
				hasNonTextOutput = true
			}
			if explicitFailure {
				terminalError = newResponsesStreamEventError(streamResponse)
				canReroute := service.IsRetryableUpstreamOverloadError(terminalError) &&
					!hasEffectiveOutput && !hasNonTextOutput && !preludeCommitted
				if canReroute {
					bufferedPrelude = bufferedPrelude[:0]
					bufferedPreludeBytes = 0
					return false
				}
				flushBufferedPrelude()
				if deliveryError != nil {
					return false
				}
				if shouldSendResponsesStreamData(streamResponse, opts) {
					if !sendEvent(streamResponse, data) {
						return false
					}
					failureForwarded = true
				}
				terminalError = types.NewError(terminalError, terminalError.GetErrorCode(), types.ErrOptionWithSkipRetry())
				if failureForwarded {
					common.SetContextKey(c, constant.ContextKeyResponsesStreamErrorWritten, true)
				}
				return false
			}
			if !delayCompletedEvent {
				shouldBuffer := !preludeCommitted && !hasEffectiveOutput && !hasNonTextOutput && !effectiveOutput && !nonTextOutput
				if !shouldBuffer || !bufferPrelude(streamResponse, data) {
					flushBufferedPrelude()
					if deliveryError != nil {
						return false
					}
					if shouldSendResponsesStreamData(streamResponse, opts) {
						if !sendEvent(streamResponse, data) {
							return false
						}
						preludeCommitted = true
					}
				}
				if effectiveOutput {
					hasEffectiveOutput = true
				}
				if nonTextOutput {
					hasNonTextOutput = true
				}
			}
			switch streamResponse.Type {
			case "response.completed":
				if streamResponse.Response == nil && hasEffectiveOutput {
					terminalError = types.NewOpenAIError(errors.New("responses completion is missing its response object"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
					return false
				}
				if streamResponse.Response != nil {
					status := strings.ToLower(strings.TrimSpace(streamResponse.Response.Status))
					if status != "" && status != "completed" {
						terminalError = types.NewOpenAIError(errors.New("responses completion has a non-terminal status"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
						return false
					}
				}
				if streamResponse.Response != nil {
					info.RecordResponsesCompletedSummary(streamResponse.Response)
					if streamResponse.Response.Usage != nil {
						if streamResponse.Response.Usage.InputTokens != 0 {
							usage.PromptTokens = streamResponse.Response.Usage.InputTokens
						}
						if streamResponse.Response.Usage.OutputTokens != 0 {
							usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
						}
						if streamResponse.Response.Usage.TotalTokens != 0 {
							usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
						}
						if streamResponse.Response.Usage.InputTokensDetails != nil {
							usage.PromptTokensDetails = streamResponse.Response.Usage.InputTokensDetails.Clone()
						}
					}
					if streamResponse.Response.HasImageGenerationCall() {
						c.Set("image_generation_call", true)
						c.Set("image_generation_call_count", streamResponse.Response.CountImageGenerationCalls())
						c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
						c.Set("image_generation_call_size", streamResponse.Response.GetSize())
					}
				}
				if shouldFailEmptyResponsesCompleted(hasEffectiveOutput) {
					reason := service.ResponsesStreamEmptyCompletedReason
					if info != nil && info.StreamStatus != nil {
						info.StreamStatus.RecordError(reason)
					}
					flushBufferedPrelude()
					sendSyntheticResponsesFailed(c, info, usage, reason, responseID, responseModel, responseCreatedAt)
					// An empty result does not establish an outage of the shared upstream.
					var summary *relaycommon.ResponsesCompletedSummary
					if streamResponse.Response != nil {
						summary = info.ResponsesCompletedSummary
					}
					service.CaptureResponsesEmptyOutputDiagnostics(c, summary, decodeErrors)
					terminalError = types.NewOpenAIError(errors.New(reason), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
					return false
				} else {
					flushBufferedPrelude()
					if deliveryError != nil {
						return false
					}
					if shouldSendResponsesStreamData(streamResponse, opts) {
						if !sendEvent(streamResponse, data) {
							return false
						}
						info.StreamStatus.MarkDeliveredCompletion()
					}
					completed = true
					return false
				}
			case "response.output_text.delta":
				// 处理输出文本
				responseTextBuilder.WriteString(streamResponse.Delta)
				billableDeltaBuilder.WriteString(streamResponse.Delta)
			case "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.refusal.delta", "response.function_call_arguments.delta":
				billableDeltaBuilder.WriteString(streamResponse.Delta)
			case dto.ResponsesOutputTypeItemDone:
				// 函数调用处理
				if streamResponse.Item != nil {
					switch streamResponse.Item.Type {
					case dto.BuildInCallWebSearchCall:
						usage.WebSearchRequests++
						if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
							if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.NormalizeBuiltInToolType(dto.BuildInToolWebSearch)]; exists && webSearchTool != nil {
								webSearchTool.CallCount++
							}
						}
					}
				}
			}
		} else {
			decodeErrors++
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
		}
		return true
	})

	if deliveryError != nil {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeFailed)
		return interruptedUsage(), newResponsesDeliveryError(c, deliveryError)
	}

	if terminalError != nil {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeFailed)
		if info != nil && info.StreamStatus != nil {
			info.StreamStatus.RecordError("responses stream terminated with explicit failure: " + string(terminalError.GetErrorCode()))
		}
		return interruptedUsage(), terminalError
	}

	if info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeCancelled)
		return interruptedUsage(), types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithHideErrMsg("client canceled while receiving responses stream"))
	}

	if usage.CompletionTokens == 0 && !hasUpstreamUsage {
		// 计算输出文本的 token 数量
		tempStr := billableDeltaBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 && !hasUpstreamUsage {
		usage.InputTokensEstimated = true
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	if !completed {
		flushBufferedPrelude()
		if deliveryError != nil {
			info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeFailed)
			return interruptedUsage(), newResponsesDeliveryError(c, deliveryError)
		}
		reason := service.ResponsesStreamMissingCompletedReason
		if info != nil && info.StreamStatus != nil {
			info.StreamStatus.RecordError(reason)
		}
		if shouldScheduleMissingResponsesCompletedCooldown(info) {
			scheduleResponsesStreamCooldownWithOptions(c, opts, reason)
		}
		if info != nil && info.IsChannelTest {
			return usage, types.NewOpenAIError(errors.New(reason), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if shouldAutoContinueResponsesStream(c, info, opts, false, hasEffectiveOutput, hasNonTextOutput, responseTextBuilder.String()) {
			continuedUsage, continued := opts.AutoContinue(ResponsesStreamAutoContinueContext{
				Usage:              usage,
				OutputText:         responseTextBuilder.String(),
				ResponseID:         responseID,
				ResponseModel:      responseModel,
				ResponseCreatedAt:  responseCreatedAt,
				HasEffectiveOutput: hasEffectiveOutput,
				EndReason:          info.StreamStatus.EndReason,
				EndError:           info.StreamStatus.EndError,
			})
			mergeResponsesStreamUsage(usage, continuedUsage)
			if continued {
				return usage, nil
			}
		}
		sendSyntheticResponsesFailed(c, info, usage, reason, responseID, responseModel, responseCreatedAt)
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeIncomplete)
		// Estimates were finalized before continuation; do not overwrite the
		// merged usage with the first attempt's delta buffer.
		if hasEffectiveOutput && (usage.TotalTokens > 0 || usage.WebSearchRequests > 0) {
			usage.InterruptedOutput = true
			return usage, types.NewOpenAIError(errors.New(reason), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
		return nil, types.NewOpenAIError(errors.New(reason), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}

	info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeCompleted)
	return usage, nil
}

func newResponsesDeliveryError(c *gin.Context, err error) *types.NewAPIError {
	if errors.Is(err, context.Canceled) || (c.Request != nil && errors.Is(c.Request.Context().Err(), context.Canceled)) {
		return types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry(), types.ErrOptionWithHideErrMsg("client canceled while receiving responses stream"))
	}
	return types.NewErrorWithStatusCode(err, helper.DownstreamWriteErrorCode, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}

func decorateResponsesPolicyFailureData(streamResponse dto.ResponsesStreamResponse, data string) string {
	apiErr := newResponsesStreamEventError(streamResponse)
	if !service.IsContentSafetyPolicyError(apiErr) {
		return data
	}
	var payload map[string]any
	if err := common.UnmarshalJsonStr(data, &payload); err != nil {
		return data
	}
	warning := "本站警告：上游内容安全策略已拒绝并记录本次请求；请勿重复提交类似内容，反复触发将进入冷静期并提交管理员复核。"
	decorate := func(raw any) {
		if object, ok := raw.(map[string]any); ok {
			message, _ := object["message"].(string)
			if !strings.Contains(message, "本站警告") {
				object["message"] = warning + " 原始提示：" + message
			}
		}
	}
	decorate(payload["error"])
	if response, ok := payload["response"].(map[string]any); ok {
		decorate(response["error"])
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return data
	}
	return string(encoded)
}

func shouldSendResponsesStreamData(streamResponse dto.ResponsesStreamResponse, opts *ResponsesStreamHandlerOptions) bool {
	if opts == nil || !opts.ContinuationOutputOnly {
		return true
	}
	switch streamResponse.Type {
	case "response.output_text.delta", "response.completed", "response.failed", "response.incomplete", "response.error", "error":
		return true
	default:
		return false
	}
}

func shouldAutoContinueResponsesStream(c *gin.Context, info *relaycommon.RelayInfo, opts *ResponsesStreamHandlerOptions, terminalWithoutCompleted bool, hasEffectiveOutput bool, hasNonTextOutput bool, outputText string) bool {
	if c == nil || c.Request == nil || c.Request.Context().Err() != nil {
		return false
	}
	if info == nil || info.StreamStatus == nil || opts == nil || opts.AutoContinue == nil || opts.DisableAutoContinuation {
		return false
	}
	if terminalWithoutCompleted || !hasEffectiveOutput || hasNonTextOutput {
		return false
	}
	return strings.TrimSpace(outputText) != ""
}

func mergeResponsesStreamUsage(dst *dto.Usage, src *dto.Usage) {
	if dst == nil || src == nil {
		return
	}
	dst.PromptTokens += src.PromptTokens
	dst.CompletionTokens += src.CompletionTokens
	dst.TotalTokens += src.TotalTokens
	dst.WebSearchRequests += src.WebSearchRequests
	dst.InputTokensEstimated = dst.InputTokensEstimated || src.InputTokensEstimated
	dst.InterruptedOutput = dst.InterruptedOutput || src.InterruptedOutput
	dst.PromptTokensDetails.Add(src.PromptTokensDetails)
	if dst.TotalTokens == 0 {
		dst.TotalTokens = dst.PromptTokens + dst.CompletionTokens
	}
}

func isNonTextResponsesStreamOutput(streamResponse dto.ResponsesStreamResponse) bool {
	if streamResponse.Item != nil {
		return isNonTextResponsesOutput(*streamResponse.Item)
	}
	if streamResponse.Response != nil {
		for _, output := range streamResponse.Response.Output {
			if isNonTextResponsesOutput(output) {
				return true
			}
		}
	}
	return false
}

func isEffectiveResponsesStreamOutput(streamResponse dto.ResponsesStreamResponse) bool {
	if streamResponse.Delta != "" {
		return true
	}
	if streamResponse.Item != nil && streamResponse.Item.Type != "" {
		return isEffectiveResponsesOutput(*streamResponse.Item)
	}
	if streamResponse.Response == nil {
		return false
	}
	for _, output := range streamResponse.Response.Output {
		if isEffectiveResponsesOutput(output) {
			return true
		}
	}
	return false
}

func isEffectiveResponsesOutput(output dto.ResponsesOutput) bool {
	if output.Type == "" {
		return false
	}
	if output.Type != "message" {
		return true
	}
	for _, content := range output.Content {
		if content.Text != "" {
			return true
		}
	}
	return false
}

func isNonTextResponsesOutput(output dto.ResponsesOutput) bool {
	switch output.Type {
	case "", "message":
		return false
	default:
		return true
	}
}

func isResponsesStreamFailureEvent(streamResponse dto.ResponsesStreamResponse) bool {
	if streamResponse.Error != nil {
		return true
	}
	if streamResponse.Response != nil {
		status := strings.ToLower(strings.TrimSpace(streamResponse.Response.Status))
		if hasResponsesOpenAIError(streamResponse.Response.GetOpenAIError()) || status == "failed" || status == "incomplete" || status == "cancelled" || status == "canceled" {
			return true
		}
	}
	switch streamResponse.Type {
	case "error", "response.error", "response.failed", "response.incomplete":
		return true
	default:
		return false
	}
}

func normalizeResponsesStreamFailureEvent(streamResponse *dto.ResponsesStreamResponse, data string) string {
	if streamResponse == nil {
		return data
	}
	eventType := "error"
	if streamResponse.Response != nil {
		eventType = "response.failed"
	}
	streamResponse.Type = eventType

	// Preserve provider-specific fields while making the SSE event and JSON type agree.
	var payload map[string]any
	if err := common.UnmarshalJsonStr(data, &payload); err != nil {
		return data
	}
	payload["type"] = eventType
	normalized, err := common.Marshal(payload)
	if err != nil {
		return data
	}
	return string(normalized)
}

func shouldScheduleMissingResponsesCompletedCooldown(info *relaycommon.RelayInfo) bool {
	if info == nil || info.StreamStatus == nil {
		return true
	}
	return info.StreamStatus.EndReason != relaycommon.StreamEndReasonClientGone
}

func shouldFailEmptyResponsesCompleted(hasEffectiveOutput bool) bool {
	return !hasEffectiveOutput
}

func scheduleResponsesStreamCooldown(c *gin.Context, reason string) {
	scheduled, trips, err := service.ScheduleCurrentChannelScopedPreDisableWait(c, reason)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("schedule responses stream cooldown failed: %v", err))
	}
	if scheduled {
		logger.LogInfo(c, fmt.Sprintf("responses stream cooldown scheduled: %s%s", reason, formatScopedCooldownTrips(trips)))
	}
}

func scheduleResponsesStreamCooldownWithOptions(c *gin.Context, opts *ResponsesStreamHandlerOptions, reason string) {
	if opts != nil && opts.scheduleCooldown != nil {
		opts.scheduleCooldown(reason)
		return
	}
	scheduleResponsesStreamCooldown(c, reason)
}

func formatScopedCooldownTrips(trips []service.CRSShortCircuitTrip) string {
	if len(trips) == 0 {
		return ""
	}
	parts := make([]string, 0, len(trips))
	for _, trip := range trips {
		state := "existing"
		if trip.Opened {
			state = "opened"
		}
		parts = append(parts, fmt.Sprintf("%s=%s ttl=%ds %s", trip.Scope, trip.Key, trip.TTLSeconds, state))
	}
	return "; scopes: " + strings.Join(parts, "; ")
}

func sendSyntheticResponsesFailed(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, reason, responseID, model string, createdAt int) {
	if c == nil {
		return
	}
	if responseID == "" {
		responseID = "resp_" + c.GetString(common.RequestIdKey)
	}
	if model == "" && info != nil {
		model = info.UpstreamModelName
		if model == "" {
			model = info.OriginModelName
		}
	}
	if createdAt == 0 {
		createdAt = int(time.Now().Unix())
	}
	message := reason
	if info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason != "" {
		message = fmt.Sprintf("%s (stream end: %s)", reason, info.StreamStatus.EndReason)
	}
	logger.LogError(c, "responses stream failed without output: "+message)
	openaiError := types.OpenAIError{
		Message: message,
		Type:    string(types.ErrorCodeBadResponseBody),
		Code:    types.ErrorCodeBadResponseBody,
	}
	responseUsage := normalizeResponsesUsage(usage)
	streamResponse := dto.ResponsesStreamResponse{
		Type: "response.failed",
		Response: &dto.OpenAIResponsesResponse{
			ID:        responseID,
			Object:    "response",
			CreatedAt: createdAt,
			Status:    "failed",
			Model:     model,
			Error:     openaiError,
			Output:    []dto.ResponsesOutput{},
			Usage:     &responseUsage,
		},
	}
	jsonData, err := common.Marshal(streamResponse)
	if err != nil {
		logger.LogError(c, "failed to marshal synthetic responses failed event: "+err.Error())
		return
	}
	if err := sendResponsesStreamData(c, streamResponse, string(jsonData)); err != nil && info != nil && info.StreamStatus != nil {
		info.StreamStatus.RecordError("responses failure event write failed: " + err.Error())
	}
	common.SetContextKey(c, constant.ContextKeyResponsesStreamErrorWritten, true)
}

func normalizeResponsesUsage(usage *dto.Usage) dto.Usage {
	if usage == nil {
		usage = &dto.Usage{}
	}
	responseUsage := *usage
	responseUsage.PromptTokensDetails = usage.PromptTokensDetails.Clone()
	if usage.InputTokensDetails != nil {
		details := usage.InputTokensDetails.Clone()
		responseUsage.InputTokensDetails = &details
	}
	if responseUsage.InputTokens == 0 {
		responseUsage.InputTokens = responseUsage.PromptTokens
	}
	if responseUsage.OutputTokens == 0 {
		responseUsage.OutputTokens = responseUsage.CompletionTokens
	}
	if responseUsage.TotalTokens == 0 {
		responseUsage.TotalTokens = responseUsage.InputTokens + responseUsage.OutputTokens
	}
	if responseUsage.InputTokensDetails == nil && (usage.PromptTokensDetails.CachedTokens != 0 || usage.PromptTokensDetails.CachedTokensDetails != nil) {
		details := usage.PromptTokensDetails.Clone()
		responseUsage.InputTokensDetails = &details
	}
	return responseUsage
}
