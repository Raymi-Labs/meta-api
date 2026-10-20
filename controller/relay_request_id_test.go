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

type requestIDTransport struct {
	stream, failure, omitID bool
	calls                   int
}

func (s *requestIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Request: req}
	resp.Header.Set("Content-Type", "application/json")
	if !s.omitID {
		resp.Header.Set(common.RequestIdKey, "supplier-request-id")
	}
	body := `{"id":"resp_body_id","object":"response","status":"completed","model":"request-id-fixture","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`
	if s.failure {
		resp.StatusCode = 400
		body = `{"error":{"type":"invalid_request_error","code":"invalid_request","message":"fixture rejected"}}`
	} else if s.stream {
		resp.Header.Set("Content-Type", "text/event-stream")
		body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n" +
			`data: {"type":"response.completed","response":` + body + "}\n\n"
	}
	resp.Body = io.NopCloser(strings.NewReader(body))
	return resp, nil
}

func TestRelayResponsesRequestIDMatchesUsageLogs(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, stream := range []bool{false, true} {
		for _, scenario := range []string{"success", "failure", "no_upstream_id"} {
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
				oldRatio, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"request-id-fixture":1}`))
				require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"request-id-fixture":1}`))
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
				})
				const initial = 1000000
				user := model.User{Id: 997801, Username: "request-id-fixture", Quota: initial, Status: common.UserStatusEnabled}
				token := model.Token{Id: 997802, UserId: user.Id, Key: "request-id-fixture", RemainQuota: initial, Status: common.TokenStatusEnabled}
				channel := model.Channel{Id: 997803, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "fixture"}
				for _, row := range []any{&user, &token, &channel} {
					require.NoError(t, db.Create(row).Error)
				}
				transport := &requestIDTransport{stream: stream, failure: scenario == "failure", omitID: scenario == "no_upstream_id"}
				client := service.GetHttpClient()
				previous := client.Transport
				client.Transport = transport
				t.Cleanup(func() { client.Transport = previous })
				r := gin.New()
				r.Use(middleware.RequestId(), middleware.RequestFailureLog(), middleware.RouteTag("relay"))
				var requestID string
				r.POST("/v1/responses", func(c *gin.Context) {
					requestID = c.GetString(common.RequestIdKey)
					common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
					common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
					common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
					common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "request-id-fixture"))
					defer common.CleanupBodyStorage(c)
					Relay(c, types.RelayFormatOpenAIResponses)
				})
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":"request-id-fixture","input":"test","stream":%t}`, stream)))
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(w, req)
				require.NotEmpty(t, requestID)
				require.Equal(t, []string{requestID}, w.Result().Header.Values(common.RequestIdKey))
				require.Equal(t, 1, transport.calls)
				charged, logType := 15, model.LogTypeConsume
				if transport.failure {
					charged, logType = 0, model.LogTypeError
					require.Equal(t, 400, w.Code, w.Body.String())
				} else {
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), "resp_body_id")
					require.Contains(t, w.Body.String(), "OK")
				}
				require.Eventually(t, func() bool {
					var u model.User
					var tok model.Token
					return db.First(&u, user.Id).Error == nil && db.First(&tok, token.Id).Error == nil &&
						u.Quota == initial-charged && u.UsedQuota == charged && tok.RemainQuota == initial-charged
				}, 2*time.Second, 10*time.Millisecond)
				var logs []model.Log
				require.Eventually(t, func() bool { return db.Find(&logs).Error == nil && len(logs) == 1 }, time.Second, time.Millisecond)
				require.Equal(t, requestID, logs[0].RequestId)
				require.Equal(t, logType, logs[0].Type)
				require.Equal(t, charged, logs[0].Quota)
				if !transport.failure {
					require.Equal(t, 10, logs[0].PromptTokens)
					require.Equal(t, 5, logs[0].CompletionTokens)
				}
				ids := gjson.Get(logs[0].Other, "admin_info.upstream_request_ids").Array()
				if transport.omitID {
					require.Empty(t, ids)
				} else {
					require.Len(t, ids, 1)
					require.Equal(t, "supplier-request-id", ids[0].Get("request_id").String())
					require.EqualValues(t, channel.Id, ids[0].Get("channel_id").Int())
					require.Equal(t, common.RequestIdKey, ids[0].Get("request_id_header").String())
				}
				public, total, err := model.GetUserLogs(user.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 10, "", requestID)
				require.NoError(t, err)
				require.EqualValues(t, 1, total)
				require.Equal(t, requestID, public[0].RequestId)
				require.NotContains(t, public[0].Other, "admin_info")
				require.NotContains(t, public[0].Other, "supplier-request-id")
			})
		}
	}
}
