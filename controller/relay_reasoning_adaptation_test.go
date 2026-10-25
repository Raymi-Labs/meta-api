package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRelayReasoningAdaptationFinalHTTPPayload(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, scenario := range []string{"messages", "chat", "override", "failure", "passthrough", "tool_history"} {
			t.Run(fmt.Sprintf("%s/stream=%v", scenario, stream), func(t *testing.T) {
				f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) {
					if scenario == "failure" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(400)
						fmt.Fprint(w, `{"error":{"type":"invalid_parameter_error","message":"'reasoning_effort' must be one of: 'low', 'high', 'max'"}}`)
						return
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: "+`{"id":"chat_fixture","model":"glm-5.3","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"},"finish_reason":null}]}`+"\n\n")
						w.(http.Flusher).Flush()
						fmt.Fprint(w, "data: "+`{"id":"chat_fixture","model":"glm-5.3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`+"\n\ndata: [DONE]\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, `{"id":"chat_fixture","model":"glm-5.3","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
					}
				})
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"company-glm":0}`))
				f.channel.Type = constant.ChannelTypeOpenAI
				f.channel.Models = "company-glm"
				f.channel.ModelMapping = common.GetPointer(`{"company-glm":"glm-5.3"}`)
				f.channel.SetSetting(dto.ChannelSettings{ReasoningAdaptation: dto.ReasoningAdaptation{Mode: "auto"}, ChatCompletionsToResponsesMode: dto.ChatCompletionsToResponsesModeDisabled, PassThroughBodyEnabled: scenario == "passthrough"})
				if scenario == "override" {
					f.channel.ParamOverride = common.GetPointer(`{"reasoning_effort":"low"}`)
				}
				format := types.RelayFormat(types.RelayFormatClaude)
				endpoint := "/v1/messages"
				body := fmt.Sprintf(`{"model":"company-glm","stream":%v,"max_tokens":256,"output_config":{"effort":"max"},"messages":[{"role":"user","content":"Reply OK"}]}`, stream)
				expected := "max"
				if scenario == "chat" || scenario == "passthrough" {
					format = types.RelayFormatOpenAI
					endpoint = "/v1/chat/completions"
					body = fmt.Sprintf(`{"model":"company-glm","stream":%v,"max_tokens":256,"reasoning_effort":"xhigh","messages":[{"role":"user","content":"Reply OK"}]}`, stream)
				}
				if scenario == "passthrough" {
					expected = "xhigh"
				}
				if scenario == "override" {
					expected = "low"
				}
				if scenario == "tool_history" {
					body = fmt.Sprintf(`{"model":"company-glm","stream":%v,"max_tokens":256,"output_config":{"effort":"high"},"tools":[{"name":"echo","input_schema":{"type":"object","properties":{"value":{"type":"string"}}}}],"messages":[{"role":"user","content":"Echo"},{"role":"assistant","content":[{"type":"tool_use","id":"call_fixture","name":"echo","input":{"value":"OK"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_fixture","content":"OK"}]}]}`, stream)
					expected = "high"
				}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set("specific_channel_id", f.channel.Id)
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, "company-glm"))
				defer common.CleanupBodyStorage(c)
				Relay(c, format)
				if scenario == "failure" {
					require.Equal(t, 400, w.Code, w.Body.String())
				} else {
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), "OK")
				}
				select {
				case request := <-f.requests:
					require.Equal(t, "/v1/chat/completions", request.path)
					require.Equal(t, expected, gjson.GetBytes(request.body, "reasoning_effort").String(), string(request.body))
					if scenario == "passthrough" {
						require.JSONEq(t, body, string(request.body))
					} else {
						require.Equal(t, "glm-5.3", gjson.GetBytes(request.body, "model").String())
					}
					if scenario == "tool_history" {
						require.Equal(t, "echo", gjson.GetBytes(request.body, "tools.0.function.name").String())
						require.Equal(t, "call_fixture", gjson.GetBytes(request.body, "messages.1.tool_calls.0.id").String())
						require.Equal(t, "call_fixture", gjson.GetBytes(request.body, "messages.2.tool_call_id").String())
					}
				case <-time.After(time.Second):
					t.Fatal("no HTTP request received")
				}
				require.Empty(t, f.requests, "parameter adaptation must not replay requests")
				if scenario != "passthrough" {
					value, exists := c.Get(string(constant.ContextKeyReasoningAdaptation))
					require.True(t, exists)
					encoded, err := common.Marshal(value)
					require.NoError(t, err)
					require.Equal(t, expected, gjson.GetBytes(encoded, "final").String())
					require.Equal(t, scenario == "override", gjson.GetBytes(encoded, "override_changed").Bool())
				}
			})
		}
	}
}

func TestRelayReasoningAdaptationSkipsResponsesBridge(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) { writeDeepSeekResponse(w, body, false) })
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"glm-5.3":0}`))
			f.channel.Type = constant.ChannelTypeOpenAI
			f.channel.Models = "glm-5.3"
			f.channel.SetSetting(dto.ChannelSettings{ReasoningAdaptation: dto.ReasoningAdaptation{Mode: "auto"}, ChatCompletionsToResponsesMode: dto.ChatCompletionsToResponsesModeEnabled})
			endpoint := "/v1/messages"
			body := `{"model":"glm-5.3","max_tokens":256,"output_config":{"effort":"max"},"messages":[{"role":"user","content":"Reply OK"}]}`
			if format == types.RelayFormatOpenAI {
				endpoint = "/v1/chat/completions"
				body = `{"model":"glm-5.3","max_tokens":256,"reasoning_effort":"xhigh","messages":[{"role":"user","content":"Reply OK"}]}`
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("specific_channel_id", f.channel.Id)
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, "glm-5.3"))
			defer common.CleanupBodyStorage(c)
			Relay(c, format)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			select {
			case request := <-f.requests:
				require.Equal(t, "/v1/responses", request.path)
				require.Equal(t, "xhigh", gjson.GetBytes(request.body, "reasoning.effort").String(), string(request.body))
			case <-time.After(time.Second):
				t.Fatal("no Responses request received")
			}
			trace, _ := c.Get(string(constant.ContextKeyReasoningAdaptation))
			require.Nil(t, trace)
			require.Empty(t, f.requests)
		})
	}
}
