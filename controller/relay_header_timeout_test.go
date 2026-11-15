package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRelayResponseHeaderTimeoutFailover(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, tc := range []struct {
		name                                     string
		retries                                  int
		ping, allFail, fixed, restricted, native bool
		fallback500                              bool
		calls                                    int
		success                                  bool
	}{
		{name: "chat_to_claude", retries: 1, calls: 2, success: true},
		{name: "chat_heartbeat", retries: 1, ping: true, calls: 2, success: true},
		{name: "messages_heartbeat", retries: 1, ping: true, native: true, calls: 2, success: true},
		{name: "both_timeout_bounded", retries: 5, allFail: true, calls: 2},
		{name: "fallback_500_still_bounded", retries: 5, fallback500: true, calls: 2},
		{name: "disabled", calls: 1},
		{name: "fixed", retries: 1, fixed: true, calls: 1},
		{name: "whitelist", retries: 1, restricted: true, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := useControllerCapacityTestDB(t)
			sqlDB, e := db.DB()
			require.NoError(t, e)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
			oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
			constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, tc.retries, false, true, true
			t.Cleanup(func() {
				constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
			})
			general := operation_setting.GetGeneralSetting()
			oldGeneral := *general
			general.PingIntervalEnabled = tc.ping
			general.PingIntervalSeconds = 1
			t.Cleanup(func() { *general = oldGeneral })
			oldRatio := ratio_setting.ModelRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-timeout-fixture":1}`))
			t.Cleanup(func() { ratio_setting.UpdateModelRatioByJSONString(oldRatio) })
			client := service.GetHttpClient()
			oldTransport := client.Transport
			transport := oldTransport.(*http.Transport).Clone()
			transport.ResponseHeaderTimeout = 40 * time.Millisecond
			if tc.ping {
				transport.ResponseHeaderTimeout = 1100 * time.Millisecond
			}
			client.Transport = transport
			t.Cleanup(func() { client.Transport = oldTransport; transport.CloseIdleConnections() })
			const initial = 1000000
			user := &model.User{Id: 998701, Username: "timeout-fixture", Quota: initial, Status: common.UserStatusEnabled}
			token := &model.Token{Id: 998702, UserId: user.Id, Key: "timeout-fixture-token", RemainQuota: initial, Status: common.TokenStatusEnabled}
			require.NoError(t, db.Create(user).Error)
			require.NoError(t, db.Create(token).Error)
			var calls atomic.Int32
			bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
				}
			}))
			t.Cleanup(bad.Close)
			good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				require.Contains(t, string(body), "look at screenshots")
				if tc.fallback500 {
					w.WriteHeader(500)
					fmt.Fprint(w, `{"error":{"type":"api_error","message":"fixture unavailable"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: "+`{"type":"message_start","message":{"id":"msg_fixture","model":"claude-timeout-fixture","usage":{"input_tokens":100,"output_tokens":1}}}`+"\n\n"+"data: "+`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}`+"\n\n"+"data: "+`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}`+"\n\ndata: "+`{"type":"message_stop"}`+"\n\n")
			}))
			t.Cleanup(good.Close)
			channels := []*model.Channel{}
			for i := 0; i < 3; i++ {
				base := good.URL
				if i == 0 || tc.allFail {
					base = bad.URL
				}
				ch := &model.Channel{Id: 998703 + i, Name: "timeout-fixture", Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "fixture", BaseURL: &base, Group: "default", Models: "claude-timeout-fixture", AutoBan: common.GetPointer(0)}
				channels = append(channels, ch)
				require.NoError(t, db.Create(ch).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: ch.Models, ChannelId: ch.Id, Enabled: true, Priority: common.GetPointer(int64(1)), Weight: 1}).Error)
			}
			path := "/v1/chat/completions"
			format := types.RelayFormat(types.RelayFormatOpenAI)
			if tc.native {
				path = "/v1/messages"
				format = types.RelayFormatClaude
			}
			r := gin.New()
			r.Use(middleware.RequestFailureLog(), middleware.RouteTag("relay"))
			r.POST(path, func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				c.Set(common.RequestIdKey, "header-timeout-"+tc.name)
				if tc.fixed {
					c.Set("specific_channel_id", channels[0].Id)
				}
				if tc.restricted {
					common.SetContextKey(c, constant.ContextKeyTokenChannelLimitEnabled, true)
					common.SetContextKey(c, constant.ContextKeyTokenChannelLimit, map[int]bool{channels[0].Id: true})
				}
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, channels[0], "claude-timeout-fixture"))
				Relay(c, format)
				require.Equal(t, tc.success, service.RequestSucceeded(c))
			})
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"claude-timeout-fixture","max_tokens":100,"messages":[{"role":"user","content":"look at screenshots"}],"stream":true}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.EqualValues(t, tc.calls, calls.Load(), w.Body.String())
			var logs []model.Log
			require.Eventually(t, func() bool {
				return db.Where("request_id = ?", "header-timeout-"+tc.name).Find(&logs).Error == nil && len(logs) == 1
			}, time.Second, time.Millisecond)
			log := logs[0]
			var details map[string]interface{}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &details))
			attempts := details["admin_info"].(map[string]interface{})["attempts"].([]interface{})
			require.EqualValues(t, 504, attempts[0].(map[string]interface{})["status_code"])
			if tc.ping {
				require.Equal(t, 200, w.Code)
				if tc.native {
					require.Contains(t, w.Body.String(), "ping")
				} else {
					require.Contains(t, w.Body.String(), ": PING")
				}
			}
			if tc.success {
				require.Equal(t, model.LogTypeConsume, log.Type)
				require.Positive(t, log.Quota)
				require.Contains(t, w.Body.String(), "OK")
				require.NotContains(t, w.Body.String(), "upstream response header timeout")
				require.Len(t, attempts, 1)
			} else {
				require.Equal(t, model.LogTypeError, log.Type)
				require.Zero(t, log.Quota)
				if tc.fallback500 {
					require.Contains(t, w.Body.String(), "fixture unavailable")
					require.EqualValues(t, 500, details["status_code"])
				} else {
					require.Contains(t, w.Body.String(), "upstream response header timeout")
					require.EqualValues(t, 504, details["status_code"])
				}
			}
			require.Eventually(t, func() bool {
				var u model.User
				var tok model.Token
				return db.First(&u, user.Id).Error == nil && db.First(&tok, token.Id).Error == nil && u.Quota == initial-log.Quota && tok.RemainQuota == initial-log.Quota
			}, time.Second, time.Millisecond)
		})
	}
}
