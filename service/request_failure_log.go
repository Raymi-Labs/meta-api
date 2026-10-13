package service

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const requestFailureKey = "final_request_failure"
const requestFailureAttemptsKey = "request_failure_attempts"
const requestUnbilledUsageKey = "request_unbilled_usage"
const RequestLogStreamKey = "request_log_stream"
const RequestOutcomeKey = "relay_request_succeeded"

func RequestSucceeded(c *gin.Context) bool {
	if c == nil || c.Writer.Status() >= 400 || (c.Request != nil && c.Request.Context().Err() != nil) {
		return false
	}
	if _, failed := c.Get(requestFailureKey); failed {
		return false
	}
	if outcome, exists := c.Get(RequestOutcomeKey); exists {
		return outcome == true
	}
	return true
}

// MarkRequestFailure is only called for the final client-facing error. Channel
// retries remain diagnostic attempts and must not become failed user requests.
func MarkRequestFailure(c *gin.Context, err *types.NewAPIError) {
	if c != nil && err != nil {
		c.Set(requestFailureKey, err)
	}
}

// CaptureUnbilledRequestUsage snapshots counters only, without settling quota or
// creating a consumption record. The final failure log keeps these admin-only
// diagnostics separate from billable usage and successful-request statistics.
func CaptureUnbilledRequestUsage(c *gin.Context, usage *dto.Usage) {
	if c == nil || usage == nil {
		return
	}
	c.Set(requestUnbilledUsageKey, map[string]interface{}{
		"prompt_tokens":             usage.PromptTokens,
		"completion_tokens":         usage.CompletionTokens,
		"total_tokens":              usage.TotalTokens,
		"prompt_tokens_details":     usage.PromptTokensDetails.Clone(),
		"completion_tokens_details": usage.CompletionTokenDetails,
		"web_search_requests":       usage.WebSearchRequests,
		"input_tokens_estimated":    usage.InputTokensEstimated,
		"interrupted_output":        usage.InterruptedOutput,
		"charged":                   false,
	})
}

var logSecretPattern = regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/=-]+|\bsk-[a-z0-9_-]+|((?:api[_-]?key|access[_-]?token|authorization|password)\s*[=:]\s*)[^\s,;]+`)
var logErrorCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,96}$`)

func requestLogMessage(err *types.NewAPIError) string {
	message := common.MaskSensitiveInfo(err.Error())
	message = logSecretPattern.ReplaceAllString(message, "[REDACTED]")
	if len(message) > 2048 {
		message = string([]rune(message)[:min(len([]rune(message)), 512)]) + "…"
	}
	return message
}

func AppendRequestFailureAttempt(c *gin.Context, channel types.ChannelError, err *types.NewAPIError) {
	if !constant.ErrorLogEnabled || c == nil || err == nil {
		return
	}
	attempts, _ := c.Get(requestFailureAttemptsKey)
	list, _ := attempts.([]map[string]interface{})
	// Keep the last five attempts; never store channel keys or request bodies.
	if len(list) >= 5 {
		list = list[len(list)-4:]
	}
	attempt := map[string]interface{}{"channel_id": channel.ChannelId, "status_code": err.StatusCode, "error_code": err.GetErrorCode(), "message": requestLogMessage(err)}
	if err.Upstream != nil {
		attempt["upstream"] = *err.Upstream
	}
	list = append(list, attempt)
	c.Set(requestFailureAttemptsKey, list)
}

func describeRequestFailure(err *types.NewAPIError, upstream bool) (category, reason, hint string) {
	code := string(err.GetErrorCode())
	message := strings.ToLower(err.Error())
	switch {
	case err.GetErrorCode() == types.ErrorCodeBadResponseBody && err.Error() == ResponsesStreamEmptyCompletedReason:
		return "empty_response", "响应结束但未识别到有效输出", "请稍后重试；若持续出现，请携带请求 ID 联系管理员。"
	case IsUpstreamCapabilityError(err):
		return "unavailable", "上游渠道不支持当前模型或工具协议", "请稍后重试；若持续失败，请携带请求 ID 联系管理员。"
	case types.ClassifyFailure(err) == types.FailureClientCanceled:
		return "client_canceled", "客户端取消请求", "客户端已断开连接；请检查是否主动停止、客户端超时或网络中断。"
	case IsContentSafetyPolicyError(err) || code == "content_safety_cooldown" || code == string(types.ErrorCodeSensitiveWordsDetected):
		return "content_policy", "请求被内容安全策略拒绝", "请调整请求内容，遵守使用政策；处于冷静期时请等待结束。"
	case code == string(types.ErrorCodeConversationStateNotFound):
		return "conversation_state", "对话上下文已失效", "请新建会话并重新发送完整上下文。"
	case code == string(types.ErrorCodeInsufficientUserQuota) || code == string(types.ErrorCodePreConsumeTokenQuotaFailed):
		return "quota", "账户或令牌额度不足", "请检查账户余额、令牌额度及套餐限制。"
	case err.StatusCode == http.StatusRequestEntityTooLarge:
		return "request_too_large", "请求体超过大小限制", "请减少消息、附件或图片大小后重试。"
	case err.StatusCode == http.StatusTooManyRequests:
		return "rate_limit", "请求受到限流或并发限制", "请按 Retry-After 等待，降低并发后重试。"
	case err.StatusCode == http.StatusRequestTimeout || err.StatusCode == http.StatusGatewayTimeout || strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded"):
		return "timeout", "请求处理超时", "请稍后重试；若持续超时，请携带请求 ID 联系管理员。"
	case upstream && (err.StatusCode == 401 || err.StatusCode == 403):
		return "upstream_auth", "上游服务鉴权失败", "请联系管理员检查上游渠道配置。"
	case err.StatusCode == 401:
		return "authentication", "API 密钥无效、过期或已停用", "请检查所用令牌的状态、有效期和额度。"
	case err.StatusCode == 403:
		return "permission", "当前请求没有访问权限", "请检查令牌的模型、分组和 IP 限制。"
	case err.StatusCode == 503 || code == string(types.ErrorCodeGetChannelFailed) || code == string(types.ErrorCodeChannelNoAvailableKey):
		return "unavailable", "当前模型服务暂不可用", "请稍后重试或选择其他可用模型、分组。"
	case code == string(types.ErrorCodeInvalidRequest) || code == string(types.ErrorCodeBadRequestBody) || err.StatusCode == 400 || err.StatusCode == 404 || err.StatusCode == 422:
		return "invalid_request", "请求参数或模型不受支持", "请检查接口路径、模型名称和请求参数。"
	default:
		return "upstream_error", "请求处理失败", "请稍后重试；仍失败时复制排查信息并联系管理员。"
	}
}

// RecordFinalRequestFailure runs once after the request handler has returned.
// It records explicit stream errors even when headers have already sent HTTP 200.
func RecordFinalRequestFailure(c *gin.Context, elapsed time.Duration) {
	if !constant.ErrorLogEnabled || c.GetInt("id") <= 0 {
		return
	}
	if done, _ := c.Get("request_failure_logged"); done == true {
		return
	}
	value, _ := c.Get(requestFailureKey)
	apiErr, _ := value.(*types.NewAPIError)
	httpStatus := c.Writer.Status()
	if apiErr == nil {
		if httpStatus < 400 {
			return
		}
		apiErr = types.NewErrorWithStatusCode(fmt.Errorf("%s", http.StatusText(httpStatus)), types.ErrorCode("request_failed"), httpStatus)
	}
	c.Set("request_failure_logged", true)
	attempts, _ := c.Get(requestFailureAttemptsKey)
	upstream := apiErr.UpstreamStatusCode > 0 || apiErr.GetErrorType() == types.ErrorTypeOpenAIError || apiErr.GetErrorType() == types.ErrorTypeClaudeError || attempts != nil
	category, reason, hint := describeRequestFailure(apiErr, upstream)
	code := string(apiErr.GetErrorCode())
	if !logErrorCodePattern.MatchString(code) {
		code = category
	}
	other := map[string]interface{}{
		"failure_class":   types.ClassifyFailure(apiErr),
		"request_failure": true, "failure_category": category, "failure_reason": reason, "failure_hint": hint,
		"error_code": code, "status_code": apiErr.StatusCode, "http_status": httpStatus, "latency_ms": elapsed.Milliseconds(),
	}
	if c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
	}
	retryAfter := apiErr.RetryAfter
	if retryAfter <= 0 {
		retryAfter = ParseRetryAfter(c.Writer.Header().Get("Retry-After"), time.Now())
	}
	if retryAfter > 0 {
		other["retry_after_seconds"] = int((retryAfter + time.Second - 1) / time.Second)
	}
	adminInfo := map[string]interface{}{"error_message": requestLogMessage(apiErr), "error_type": apiErr.GetErrorType(), "use_channel": c.GetStringSlice("use_channel")}
	adminInfo["channel_name"] = c.GetString("channel_name")
	adminInfo["channel_type"] = c.GetInt("channel_type")
	if common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey) {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
	}
	if attempts != nil {
		adminInfo["attempts"] = attempts
	}
	if usage, exists := c.Get(requestUnbilledUsageKey); exists {
		adminInfo["unbilled_usage"] = usage
	}
	if diagnostics, exists := c.Get(requestResponsesStreamFailureKey); exists {
		adminInfo["responses_stream_diagnostics"] = diagnostics
	}
	if apiErr.Upstream != nil && !apiErr.Upstream.IsZero() {
		adminInfo["upstream"] = apiErr.Upstream
	}
	AppendChannelAffinityAdminInfo(c, adminInfo)
	AppendRelayHTTPTimeout(c, adminInfo)
	other["admin_info"] = adminInfo
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = c.GetString("group")
	}
	isStream := c.GetBool(RequestLogStreamKey) || strings.Contains(c.Writer.Header().Get("Content-Type"), "text/event-stream")
	model.RecordErrorLog(c, c.GetInt("id"), c.GetInt("channel_id"), c.GetString("original_model"), c.GetString("token_name"), reason, c.GetInt("token_id"), int(elapsed.Seconds()), isStream, group, other)
}
