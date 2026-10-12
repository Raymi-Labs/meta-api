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
)

const relayCapabilityModel = "test-capability-model"
const relayCapabilityProviderError = `{"error":{"message":"unknown provider for model test-capability-model","type":"invalid_request_error","code":"model_not_found","param":"model"}}`
const relayCapabilityToolError = `{"error":{"message":"test-capability-model requires Responses for tool calls; this account only supports Chat Completions","type":"invalid_request_error"}}` + "event: response.failed\ndata: " + `{"type":"response.failed","response":{"status":"failed","output":[],"error":{"code":"upstream_error","message":"Upstream request failed"}}}` + "\n\n"

func TestRelayUpstreamCapabilityFailover(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, tc := range []struct {
		name, body                 string
		retries, calls, rounds     int
		success, fixed, restricted bool
		foreign, allFail, partial  bool
		previous, conversation     bool
	}{
		{name: "provider_tool_loop", body: relayCapabilityProviderError, retries: 1, calls: 2, rounds: 3, success: true},
		{name: "mixed_error_tool_loop", body: relayCapabilityToolError, retries: 1, calls: 2, rounds: 3, success: true},
		{name: "retry_disabled", body: relayCapabilityProviderError, calls: 1, rounds: 1},
		{name: "explicit_channel", body: relayCapabilityToolError, retries: 2, calls: 1, rounds: 1, fixed: true},
		{name: "token_channel_limit", body: relayCapabilityProviderError, retries: 2, calls: 1, rounds: 1, restricted: true},
		{name: "no_permitted_group", body: relayCapabilityProviderError, retries: 2, calls: 1, rounds: 1, foreign: true},
		{name: "budget_exhausted", body: relayCapabilityProviderError, retries: 1, calls: 2, rounds: 1, allFail: true},
		{name: "previous_response_state", body: relayCapabilityProviderError, retries: 2, calls: 1, rounds: 1, previous: true},
		{name: "conversation_state", body: relayCapabilityToolError, retries: 2, calls: 1, rounds: 1, conversation: true},
		{name: "ordinary_bad_request", body: `{"error":{"message":"invalid tool arguments","type":"invalid_request_error"}}`, retries: 2, calls: 1, rounds: 1},
		{name: "partial_tool_output", retries: 2, calls: 1, rounds: 1, partial: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := useControllerCapacityTestDB(t)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
			oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
			constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, tc.retries, false, true, true
			t.Cleanup(func() {
				constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
			})
			general := operation_setting.GetGeneralSetting()
			oldPing := general.PingIntervalEnabled
			general.PingIntervalEnabled = false
			t.Cleanup(func() { general.PingIntervalEnabled = oldPing })
			oldRatio := ratio_setting.ModelRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-capability-model":1}`))
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
			const initialQuota = 1000000
			user := &model.User{Id: 998201, Username: "capability-fixture", Quota: initialQuota, Status: common.UserStatusEnabled}
			token := &model.Token{Id: 998202, UserId: user.Id, Key: "capability-fixture-token", Status: common.TokenStatusEnabled, RemainQuota: initialQuota}
			require.NoError(t, db.Create(user).Error)
			require.NoError(t, db.Create(token).Error)
			var badCalls, goodCalls atomic.Int32
			var bodyMu sync.Mutex
			var badBodies, goodBodies []string
			badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				badCalls.Add(1)
				body, _ := io.ReadAll(r.Body)
				bodyMu.Lock()
				badBodies = append(badBodies, string(body))
				bodyMu.Unlock()
				w.Header().Set("X-Request-ID", "fixture-upstream-rejection")
				if tc.partial {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: "+`{"type":"response.function_call_arguments.delta","item_id":"fc_partial","delta":"{\"value\":"}`+"\n\n")
					w.(http.Flusher).Flush()
					fmt.Fprint(w, "data: "+`{"type":"response.failed","response":{"status":"failed","error":{"type":"invalid_request_error","code":"model_not_found","message":"unknown provider for model test-capability-model"}}}`+"\n\n")
					return
				}
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(badServer.Close)
			goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := goodCalls.Add(1)
				body, _ := io.ReadAll(r.Body)
				bodyMu.Lock()
				goodBodies = append(goodBodies, string(body))
				bodyMu.Unlock()
				var req struct{ Input []map[string]interface{} }
				if common.Unmarshal(body, &req) != nil || (n > 1 && !strings.Contains(string(body), "function_call_output")) {
					http.Error(w, "missing tool history", 400)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				output := []map[string]interface{}{{"type": "function_call", "id": fmt.Sprintf("fc_%d", n), "call_id": fmt.Sprintf("call_%d", n), "name": "echo", "arguments": `{"value":"ok"}`, "status": "completed"}}
				if n == 3 {
					output = []map[string]interface{}{{"type": "message", "id": "msg_final", "role": "assistant", "status": "completed", "content": []map[string]string{{"type": "output_text", "text": "OK"}}}}
					fmt.Fprint(w, "data: "+`{"type":"response.output_text.delta","delta":"OK"}`+"\n\n")
				} else {
					fmt.Fprint(w, "data: "+`{"type":"response.function_call_arguments.delta","delta":"{\"value\":\"ok\"}"}`+"\n\n")
				}
				completed, _ := common.Marshal(map[string]interface{}{"type": "response.completed", "response": map[string]interface{}{"id": fmt.Sprintf("resp_%d", n), "model": relayCapabilityModel, "status": "completed", "output": output, "usage": map[string]int{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}})
				fmt.Fprintf(w, "data: %s\n\n", completed)
			}))
			t.Cleanup(goodServer.Close)
			channelA := &model.Channel{Id: 998203, Name: "capability-failure", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "fixture-key", BaseURL: &badServer.URL, Group: "default", Models: relayCapabilityModel, AutoBan: common.GetPointer(0)}
			baseB, groupB := goodServer.URL, "default"
			if tc.allFail {
				baseB = badServer.URL
			}
			if tc.foreign {
				groupB = "private-group"
			}
			channelB := &model.Channel{Id: 998204, Name: "capability-alternative", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "fixture-key", BaseURL: &baseB, Group: groupB, Models: relayCapabilityModel, AutoBan: common.GetPointer(0)}
			for _, channel := range []*model.Channel{channelA, channelB} {
				require.NoError(t, db.Create(channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: channel.Group, Model: relayCapabilityModel, ChannelId: channel.Id, Enabled: true, Priority: common.GetPointer(int64(1)), Weight: 1}).Error)
			}
			if tc.allFail {
				channelC := *channelB
				channelC.Id = 998205
				require.NoError(t, db.Create(&channelC).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: relayCapabilityModel, ChannelId: channelC.Id, Enabled: true, Priority: common.GetPointer(int64(1)), Weight: 1}).Error)
			}
			r := gin.New()
			r.Use(middleware.RequestFailureLog(), middleware.RouteTag("relay"))
			r.POST("/v1/responses", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				c.Set(common.RequestIdKey, c.GetHeader("X-Test-Request-ID"))
				if tc.fixed {
					c.Set("specific_channel_id", channelA.Id)
				}
				if tc.restricted {
					common.SetContextKey(c, constant.ContextKeyTokenChannelLimitEnabled, true)
					common.SetContextKey(c, constant.ContextKeyTokenChannelLimit, map[int]bool{channelA.Id: true})
				}
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, channelA, relayCapabilityModel))
				Relay(c, types.RelayFormatOpenAIResponses)
				require.Equal(t, tc.success, service.RequestSucceeded(c))
			})
			history := []map[string]interface{}{{"role": "user", "content": "Call echo with value ok twice, then reply OK."}}
			charged := 0
			for round := 0; round < tc.rounds; round++ {
				payload := map[string]interface{}{"model": relayCapabilityModel, "input": history, "stream": true, "tools": []map[string]interface{}{{"type": "function", "name": "echo", "parameters": map[string]string{"type": "object"}}}}
				if tc.previous {
					payload["previous_response_id"] = "resp_prior"
				}
				if tc.conversation {
					payload["conversation"] = "conv_prior"
				}
				body, err := common.Marshal(payload)
				require.NoError(t, err)
				requestID := fmt.Sprintf("capability-%s-%d", tc.name, round)
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Test-Request-ID", requestID)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.EqualValues(t, tc.calls*(round+1), badCalls.Load()+goodCalls.Load(), w.Body.String())
				var logs []model.Log
				require.Eventually(t, func() bool { return db.Where("request_id = ?", requestID).Find(&logs).Error == nil && len(logs) == 1 }, time.Second, time.Millisecond)
				log := logs[0]
				charged += log.Quota
				var details map[string]interface{}
				require.NoError(t, common.UnmarshalJsonStr(log.Other, &details))
				attempts := details["admin_info"].(map[string]interface{})["attempts"].([]interface{})
				if tc.success {
					require.Equal(t, 200, w.Code)
					require.Equal(t, model.LogTypeConsume, log.Type)
					require.Positive(t, log.Quota)
					require.Len(t, attempts, 1)
					require.Equal(t, "fixture-upstream-rejection", attempts[0].(map[string]interface{})["upstream"].(map[string]interface{})["request_id"])
					require.NotContains(t, w.Body.String(), "unknown provider")
					require.NotContains(t, w.Body.String(), "response.failed")
					var completed struct {
						Type     string
						Response struct{ Output []map[string]interface{} }
					}
					for _, line := range strings.Split(w.Body.String(), "\n") {
						if strings.HasPrefix(line, "data:") && strings.Contains(line, `"type":"response.completed"`) {
							require.NoError(t, common.UnmarshalJsonStr(strings.TrimSpace(strings.TrimPrefix(line, "data:")), &completed))
						}
					}
					require.Equal(t, "response.completed", completed.Type)
					history = append(history, completed.Response.Output...)
					if round < 2 {
						require.Equal(t, "function_call", completed.Response.Output[0]["type"])
						history = append(history, map[string]interface{}{"type": "function_call_output", "call_id": completed.Response.Output[0]["call_id"], "output": "ok"})
					} else {
						require.Contains(t, w.Body.String(), `"text":"OK"`)
					}
					bodyMu.Lock()
					require.JSONEq(t, badBodies[round], goodBodies[round], "failover must preserve tool history")
					bodyMu.Unlock()
				} else {
					require.Equal(t, model.LogTypeError, log.Type)
					require.Zero(t, log.Quota)
					require.Zero(t, goodCalls.Load())
					require.Len(t, attempts, tc.calls)
					if !tc.partial {
						require.Equal(t, 400, w.Code)
						require.NotContains(t, w.Body.String(), "可用渠道不存在")
					}
				}
			}
			require.Eventually(t, func() bool {
				var actualUser model.User
				var actualToken model.Token
				return db.First(&actualUser, user.Id).Error == nil && db.First(&actualToken, token.Id).Error == nil && actualUser.Quota == initialQuota-charged && actualToken.RemainQuota == initialQuota-charged && actualUser.UsedQuota == charged && actualToken.UsedQuota == charged
			}, 3*time.Second, 10*time.Millisecond, "only completed requests are charged once; failures refund all reservations")
			publicLogs, err := model.GetLogByTokenId(token.Id)
			require.NoError(t, err)
			require.Len(t, publicLogs, tc.rounds)
			for _, log := range publicLogs {
				require.NotContains(t, log.Other, "admin_info")
				require.NotContains(t, log.Other, "fixture-upstream-rejection")
			}
		})
	}
}
