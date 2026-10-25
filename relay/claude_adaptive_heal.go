package relay

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const claudeAdaptiveThinkingAdjustment = "claude_adaptive_thinking"

// The exact upstream refusal, rather than the model's name, authorizes a
// one-request migration. Keep unsupported extensions and explicit off intact.
// See .agents/notes/implemented/bug-fix/2026-10-25-claude-adaptive-refusal-heal.md.
func adaptRejectedClaudeThinking(info *relaycommon.RelayInfo, requestJSON []byte, message string) ([]byte, string, bool) {
	if info == nil || info.ChannelMeta == nil || info.ChannelSetting.PassThroughBodyEnabled ||
		model_setting.GetGlobalSettings().PassThroughRequestEnabled || !gjson.ValidBytes(requestJSON) {
		return nil, "", false
	}
	switch original := info.Request.(type) {
	case *dto.ClaudeRequest:
		if original != nil && original.Thinking != nil && original.Thinking.Type == "disabled" {
			return nil, "", false
		}
	case *dto.GeneralOpenAIRequest:
		if original != nil && (original.ReasoningEffort == "none" || gjson.GetBytes(original.THINKING, "type").String() == "disabled") {
			return nil, "", false
		}
	}
	request := gjson.ParseBytes(requestJSON)
	model := request.Get("model")
	if !request.IsObject() || model.Type != gjson.String || model.String() == "" ||
		(info.UpstreamModelName != "" && info.UpstreamModelName != model.String()) {
		return nil, "", false
	}
	want := model.String() + " requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort"
	if strings.TrimSuffix(strings.TrimSpace(message), ".") != want {
		return nil, "", false
	}
	thinking := request.Get("thinking")
	if !thinking.IsObject() || thinking.Get("type").String() != "enabled" {
		return nil, "", false
	}
	known := true
	thinking.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "type":
		case "budget_tokens":
			budget, err := strconv.ParseInt(value.Raw, 10, 64)
			known = value.Type == gjson.Number && err == nil && budget > 0
		case "display":
			known = value.Type == gjson.String
		case "block_binding":
			known = value.IsObject() || value.Type == gjson.Null
		default:
			known = false
		}
		return known
	})
	if !known {
		return nil, "", false
	}
	output := request.Get("output_config")
	if output.Exists() && !output.IsObject() {
		return nil, "", false
	}
	effort := output.Get("effort")
	chosenEffort := effort.String()
	if effort.Exists() {
		if effort.Type != gjson.String {
			return nil, "", false
		}
		switch effort.String() {
		case "low", "medium", "high", "xhigh", "max":
		default:
			return nil, "", false
		}
	} else if original, ok := info.Request.(*dto.GeneralOpenAIRequest); ok && original != nil && original.ReasoningEffort != "" {
		// Chat -> Messages encodes these explicit levels as manual budgets.
		// Recover the caller's level, never infer a level from an arbitrary budget.
		if len(info.ParamOverride) != 0 {
			return nil, "", false
		}
		switch original.ReasoningEffort {
		case "low", "medium", "high":
			chosenEffort = original.ReasoningEffort
		default:
			return nil, "", false
		}
	}
	adjusted, err := sjson.SetBytes(requestJSON, "thinking.type", "adaptive")
	if err != nil {
		return nil, "", false
	}
	adjusted, err = sjson.DeleteBytes(adjusted, "thinking.budget_tokens")
	if err == nil && !effort.Exists() && chosenEffort != "" {
		adjusted, err = sjson.SetBytes(adjusted, "output_config.effort", chosenEffort)
	}
	return adjusted, chosenEffort, err == nil
}

func retryClaudeAdaptiveThinking(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, resp *http.Response, errorBody, adjusted []byte, effort string) (*http.Response, bool) {
	if c.Request.Context().Err() != nil || c.Writer.Written() {
		return resp, false
	}
	c.Set(claudeParamHealDoneKey, true)
	recordClaudeAdjustment(c, info, resp, errorBody, claudeAdaptiveThinkingAdjustment)
	logger.LogWarn(c, fmt.Sprintf("request adjustment: upstream requires adaptive thinking; retrying once (channel #%d, model %s)", info.ChannelId, info.UpstreamModelName))
	retry, err := adaptor.DoRequest(c, info, bytes.NewReader(adjusted))
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("adaptive thinking adjustment failed, preserving original 400: %v", err))
		return resp, false
	}
	healed, ok := retry.(*http.Response)
	if !ok || healed == nil {
		return resp, false
	}
	if healed != resp && resp.Body != nil {
		_ = resp.Body.Close()
	}
	info.ReasoningEffort = effort
	// The replay may have waited long enough for a heartbeat to commit headers.
	// Join it before checking so a pending ping cannot race the header update.
	helper.StopStreamKeepAlive(c)
	if !c.Writer.Written() {
		c.Header("X-Request-Adjusted", claudeAdaptiveThinkingAdjustment)
	} else if value, exists := c.Get("request_failure_attempts"); exists {
		if attempts, ok := value.([]map[string]interface{}); ok && len(attempts) > 0 {
			attempt := attempts[len(attempts)-1]
			if attempt["adjustment"] == claudeAdaptiveThinkingAdjustment {
				attempt["adjustment_header_not_delivered"] = true
			}
		}
	}
	return healed, true
}
