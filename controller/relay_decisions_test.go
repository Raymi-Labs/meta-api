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
func TestRelayDecisionsBilling(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql"} {
		t.Run(dialect, func(t *testing.T) {
			var db *gorm.DB
			if dialect == "mysql" {
				dsn := os.Getenv("TEST_DECISIONS_MYSQL_DSN")
				if dsn == "" {
					t.Skip("disposable TEST_DECISIONS_MYSQL_DSN not configured")
				}
				require.Contains(t, dsn, "/jev_decisions_test")
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
			common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.MemoryCacheEnabled = false, true, 3, false, true, false, false
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
				{ratio_setting.ModelRatio2JSONString, ratio_setting.UpdateModelRatioByJSONString, `{"jev-billing-fixture":1}`},
				{ratio_setting.CompletionRatio2JSONString, ratio_setting.UpdateCompletionRatioByJSONString, `{"jev-billing-fixture":0}`},
			} {
				old := config.get()
				setter := config.set
				require.NoError(t, setter(config.value))
				t.Cleanup(func() { require.NoError(t, setter(old)) })
			}
			const initial = 1000000
			user := model.User{Id: 995801, Username: "jev-billing-fixture", Quota: initial, Status: common.UserStatusEnabled}
			token := model.Token{Id: 995802, UserId: user.Id, Key: "fixture-jev", Status: common.TokenStatusEnabled, RemainQuota: initial}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Create(&token).Error)
			var sends atomic.Int32
			var mode atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				require.Equal(t, "/alpha/decisions", r.URL.Path)
				body, _ := io.ReadAll(r.Body)
				require.Equal(t, "typesafe/jev-1.14", gjson.GetBytes(body, "model").String())
				require.Equal(t, 0.042, gjson.GetBytes(body, "provider.max_price.prompt").Float())
				require.Equal(t, "Hello", gjson.GetBytes(body, "state").String())
				w.Header().Set("Content-Type", "application/json")
				switch mode.Load() {
				case 1:
					fmt.Fprint(w, `{"model":"jev","answers":{"q":{"type":"noul","noul":0.99}}}`)
				case 2:
					w.WriteHeader(503)
					fmt.Fprint(w, `{"error":{"message":"unavailable","type":"server_error"}}`)
				default:
					fmt.Fprint(w, `{"model":"typesafe/jev-1.14-20261014","answers":{"q":{"type":"noul","noul":0.99}},"usage":{"input_tokens":272,"output_tokens":22}}`)
				}
			}))
			defer server.Close()
			mapping := `{"jev-billing-fixture":"typesafe/jev-1.14"}`
			policy := `{"provider":{"max_price":{"prompt":0.042,"completion":0}}}`
			ch := model.Channel{Id: 995803, Name: "jev-test", Type: constant.ChannelTypeOpenRouter, Status: 1, Key: "fixture", BaseURL: &server.URL, Models: "jev-billing-fixture", ModelMapping: &mapping, ParamOverride: &policy, AutoBan: common.GetPointer(0)}
			require.NoError(t, db.Create(&ch).Error)
			for _, m := range []int32{0, 1, 2, 3, 4, 5} {
				mode.Store(m)
				before := sends.Load()
				body := `{"model":"jev-billing-fixture","state":"Hello","questions":{"q":{"type":"noul","instructions":"Is this a greeting?"}}}`
				if m == 3 {
					body = strings.Replace(body, `"state":`, `"stream":true,"state":`, 1)
				}
				if m == 4 {
					body = `null`
				}
				if m == 5 {
					ch.Type = constant.ChannelTypeOpenAI
				}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("POST", "/v1/decisions", strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				c.Set(common.RequestIdKey, fmt.Sprintf("jev-%s-%d", dialect, m))
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &ch, "jev-billing-fixture"))
				Relay(c, types.RelayFormatDecisions)
				common.CleanupBodyStorage(c)
				if m == 0 {
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Equal(t, 0.99, gjson.Get(w.Body.String(), "answers.q.noul").Float())
				} else {
					require.GreaterOrEqual(t, w.Code, 400, w.Body.String())
				}
				wantSends := int32(1)
				if m >= 3 {
					wantSends = 0
				}
				require.Equal(t, wantSends, sends.Load()-before, "decisions must not be resubmitted")
				require.Eventually(t, func() bool {
					var u model.User
					var tk model.Token
					return db.First(&u, user.Id).Error == nil && db.First(&tk, token.Id).Error == nil && u.Quota == initial-272 && tk.RemainQuota == initial-272 && u.UsedQuota == 272 && tk.UsedQuota == 272
				}, 3*time.Second, 10*time.Millisecond, "actual input charged once, output free, failures refunded")
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, 272, logs[0].Quota)
			require.Equal(t, 22, logs[0].CompletionTokens)
		})
	}
}

func TestJevChannelTestUsesDecisions(t *testing.T) {
	ch := &model.Channel{Type: constant.ChannelTypeOpenRouter}
	execution := buildChannelStyleTestExecution(ch, "typesafe/jev-1.14", "", nil)
	require.Equal(t, "/v1/decisions", execution.requestPath)
	require.Equal(t, types.RelayFormat(types.RelayFormatDecisions), execution.relayFormat)
	require.False(t, execution.isStream)
	require.IsType(t, &dto.DecisionsRequest{}, execution.request)
}
