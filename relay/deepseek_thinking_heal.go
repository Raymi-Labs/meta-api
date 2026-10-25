package relay

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const deepSeekThinkingHealDoneKey = "deepseek_thinking_heal_done"
const deepSeekThinkingAdjustment = "deepseek_thinking_disabled"

func isOfficialDeepSeekMessagesURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && u.Scheme == "https" && u.Host == "api.deepseek.com" &&
		u.User == nil && u.Path == "/anthropic/v1/messages" && u.RawQuery == "" && u.Fragment == ""
}

// A missing historical thinking block cannot be reconstructed. Only an explicit
// refusal from the official native Messages API permits this one-request
// downgrade; normal thinking requests and all historical content stay intact.
func tryHealDeepSeekThinkingError(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, requestJSON []byte, resp *http.Response) (*http.Response, bool) {
	if c == nil || info == nil || info.ChannelMeta == nil || adaptor == nil || resp == nil || len(requestJSON) == 0 {
		return resp, false
	}
	if info.ChannelType != constant.ChannelTypeDeepSeek || info.RelayFormat != types.RelayFormatClaude || info.GetFinalRequestRelayFormat() != types.RelayFormatClaude || resp.StatusCode != http.StatusBadRequest {
		return resp, false
	}
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled || c.GetBool(deepSeekThinkingHealDoneKey) {
		return resp, false
	}
	if !canAdjustDeepSeekThinking(c) {
		return resp, false
	}
	requestURL, err := adaptor.GetRequestURL(info)
	if err != nil || !isOfficialDeepSeekMessagesURL(requestURL) || (resp.Request != nil && (resp.Request.URL == nil || !isOfficialDeepSeekMessagesURL(resp.Request.URL.String()))) {
		return resp, false
	}
	if !gjson.ValidBytes(requestJSON) || !gjson.ParseBytes(requestJSON).IsObject() || gjson.GetBytes(requestJSON, "thinking.type").String() == "disabled" {
		return resp, false
	}
	body, ok := peekClaudeHealError(resp)
	if !ok || !canAdjustDeepSeekThinking(c) || !service.IsDeepSeekMissingThinkingHistory(claudeHealErrorMessage(body)) {
		return resp, false
	}
	adjusted, err := sjson.SetBytes(requestJSON, "thinking.type", "disabled")
	if err != nil || bytes.Equal(adjusted, requestJSON) {
		return resp, false
	}
	c.Set(deepSeekThinkingHealDoneKey, true)
	recordClaudeAdjustment(c, info, resp, body, deepSeekThinkingAdjustment)
	if c.Writer.Written() {
		// Headers were already committed by a known ping. Record the exception
		// in the existing admin-only attempt instead of claiming a header was sent.
		if value, exists := c.Get("request_failure_attempts"); exists {
			if attempts, ok := value.([]map[string]interface{}); ok && len(attempts) > 0 {
				attempts[len(attempts)-1]["after_keepalive"] = true
			}
		}
	}
	logger.LogWarn(c, fmt.Sprintf("request adjustment: DeepSeek rejected missing historical thinking; disabling thinking for this request and retrying once (channel #%d, model %s)", info.ChannelId, info.UpstreamModelName))
	retry, retryErr := adaptor.DoRequest(c, info, bytes.NewReader(adjusted))
	if retryErr != nil {
		logger.LogWarn(c, fmt.Sprintf("DeepSeek thinking adjustment retry failed, falling back to original 400: %v", retryErr))
		return resp, false
	}
	healed, ok := retry.(*http.Response)
	if !ok || healed == nil {
		return resp, false
	}
	if healed != resp && resp.Body != nil {
		_ = resp.Body.Close()
	}
	info.ReasoningEffort = "none"
	// A replay can restart heartbeats after the eligibility check stopped them.
	// Join before deciding whether the adjustment header can still be delivered.
	helper.StopStreamKeepAlive(c)
	if !c.Writer.Written() {
		c.Header("X-Request-Adjusted", deepSeekThinkingAdjustment)
	} else if value, exists := c.Get("request_failure_attempts"); exists {
		if attempts, ok := value.([]map[string]interface{}); ok && len(attempts) > 0 {
			attempt := attempts[len(attempts)-1]
			if attempt["adjustment"] == deepSeekThinkingAdjustment {
				attempt["adjustment_header_not_delivered"] = true
			}
		}
	}
	return healed, true
}

func canAdjustDeepSeekThinking(c *gin.Context) bool {
	return helper.CanAdjustClaudeRequestBeforeOutput(c)
}
