package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// Explicit upstream refusals are evidence for this request only. Do not cache
// them as model-wide capabilities or strip parameters on other channels.
const claudeParamHealDoneKey = "claude_param_heal_done"
const emptyClaudeThinkingError = "each thinking block must contain thinking"
const maxClaudeHealErrorBytes = 64 << 10

type claudeHealError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func claudeHealErrorMessage(body []byte) string {
	var response claudeHealError
	if common.Unmarshal(body, &response) != nil || response.Error.Type != "invalid_request_error" {
		return ""
	}
	return response.Error.Message
}

func extractHealableParam(bodyText string) string {
	return service.RejectedSamplingParameter(claudeHealErrorMessage([]byte(bodyText)))
}

type claudeHealReplayBody struct {
	io.Reader
	io.Closer
}

// Always restore consumed bytes and preserve the underlying Close, including
// oversized/error bodies that are ineligible for healing.
func peekClaudeHealError(resp *http.Response) ([]byte, bool) {
	if resp.Body == nil {
		return nil, false
	}
	original := resp.Body
	body, err := io.ReadAll(io.LimitReader(original, maxClaudeHealErrorBytes+1))
	resp.Body = &claudeHealReplayBody{Reader: io.MultiReader(bytes.NewReader(body), original), Closer: original}
	return body, err == nil && len(body) <= maxClaudeHealErrorBytes
}

func recordClaudeAdjustment(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, body []byte, adjustment string) {
	snapshot := *resp
	snapshot.Body = io.NopCloser(bytes.NewReader(body))
	apiErr := service.RelayErrorHandler(c.Request.Context(), &snapshot, false)
	service.AppendRequestAdjustmentAttempt(c, *types.NewChannelError(info.ChannelId, info.ChannelType, "", false, "", false), apiErr, adjustment)
}

// tryHealClaudeParamError 在上游返回 400 时尝试剥参重发一次。
// 返回值：(生效的响应, 是否重发过)。任何环节不满足条件都原样返回首次响应。
func tryHealClaudeParamError(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	adaptor channel.Adaptor,
	requestJSON []byte,
	httpResp *http.Response,
) (*http.Response, bool) {
	if c == nil || info == nil || adaptor == nil || httpResp == nil || len(requestJSON) == 0 {
		return httpResp, false
	}
	if info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeAnthropic {
		return httpResp, false
	}
	if httpResp.StatusCode != http.StatusBadRequest {
		return httpResp, false
	}
	if c.Request == nil || c.Request.Context().Err() != nil || c.Writer == nil || c.Writer.Written() {
		return httpResp, false
	}
	// 每请求最多自愈一次，防止循环
	if c.GetBool(claudeParamHealDoneKey) {
		return httpResp, false
	}

	body, ok := peekClaudeHealError(httpResp)
	if !ok || c.Request.Context().Err() != nil || c.Writer.Written() {
		return httpResp, false
	}
	message := claudeHealErrorMessage(body)
	if message == "" {
		return httpResp, false
	}
	if strings.Contains(strings.ToLower(message), emptyClaudeThinkingError) {
		cleanedJSON, result, err := sanitizeEmptyClaudeThinkingJSON(requestJSON)
		if err != nil || !result.Changed() {
			return httpResp, false
		}

		c.Set(claudeParamHealDoneKey, true)
		recordClaudeAdjustment(c, info, httpResp, body, "empty_thinking")
		logger.LogWarn(c, fmt.Sprintf(
			"request self-heal: upstream rejected empty thinking; removed %d block(s), removed %d message(s), merged %d message(s), retrying once (channel #%d, model %s)",
			result.RemovedBlocks,
			result.RemovedMessages,
			result.MergedMessages,
			info.ChannelId,
			info.UpstreamModelName,
		))

		retryResp, retryErr := adaptor.DoRequest(c, info, bytes.NewBuffer(cleanedJSON))
		if retryErr != nil {
			logger.LogWarn(c, fmt.Sprintf("empty thinking self-heal retry failed, falling back to original 400: %v", retryErr))
			return httpResp, false
		}
		healedResp, ok := retryResp.(*http.Response)
		if !ok || healedResp == nil {
			return httpResp, false
		}
		if healedResp != httpResp && httpResp.Body != nil {
			_ = httpResp.Body.Close()
		}
		c.Header("X-Request-Adjusted", "empty_thinking")
		return healedResp, true
	}
	param := service.RejectedSamplingParameter(message)
	if param == "" {
		return httpResp, false
	}
	// 请求体里确实带了这个参数才有剥除的意义
	strippedJSON, err := sjson.DeleteBytes(requestJSON, param)
	if err != nil || bytes.Equal(strippedJSON, requestJSON) {
		return httpResp, false
	}

	c.Set(claudeParamHealDoneKey, true)
	recordClaudeAdjustment(c, info, httpResp, body, "remove_"+param)
	logger.LogWarn(c, fmt.Sprintf(
		"param self-heal: upstream 400 rejected `%s`, stripped and retrying once (channel #%d, model %s)",
		param, info.ChannelId, info.UpstreamModelName,
	))

	retryResp, err := adaptor.DoRequest(c, info, bytes.NewBuffer(strippedJSON))
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("param self-heal retry failed, falling back to original 400: %v", err))
		return httpResp, false
	}
	healedResp, ok := retryResp.(*http.Response)
	if !ok || healedResp == nil {
		return httpResp, false
	}
	if healedResp != httpResp && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	// 可观测性：让调用方知道参数被服务端调整过
	c.Header("X-Param-Adjusted", param)
	return healedResp, true
}
