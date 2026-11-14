package deepseek

import (
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekDeveloperRole(t *testing.T) {
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"deepseek-flash-none","messages":[{"role":"developer","content":"rules"},{"role":"user","content":"hello"},{"role":"developer","content":"more"},{"role":"assistant","content":"ok"}]}`), &request))
	_, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, &request)
	require.NoError(t, err)
	require.Equal(t, "system", request.Messages[0].Role)
	require.Equal(t, "user", request.Messages[1].Role)
	require.Equal(t, "system", request.Messages[2].Role)
	require.Equal(t, "assistant", request.Messages[3].Role)
	require.Equal(t, "deepseek-flash", request.Model)
}

func TestDeepSeekFinalNormalizationRespectsOverridesAndPassthrough(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	old := settings.PassThroughRequestEnabled
	t.Cleanup(func() { settings.PassThroughRequestEnabled = old })
	const body = `{"input":[{"type":"function_call","call_id":"x","name":"tool","arguments":"{}"}],"stream_options":{},"unknown":9007199254740993}`
	for _, tc := range []struct {
		name                                 string
		global, channel, none, wantReasoning bool
	}{
		{"normal", false, false, false, true},
		{"global passthrough", true, false, false, false},
		{"channel passthrough", false, true, false, false},
		{"override disables", false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings.PassThroughRequestEnabled = tc.global
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.deepseek.com"}}
			info.ChannelSetting.PassThroughBodyEnabled = tc.channel
			data := []byte(body)
			if tc.none {
				require.NoError(t, common.Unmarshal([]byte(`{"operations":[{"mode":"set","path":"reasoning.effort","value":"none"}]}`), &info.ParamOverride))
				var err error
				data, err = relaycommon.ApplyParamOverrideWithRelayInfo(data, info)
				require.NoError(t, err)
			}
			prepared, err := prepareResponsesRequestBody(strings.NewReader(string(data)), info)
			require.NoError(t, err)
			got, err := io.ReadAll(prepared)
			require.NoError(t, err)
			require.Equal(t, tc.wantReasoning, strings.Contains(string(got), `"reasoning_text"`))
			require.NotContains(t, string(got), "stream_options")
			require.Contains(t, string(got), "9007199254740993")
		})
	}
}

func TestDeepSeekRequestURL(t *testing.T) {
	for _, tc := range []struct {
		base, want string
		mode       int
		format     types.RelayFormat
	}{
		{"https://api.deepseek.com", "https://api.deepseek.com/responses", constant.RelayModeResponses, types.RelayFormatOpenAIResponses},
		{"https://api.deepseek.com/", "https://api.deepseek.com/responses", constant.RelayModeResponses, types.RelayFormatOpenAIResponses},
		{"https://proxy.example/v1/", "https://proxy.example/v1/responses", constant.RelayModeResponses, types.RelayFormatOpenAIResponses},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions", constant.RelayModeChatCompletions, types.RelayFormatOpenAI},
		{"https://api.deepseek.com", "https://api.deepseek.com/anthropic/v1/messages", constant.RelayModeChatCompletions, types.RelayFormatClaude},
		{"https://api.deepseek.com", "https://api.deepseek.com/beta/completions", constant.RelayModeCompletions, types.RelayFormatOpenAI},
	} {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: tc.base}, RelayMode: tc.mode, RelayFormat: tc.format}
		url, err := (&Adaptor{}).GetRequestURL(info)
		require.NoError(t, err)
		require.Equal(t, tc.want, url)
	}
}

func TestDeepSeekResponsesReasoning(t *testing.T) {
	for _, tc := range []struct{ model, explicit, wantModel, wantEffort string }{
		{"deepseek-flash", "", "deepseek-flash", ""},
		{"deepseek-flash", "none", "deepseek-flash", "none"},
		{"deepseek-flash", "low", "deepseek-flash", "low"},
		{"deepseek-flash", "high", "deepseek-flash", "high"},
		{"deepseek-flash", "max", "deepseek-flash", "max"},
		{"deepseek-flash-none", "high", "deepseek-flash", "none"},
		{"deepseek-flash-low", "max", "deepseek-flash", "low"},
		{"deepseek-v4-pro-low", "", "deepseek-v4-pro", "low"},
		{"deepseek-flash-max", "low", "deepseek-flash", "max"},
		{"deepseek-v4-pro-none", "", "deepseek-v4-pro", "none"},
		{"deepseek-v4-flash-max", "", "deepseek-v4-flash", "max"},
		{"custom-max", "low", "custom-max", "low"},
	} {
		t.Run(tc.model+"/"+tc.explicit, func(t *testing.T) {
			info := &relaycommon.RelayInfo{}
			request := dto.OpenAIResponsesRequest{Model: tc.model, Reasoning: &dto.Reasoning{Effort: tc.explicit}}
			converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
			require.NoError(t, err)
			got := converted.(*dto.OpenAIResponsesRequest)
			require.Equal(t, tc.wantModel, got.Model)
			require.Equal(t, tc.wantEffort, got.Reasoning.Effort)
			require.Equal(t, tc.wantEffort, info.ReasoningEffort)
			require.Equal(t, tc.explicit, request.Reasoning.Effort, "conversion must not mutate retry input")
		})
	}
}

func TestDeepSeekResponsesFinalPayload(t *testing.T) {
	body := `{"model":"deepseek-flash","input":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"fixture reasoning"}]},{"type":"function_call","call_id":"fc_1","name":"echo","arguments":"{}"},{"type":"function_call_output","call_id":"fc_1","output":"OK"}],"stream":true,"stream_options":{"include_usage":true},"opaque":9007199254740993,"previous_response_id":null}`
	prepared, err := prepareResponsesRequestBody(strings.NewReader(body), nil)
	require.NoError(t, err)
	got, err := io.ReadAll(prepared)
	require.NoError(t, err)
	require.JSONEq(t, strings.Replace(body, `,"stream_options":{"include_usage":true}`, "", 1), string(got))
	require.Contains(t, string(got), "9007199254740993")
	for _, field := range []string{`"previous_response_id":"resp_1"`, `"conversation":"conv_1"`, `"conversation":{"id":"conv_1"}`} {
		_, err := prepareResponsesRequestBody(strings.NewReader(`{"input":"OK",`+field+`}`), nil)
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, 400, apiErr.StatusCode)
		require.True(t, types.IsSkipRetryError(apiErr))
	}
	_, err = prepareResponsesRequestBody(strings.NewReader(`{"input":"OK"`), nil)
	require.Error(t, err)
}

func TestDeepSeekFlashSuffixKeepsChatAndClaudeSemantics(t *testing.T) {
	chat := &dto.GeneralOpenAIRequest{Model: "deepseek-flash-none"}
	_, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, chat)
	require.NoError(t, err)
	require.Equal(t, "deepseek-flash", chat.Model)
	require.JSONEq(t, `{"type":"disabled"}`, string(chat.THINKING))
	claude := &dto.ClaudeRequest{Model: "deepseek-flash-max"}
	require.NoError(t, applyDeepSeekV4ClaudeThinkingSuffix(nil, claude))
	require.Equal(t, "deepseek-flash", claude.Model)
	require.Equal(t, "enabled", claude.Thinking.Type)
	var output map[string]string
	require.NoError(t, common.Unmarshal(claude.OutputConfig, &output))
	require.Equal(t, "max", output["effort"])
}

func TestDeepSeekLowSuffixAndOutputConfigPreservation(t *testing.T) {
	for _, model := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro"} {
		t.Run(model, func(t *testing.T) {
			chat := &dto.GeneralOpenAIRequest{Model: model + "-low"}
			_, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, chat)
			require.NoError(t, err)
			require.Equal(t, model, chat.Model)
			require.Equal(t, "low", chat.ReasoningEffort)
			require.JSONEq(t, `{"type":"enabled"}`, string(chat.THINKING))
			for _, suffix := range []string{"low", "max", "none"} {
				original := `{"effort":"high","format":{"type":"json_schema","schema":{"z":9007199254740993,"a":true}},"verbosity":"low"}`
				request := &dto.ClaudeRequest{Model: model + "-" + suffix, OutputConfig: []byte(original)}
				require.NoError(t, applyDeepSeekV4ClaudeThinkingSuffix(nil, request))
				require.Equal(t, model, request.Model)
				require.Contains(t, string(request.OutputConfig), `"z":9007199254740993,"a":true`)
				require.Contains(t, string(request.OutputConfig), `"verbosity":"low"`)
				if suffix == "none" {
					require.NotContains(t, string(request.OutputConfig), `"effort"`)
					require.Equal(t, "disabled", request.Thinking.Type)
				} else {
					require.Contains(t, string(request.OutputConfig), `"effort":"`+suffix+`"`)
					require.Equal(t, "enabled", request.Thinking.Type)
				}
			}
		})
	}
	request := &dto.ClaudeRequest{Model: "deepseek-flash-none", OutputConfig: []byte(`{"effort":"high"}`)}
	require.NoError(t, applyDeepSeekV4ClaudeThinkingSuffix(nil, request))
	require.Nil(t, request.OutputConfig)
	request = &dto.ClaudeRequest{Model: "custom-low", OutputConfig: []byte(`{"effort":"high"}`)}
	require.NoError(t, applyDeepSeekV4ClaudeThinkingSuffix(nil, request))
	require.Equal(t, "custom-low", request.Model)
	require.JSONEq(t, `{"effort":"high"}`, string(request.OutputConfig))
}
