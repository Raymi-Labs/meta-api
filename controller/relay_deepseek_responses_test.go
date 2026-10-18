package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type deepSeekCapturedRequest struct {
	path, auth string
	body       []byte
}

type deepSeekRelayFixture struct {
	channel  *model.Channel
	requests chan deepSeekCapturedRequest
}

func newDeepSeekRelayFixture(t *testing.T, handler func(http.ResponseWriter, []byte)) *deepSeekRelayFixture {
	t.Helper()
	service.InitTokenEncoders()
	db := useControllerCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.ContentSafetyViolation{}))
	oldRedis, oldCount, oldRetry, oldLog := common.RedisEnabled, constant.CountToken, common.RetryTimes, constant.ErrorLogEnabled
	common.RedisEnabled, constant.CountToken, common.RetryTimes, constant.ErrorLogEnabled = false, false, 3, false
	t.Cleanup(func() {
		common.RedisEnabled, constant.CountToken, common.RetryTimes, constant.ErrorLogEnabled = oldRedis, oldCount, oldRetry, oldLog
	})
	oldRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"deepseek-flash":0,"company-deepseek":0,"deepseek-flash-max":0}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	quota := operation_setting.GetQuotaSetting()
	oldFree := quota.EnableFreeModelPreConsume
	quota.EnableFreeModelPreConsume = false
	t.Cleanup(func() { quota.EnableFreeModelPreConsume = oldFree })
	general := operation_setting.GetGeneralSetting()
	oldPing := general.PingIntervalEnabled
	general.PingIntervalEnabled = false
	t.Cleanup(func() { general.PingIntervalEnabled = oldPing })
	global := model_setting.GetGlobalSettings()
	oldPass := global.PassThroughRequestEnabled
	global.PassThroughRequestEnabled = false
	t.Cleanup(func() { global.PassThroughRequestEnabled = oldPass })
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	fixture := &deepSeekRelayFixture{requests: make(chan deepSeekCapturedRequest, 16)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		fixture.requests <- deepSeekCapturedRequest{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
		handler(w, body)
	}))
	t.Cleanup(server.Close)
	fixture.channel = &model.Channel{Id: 997801, Name: "deepseek-responses-fixture", Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Key: "deepseek-fixture-key", BaseURL: &server.URL, AutoBan: common.GetPointer(0)}
	return fixture
}

func (f *deepSeekRelayFixture) relay(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalJsonStr(body, &request))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("specific_channel_id", f.channel.Id)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, request.Model))
	Relay(c, types.RelayFormatOpenAIResponses)
	return w
}

func (f *deepSeekRelayFixture) take(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	select {
	case request := <-f.requests:
		require.Equal(t, "/responses", request.path)
		require.Equal(t, "Bearer deepseek-fixture-key", request.auth)
		var body map[string]json.RawMessage
		require.NoError(t, common.Unmarshal(request.body, &body))
		require.NotContains(t, body, "stream_options")
		return body
	case <-time.After(5 * time.Second):
		t.Fatal("no request reached the HTTP upstream")
		return nil
	}
}

const deepSeekMessageOutput = `[{"type":"message","id":"msg_fixture","role":"assistant","status":"completed","content":[{"type":"output_text","text":"OK"}]}]`
const deepSeekToolOutput = `[{"type":"reasoning","id":"rs_fixture","content":[{"type":"reasoning_text","text":"fixture reasoning"}]},{"type":"function_call","id":"fc_fixture","call_id":"call_fixture","name":"echo","arguments":"{\"value\":\"OK\"}","status":"completed"}]`

// This fixture uses the documented DeepSeek protocol (no [DONE], raw reasoning
// content, terminal usage). It is not a live provider availability test.
func writeDeepSeekResponse(w http.ResponseWriter, body []byte, tool bool) {
	var req struct{ Stream bool }
	_ = common.Unmarshal(body, &req)
	output := deepSeekMessageOutput
	if tool {
		output = deepSeekToolOutput
	}
	response := `{"id":"resp_fixture","object":"response","status":"completed","model":"deepseek-flash","store":false,"output":` + output + `,"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}}`
	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, "event: response.created\ndata: "+`{"type":"response.created","sequence_number":0,"response":{"id":"resp_fixture","status":"in_progress"}}`+"\n\n")
	if tool {
		fmt.Fprint(w, "event: response.reasoning_text.delta\ndata: "+`{"type":"response.reasoning_text.delta","sequence_number":1,"item_id":"rs_fixture","delta":"fixture reasoning"}`+"\n\n")
		fmt.Fprint(w, "event: response.function_call_arguments.delta\ndata: "+`{"type":"response.function_call_arguments.delta","sequence_number":2,"item_id":"fc_fixture","delta":"{\"value\":\"OK\"}"}`+"\n\n")
	} else {
		fmt.Fprint(w, "event: response.output_text.delta\ndata: "+`{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_fixture","delta":"OK"}`+"\n\n")
	}
	w.(http.Flusher).Flush()
	fmt.Fprint(w, "event: response.completed\ndata: "+`{"type":"response.completed","sequence_number":3,"response":`+response+"}\n\n")
}

func deepSeekClientOutput(t *testing.T, w *httptest.ResponseRecorder, stream bool) []json.RawMessage {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Output []json.RawMessage
		Usage  json.RawMessage
	}
	if !stream {
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	} else {
		require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
		require.NotContains(t, w.Body.String(), "response.failed")
		completed := 0
		for _, line := range strings.Split(w.Body.String(), "\n") {
			if !strings.HasPrefix(line, "data: {") {
				continue
			}
			var event struct {
				Type     string
				Response json.RawMessage
			}
			require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
			if event.Type == "response.completed" {
				completed++
				require.NoError(t, common.Unmarshal(event.Response, &response))
			}
		}
		require.Equal(t, 1, completed)
	}
	require.JSONEq(t, `{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}`, string(response.Usage))
	return response.Output
}

func TestRelayDeepSeekResponsesNative(t *testing.T) {
	f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) { writeDeepSeekResponse(w, body, false) })
	// The exact nonstream request from the report, including string input.
	w := f.relay(t, `{"model":"deepseek-flash","input":"Reply OK.","max_output_tokens":256,"stream":false}`)
	deepSeekClientOutput(t, w, false)
	req := f.take(t)
	require.Equal(t, `"Reply OK."`, string(req["input"]))
	require.Equal(t, "256", string(req["max_output_tokens"]))
	for _, stream := range []bool{false, true} {
		for _, effort := range []string{"none", "low", "high", "max"} {
			t.Run(fmt.Sprintf("%t/%s", stream, effort), func(t *testing.T) {
				w := f.relay(t, fmt.Sprintf(`{"model":"deepseek-flash","input":"Reply OK.","max_output_tokens":256,"stream":%t,"reasoning":{"effort":%q},"stream_options":{"include_usage":true}}`, stream, effort))
				deepSeekClientOutput(t, w, stream)
				req := f.take(t)
				require.JSONEq(t, fmt.Sprintf(`{"effort":%q}`, effort), string(req["reasoning"]))
			})
		}
	}
	f.channel.ModelMapping = common.GetPointer(`{"company-deepseek":"deepseek-flash-max"}`)
	w = f.relay(t, `{"model":"company-deepseek","input":"Reply OK."}`)
	deepSeekClientOutput(t, w, false)
	req = f.take(t)
	require.Equal(t, `"deepseek-flash"`, string(req["model"]))
	require.JSONEq(t, `{"effort":"max"}`, string(req["reasoning"]))
}

func TestRelayDeepSeekResponsesToolContinuation(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, pass := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/pass=%t", stream, pass), func(t *testing.T) {
				f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) {
					writeDeepSeekResponse(w, body, !strings.Contains(string(body), "function_call_output"))
				})
				f.channel.SetSetting(dto.ChannelSettings{PassThroughBodyEnabled: pass})
				history := []json.RawMessage{json.RawMessage(`{"role":"user","content":"Call echo then reply OK."}`)}
				for turn := 0; turn < 2; turn++ {
					input, err := common.Marshal(history)
					require.NoError(t, err)
					body := fmt.Sprintf(`{"model":"deepseek-flash","input":%s,"stream":%t,"reasoning":{"effort":"high"},"tools":[{"type":"function","name":"echo","parameters":{"type":"object","properties":{"value":{"type":"string"}}}}],"tool_choice":"auto","stream_options":{"include_usage":true}}`, input, stream)
					w := f.relay(t, body)
					output := deepSeekClientOutput(t, w, stream)
					req := f.take(t)
					require.JSONEq(t, string(input), string(req["input"]))
					require.Contains(t, string(req["tools"]), "echo")
					if turn == 0 {
						encoded, err := common.Marshal(output)
						require.NoError(t, err)
						require.JSONEq(t, deepSeekToolOutput, string(encoded))
						if stream {
							require.Contains(t, w.Body.String(), "response.reasoning_text.delta")
							require.Contains(t, w.Body.String(), "response.function_call_arguments.delta")
						}
						history = append(history, output...)
						history = append(history, json.RawMessage(`{"type":"function_call_output","call_id":"call_fixture","output":"OK"}`))
					} else {
						require.Contains(t, string(req["input"]), "fixture reasoning")
						require.Contains(t, string(req["input"]), "call_fixture")
						require.Len(t, output, 1)
						require.Contains(t, string(output[0]), `"text":"OK"`)
					}
				}
			})
		}
	}
}

func TestRelayDeepSeekResponsesStateAndErrors(t *testing.T) {
	f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) { writeDeepSeekResponse(w, body, false) })
	for _, pass := range []bool{false, true} {
		f.channel.SetSetting(dto.ChannelSettings{PassThroughBodyEnabled: pass})
		for _, field := range []string{`"previous_response_id":"resp_missing"`, `"conversation":{"id":"conv_missing"}`} {
			w := f.relay(t, `{"model":"deepseek-flash","input":"Reply OK.",`+field+`}`)
			require.Equal(t, 400, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "send the full conversation history")
			require.Empty(t, f.requests, "unsupported state must not reach upstream or be retried")
		}
	}
	f.channel.SetSetting(dto.ChannelSettings{})
	f.channel.ParamOverride = common.GetPointer(`{"previous_response_id":"resp_override"}`)
	w := f.relay(t, `{"model":"deepseek-flash","input":"OK"}`)
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Empty(t, f.requests)
	f.channel.ParamOverride = common.GetPointer(`{"stream_options":{"include_usage":true}}`)
	w = f.relay(t, `{"model":"deepseek-flash","input":"OK","stream":true}`)
	deepSeekClientOutput(t, w, true)
	f.take(t)

	for _, status := range []int{200, 400, 502} {
		t.Run(fmt.Sprintf("upstream_%d", status), func(t *testing.T) {
			f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-ID", "deepseek-upstream-fixture")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"type":"invalid_request_error","code":"fixture_error","message":"original DeepSeek fixture error"}}`)
			})
			w := f.relay(t, `{"model":"deepseek-flash","input":"OK"}`)
			if status == 200 {
				require.GreaterOrEqual(t, w.Code, 400, "JSON error over HTTP 200 must not succeed")
			} else {
				require.Equal(t, status, w.Code, w.Body.String())
			}
			require.Contains(t, w.Body.String(), "original DeepSeek fixture error")
			require.Contains(t, w.Body.String(), "fixture_error")
			f.take(t)
			require.Empty(t, f.requests)
		})
	}
}

func TestRelayDeepSeekResponsesStreamFailure(t *testing.T) {
	for _, terminal := range []string{"failed", "incomplete"} {
		t.Run(terminal, func(t *testing.T) {
			f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "event: response.reasoning_text.delta\ndata: "+`{"type":"response.reasoning_text.delta","delta":"fixture reasoning"}`+"\n\n")
				w.(http.Flusher).Flush()
				fmt.Fprintf(w, "event: response.%s\ndata: {\"type\":\"response.%s\",\"response\":{\"status\":%q,\"error\":{\"code\":\"fixture_error\",\"message\":\"original stream failure\"},\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n", terminal, terminal, terminal)
			})
			w := f.relay(t, `{"model":"deepseek-flash","input":"OK","stream":true}`)
			require.Contains(t, w.Body.String(), "event: response."+terminal)
			require.Contains(t, w.Body.String(), "original stream failure")
			require.NotContains(t, w.Body.String(), "event: response.completed")
			f.take(t)
			require.Empty(t, f.requests, "never replay after reasoning output")
		})
	}
}
