package controller

import (
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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type deepSeekThinkingTransport struct {
	requests                   []string
	urls                       []string
	stream, alwaysFail, accept bool
}

func (s *deepSeekThinkingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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

func TestRelayDeepSeekThinkingHistoryCompatibility(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, stream := range []bool{false, true} {
		for _, scenario := range []string{"healed", "default_thinking", "final_failure", "passthrough", "global_passthrough", "accepted", "disabled"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, scenario), func(t *testing.T) {
				db := useControllerCapacityTestDB(t)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)
				t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
				require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
				oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
				constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, 0, false, true, true
				t.Cleanup(func() {
					constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
				})
				general := operation_setting.GetGeneralSetting()
				oldPing := general.PingIntervalEnabled
				general.PingIntervalEnabled = false
				t.Cleanup(func() { general.PingIntervalEnabled = oldPing })
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
				channel := model.Channel{Id: 997703, Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Key: "fixture", BaseURL: common.GetPointer("https://api.deepseek.com")}
				if scenario == "passthrough" {
					channel.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
				}
				for _, row := range []any{&user, &token, &channel} {
					require.NoError(t, db.Create(row).Error)
				}
				transport := &deepSeekThinkingTransport{stream: stream, alwaysFail: scenario == "final_failure", accept: scenario == "accepted"}
				client := service.GetHttpClient()
				previous := client.Transport
				client.Transport = transport
				t.Cleanup(func() { client.Transport = previous })
				request := fmt.Sprintf(`{"model":"deepseek-v4-pro","max_tokens":256,"thinking":{"type":"enabled","budget_tokens":4000},"output_config":{"effort":"high"},"tools":[{"name":"echo","description":"Echo input","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"Use echo"},{"role":"assistant","content":[{"type":"thinking","thinking":"preserve history","signature":"keep"},{"type":"tool_use","id":"call_1","name":"echo","input":{"value":"first"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"first"}]},{"role":"assistant","content":[{"type":"tool_use","id":"call_2","name":"echo","input":{"value":"second"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_2","content":"second"}]}],"stream":%t}`, stream)
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
					common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "deepseek-v4-pro"))
					defer common.CleanupBodyStorage(c)
					Relay(c, types.RelayFormatClaude)
				})
				req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(request))
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(w, req)
				require.NotEmpty(t, requestID)
				require.Equal(t, []string{requestID}, w.Result().Header.Values(common.RequestIdKey))
				adjusted := scenario == "healed" || scenario == "default_thinking" || scenario == "final_failure"
				calls := 1
				if adjusted {
					calls = 2
				}
				require.Len(t, transport.requests, calls)
				for _, target := range transport.urls {
					require.Equal(t, "https://api.deepseek.com/anthropic/v1/messages", target)
				}
				if adjusted {
					expected, err := sjson.Set(transport.requests[0], "thinking.type", "disabled")
					require.NoError(t, err)
					require.Equal(t, expected, transport.requests[1], "only this request's thinking switch may change")
					require.Equal(t, "deepseek_thinking_disabled", w.Header().Get("X-Request-Adjusted"))
				} else {
					require.Empty(t, w.Header().Get("X-Request-Adjusted"))
				}
				success := scenario == "healed" || scenario == "default_thinking" || scenario == "accepted"
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
					require.Equal(t, "thinking_history", gjson.Get(logs[0].Other, "failure_category").String())
				}
				ids := gjson.Get(logs[0].Other, "admin_info.upstream_request_ids").Array()
				require.Len(t, ids, calls)
				for i, id := range ids {
					require.Equal(t, fmt.Sprintf("thinking-upstream-fixture-%d", i+1), id.Get("request_id").String())
					require.EqualValues(t, channel.Id, id.Get("channel_id").Int())
				}
				attempts := gjson.Get(logs[0].Other, "admin_info.attempts").Array()
				if adjusted {
					require.Equal(t, "request_adjustment", attempts[0].Get("retry_kind").String())
					require.Equal(t, "deepseek_thinking_disabled", attempts[0].Get("adjustment").String())
					require.Equal(t, "thinking-upstream-fixture-1", attempts[0].Get("upstream.request_id").String())
				}
				if success && adjusted {
					require.Len(t, attempts, 1)
					require.Equal(t, "none", gjson.Get(logs[0].Other, "reasoning_effort").String())
				}
				if scenario == "final_failure" {
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
