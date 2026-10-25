package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type deepSeekThinkingTransport struct {
	mu                         sync.Mutex
	requests                   []string
	urls                       []string
	stream, alwaysFail, accept bool
}

func (s *deepSeekThinkingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	s.requests = append(s.requests, string(body))
	s.urls = append(s.urls, req.URL.String())
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Request: req}
	response.Header.Set("Content-Type", "application/json")
	response.Header.Set(common.RequestIdKey, fmt.Sprintf("thinking-upstream-fixture-%d", len(s.requests)))
	var data string
	if !s.accept && (len(s.requests) == 1 || s.alwaysFail) {
		response.StatusCode = 400
		data = "{\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The `content[].thinking` in the thinking mode must be passed back to the API. (request_id: fixture-123)\"}}"
	} else if s.stream {
		response.Header.Set("Content-Type", "text/event-stream")
		data = `data: {"type":"message_start","message":{"id":"msg_sample","model":"deepseek-v4-pro","usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n" +
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}` + "\n\n" +
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\n" +
			`data: {"type":"message_stop"}` + "\n\n"
	} else {
		data = `{"type":"message","id":"msg_sample","model":"deepseek-v4-pro","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
	}
	response.Body = io.NopCloser(strings.NewReader(data))
	return response, nil
}

// Keep the configured official URL as the response origin while sending test
// traffic exclusively to a local HTTP server.
type deepSeekLocalHTTPTransport struct{ target *url.URL }

func (s deepSeekLocalHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	local := req.Clone(req.Context())
	local.URL.Scheme, local.URL.Host = s.target.Scheme, s.target.Host
	local.Header.Set("X-Fixture-Original-Host", req.URL.Host)
	resp, err := http.DefaultTransport.RoundTrip(local)
	if resp != nil {
		resp.Request = req
	}
	return resp, err
}

type deepSeekPingObserver struct {
	http.ResponseWriter
	observed chan struct{}
	once     sync.Once
	ping     bool
}

func (w *deepSeekPingObserver) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err == nil && n == len(data) && string(data) == "event: ping\ndata: {\"type\":\"ping\"}\n\n" {
		w.ping = true
	}
	return n, err
}
func (w *deepSeekPingObserver) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
	if w.ping {
		w.once.Do(func() { close(w.observed) })
	}
}

func TestRelayDeepSeekThinkingHistoryCompatibility(t *testing.T) {
	if runRelayMatrixInSubprocess(t) {
		return
	}
	db := useRelayMatrixProcessDB(t)
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, stream := range []bool{false, true} {
		for _, scenario := range []string{"healed", "default_thinking", "final_failure", "passthrough", "global_passthrough", "accepted", "disabled", "healed_after_ping", "final_failure_after_ping", "fallback_after_ping", "healed_after_replay_ping"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, scenario), func(t *testing.T) {
				afterPing := strings.HasSuffix(scenario, "_after_ping")
				replayPing := scenario == "healed_after_replay_ping"
				withPing := afterPing || replayPing
				if withPing && !stream {
					t.Skip("heartbeat is a streaming case")
				}
				baseScenario := strings.TrimSuffix(scenario, "_after_ping")
				crossChannel := scenario == "fallback_after_ping"
				if crossChannel || replayPing {
					baseScenario = "healed"
				}
				resetRelayMatrixFixture(t, db)
				oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
				constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, 0, false, true, true
				t.Cleanup(func() {
					constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
				})
				if crossChannel {
					common.RetryTimes = 1
				}
				general := operation_setting.GetGeneralSetting()
				oldPing, oldInterval := general.PingIntervalEnabled, general.PingIntervalSeconds
				general.PingIntervalEnabled, general.PingIntervalSeconds = withPing, 1
				t.Cleanup(func() { general.PingIntervalEnabled, general.PingIntervalSeconds = oldPing, oldInterval })
				global := model_setting.GetGlobalSettings()
				oldPass := global.PassThroughRequestEnabled
				global.PassThroughRequestEnabled = scenario == "global_passthrough"
				t.Cleanup(func() { global.PassThroughRequestEnabled = oldPass })
				oldRatio, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"deepseek-v4-pro":1}`))
				require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"deepseek-v4-pro":1}`))
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
				})
				const initial = 1000000
				user := model.User{Id: 997701, Username: "thinking-fixture", Quota: initial, Status: common.UserStatusEnabled}
				token := model.Token{Id: 997702, UserId: user.Id, Key: "thinking-fixture", RemainQuota: initial, Status: common.TokenStatusEnabled}
				channel := model.Channel{Id: 997703, Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Key: "fixture", BaseURL: common.GetPointer("https://api.deepseek.com"), Group: "default", Models: "deepseek-v4-pro", AutoBan: common.GetPointer(0)}
				if scenario == "passthrough" {
					channel.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
				}
				for _, row := range []any{&user, &token, &channel} {
					require.NoError(t, db.Create(row).Error)
				}
				selectedChannel := &channel
				if crossChannel {
					primary := model.Channel{Id: 997704, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "fixture", BaseURL: common.GetPointer("https://thinking-primary.example"), Group: "default", Models: "deepseek-v4-pro", AutoBan: common.GetPointer(0)}
					require.NoError(t, db.Create(&primary).Error)
					require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "deepseek-v4-pro", ChannelId: channel.Id, Enabled: true, Priority: common.GetPointer(int64(1)), Weight: 1}).Error)
					selectedChannel = &primary
				}
				var primaryCalls, officialCalls atomic.Int32
				pingObserved := make(chan struct{})
				waitForPing := func(r *http.Request) {
					select {
					case <-pingObserved:
					case <-r.Context().Done():
					case <-time.After(3 * time.Second):
					}
				}
				transport := &deepSeekThinkingTransport{stream: stream, alwaysFail: baseScenario == "final_failure", accept: scenario == "accepted"}
				client := service.GetHttpClient()
				previous := client.Transport
				client.Transport = transport
				if withPing {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if crossChannel && r.Header.Get("X-Fixture-Original-Host") == "thinking-primary.example" {
							primaryCalls.Add(1)
							waitForPing(r)
							w.Header().Set(common.RequestIdKey, "thinking-primary-rejection")
							w.WriteHeader(502)
							_, _ = io.WriteString(w, `{"error":{"type":"upstream_error","message":"fixture upstream unavailable"}}`)
							return
						}
						if n := officialCalls.Add(1); !crossChannel && ((afterPing && n == 1) || (replayPing && n == 2)) {
							waitForPing(r)
						}
						response, err := transport.RoundTrip(r)
						if err != nil {
							http.Error(w, err.Error(), 500)
							return
						}
						defer response.Body.Close()
						for name, values := range response.Header {
							w.Header()[name] = values
						}
						w.WriteHeader(response.StatusCode)
						_, _ = io.Copy(w, response.Body)
					}))
					t.Cleanup(upstream.Close)
					target, err := url.Parse(upstream.URL)
					require.NoError(t, err)
					client.Transport = deepSeekLocalHTTPTransport{target: target}
				}
				t.Cleanup(func() { client.Transport = previous })
				request := fmt.Sprintf(`{"model":"deepseek-v4-pro","max_tokens":256,"thinking":{"type":"enabled","budget_tokens":4000},"output_config":{"effort":"high"},"tools":[{"name":"echo","description":"Echo input","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"Use echo"},{"role":"assistant","content":[{"type":"thinking","thinking":"preserve history","signature":"keep"},{"type":"tool_use","id":"call_1","name":"echo","input":{"value":"first"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"first"}]},{"role":"assistant","content":[{"type":"tool_use","id":"call_2","name":"echo","input":{"value":"second"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_2","content":"second"}]}],"stream":%t}`, stream)
				var err error
				if scenario == "default_thinking" {
					request, err = sjson.Delete(request, "thinking")
					require.NoError(t, err)
				}
				if scenario == "disabled" {
					request, err = sjson.Set(request, "thinking.type", "disabled")
					require.NoError(t, err)
				}
				var requestID string
				w := httptest.NewRecorder()
				r := gin.New()
				r.Use(middleware.RequestId(), middleware.RequestFailureLog(), middleware.RouteTag("relay"))
				r.POST("/v1/messages", func(c *gin.Context) {
					storage, err := common.GetBodyStorage(c)
					require.NoError(t, err)
					c.Request.Body = io.NopCloser(storage)
					requestID = c.GetString(common.RequestIdKey)
					common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
					common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
					common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
					common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
					common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, selectedChannel, "deepseek-v4-pro"))
					defer common.CleanupBodyStorage(c)
					Relay(c, types.RelayFormatClaude)
				})
				req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(request))
				req.Header.Set("Content-Type", "application/json")
				var clientWriter http.ResponseWriter = w
				if withPing {
					clientWriter = &deepSeekPingObserver{ResponseWriter: w, observed: pingObserved}
				}
				r.ServeHTTP(clientWriter, req)
				transport.mu.Lock()
				defer transport.mu.Unlock()
				require.NotEmpty(t, requestID)
				require.Equal(t, []string{requestID}, w.Result().Header.Values(common.RequestIdKey))
				adjusted := baseScenario == "healed" || baseScenario == "default_thinking" || baseScenario == "final_failure"
				calls := 1
				if adjusted {
					calls = 2
				}
				require.Len(t, transport.requests, calls)
				if crossChannel {
					require.EqualValues(t, 1, primaryCalls.Load())
				}
				for _, target := range transport.urls {
					if !withPing {
						require.Equal(t, "https://api.deepseek.com/anthropic/v1/messages", target)
					}
				}
				if adjusted {
					expected, err := sjson.Set(transport.requests[0], "thinking.type", "disabled")
					require.NoError(t, err)
					require.Equal(t, expected, transport.requests[1], "only this request's thinking switch may change")
					if withPing {
						require.Empty(t, w.Result().Header.Get("X-Request-Adjusted"), "committed headers cannot announce a later adjustment")
						require.Contains(t, w.Body.String(), "event: ping")
					} else {
						require.Equal(t, "deepseek_thinking_disabled", w.Header().Get("X-Request-Adjusted"))
					}
				} else {
					require.Empty(t, w.Header().Get("X-Request-Adjusted"))
				}
				success := baseScenario == "healed" || baseScenario == "default_thinking" || baseScenario == "accepted"
				charged := 0
				if success {
					charged = 15
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), "OK")
				} else {
					if afterPing {
						require.Equal(t, 200, w.Code)
						require.Contains(t, w.Body.String(), "event: error")
						require.NotContains(t, w.Body.String(), "event: message_stop")
					} else {
						require.Equal(t, 400, w.Code, w.Body.String())
					}
				}
				require.Eventually(t, func() bool {
					var u model.User
					var tok model.Token
					if db.First(&u, user.Id).Error != nil || db.First(&tok, token.Id).Error != nil {
						return false
					}
					return u.Quota == initial-charged && u.UsedQuota == charged && tok.RemainQuota == initial-charged
				}, 2*time.Second, 10*time.Millisecond)
				var logs []model.Log
				require.Eventually(t, func() bool { return db.Where("request_id = ?", requestID).Find(&logs).Error == nil && len(logs) == 1 }, time.Second, time.Millisecond)
				require.Equal(t, charged, logs[0].Quota)
				if success {
					require.Equal(t, model.LogTypeConsume, logs[0].Type)
				} else {
					require.Equal(t, model.LogTypeError, logs[0].Type)
					require.Equal(t, "thinking_history", gjson.Get(logs[0].Other, "failure_category").String())
				}
				ids := gjson.Get(logs[0].Other, "admin_info.upstream_request_ids").Array()
				if crossChannel {
					require.Len(t, ids, calls+1)
					require.Equal(t, "thinking-primary-rejection", ids[0].Get("request_id").String())
					require.EqualValues(t, selectedChannel.Id, ids[0].Get("channel_id").Int())
					ids = ids[1:]
				}
				require.Len(t, ids, calls)
				for i, id := range ids {
					require.Equal(t, fmt.Sprintf("thinking-upstream-fixture-%d", i+1), id.Get("request_id").String())
					require.EqualValues(t, channel.Id, id.Get("channel_id").Int())
				}
				attempts := gjson.Get(logs[0].Other, "admin_info.attempts").Array()
				if crossChannel {
					require.Len(t, attempts, 2)
					require.EqualValues(t, selectedChannel.Id, attempts[0].Get("channel_id").Int())
					attempts = attempts[1:]
				}
				if adjusted {
					require.Equal(t, "request_adjustment", attempts[0].Get("retry_kind").String())
					require.Equal(t, "deepseek_thinking_disabled", attempts[0].Get("adjustment").String())
					require.Equal(t, afterPing, attempts[0].Get("after_keepalive").Bool())
					require.Equal(t, withPing, attempts[0].Get("adjustment_header_not_delivered").Bool())
					require.Equal(t, "thinking-upstream-fixture-1", attempts[0].Get("upstream.request_id").String())
				}
				if success && adjusted {
					require.Len(t, attempts, 1)
					require.Equal(t, "none", gjson.Get(logs[0].Other, "reasoning_effort").String())
				}
				if baseScenario == "final_failure" {
					require.Len(t, attempts, 2)
				}
				if scenario == "accepted" {
					require.Empty(t, attempts)
					require.Equal(t, "enabled", gjson.Get(transport.requests[0], "thinking.type").String())
				}
				public, total, err := model.GetUserLogs(user.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 10, "", requestID)
				require.NoError(t, err)
				require.EqualValues(t, 1, total)
				require.NotContains(t, public[0].Other, "admin_info")
				require.NotContains(t, public[0].Other, "thinking-upstream-fixture")
			})
		}
	}
}
