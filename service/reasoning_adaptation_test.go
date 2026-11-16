package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func reasoningInfo(mode, vendor string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, OriginModelName: "public-alias", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelId: 1549, ChannelVendor: vendor, ChannelSetting: dto.ChannelSettings{ReasoningAdaptation: dto.ReasoningAdaptation{Mode: mode}}}}
}

func TestReasoningProfilesUseOriginalClaudeEffort(t *testing.T) {
	setConvertReasoningTestOptions(t, map[string]string{claudeToOpenAIReasoningMapOption: `{"low":"medium","medium":"high","high":"xhigh","max":"xhigh"}`})
	for _, tc := range []struct{ level, want string }{{"low", "low"}, {"medium", "high"}, {"high", "high"}, {"max", "max"}, {"xhigh", "max"}} {
		info := reasoningInfo("auto", "zhipu")
		req := dto.ClaudeRequest{Model: "glm-5.3", OutputConfig: []byte(`{"effort":"` + tc.level + `"}`)}
		out, err := ClaudeToOpenAIRequest(nil, req, info)
		require.NoError(t, err)
		require.Equal(t, tc.want, out.ReasoningEffort)
		require.Equal(t, tc.level, info.ReasoningAdaptation.Original)
	}
	for _, tc := range []struct {
		budget int
		want   string
	}{{1024, "low"}, {2048, "high"}, {4096, "high"}, {8192, "max"}} {
		req := dto.ClaudeRequest{Model: "glm-5.3", Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(tc.budget)}}
		out, err := ClaudeToOpenAIRequest(nil, req, reasoningInfo("auto", "zhipu"))
		require.NoError(t, err)
		require.Equal(t, tc.want, out.ReasoningEffort)
	}
}

func TestReasoningProfileScopeAndPrecedence(t *testing.T) {
	setConvertReasoningTestOptions(t, map[string]string{claudeToOpenAIReasoningMapOption: `{"low":"medium","medium":"high","high":"xhigh","max":"xhigh"}`})
	for _, tc := range []struct{ mode, vendor, model, protocol, want string }{
		{"", "zhipu", "glm-5.3", "chat", "xhigh"},
		{"inherit", "zhipu", "glm-5.3", "chat", "xhigh"},
		{"auto", "", "glm-5.3", "chat", "xhigh"},
		{"auto", "openai", "glm-5.3", "chat", "xhigh"},
		{"auto", "zhipu", "glm-5.3-flash", "chat", "xhigh"},
		{"auto", "zhipu", "glm-5.3-flashx", "chat", "xhigh"},
		{"auto", "zhipu", "glm-5.3-other", "chat", "xhigh"},
		{"auto", "zhipu", "glm-5.3", "responses", "xhigh"},
		{dto.ReasoningTemplateZhipuGLM53, "", "glm-5.3", "chat", "max"},
	} {
		info := reasoningInfo(tc.mode, tc.vendor)
		out, err := ClaudeToOpenAIRequestForProtocol(nil, dto.ClaudeRequest{Model: tc.model, OutputConfig: []byte(`{"effort":"max"}`)}, info, tc.protocol)
		require.NoError(t, err)
		require.Equal(t, tc.want, out.ReasoningEffort, "%+v", tc)
	}
	info := reasoningInfo("auto", "zhipu")
	info.ChannelSetting.ReasoningAdaptation.Rules = []dto.ReasoningAdaptationRule{{Model: "glm-5.3", Protocol: "chat", Mapping: map[string]string{"max": "low"}}}
	out, err := ClaudeToOpenAIRequest(nil, dto.ClaudeRequest{Model: "glm-5.3", OutputConfig: []byte(`{"effort":"max"}`)}, info)
	require.NoError(t, err)
	require.Equal(t, "low", out.ReasoningEffort)
	require.Equal(t, "channel_rule", info.ReasoningAdaptation.Template)
	// Sparse exact rules own the model: unmapped levels retain legacy behavior,
	// they do not silently fall through to a different provider template.
	require.Equal(t, "xhigh", claudeReasoningForProtocol(dto.ClaudeRequest{Model: "glm-5.3", OutputConfig: []byte(`{"effort":"high"}`)}, info, "chat"))
	info.ChannelSetting.PassThroughBodyEnabled = true
	require.Equal(t, "xhigh", claudeReasoningForProtocol(dto.ClaudeRequest{Model: "glm-5.3", OutputConfig: []byte(`{"effort":"max"}`)}, info, "chat"))
}

func TestReasoningProfileOmittedDisabledAndNativeChat(t *testing.T) {
	info := reasoningInfo("auto", "zhipu")
	out, err := ClaudeToOpenAIRequest(nil, dto.ClaudeRequest{Model: "glm-5.3"}, info)
	require.NoError(t, err)
	require.Empty(t, out.ReasoningEffort)
	require.Nil(t, info.ReasoningAdaptation)
	out, err = ClaudeToOpenAIRequest(nil, dto.ClaudeRequest{Model: "glm-5.3", Thinking: &dto.Thinking{Type: "disabled"}, OutputConfig: []byte(`{"effort":"max"}`)}, info)
	require.NoError(t, err)
	require.Empty(t, out.ReasoningEffort)
	for _, tc := range []struct{ effort, want string }{{"", ""}, {"none", "none"}, {"low", "low"}, {"high", "high"}, {"medium", "high"}, {"xhigh", "max"}, {"max", "max"}, {"unknown", "unknown"}} {
		info := reasoningInfo("auto", "zhipu")
		info.RelayFormat = types.RelayFormatOpenAI
		req := &dto.GeneralOpenAIRequest{Model: "glm-5.3", ReasoningEffort: tc.effort}
		AdaptNativeChatReasoning(info, req)
		require.Equal(t, tc.want, req.ReasoningEffort)
	}
}

func TestReasoningDiagnosticsUseFinalOverrideAndResetPerAttempt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := reasoningInfo("auto", "zhipu")
	_, err := ClaudeToOpenAIRequest(c, dto.ClaudeRequest{Model: "glm-5.3", OutputConfig: []byte(`{"effort":"max"}`)}, info)
	require.NoError(t, err)
	CaptureFinalReasoningAdaptation(c, info, []byte(`{"reasoning_effort":"high"}`))
	admin := map[string]interface{}{}
	AppendReasoningAdaptationAdminInfo(c, admin)
	trace := admin["reasoning_adaptation"].(relaycommon.ReasoningAdaptationTrace)
	require.Equal(t, "max", trace.Original)
	require.Equal(t, "max", trace.Mapped)
	require.Equal(t, "high", trace.Final)
	require.True(t, trace.OverrideChanged)
	require.Equal(t, "high", info.ReasoningEffort)
	CaptureFinalReasoningAdaptation(c, info, []byte(`{}`))
	require.Empty(t, info.ReasoningEffort)
	require.False(t, info.ReasoningAdaptation.FinalPresent)
	info.InitChannelMeta(c)
	admin = map[string]interface{}{}
	AppendReasoningAdaptationAdminInfo(c, admin)
	require.Empty(t, admin)
	require.Nil(t, info.ReasoningAdaptation)
}

func TestReasoningRejectionDiagnostics(t *testing.T) {
	c := failureLogContext(t)
	info := reasoningInfo("auto", "zhipu")
	_, err := ClaudeToOpenAIRequest(c, dto.ClaudeRequest{Model: "glm-5.3", OutputConfig: []byte(`{"effort":"max"}`)}, info)
	require.NoError(t, err)
	CaptureFinalReasoningAdaptation(c, info, []byte(`{"reasoning_effort":"max"}`))
	apiErr := types.WithOpenAIError(types.OpenAIError{Type: "invalid_parameter_error", Code: "invalid_parameter_error", Message: "'reasoning_effort' must be one of: 'low', 'high', 'max'"}, 400)
	category, _, _ := describeRequestFailure(apiErr, true)
	require.Equal(t, "reasoning_parameter", category)
	require.False(t, ShouldRetryChannelError(c, apiErr, 1))
	AppendRequestFailureAttempt(c, *types.NewChannelError(1549, 1, "fixture", false, "", false), apiErr)
	MarkRequestFailure(c, apiErr)
	RecordFinalRequestFailure(c, time.Second)
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-stream-failure").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Zero(t, rows[0].Quota)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(rows[0].Other, &other))
	admin := other["admin_info"].(map[string]interface{})
	require.Equal(t, "max", admin["reasoning_adaptation"].(map[string]interface{})["final"])
	require.Contains(t, admin["attempts"].([]interface{})[0], "reasoning_adaptation")
	otherErr := types.WithOpenAIError(types.OpenAIError{Message: "invalid unrelated request"}, 400)
	category, _, _ = describeRequestFailure(otherErr, true)
	require.Equal(t, "invalid_request", category)
}

func TestMessagesMaxResponsesRule(t *testing.T) {
	info := reasoningInfo("inherit", "")
	info.ChannelSetting.ReasoningAdaptation.Rules = []dto.ReasoningAdaptationRule{{Model: "gpt-6-astra", Protocol: "responses", Mapping: map[string]string{"max": "max"}}}
	req := dto.ClaudeRequest{Model: "gpt-6-astra", OutputConfig: []byte(`{"effort":"max"}`)}
	out, err := ClaudeToOpenAIRequestForProtocol(nil, req, info, "responses")
	require.NoError(t, err)
	require.Equal(t, "max", out.ReasoningEffort)
	require.Equal(t, "max", info.ReasoningEffort)
	require.NotNil(t, info.ReasoningAdaptation)
	require.Equal(t, "responses", info.ReasoningAdaptation.Protocol)
}

func TestGlobalReasoningMappingAllowsMax(t *testing.T) {
	setConvertReasoningTestOptions(t, map[string]string{claudeToOpenAIReasoningMapOption: `{"low":"low","medium":"medium","high":"high","max":"max"}`})
	require.Equal(t, "max", getClaudeToOpenAIReasoningMap()["max"])
}

func TestMessagesMaxRuleBoundariesAndPublicLog(t *testing.T) {
	setConvertReasoningTestOptions(t, map[string]string{claudeToOpenAIReasoningMapOption: ""})
	for _, protocol := range []string{"chat", "responses"} {
		for _, tc := range []struct {
			name, effort, thinking, want, source string
			budget                               int
			rule                                 bool
		}{
			{name: "explicit", effort: "max", want: "max", source: "output_config", rule: true},
			{name: "xhigh_not_promoted", effort: "xhigh", want: "xhigh", rule: true},
			{name: "unknown", effort: "ultra", want: "", rule: true},
			{name: "omitted", want: "", rule: true},
			{name: "disabled", effort: "max", thinking: "disabled", want: "", source: "thinking", rule: true},
			{name: "disabled_without_rule", effort: "max", thinking: "disabled", want: ""},
			{name: "large_budget_legacy", thinking: "enabled", budget: 8192, want: "xhigh"},
			{name: "large_budget_explicit_rule", thinking: "enabled", budget: 8192, want: "max", source: "thinking_budget", rule: true},
			{name: "unknown_budget", thinking: "enabled", want: "medium", rule: true},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				info := reasoningInfo("inherit", "")
				if tc.rule {
					info.ChannelSetting.ReasoningAdaptation.Rules = []dto.ReasoningAdaptationRule{{Model: "gpt-6-astra", Protocol: protocol, Mapping: map[string]string{"max": "max"}}}
				}
				req := dto.ClaudeRequest{Model: "gpt-6-astra"}
				if tc.effort != "" {
					req.OutputConfig = []byte(`{"effort":"` + tc.effort + `"}`)
				}
				if tc.thinking != "" {
					req.Thinking = &dto.Thinking{Type: tc.thinking, BudgetTokens: common.GetPointer(tc.budget)}
				}
				out, err := ClaudeToOpenAIRequestForProtocol(nil, req, info, protocol)
				require.NoError(t, err)
				require.Equal(t, tc.want, out.ReasoningEffort)
				if tc.source != "" {
					require.NotNil(t, info.ReasoningAdaptation)
					require.Equal(t, tc.source, info.ReasoningAdaptation.Source)
				} else {
					require.Nil(t, info.ReasoningAdaptation)
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
				if tc.want == "" {
					require.NotContains(t, other, "reasoning_effort")
				} else {
					require.Equal(t, tc.want, other["reasoning_effort"])
				}
			})
		}
	}
}

func TestResponsesRuleProtocolIsolationAndFallbackTrace(t *testing.T) {
	setConvertReasoningTestOptions(t, map[string]string{claudeToOpenAIReasoningMapOption: ""})
	info := reasoningInfo("inherit", "")
	info.ChannelSetting.ReasoningAdaptation.Rules = []dto.ReasoningAdaptationRule{{Model: "gpt-6-astra", Protocol: "responses", Mapping: map[string]string{"max": "max"}}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := dto.ClaudeRequest{Model: "gpt-6-astra", OutputConfig: []byte(`{"effort":"max"}`)}
	_, err := ClaudeToOpenAIRequestForProtocol(c, req, info, "responses")
	require.NoError(t, err)
	CaptureFinalReasoningAdaptation(c, info, []byte(`{"reasoning":{"effort":"max"}}`))
	out, err := ClaudeToOpenAIRequestForProtocol(c, req, info, "chat")
	require.NoError(t, err)
	require.Equal(t, "xhigh", out.ReasoningEffort)
	CaptureFinalReasoningAdaptation(c, info, []byte(`{"reasoning_effort":"xhigh"}`))
	require.Nil(t, info.ReasoningAdaptation)
	admin := map[string]interface{}{}
	AppendReasoningAdaptationAdminInfo(c, admin)
	require.Empty(t, admin)
	// A rule for another exact model cannot change this model.
	req.Model = "gpt-6-astra-other"
	out, err = ClaudeToOpenAIRequestForProtocol(c, req, info, "responses")
	require.NoError(t, err)
	require.Equal(t, "xhigh", out.ReasoningEffort)
	require.Nil(t, info.ReasoningAdaptation)
}
