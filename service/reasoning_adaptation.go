package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// These are explicitly scoped compatibility choices, not inferred universal
// GLM capabilities. Add models only after their actual endpoint is verified.
var zhipuGLM53ReasoningMap = map[string]string{
	"minimal": "low", "low": "low", "medium": "high", "high": "high", "xhigh": "max", "max": "max",
}

func reasoningRule(info *relaycommon.RelayInfo, modelName, protocol string) (map[string]string, string) {
	if info == nil || info.ChannelMeta == nil || protocol != "chat" ||
		info.ChannelType != constant.ChannelTypeOpenAI && info.ChannelType != constant.ChannelTypeAzure ||
		info.ChannelSetting.PassThroughBodyEnabled || model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return nil, ""
	}
	policy := info.ChannelSetting.ReasoningAdaptation
	// Runtime guards also cover old/manual database edits that bypass the API.
	if policy.Validate() != nil {
		return nil, ""
	}
	for _, rule := range policy.Rules {
		if rule.Model == modelName && rule.Protocol == protocol {
			return rule.Mapping, "channel_rule"
		}
	}
	if modelName != "glm-5.3" {
		return nil, ""
	}
	if policy.Mode == dto.ReasoningTemplateZhipuGLM53 || policy.Mode == "auto" && info.ChannelVendor == "zhipu" {
		return zhipuGLM53ReasoningMap, dto.ReasoningTemplateZhipuGLM53
	}
	return nil, ""
}

func recordReasoningAdaptation(info *relaycommon.RelayInfo, template, source, original, mapped string) {
	if !dto.IsReasoningEffort(original) && original != "disabled" {
		return
	}
	info.ReasoningAdaptation = &relaycommon.ReasoningAdaptationTrace{
		ChannelID: info.ChannelId, Template: template, Protocol: "chat", Source: source, Original: original, Mapped: mapped,
	}
	info.ReasoningEffort = mapped
}

func claudeReasoningForProtocol(request dto.ClaudeRequest, info *relaycommon.RelayInfo, protocol string) string {
	mapping, template := reasoningRule(info, request.Model, protocol)
	if mapping == nil {
		return inferClaudeReasoningEffort(request)
	}
	if request.Thinking != nil && request.Thinking.Type == "disabled" {
		// Do not manufacture an enabled effort. This does not invent an upstream
		// thinking switch or claim that omission disables the provider's default.
		recordReasoningAdaptation(info, template, "thinking", "disabled", "")
		return ""
	}
	original := claudeOutputConfigEffort(request.OutputConfig)
	source := "output_config"
	if original == "" {
		original = inferClaudeThinkingLevel(request.Thinking)
		source = "thinking_budget"
	}
	if mapped, ok := mapping[original]; ok {
		recordReasoningAdaptation(info, template, source, original, mapped)
		return mapped
	}
	return inferClaudeReasoningEffort(request)
}

func AdaptNativeChatReasoning(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil || info.RelayFormat != types.RelayFormatOpenAI {
		return
	}
	// Honor explicit off/none and absent values. Unknown values remain upstream's
	// responsibility; this adapter does not silently invent a supported level.
	if len(request.THINKING) > 0 && gjson.GetBytes(request.THINKING, "type").String() == "disabled" {
		return
	}
	mapping, template := reasoningRule(info, request.Model, "chat")
	if mapped, ok := mapping[request.ReasoningEffort]; ok {
		original := request.ReasoningEffort
		request.ReasoningEffort = mapped
		recordReasoningAdaptation(info, template, "reasoning_effort", original, mapped)
	}
}

// Capture after all explicit parameter overrides. No request-body copy is kept.
func CaptureFinalReasoningAdaptation(c *gin.Context, info *relaycommon.RelayInfo, payload []byte) {
	if info == nil || info.ReasoningAdaptation == nil {
		return
	}
	trace := *info.ReasoningAdaptation
	value := gjson.GetBytes(payload, "reasoning_effort")
	trace.FinalPresent = value.Exists()
	trace.Final = ""
	if value.Exists() {
		if dto.IsReasoningEffort(value.String()) || value.String() == "none" {
			trace.Final = value.String()
		} else {
			trace.Final = "other"
		}
	}
	trace.OverrideChanged = trace.Final != trace.Mapped || trace.FinalPresent != (trace.Mapped != "")
	info.ReasoningEffort = trace.Final
	info.ReasoningAdaptation = &trace
	if c != nil {
		common.SetContextKey(c, constant.ContextKeyReasoningAdaptation, trace)
	}
}

func AppendReasoningAdaptationAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil {
		return
	}
	if trace, ok := common.GetContextKeyType[relaycommon.ReasoningAdaptationTrace](c, constant.ContextKeyReasoningAdaptation); ok {
		adminInfo["reasoning_adaptation"] = trace
	}
}

func isReasoningEffortRejection(err *types.NewAPIError) bool {
	if err == nil || err.StatusCode != 400 {
		return false
	}
	message := strings.ToLower(err.ToOpenAIError().Message)
	return strings.Contains(message, "reasoning_effort") && (strings.Contains(message, "must be one of") || strings.Contains(message, "unsupported value") || strings.Contains(message, "invalid value"))
}
