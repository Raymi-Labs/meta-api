package controller

import (
	"fmt"
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
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Verify the real Messages conversion, usage parser, wallet/token settlement and
// log statistics together. HTTP responses and all database rows are local fixtures.
func TestMessagesInclusiveCacheAcrossUpstreams(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, responses := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, tiered := range []bool{false, true} {
				t.Run(fmt.Sprintf("responses=%t/stream=%t/tiered=%t", responses, stream, tiered), func(t *testing.T) {
					db := useControllerCapacityTestDB(t)
					sqlDB, err := db.DB()
					require.NoError(t, err)
					sqlDB.SetMaxOpenConns(1)
					t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
					require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
					oldRedis, oldCount, oldBatch, oldConsume, oldSQLite := common.RedisEnabled, constant.CountToken, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
					common.RedisEnabled, constant.CountToken, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = false, false, false, true, true
					t.Cleanup(func() {
						common.RedisEnabled, constant.CountToken, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldRedis, oldCount, oldBatch, oldConsume, oldSQLite
					})
					mr, cr, outr := ratio_setting.ModelRatio2JSONString(), ratio_setting.CacheRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
					bundle := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"messages-cache-fixture":1}`))
					require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"messages-cache-fixture":0.1}`))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"messages-cache-fixture":1}`))
					billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{}, BillingExpr: map[string]string{}})
					if tiered {
						billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{"messages-cache-fixture": "tiered_expr"},
							BillingExpr: map[string]string{"messages-cache-fixture": `tier("base", p*2+c*2+cr*0.2)`}})
					}
					t.Cleanup(func() {
						require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(mr))
						require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(cr))
						require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(outr))
						billing_setting.ReplaceBundle(bundle)
					})
					const initial = 1000000
					user := model.User{Id: 995711, Username: "messages-cache-fixture", Quota: initial, Status: common.UserStatusEnabled}
					token := model.Token{Id: 995712, UserId: user.Id, Key: "messages-cache-fixture", RemainQuota: initial, Status: common.TokenStatusEnabled}
					channel := model.Channel{Id: 995713, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "fixture"}
					mode := dto.ChatCompletionsToResponsesModeDisabled
					if responses {
						mode = dto.ChatCompletionsToResponsesModeEnabled
					}
					channel.SetSetting(dto.ChannelSettings{ChatCompletionsToResponsesMode: mode})
					for _, row := range []any{&user, &token, &channel} {
						require.NoError(t, db.Create(row).Error)
					}
					body := `{"model":"messages-cache-fixture","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":20,"total_tokens":1020,"prompt_tokens_details":{"cached_tokens":800}}}`
					transport := &kimiUsageBillingTransport{body: body, contentType: "application/json"}
					if responses {
						body = `{"id":"resp_fixture","object":"response","status":"completed","model":"messages-cache-fixture","output":[{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1000,"output_tokens":20,"total_tokens":1020,"input_tokens_details":{"cached_tokens":800}}}`
						transport.body = body
					}
					if stream {
						transport.contentType = "text/event-stream"
						if responses {
							transport.body = "event: response.created\ndata: " + `{"type":"response.created","response":{"id":"resp_fixture","status":"in_progress"}}` + "\n\n" +
								"event: response.output_text.delta\ndata: " + `{"type":"response.output_text.delta","item_id":"msg_fixture","delta":"OK"}` + "\n\n" +
								"event: response.completed\ndata: " + `{"type":"response.completed","response":` + body + "}\n\n"
						} else {
							transport.body = "data: " + strings.Replace(body, `"message":{"role":"assistant","content":"OK"}`, `"delta":{"content":"OK"}`, 1) + "\n\ndata: [DONE]\n\n"
						}
					}
					client := service.GetHttpClient()
					previousTransport := client.Transport
					client.Transport = transport
					t.Cleanup(func() { client.Transport = previousTransport })
					w := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(w)
					ctx.Request = httptest.NewRequest("POST", "/v1/messages", strings.NewReader(fmt.Sprintf(`{"model":"messages-cache-fixture","max_tokens":256,"messages":[{"role":"user","content":"OK"}],"stream":%t}`, stream)))
					ctx.Request.Header.Set("Content-Type", "application/json")
					requestID := "messages-inclusive-cache-fixture"
					ctx.Set(common.RequestIdKey, requestID)
					common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
					common.SetContextKey(ctx, constant.ContextKeyTokenId, token.Id)
					common.SetContextKey(ctx, constant.ContextKeyTokenKey, token.Key)
					common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
					common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
					require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, &channel, "messages-cache-fixture"))
					Relay(ctx, types.RelayFormatClaude)
					common.CleanupBodyStorage(ctx)
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Equal(t, 1, transport.calls)
					require.Eventually(t, func() bool {
						var u model.User
						var tok model.Token
						return db.First(&u, user.Id).Error == nil && db.First(&tok, token.Id).Error == nil &&
							u.Quota == initial-300 && tok.RemainQuota == initial-300 && u.UsedQuota == 300
					}, 2*time.Second, 10*time.Millisecond)
					var logs []model.Log
					require.NoError(t, db.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					require.Equal(t, 1000, logs[0].PromptTokens)
					require.Equal(t, 300, logs[0].Quota)
					require.Equal(t, "inclusive", gjson.Get(logs[0].Other, "prompt_tokens_semantic").String())
					require.False(t, gjson.Get(logs[0].Other, "claude").Bool())
					stat, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 0, "", requestID, false)
					require.NoError(t, err)
					require.InDelta(t, 0.8, stat.CacheGlobalRate, 1e-12)
				})
			}
		}
	}
}
