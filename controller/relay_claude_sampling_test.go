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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type samplingCompatTransport struct {
	requests                   []string
	stream, alwaysFail, accept bool
}

func (s *samplingCompatTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	s.requests = append(s.requests, string(body))
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Request: req}
	response.Header.Set("Content-Type", "application/json")
	response.Header.Set(common.RequestIdKey, fmt.Sprintf("sampling-upstream-fixture-%d", len(s.requests)))
	var data string
	if !s.accept && (len(s.requests) == 1 || s.alwaysFail) {
		response.StatusCode = 400
		data = `{"type":"error","error":{"type":"invalid_request_error","message":"claude-haiku-5-5 does not support non-default temperature"}}`
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

func TestRelayClaudeSamplingCompatibility(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		for _, stream := range []bool{false, true} {
			for _, scenario := range []string{"healed", "final_failure", "passthrough", "accepted", "default_accepted"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", format, stream, scenario), func(t *testing.T) {
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
					oldRatio, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-haiku-5-5":1}`))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"claude-haiku-5-5":1}`))
					t.Cleanup(func() {
						require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
						require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
					})
					const initial = 1000000
					user := model.User{Id: 997601, Username: "sampling-fixture", Quota: initial, Status: common.UserStatusEnabled}
					token := model.Token{Id: 997602, UserId: user.Id, Key: "sampling-fixture", RemainQuota: initial, Status: common.TokenStatusEnabled}
					channelID := 997603
					if format == types.RelayFormatClaude {
						channelID = 997604
					}
					channel := model.Channel{Id: channelID, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "fixture"}
					if scenario == "passthrough" {
						channel.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
					}
					for _, row := range []any{&user, &token, &channel} {
						require.NoError(t, db.Create(row).Error)
					}
					accepted := scenario == "accepted" || scenario == "default_accepted"
					transport := &samplingCompatTransport{stream: stream, alwaysFail: scenario == "final_failure", accept: accepted}
					client := service.GetHttpClient()
					previous := client.Transport
					client.Transport = transport
					t.Cleanup(func() { client.Transport = previous })
					endpoint := "/v1/messages"
					if format == types.RelayFormatOpenAI {
						endpoint = "/v1/chat/completions"
					}
					var requestID string
					request := fmt.Sprintf(`{"model":"claude-haiku-5-5","max_tokens":256,"temperature":0.2,"messages":[{"role":"user","content":"preserve body"}],"stream":%t}`, stream)
					temperature := 0.2
					if scenario == "default_accepted" {
						temperature = 1
						request = strings.Replace(request, `"temperature":0.2`, `"temperature":1`, 1)
					}
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
					r.ServeHTTP(w, req)
					require.NotEmpty(t, requestID)
					require.Equal(t, []string{requestID}, w.Result().Header.Values(common.RequestIdKey))
					adjusted := scenario == "healed" || scenario == "final_failure"
					calls := 1
					if adjusted {
						calls = 2
					}
					require.Len(t, transport.requests, calls)
					require.InDelta(t, temperature, gjson.Get(transport.requests[0], "temperature").Float(), 0.00001, transport.requests)
					if adjusted {
						require.False(t, gjson.Get(transport.requests[1], "temperature").Exists())
						var expected map[string]any
						require.NoError(t, common.UnmarshalJsonStr(transport.requests[0], &expected))
						delete(expected, "temperature")
						expectedJSON, err := common.Marshal(expected)
						require.NoError(t, err)
						require.JSONEq(t, string(expectedJSON), transport.requests[1])
						require.Equal(t, "temperature", w.Header().Get("X-Param-Adjusted"))
					}
					success := scenario == "healed" || accepted
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
						require.Equal(t, fmt.Sprintf("sampling-upstream-fixture-%d", i+1), id.Get("request_id").String())
						require.EqualValues(t, channelID, id.Get("channel_id").Int())
					}
					attempts := gjson.Get(logs[0].Other, "admin_info.attempts").Array()
					if adjusted {
						require.Equal(t, "request_adjustment", attempts[0].Get("retry_kind").String())
						require.Equal(t, "remove_temperature", attempts[0].Get("adjustment").String())
						require.Equal(t, "sampling-upstream-fixture-1", attempts[0].Get("upstream.request_id").String())
						require.EqualValues(t, channelID, attempts[0].Get("channel_id").Int())
					}
					if scenario == "healed" {
						require.Len(t, attempts, 1)
					}
					if scenario == "final_failure" {
						require.Len(t, attempts, 2)
						require.Equal(t, "sampling_parameter", gjson.Get(logs[0].Other, "failure_category").String())
					}
					if accepted {
						require.Empty(t, attempts)
						require.Empty(t, w.Header().Get("X-Param-Adjusted"))
					}
					public, total, err := model.GetUserLogs(user.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 10, "", requestID)
					require.NoError(t, err)
					require.EqualValues(t, 1, total)
					require.NotContains(t, public[0].Other, "admin_info")
					require.NotContains(t, public[0].Other, "sampling-upstream-fixture")
				})
			}
		}
	}
}
