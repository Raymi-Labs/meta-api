package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// The MySQL DSN is an explicitly disposable database, optionally loaded with the
// production channels/abilities/models/options schema (no production data).
func TestRelayOpenRouterAudioBilling(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql"} {
		t.Run(dialect, func(t *testing.T) {
			var db *gorm.DB
			if dialect == "mysql" {
				dsn := os.Getenv("TEST_OPENROUTER_MYSQL_DSN")
				if dsn == "" {
					t.Skip("disposable TEST_OPENROUTER_MYSQL_DSN not configured")
				}
				require.Contains(t, dsn, "/openrouter_audio_test")
				var err error
				db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
				require.NoError(t, err)
				oldDB, oldLog := model.DB, model.LOG_DB
				model.DB, model.LOG_DB = db, db
				t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLog })
			} else {
				db = useControllerCapacityTestDB(t)
			}
			oldSQLite, oldMySQL := common.UsingSQLite, common.UsingMySQL
			common.UsingSQLite, common.UsingMySQL = dialect == "sqlite", dialect == "mysql"
			t.Cleanup(func() { common.UsingSQLite, common.UsingMySQL = oldSQLite, oldMySQL })
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Option{}, &model.Model{}, &model.Log{}, &model.User{}, &model.Token{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldError, oldMemory := common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.MemoryCacheEnabled
			common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.MemoryCacheEnabled = false, false, 3, false, true, false, false
			t.Cleanup(func() {
				common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.MemoryCacheEnabled = oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldError, oldMemory
			})
			service.InitTokenEncoders()
			if service.GetHttpClient() == nil {
				service.InitHttpClient()
			}
			for _, config := range []struct {
				get   func() string
				set   func(string) error
				value string
			}{
				{ratio_setting.ModelRatio2JSONString, ratio_setting.UpdateModelRatioByJSONString, `{"gemini-tts-fixture":1,"gemini-stt-fixture":1}`},
				{ratio_setting.CompletionRatio2JSONString, ratio_setting.UpdateCompletionRatioByJSONString, `{"gemini-stt-fixture":6}`},
				{ratio_setting.AudioRatio2JSONString, ratio_setting.UpdateAudioRatioByJSONString, `{"gemini-tts-fixture":1,"gemini-stt-fixture":1}`},
				{ratio_setting.AudioCompletionRatio2JSONString, ratio_setting.UpdateAudioCompletionRatioByJSONString, `{"gemini-tts-fixture":12}`},
			} {
				old := config.get()
				setter := config.set
				require.NoError(t, setter(config.value))
				t.Cleanup(func() { require.NoError(t, setter(old)) })
			}
			const initial = 1000000
			user := model.User{Id: 995801, Username: "openrouter-audio-fixture", Quota: initial, Status: common.UserStatusEnabled}
			token := model.Token{Id: 995802, UserId: user.Id, Key: "fixture-audio", Status: common.TokenStatusEnabled, RemainQuota: initial}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Create(&token).Error)
			var sends atomic.Int32
			var mode atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/endpoints"):
					price := "0.000006"
					if mode.Load() == 1 {
						price = "0.000007"
					}
					fmt.Fprintf(w, `{"data":{"endpoints":[{"tag":"google-ai-studio","pricing":{"prompt":"0.0000005","completion":"%s"}}]}}`, price)
				case r.URL.Path == "/v1/generation":
					if mode.Load() == 2 {
						w.WriteHeader(503)
						return
					}
					fmt.Fprint(w, `{"data":{"id":"gen-fixture","native_tokens_prompt":7,"native_tokens_completion":79}}`)
				case r.URL.Path == "/v1/audio/speech":
					sends.Add(1)
					b, _ := io.ReadAll(r.Body)
					require.Equal(t, "google/gemini-3.8-flash-lite-tts", gjson.GetBytes(b, "model").String())
					require.False(t, gjson.GetBytes(b, "provider.max_price").Exists())
					require.Equal(t, "application/json", r.Header.Get("Content-Type"))
					if mode.Load() == 3 {
						w.WriteHeader(503)
						fmt.Fprint(w, `{"error":{"message":"unavailable","type":"server_error"}}`)
						return
					}
					w.Header().Set("Content-Type", "audio/pcm")
					w.Header().Set("X-Generation-Id", "gen-fixture")
					fmt.Fprint(w, "PCM fixture")
				case r.URL.Path == "/v1/audio/transcriptions":
					sends.Add(1)
					b, _ := io.ReadAll(r.Body)
					require.Equal(t, "google/gemini-3.5-transcribe", gjson.GetBytes(b, "model").String())
					require.Equal(t, "UklGRg==", gjson.GetBytes(b, "input_audio.data").String())
					fmt.Fprint(w, `{"text":"Hello.","usage":{"input_tokens":29,"output_tokens":2}}`)
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			mapping := `{"gemini-tts-fixture":"google/gemini-3.8-flash-lite-tts","gemini-stt-fixture":"google/gemini-3.5-transcribe"}`
			policy := `{"provider":{"only":["google-ai-studio"],"max_price":{"prompt":0.5,"completion":6}}}`
			ch := model.Channel{Id: 995803, Name: "audio-test", Type: constant.ChannelTypeOpenRouter, Status: 1, Key: "fixture", BaseURL: &server.URL, Models: "gemini-tts-fixture", ModelMapping: &mapping, ParamOverride: &policy, AutoBan: common.GetPointer(0)}
			require.NoError(t, db.Create(&ch).Error)
			for _, m := range []int32{0, 1, 2, 3, 4, 5} {
				mode.Store(m)
				before := sends.Load()
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("POST", "/v1/audio/speech", strings.NewReader(`{"model":"gemini-tts-fixture","input":"Hello","voice":"Kore"}`))
				modelName := "gemini-tts-fixture"
				wantQuota := 955
				if m >= 4 {
					constant.CountToken = true
					modelName = "gemini-stt-fixture"
					wantQuota += 41 // 29 audio input + 2 text output * 6.
					payload := `{"model":"gemini-stt-fixture","input_audio":{"data":"UklGRg==","format":"wav"}}`
					if m == 5 {
						payload = `{"model":"gemini-stt-fixture","input_audio":{"data":"not-base64","format":"wav"}}`
					}
					c.Request = httptest.NewRequest("POST", "/v1/audio/transcriptions", strings.NewReader(payload))
				}
				c.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				c.Set(common.RequestIdKey, fmt.Sprintf("audio-%s-%d", dialect, m))
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &ch, modelName))
				Relay(c, types.RelayFormatOpenAIAudio)
				common.CleanupBodyStorage(c)
				if m == 0 || m == 4 {
					require.Equal(t, 200, w.Code, w.Body.String())
					if m == 0 {
						require.Equal(t, "PCM fixture", w.Body.String())
					} else {
						require.Equal(t, "Hello.", gjson.Get(w.Body.String(), "text").String())
					}
				} else {
					require.GreaterOrEqual(t, w.Code, 400, w.Body.String())
				}
				wantSends := int32(1)
				if m == 1 || m == 5 {
					wantSends = 0
				}
				require.Equal(t, wantSends, sends.Load()-before, "failure must not resubmit billable audio")
				require.Eventually(t, func() bool {
					var u model.User
					var tk model.Token
					if db.First(&u, user.Id).Error != nil || db.First(&tk, token.Id).Error != nil {
						return false
					}
					return u.Quota == initial-wantQuota && u.UsedQuota == wantQuota && tk.RemainQuota == initial-wantQuota && tk.UsedQuota == wantQuota
				}, 3*time.Second, 10*time.Millisecond, "actual tokens charged once; failures refunded")
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 2)
			require.Equal(t, 955, logs[0].Quota)
			require.Equal(t, 7, logs[0].PromptTokens)
			require.Equal(t, 79, logs[0].CompletionTokens)
			require.Equal(t, 41, logs[1].Quota)
			require.Equal(t, 29, logs[1].PromptTokens)
			require.Equal(t, 2, logs[1].CompletionTokens)
		})
	}
}
