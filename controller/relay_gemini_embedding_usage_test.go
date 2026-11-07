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
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRelayGeminiEmbeddingReportedUsageSettlement(t *testing.T) {
	for _, funding := range []string{service.BillingSourceWallet, service.BillingSourceSubscription, service.BillingSourceToken} {
		for _, mode := range []string{"compatible", "single", "batch"} {
			t.Run(funding+"/"+mode, func(t *testing.T) {
				response := `{"embeddings":[{"values":[0.1]}],"usageMetadata":{"promptTokenCount":37}}`
				if mode == "single" {
					response = `{"embedding":{"values":[0.1]},"usageMetadata":{"promptTokenCount":37}}`
				}
				f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, _ []byte) {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, response)
				})
				db := model.DB
				require.NoError(t, db.AutoMigrate(&model.Token{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
				oldBatch, oldConsume := common.BatchUpdateEnabled, common.LogConsumeEnabled
				common.BatchUpdateEnabled, common.LogConsumeEnabled = false, true
				t.Cleanup(func() { common.BatchUpdateEnabled, common.LogConsumeEnabled = oldBatch, oldConsume })
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gemini-embedding-test":1}`))
				const initial = 1000000
				user := &model.User{Id: 995801, Username: "embedding-usage", Quota: initial, Status: common.UserStatusEnabled}
				token := &model.Token{Id: 995802, UserId: user.Id, Key: "fixture-embedding-usage", Status: common.TokenStatusEnabled, RemainQuota: initial}
				plan := &model.SubscriptionPlan{Id: 995803, Title: "fixture", QuotaResetPeriod: "never"}
				sub := &model.UserSubscription{Id: 995804, UserId: user.Id, PlanId: plan.Id, AmountTotal: initial, Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix()}
				for _, row := range []any{user, token, plan, sub} {
					require.NoError(t, db.Create(row).Error)
				}
				f.channel.Type = constant.ChannelTypeGemini
				path, body, format := "/v1/embeddings", `{"model":"gemini-embedding-test","input":"hi"}`, types.RelayFormatEmbedding
				if mode != "compatible" {
					format = types.RelayFormatGemini
					path = "/v1beta/models/gemini-embedding-test:embedContent"
					body = `{"content":{"parts":[{"text":"hi"}]}}`
				}
				if mode == "batch" {
					path = "/v1beta/models/gemini-embedding-test:batchEmbedContents"
					body = `{"requests":[{"content":{"parts":[{"text":"hi"}]}}]}`
				}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				pref := "wallet_only"
				if funding == service.BillingSourceSubscription {
					pref = "subscription_only"
				}
				if funding == service.BillingSourceToken {
					common.SetContextKey(c, constant.ContextKeyTokenBillingMode, model.TokenBillingModeTokenOnly)
				}
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: pref})
				c.Set(common.RequestIdKey, "embedding-usage-"+funding+mode)
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, "gemini-embedding-test"))
				Relay(c, types.RelayFormat(format))
				common.CleanupBodyStorage(c)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				if mode == "compatible" {
					require.EqualValues(t, 37, gjson.GetBytes(w.Body.Bytes(), "usage.prompt_tokens").Int())
				} else {
					require.JSONEq(t, response, w.Body.String())
				}
				select {
				case request := <-f.requests:
					expected := "/v1beta/models/gemini-embedding-test:batchEmbedContents"
					if mode == "single" {
						expected = "/v1beta/models/gemini-embedding-test:embedContent"
					}
					require.Equal(t, expected, request.path)
				case <-time.After(time.Second):
					t.Fatal("missing upstream request")
				}
				require.Eventually(t, func() bool {
					var rows []model.Log
					if db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume).Find(&rows).Error != nil || len(rows) != 1 {
						return false
					}
					return rows[0].PromptTokens == 37 && rows[0].CompletionTokens == 0 && rows[0].Quota == 37
				}, 3*time.Second, 10*time.Millisecond)
				require.Eventually(t, func() bool {
					var u model.User
					var tk model.Token
					var s model.UserSubscription
					if db.First(&u, user.Id).Error != nil || db.First(&tk, token.Id).Error != nil || db.First(&s, sub.Id).Error != nil {
						return false
					}
					wallet, subscription := initial, 0
					if funding == service.BillingSourceWallet {
						wallet -= 37
					}
					if funding == service.BillingSourceSubscription {
						subscription = 37
					}
					return u.Quota == wallet && tk.RemainQuota == initial-37 && s.AmountUsed == int64(subscription)
				}, 3*time.Second, 10*time.Millisecond)
			})
		}
	}
}
