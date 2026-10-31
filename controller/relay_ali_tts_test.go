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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAliTTSRelaySettlesConfirmedUsageOnSuccessAndFailure(t *testing.T) {
	service.InitTokenEncoders()
	service.InitHttpClient()
	for _, funding := range []string{"wallet", "subscription", "token"} {
		for _, scenario := range []string{"success", "partial", "no_usage"} {
			for _, duplex := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/duplex=%t", funding, scenario, duplex), func(t *testing.T) {
					db := useControllerCapacityTestDB(t)
					sqlDB, err := db.DB()
					require.NoError(t, err)
					sqlDB.SetMaxOpenConns(1)
					t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
					require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
					oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
					constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, 3, false, true, true
					t.Cleanup(func() {
						constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldError, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
					})
					oldRatio := ratio_setting.ModelRatio2JSONString()
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"voice-fixture":1}`))
					t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
					const initial = 1000000
					user := model.User{Id: 998601, Username: "tts-fixture", Quota: initial, Status: common.UserStatusEnabled}
					token := model.Token{Id: 998602, UserId: user.Id, Key: "fixture-tts-token", RemainQuota: initial, Status: common.TokenStatusEnabled}
					plan := model.SubscriptionPlan{Id: 998604, Title: "fixture", QuotaResetPeriod: "never"}
					sub := model.UserSubscription{Id: 998605, UserId: user.Id, PlanId: plan.Id, AmountTotal: initial, Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix()}
					for _, row := range []any{&user, &token, &plan, &sub} {
						require.NoError(t, db.Create(row).Error)
					}
					calls := make(chan struct{}, 4)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls <- struct{}{}
						u := websocket.Upgrader{}
						conn, err := u.Upgrade(w, r, nil)
						if err != nil {
							return
						}
						defer conn.Close()
						_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
						_, data, err := conn.ReadMessage()
						if err != nil {
							return
						}
						id := gjson.GetBytes(data, "header.task_id").String()
						write := func(event string, usage bool) {
							payload := map[string]any{}
							if usage {
								payload["usage"] = map[string]any{"characters": 7}
							}
							b, _ := common.Marshal(map[string]any{"header": map[string]any{"task_id": id, "event": event}, "payload": payload})
							_ = conn.WriteMessage(websocket.TextMessage, b)
						}
						write("task-started", false)
						if _, _, err = conn.ReadMessage(); err != nil {
							return
						}
						if _, _, err = conn.ReadMessage(); err != nil {
							return
						}
						_ = conn.WriteMessage(websocket.BinaryMessage, []byte("audio"))
						if scenario != "no_usage" {
							write("result-generated", true)
						}
						if scenario == "success" {
							write("task-finished", true)
						}
					}))
					defer upstream.Close()
					channel := model.Channel{Id: 998603, Type: constant.ChannelTypeAli, Status: common.ChannelStatusEnabled, Key: "fixture", BaseURL: common.GetPointer(upstream.URL), ModelMapping: common.GetPointer(`{"voice-fixture":"cosyvoice-v3-flash"}`)}
					require.NoError(t, db.Create(&channel).Error)
					r := gin.New()
					handled := make(chan struct{}, 1)
					// WebSocket hijacks escape httptest.Server.Close's wait. Wait for
					// the entire middleware chain before resetting shared test settings.
					r.Use(func(c *gin.Context) {
						defer func() { handled <- struct{}{} }()
						c.Next()
					}, middleware.RequestId(), middleware.RequestFailureLog(), middleware.RouteTag("relay"))
					requestID := ""
					handle := func(c *gin.Context) {
						requestID = c.GetString(common.RequestIdKey)
						common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
						common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
						common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
						common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
						pref := "wallet_only"
						if funding == "subscription" {
							pref = "subscription_only"
						}
						if funding == "token" {
							common.SetContextKey(c, constant.ContextKeyTokenBillingMode, model.TokenBillingModeTokenOnly)
						}
						common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: pref})
						require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "voice-fixture"))
						defer common.CleanupBodyStorage(c)
						format := types.RelayFormat(types.RelayFormatOpenAIAudio)
						if duplex {
							format = types.RelayFormatAliTTS
						}
						Relay(c, format)
					}
					r.POST("/v1/audio/speech", handle)
					r.GET("/v1/audio/speech/stream", handle)
					w := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"voice-fixture","input":"你好","voice":"saved","response_format":"pcm"}`))
					req.Header.Set("Content-Type", "application/json")
					if !duplex {
						r.ServeHTTP(w, req)
					} else {
						server := httptest.NewServer(r)
						defer server.Close()
						conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/audio/speech/stream?model=voice-fixture", nil)
						require.NoError(t, err)
						defer conn.Close()
						_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
						send := func(action string, payload map[string]any) {
							data, _ := common.Marshal(map[string]any{"header": map[string]any{"action": action, "task_id": "fixture-task", "streaming": "duplex"}, "payload": payload})
							require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
						}
						send("run-task", map[string]any{"task_group": "audio", "task": "tts", "function": "SpeechSynthesizer", "model": "voice-fixture", "parameters": map[string]any{"voice": "saved"}, "input": map[string]any{}})
						for {
							kind, data, err := conn.ReadMessage()
							if err != nil {
								break
							}
							if kind == websocket.BinaryMessage {
								_, _ = w.Body.Write(data)
								continue
							}
							switch gjson.GetBytes(data, "header.event").String() {
							case "task-started":
								send("continue-task", map[string]any{"input": map[string]any{"text": "你好"}})
								send("finish-task", map[string]any{"input": map[string]any{}})
							case "task-finished":
								_ = conn.Close()
							}
						}
						_ = conn.Close()
					}
					select {
					case <-handled:
					case <-time.After(6 * time.Second):
						t.Fatal("relay did not stop")
					}
					require.Equal(t, "audio", w.Body.String(), "never append JSON to partial audio")
					require.Len(t, calls, 1, "never replay submitted speech")
					charged := 7
					if scenario == "no_usage" {
						charged = 0
					}
					require.Eventually(t, func() bool {
						var u model.User
						var tok model.Token
						var subscription model.UserSubscription
						if db.First(&u, user.Id).Error != nil || db.First(&tok, token.Id).Error != nil || db.First(&subscription, sub.Id).Error != nil {
							return false
						}
						wallet := initial
						if funding == "wallet" {
							wallet -= charged
						}
						subscriptionUsed := int64(0)
						if funding == "subscription" {
							subscriptionUsed = int64(charged)
						}
						return u.Quota == wallet && tok.RemainQuota == initial-charged && subscription.AmountUsed == subscriptionUsed
					}, 3*time.Second, 10*time.Millisecond)
					var logs []model.Log
					require.NoError(t, db.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
					if charged == 0 {
						require.Empty(t, logs)
					} else {
						require.Len(t, logs, 1)
						require.Equal(t, charged, logs[0].Quota)
						require.Equal(t, 7, logs[0].PromptTokens)
						status := "ok"
						if scenario != "success" {
							status = "error"
						}
						require.Equal(t, status, gjson.Get(logs[0].Other, "stream_status.status").String())
					}
				})
			}
		}
	}
}
