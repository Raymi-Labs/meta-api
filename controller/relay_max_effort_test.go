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

// Use actual relay entrypoints and an isolated HTTP upstream. No paid API calls.
func TestRelayMaxEffortAcrossProtocols(t *testing.T) {
	for _, route := range []struct{ in, out string }{
		{"messages", "chat"}, {"messages", "responses"}, {"chat", "chat"}, {"chat", "responses"}, {"responses", "responses"},
	} {
		for _, stream := range []bool{false, true} {
			for _, scenario := range []string{"max", "mapped", "xhigh", "legacy", "wrong_protocol", "override", "delete", "omitted", "failure", "passthrough"} {
				if scenario == "passthrough" && (route.in == "messages" || route.in != route.out) {
					continue
				}
				t.Run(fmt.Sprintf("%s-to-%s/%s/stream=%v", route.in, route.out, scenario, stream), func(t *testing.T) {
					f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) {
						if scenario == "failure" {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(400)
							fmt.Fprint(w, `{"error":{"type":"invalid_parameter_error","message":"reasoning_effort must be one of: low, high, xhigh"}}`)
							return
						}
						if route.out == "responses" {
							writeDeepSeekResponse(w, body, false)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, "data: "+`{"id":"chat_fixture","model":"gpt-6-astra","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"},"finish_reason":null}]}`+"\n\n")
							w.(http.Flusher).Flush()
							fmt.Fprint(w, "data: "+`{"id":"chat_fixture","model":"gpt-6-astra","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`+"\n\ndata: [DONE]\n\n")
						} else {
							fmt.Fprint(w, `{"id":"chat_fixture","model":"gpt-6-astra","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
						}
					})
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"company-astra":0}`))
					f.channel.Type = constant.ChannelTypeOpenAI
					f.channel.Models = "company-astra"
					f.channel.ModelMapping = common.GetPointer(`{"company-astra":"gpt-6-astra"}`)
					mode := dto.ChatCompletionsToResponsesModeDisabled
					if route.out == "responses" {
						mode = dto.ChatCompletionsToResponsesModeEnabled
					}
					rules := []dto.ReasoningAdaptationRule{{Model: "gpt-6-astra", Protocol: route.out, Mapping: map[string]string{"max": "max"}}}
					if scenario == "mapped" {
						rules[0].Mapping = map[string]string{"max": "high", "high": "low"}
					}
					if scenario == "legacy" {
						rules = nil
					}
					if scenario == "wrong_protocol" {
						if route.out == "chat" {
							rules[0].Protocol = "responses"
						} else {
							rules[0].Protocol = "chat"
						}
					}
					if scenario == "passthrough" {
						rules[0].Mapping["max"] = "low"
					}
					f.channel.SetSetting(dto.ChannelSettings{ReasoningAdaptation: dto.ReasoningAdaptation{Mode: "inherit", Rules: rules}, ChatCompletionsToResponsesMode: mode, PassThroughBodyEnabled: scenario == "passthrough"})
					overrideField := "reasoning_effort"
					if route.in == "responses" {
						overrideField = "reasoning.effort"
					}
					if scenario == "override" {
						f.channel.ParamOverride = common.GetPointer(fmt.Sprintf(`{"operations":[{"path":%q,"mode":"set","value":"high"}]}`, overrideField))
					}
					if scenario == "delete" {
						f.channel.ParamOverride = common.GetPointer(fmt.Sprintf(`{"operations":[{"path":%q,"mode":"delete"}]}`, overrideField))
					}
					effort := "max"
					if scenario == "xhigh" {
						effort = "xhigh"
					}
					payload := map[string]interface{}{"model": "company-astra", "stream": stream, "max_tokens": 256, "messages": []interface{}{map[string]interface{}{"role": "user", "content": "Reply OK"}}}
					endpoint := "/v1/messages"
					format := types.RelayFormat(types.RelayFormatClaude)
					switch route.in {
					case "messages":
						payload["output_config"] = map[string]interface{}{"effort": effort}
					case "chat":
						endpoint = "/v1/chat/completions"
						format = types.RelayFormatOpenAI
						payload["reasoning_effort"] = effort
					case "responses":
						endpoint = "/v1/responses"
						format = types.RelayFormatOpenAIResponses
						delete(payload, "messages")
						delete(payload, "max_tokens")
						payload["input"] = "Reply OK"
						payload["reasoning"] = map[string]interface{}{"effort": effort}
					}
					if scenario == "omitted" {
						delete(payload, "output_config")
						delete(payload, "reasoning_effort")
						delete(payload, "reasoning")
					}
					body, err := common.Marshal(payload)
					require.NoError(t, err)
					w := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(w)
					c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
					c.Request.Header.Set("Content-Type", "application/json")
					c.Set("specific_channel_id", f.channel.Id)
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, "company-astra"))
					defer common.CleanupBodyStorage(c)
					Relay(c, format)
					if scenario == "failure" {
						require.Equal(t, 400, w.Code, w.Body.String())
					} else {
						require.Equal(t, 200, w.Code, w.Body.String())
						require.Contains(t, w.Body.String(), "OK")
					}
					want := effort
					if route.in == "messages" && (scenario == "legacy" || scenario == "wrong_protocol") {
						want = "xhigh"
					}
					if scenario == "override" || scenario == "mapped" {
						want = "high"
					}
					if scenario == "delete" || scenario == "omitted" {
						want = ""
					}
					select {
					case request := <-f.requests:
						path, field := "/v1/chat/completions", "reasoning_effort"
						if route.out == "responses" {
							path, field = "/v1/responses", "reasoning.effort"
						}
						require.Equal(t, path, request.path)
						require.Equal(t, want, gjson.GetBytes(request.body, field).String(), string(request.body))
						if scenario == "delete" || scenario == "omitted" {
							require.False(t, gjson.GetBytes(request.body, field).Exists())
						}
					case <-time.After(time.Second):
						t.Fatal("no request reached HTTP fixture")
					}
					require.Empty(t, f.requests, "effort adaptation must not replay requests")
					if scenario == "max" || scenario == "mapped" || scenario == "override" || scenario == "delete" || scenario == "failure" {
						value, exists := c.Get(string(constant.ContextKeyReasoningAdaptation))
						require.True(t, exists)
						trace, err := common.Marshal(value)
						require.NoError(t, err)
						require.Equal(t, route.out, gjson.GetBytes(trace, "protocol").String())
						require.Equal(t, "max", gjson.GetBytes(trace, "original").String())
						require.Equal(t, want, gjson.GetBytes(trace, "final").String())
						require.Equal(t, scenario == "override" || scenario == "delete", gjson.GetBytes(trace, "override_changed").Bool())
					}
				})
			}
		}
	}
}
