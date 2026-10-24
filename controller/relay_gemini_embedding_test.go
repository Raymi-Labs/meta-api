package controller

import (
	"fmt"
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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestRelayGeminiEmbeddingInvalidInputNeverSendsOrCharges(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, funding := range []string{service.BillingSourceWallet, service.BillingSourceSubscription, service.BillingSourceToken} {
		t.Run(funding, func(t *testing.T) {
			db := useControllerCapacityTestDB(t)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.Token{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
			oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
			constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = false, false, false, 3, false, true, true
			t.Cleanup(func() {
				constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
			})
			oldRatio := ratio_setting.ModelRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gemini-embedding-test":1}`))
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
			const initialQuota = 1000000
			user := &model.User{Id: 995701, Username: "embedding-rejection-fixture", Quota: initialQuota, Status: common.UserStatusEnabled}
			token := &model.Token{Id: 995702, UserId: user.Id, Key: "fixture-embedding-rejection", Status: common.TokenStatusEnabled, RemainQuota: initialQuota}
			plan := &model.SubscriptionPlan{Id: 995703, Title: "fixture", QuotaResetPeriod: "never"}
			sub := &model.UserSubscription{Id: 995704, UserId: user.Id, PlanId: plan.Id, AmountTotal: initialQuota, Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix()}
			for _, row := range []any{user, token, plan, sub} {
				require.NoError(t, db.Create(row).Error)
			}
			var precharges atomic.Int32
			require.NoError(t, db.Callback().Update().After("gorm:update").Register("embedding_test:observe_precharge", func(tx *gorm.DB) {
				if tx.Error == nil && tx.RowsAffected > 0 && tx.Statement.Table == "tokens" && strings.Contains(tx.Statement.SQL.String(), "remain_quota -") {
					precharges.Add(1)
				}
			}))
			var upstreamCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"embeddings":[{"values":[0.1]}]}`)
			}))
			t.Cleanup(server.Close)
			channel := &model.Channel{Id: 995705, Name: "embedding-fixture", Type: constant.ChannelTypeGemini, Status: common.ChannelStatusEnabled, Key: "fixture", BaseURL: &server.URL, AutoBan: common.GetPointer(0)}
			require.NoError(t, db.Create(channel).Error)
			for i, input := range []string{`["valid",{"type":"text","text":"dropped"}]`, `["valid",42]`, `{"type":"text","text":"text"}`, `[1,2]`, `""`, `[]`, `["valid",""]`, `null`} {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"gemini-embedding-test","input":`+input+`}`))
				c.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				c.Set(common.RequestIdKey, fmt.Sprintf("embedding-rejection-%s-%d", funding, i))
				pref := "wallet_only"
				if funding == service.BillingSourceSubscription {
					pref = "subscription_only"
				}
				if funding == service.BillingSourceToken {
					common.SetContextKey(c, constant.ContextKeyTokenBillingMode, model.TokenBillingModeTokenOnly)
				}
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: pref})
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gemini-embedding-test"))
				Relay(c, types.RelayFormatEmbedding)
				common.CleanupBodyStorage(c)
				require.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", input, w.Body.String())
				require.Equal(t, string(types.ErrorCodeInvalidRequest), gjson.GetBytes(w.Body.Bytes(), "error.code").String())
				require.Zero(t, upstreamCalls.Load(), "invalid input must never reach the HTTP upstream")
				require.Eventually(t, func() bool {
					var actualUser model.User
					var actualToken model.Token
					var actualSub model.UserSubscription
					if db.First(&actualUser, user.Id).Error != nil || db.First(&actualToken, token.Id).Error != nil || db.First(&actualSub, sub.Id).Error != nil {
						return false
					}
					return actualUser.Quota == initialQuota && actualUser.UsedQuota == 0 && actualToken.RemainQuota == initialQuota && actualToken.UsedQuota == 0 && actualSub.AmountUsed == 0
				}, 3*time.Second, 10*time.Millisecond, input)
				var count int64
				require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&count).Error)
				require.Zero(t, count)
			}
			require.Positive(t, precharges.Load(), "the fixture must exercise the existing precharge/refund path")
		})
	}
}

func TestRelayGeminiEmbeddingValidTextPreservesPayload(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, upstreamPath, response string
		format                                   types.RelayFormat
	}{
		{"compatible_string", "/v1/embeddings", `{"model":"gemini-embedding-001","input":"first","dimensions":128}`, "/v1beta/models/gemini-embedding-001:batchEmbedContents", `{"embeddings":[{"values":[0.1]}]}`, types.RelayFormatEmbedding},
		{"compatible_array", "/v1/embeddings", `{"model":"gemini-embedding-001","input":["first"," second ","first"],"dimensions":128}`, "/v1beta/models/gemini-embedding-001:batchEmbedContents", `{"embeddings":[{"values":[0.1]},{"values":[0.2]},{"values":[0.3]}]}`, types.RelayFormatEmbedding},
		{"native_single", "/v1beta/models/gemini-embedding-001:embedContent", `{"model":"gemini-embedding-001","content":{"parts":[{"text":"native"}]},"taskType":"RETRIEVAL_QUERY","outputDimensionality":128}`, "/v1beta/models/gemini-embedding-001:embedContent", `{"embedding":{"values":[0.4]}}`, types.RelayFormatGemini},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, _ []byte) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.response)
			})
			f.channel.Type = constant.ChannelTypeGemini
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gemini-embedding-001":0}`))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("specific_channel_id", f.channel.Id)
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, "gemini-embedding-001"))
			defer common.CleanupBodyStorage(c)
			Relay(c, tc.format)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			select {
			case sent := <-f.requests:
				require.Equal(t, tc.upstreamPath, sent.path)
				if tc.format == types.RelayFormatEmbedding {
					require.Equal(t, "first", gjson.GetBytes(sent.body, "requests.0.content.parts.0.text").String())
					require.EqualValues(t, 128, gjson.GetBytes(sent.body, "requests.0.outputDimensionality").Int())
					wantCount := 1
					if tc.name == "compatible_array" {
						wantCount = 3
						require.Equal(t, " second ", gjson.GetBytes(sent.body, "requests.1.content.parts.0.text").String())
						require.Equal(t, "first", gjson.GetBytes(sent.body, "requests.2.content.parts.0.text").String())
					}
					require.Len(t, gjson.GetBytes(sent.body, "requests").Array(), wantCount)
					require.Len(t, gjson.GetBytes(w.Body.Bytes(), "data").Array(), wantCount)
				} else {
					require.Equal(t, "native", gjson.GetBytes(sent.body, "content.parts.0.text").String())
					require.Equal(t, "RETRIEVAL_QUERY", gjson.GetBytes(sent.body, "taskType").String())
					require.EqualValues(t, 128, gjson.GetBytes(sent.body, "outputDimensionality").Int())
				}
			case <-time.After(time.Second):
				t.Fatal("valid embedding request did not reach the local HTTP upstream")
			}
			require.Empty(t, f.requests)
		})
	}
}
