package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type adaptiveCompatTransport struct {
	mu                         sync.Mutex
	requests                   []string
	stream, alwaysFail, accept bool
}

func (s *adaptiveCompatTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	s.requests = append(s.requests, string(body))
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Request: req}
	response.Header.Set("Content-Type", "application/json")
	response.Header.Set(common.RequestIdKey, fmt.Sprintf("adaptive-upstream-fixture-%d", len(s.requests)))
	var data string
	if !s.accept && (len(s.requests) == 1 || s.alwaysFail) {
		response.StatusCode = 400
		data = `{"type":"error","error":{"type":"invalid_request_error","message":"claude-haiku-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort"}}`
	} else if s.stream {
		response.Header.Set("Content-Type", "text/event-stream")
		data = `data: {"type":"message_start","message":{"id":"msg_sample","model":"claude-haiku-5-5","usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n" +
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}` + "\n\n" +
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\n" +
			`data: {"type":"message_stop"}` + "\n\n"
	} else {
		data = `{"type":"message","id":"msg_sample","model":"claude-haiku-5-5","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
	}
	response.Body = io.NopCloser(strings.NewReader(data))
	return response, nil
}

// Release the delayed replay only after a protocol-standard heartbeat has
// actually reached and flushed the downstream writer.
type adaptivePingObserver struct {
	http.ResponseWriter
	observed chan struct{}
	once     sync.Once
	ping     bool
	expected string
}

func (w *adaptivePingObserver) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err == nil && n == len(data) && string(data) == w.expected {
		w.ping = true
	}
	return n, err
}

func (w *adaptivePingObserver) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
	if w.ping {
		w.once.Do(func() { close(w.observed) })
	}
}

func TestRelayClaudeAdaptiveThinkingCompatibility(t *testing.T) {
	if runRelayMatrixInSubprocess(t) {
		return
	}
	db := useRelayMatrixProcessDB(t)
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		for _, stream := range []bool{false, true} {
			for _, scenario := range []string{"healed", "final_failure", "passthrough", "accepted", "disabled", "healed_after_ping"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", format, stream, scenario), func(t *testing.T) {
					afterPing := strings.HasSuffix(scenario, "_after_ping")
					if afterPing && !stream {
						t.Skip("heartbeat is a streaming case")
					}
					baseScenario := strings.TrimSuffix(scenario, "_after_ping")
					resetRelayMatrixFixture(t, db)
					oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
					constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, 0, false, true, true
					t.Cleanup(func() {
						constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
					})
					general := operation_setting.GetGeneralSetting()
					oldPing, oldInterval := general.PingIntervalEnabled, general.PingIntervalSeconds
					general.PingIntervalEnabled, general.PingIntervalSeconds = afterPing, 1
					t.Cleanup(func() { general.PingIntervalEnabled, general.PingIntervalSeconds = oldPing, oldInterval })
					oldRatio, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-haiku-5-5":1}`))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"claude-haiku-5-5":1}`))
					t.Cleanup(func() {
						require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
						require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
					})
					const initial = 1000000
					user := model.User{Id: 997601, Username: "adaptive-fixture", Quota: initial, Status: common.UserStatusEnabled}
					token := model.Token{Id: 997602, UserId: user.Id, Key: "adaptive-fixture", RemainQuota: initial, Status: common.TokenStatusEnabled}
					channelID := 997603
					if format == types.RelayFormatClaude {
						channelID = 997604
					}
					channel := model.Channel{Id: channelID, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "fixture"}
					if baseScenario == "passthrough" {
						channel.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
					}
					for _, row := range []any{&user, &token, &channel} {
						require.NoError(t, db.Create(row).Error)
					}
					accepted := baseScenario == "accepted"
					transport := &adaptiveCompatTransport{stream: stream, alwaysFail: baseScenario == "final_failure", accept: accepted}
					client := service.GetHttpClient()
					previous := client.Transport
					client.Transport = transport
					pingObserved := make(chan struct{})
					if afterPing {
						var calls atomic.Int32
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							if calls.Add(1) == 2 {
								select {
								case <-pingObserved:
								case <-r.Context().Done():
									return
								case <-time.After(3 * time.Second):
									http.Error(w, "fixture did not observe a flushed ping", 500)
									return
								}
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
						channel.BaseURL = common.GetPointer(upstream.URL)
						client.Transport = http.DefaultTransport
					}
					t.Cleanup(func() { client.Transport = previous })
					endpoint := "/v1/messages"
					if format == types.RelayFormatOpenAI {
						endpoint = "/v1/chat/completions"
					}
					var requestID string
					thinking := `"thinking":{"type":"enabled","budget_tokens":4096},"output_config":{"effort":"xhigh"}`
					if format == types.RelayFormatOpenAI {
						thinking = `"reasoning_effort":"high"`
					}
					if baseScenario == "disabled" {
						thinking = `"thinking":{"type":"disabled"}`
						if format == types.RelayFormatOpenAI {
							thinking = `"reasoning_effort":"none"`
						}
					}
					request := fmt.Sprintf(`{"model":"claude-haiku-5-5","max_tokens":8192,%s,"messages":[{"role":"user","content":"preserve body"}],"stream":%t}`, thinking, stream)
					w := httptest.NewRecorder()
					r := gin.New()
					r.Use(middleware.RequestId(), middleware.RequestFailureLog(), middleware.RouteTag("relay"))
					r.POST(endpoint, func(c *gin.Context) {
						// Distribution caches the body before native Claude validation reads it.
						storage, err := common.GetBodyStorage(c)
						require.NoError(t, err)
						c.Request.Body = io.NopCloser(storage)
						requestID = c.GetString(common.RequestIdKey)
						common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
						common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
						common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
						common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
						common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
						require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "claude-haiku-5-5"))
						defer common.CleanupBodyStorage(c)
						Relay(c, format)
					})
					req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(request))
					req.Header.Set("Content-Type", "application/json")
					if afterPing {
						ping := ": PING\n\n"
						if format == types.RelayFormatClaude {
							ping = "event: ping\ndata: {\"type\":\"ping\"}\n\n"
						}
						r.ServeHTTP(&adaptivePingObserver{ResponseWriter: w, observed: pingObserved, expected: ping}, req)
						require.Contains(t, w.Body.String(), ping)
					} else {
						r.ServeHTTP(w, req)
					}
					require.NotEmpty(t, requestID)
					require.Equal(t, []string{requestID}, w.Result().Header.Values(common.RequestIdKey))
					adjusted := baseScenario == "healed" || baseScenario == "final_failure"
					calls := 1
					if adjusted {
						calls = 2
					}
					require.Len(t, transport.requests, calls)
					if baseScenario != "passthrough" {
						mode := "enabled"
						if baseScenario == "disabled" {
							mode = "disabled"
							if format == types.RelayFormatOpenAI {
								mode = ""
							}
						}
						require.Equal(t, mode, gjson.Get(transport.requests[0], "thinking.type").String())
					}
					if adjusted {
						require.Equal(t, "adaptive", gjson.Get(transport.requests[1], "thinking.type").String())
						require.False(t, gjson.Get(transport.requests[1], "thinking.budget_tokens").Exists())
						effort := "xhigh"
						if format == types.RelayFormatOpenAI {
							effort = "high"
						}
						require.Equal(t, effort, gjson.Get(transport.requests[1], "output_config.effort").String())
						var expected map[string]any
						require.NoError(t, common.UnmarshalJsonStr(transport.requests[0], &expected))
						thinking := expected["thinking"].(map[string]any)
						thinking["type"] = "adaptive"
						delete(thinking, "budget_tokens")
						if format == types.RelayFormatOpenAI {
							expected["output_config"] = map[string]any{"effort": "high"}
						}
						expectedJSON, err := common.Marshal(expected)
						require.NoError(t, err)
						require.JSONEq(t, string(expectedJSON), transport.requests[1])
						if afterPing {
							require.Empty(t, w.Result().Header.Get("X-Request-Adjusted"), "committed headers cannot announce a later adjustment")
							require.Empty(t, w.Header().Get("X-Request-Adjusted"), "do not mutate committed headers")
						} else {
							require.Equal(t, "claude_adaptive_thinking", w.Result().Header.Get("X-Request-Adjusted"))
						}
					}
					success := baseScenario == "healed" || accepted
					charged := 0
					if success {
						charged = 15
						require.Equal(t, 200, w.Code, w.Body.String())
						require.Contains(t, w.Body.String(), "OK")
					} else {
						require.Equal(t, 400, w.Code, w.Body.String())
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
					}
					ids := gjson.Get(logs[0].Other, "admin_info.upstream_request_ids").Array()
					require.Len(t, ids, calls)
					for i, id := range ids {
						require.Equal(t, fmt.Sprintf("adaptive-upstream-fixture-%d", i+1), id.Get("request_id").String())
						require.EqualValues(t, channelID, id.Get("channel_id").Int())
					}
					attempts := gjson.Get(logs[0].Other, "admin_info.attempts").Array()
					if adjusted {
						require.Equal(t, "request_adjustment", attempts[0].Get("retry_kind").String())
						require.Equal(t, "claude_adaptive_thinking", attempts[0].Get("adjustment").String())
						require.Equal(t, afterPing, attempts[0].Get("adjustment_header_not_delivered").Bool())
						require.Equal(t, "adaptive-upstream-fixture-1", attempts[0].Get("upstream.request_id").String())
						require.EqualValues(t, channelID, attempts[0].Get("channel_id").Int())
					}
					if baseScenario == "healed" {
						require.Len(t, attempts, 1)
					}
					if baseScenario == "final_failure" {
						require.Len(t, attempts, 2)
						require.Equal(t, "invalid_request", gjson.Get(logs[0].Other, "failure_category").String())
					}
					if accepted {
						require.Empty(t, attempts)
						require.Empty(t, w.Header().Get("X-Request-Adjusted"))
					}
					public, total, err := model.GetUserLogs(user.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 10, "", requestID)
					require.NoError(t, err)
					require.EqualValues(t, 1, total)
					require.NotContains(t, public[0].Other, "admin_info")
					require.NotContains(t, public[0].Other, "adaptive-upstream-fixture")
				})
			}
		}
	}
}
