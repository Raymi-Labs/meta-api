package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestVertexMessagesRelay(t *testing.T) {
	const candidate = `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"key":"ok"}}}]},"finishReason":"STOP"}]}`
	const usage = `"usageMetadata":{"promptTokenCount":15,"candidatesTokenCount":5,"totalTokenCount":20}`
	for _, scenario := range []string{"message", "stream", "truncated", "rejected", "blocked", "native_claude"} {
		t.Run(scenario, func(t *testing.T) {
			stream := scenario == "stream" || scenario == "truncated"
			f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, _ []byte) {
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					if scenario == "truncated" {
						fmt.Fprint(w, "data: "+`{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`+"\n\n")
						return
					}
					fmt.Fprint(w, "data: "+candidate+"\n\ndata: {"+usage+"}\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch scenario {
				case "native_claude":
					fmt.Fprint(w, `{"type":"message","role":"assistant","content":[{"type":"text","text":"native"}],"stop_reason":"end_turn","usage":{"input_tokens":15,"output_tokens":5}}`)
				case "blocked":
					fmt.Fprint(w, `{"promptFeedback":{"blockReason":"SAFETY"}}`)
				case "rejected":
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"code":400,"message":"fixture rejection","status":"INVALID_ARGUMENT"}}`)
				default:
					fmt.Fprint(w, strings.TrimSuffix(candidate, "}")+","+usage+"}")
				}
			})
			target, err := url.Parse(*f.channel.BaseURL)
			require.NoError(t, err)
			client := service.GetHttpClient()
			previous := client.Transport
			client.Transport = deepSeekLocalHTTPTransport{target: target}
			t.Cleanup(func() { client.Transport = previous })
			f.channel.Type = constant.ChannelTypeVertexAi
			f.channel.Other = "global"
			f.channel.SetOtherSettings(dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey, VertexProjectID: "fixture"})
			f.channel.ModelMapping = common.GetPointer(`{"client-alias":"gemini-4-carbon"}`)
			if scenario == "native_claude" {
				f.channel.ModelMapping = common.GetPointer(`{"client-alias":"claude-opus-4-7"}`)
			}
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"client-alias":0}`))
			request := fmt.Sprintf(`{"model":"client-alias","stream":%t,"system":"keep it brief","max_tokens":256,"top_k":3,"output_config":{"effort":"high"},"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"key":{"type":"string"}}}}],"messages":[{"role":"user","content":"go"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"key":"before"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"found"}]}]}`, stream)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(request))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("specific_channel_id", f.channel.Id)
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, "client-alias"))
			defer common.CleanupBodyStorage(c)
			Relay(c, types.RelayFormatClaude)
			require.Len(t, f.requests, 1, w.Body.String())
			sent := <-f.requests
			if scenario == "native_claude" {
				require.Contains(t, sent.path, "/publishers/anthropic/models/claude-opus-4-7:rawPredict")
				require.True(t, gjson.GetBytes(sent.body, "anthropic_version").Exists())
				require.Equal(t, "native", gjson.Get(w.Body.String(), "content.0.text").String())
				return
			}
			method := "generateContent"
			if stream {
				method = "streamGenerateContent"
			}
			require.Equal(t, "/v1/projects/fixture/locations/global/publishers/google/models/gemini-4-carbon:"+method, sent.path)
			wire := gjson.ParseBytes(sent.body)
			require.Equal(t, "go", wire.Get("contents.0.parts.0.text").String())
			require.Equal(t, "lookup", wire.Get("contents.1.parts.0.functionCall.name").String())
			require.Equal(t, "lookup", wire.Get("contents.2.parts.0.functionResponse.name").String())
			require.Contains(t, wire.Get("contents.2.parts.0.functionResponse.response").String(), "found")
			require.Equal(t, "keep it brief", wire.Get("systemInstruction.parts.0.text").String())
			require.Equal(t, "lookup", wire.Get("tools.0.functionDeclarations.0.name").String())
			require.EqualValues(t, 256, wire.Get("generationConfig.maxOutputTokens").Int())
			require.EqualValues(t, 3, wire.Get("generationConfig.topK").Int())
			require.Equal(t, "high", wire.Get("generationConfig.thinkingConfig.thinkingLevel").String())
			for _, key := range []string{"messages", "system", "anthropic_version", "stream", "max_tokens", "output_config"} {
				require.False(t, wire.Get(key).Exists(), key)
			}
			reply := w.Body.String()
			switch scenario {
			case "rejected", "blocked":
				require.Equal(t, 400, w.Code)
				require.Equal(t, "error", gjson.Get(reply, "type").String())
			case "truncated":
				require.Contains(t, reply, "partial")
				require.Contains(t, reply, "event: error")
				require.NotContains(t, reply, "message_stop")
			case "stream":
				require.Equal(t, 200, w.Code)
				for _, fragment := range []string{"event: message_start", "input_json_delta", `"name":"lookup"`, `"stop_reason":"tool_use"`, `"output_tokens":5`, "event: message_stop"} {
					require.Contains(t, reply, fragment)
				}
				require.Equal(t, 1, strings.Count(reply, "event: message_stop"))
				require.NotContains(t, reply, "[DONE]")
			default:
				require.Equal(t, 200, w.Code)
				require.Equal(t, "message", gjson.Get(reply, "type").String())
				require.Equal(t, "tool_use", gjson.Get(reply, "stop_reason").String())
				require.Equal(t, "ok", gjson.Get(reply, "content.0.input.key").String())
				require.EqualValues(t, 5, gjson.Get(reply, "usage.output_tokens").Int())
			}
		})
	}
}
