package controller

import (
	"fmt"
	"net/http/httptest"
	"os"

	mysqldsn "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
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

// Verify native OpenRouter Chat/Responses parsing, wallet/token settlement and
// log statistics together. HTTP responses and all database rows are local fixtures.
func TestOpenRouterCacheWritesAcrossUpstreams(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, responses := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, tiered := range []bool{false, true} {
				t.Run(fmt.Sprintf("responses=%t/stream=%t/tiered=%t", responses, stream, tiered), func(t *testing.T) {
					db := openAICacheWriteTestDB(t)
					sqlDB, err := db.DB()
					require.NoError(t, err)
					sqlDB.SetMaxOpenConns(1)
					t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
					require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
					oldRedis, oldCount, oldBatch, oldConsume := common.RedisEnabled, constant.CountToken, common.BatchUpdateEnabled, common.LogConsumeEnabled
					common.RedisEnabled, constant.CountToken, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, false, true
					t.Cleanup(func() {
						common.RedisEnabled, constant.CountToken, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldRedis, oldCount, oldBatch, oldConsume
					})
					cc := ratio_setting.CreateCacheRatio2JSONString()
					require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString(`{"openrouter-write-fixture":1.25}`))
					mr, cr, outr := ratio_setting.ModelRatio2JSONString(), ratio_setting.CacheRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
					bundle := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"openrouter-write-fixture":1}`))
					require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"openrouter-write-fixture":0.1}`))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"openrouter-write-fixture":1}`))
					billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{}, BillingExpr: map[string]string{}})
					if tiered {
						billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{"openrouter-write-fixture": "tiered_expr"},
							BillingExpr: map[string]string{"openrouter-write-fixture": `tier("base", p*2+c*2+cr*0.2+cc*2.5)`}})
					}
					t.Cleanup(func() {
						require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString(cc))
						require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(mr))
						require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(cr))
						require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(outr))
						billing_setting.ReplaceBundle(bundle)
					})
					const initial = 1000000
					user := model.User{Id: 995811, Username: "openrouter-write-fixture", Quota: initial, Status: common.UserStatusEnabled}
					token := model.Token{Id: 995812, UserId: user.Id, Key: "openrouter-write-fixture", RemainQuota: initial, Status: common.TokenStatusEnabled}
					channel := model.Channel{Id: 995813, Type: constant.ChannelTypeOpenRouter, Status: common.ChannelStatusEnabled, Key: "fixture"}
					for _, row := range []any{&user, &token, &channel} {
						require.NoError(t, db.Create(row).Error)
					}
					body := `{"model":"openrouter-write-fixture","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":20,"total_tokens":1020,"prompt_tokens_details":{"cached_tokens":800,"cache_write_tokens":100}}}`
					transport := &kimiUsageBillingTransport{body: body, contentType: "application/json"}
					if responses {
						body = `{"id":"resp_fixture","object":"response","status":"completed","model":"openrouter-write-fixture","output":[{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1000,"output_tokens":20,"total_tokens":1020,"input_tokens_details":{"cached_tokens":800,"cache_write_tokens":100}}}`
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
					format := types.RelayFormatOpenAI
					path := "/v1/chat/completions"
					requestBody := fmt.Sprintf(`{"model":"openrouter-write-fixture","max_tokens":256,"messages":[{"role":"user","content":"OK"}],"stream":%t}`, stream)
					if responses {
						format = types.RelayFormatOpenAIResponses
						path = "/v1/responses"
						requestBody = fmt.Sprintf(`{"model":"openrouter-write-fixture","max_output_tokens":256,"input":"OK","stream":%t}`, stream)
					}
					ctx.Request = httptest.NewRequest("POST", path, strings.NewReader(requestBody))
					ctx.Request.Header.Set("Content-Type", "application/json")
					requestID := "openrouter-cache-write-fixture"
					ctx.Set(common.RequestIdKey, requestID)
					common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
					common.SetContextKey(ctx, constant.ContextKeyTokenId, token.Id)
					common.SetContextKey(ctx, constant.ContextKeyTokenKey, token.Key)
					common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
					common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
					require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, &channel, "openrouter-write-fixture"))
					Relay(ctx, format)
					common.CleanupBodyStorage(ctx)
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Equal(t, 1, transport.calls)
					require.Eventually(t, func() bool {
						var u model.User
						var tok model.Token
						return db.First(&u, user.Id).Error == nil && db.First(&tok, token.Id).Error == nil &&
							u.Quota == initial-325 && tok.RemainQuota == initial-325 && u.UsedQuota == 325
					}, 2*time.Second, 10*time.Millisecond)
					var logs []model.Log
					require.NoError(t, db.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					require.Equal(t, 1000, logs[0].PromptTokens)
					require.Equal(t, 325, logs[0].Quota)
					require.Equal(t, 100, int(gjson.Get(logs[0].Other, "cache_creation_tokens").Int()))
					if tiered {
						require.Equal(t, int64(100), gjson.Get(logs[0].Other, "tiered_params.cc").Int())
					}
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

// A private, explicitly disposable schema lets this same settlement test exercise
// the production MySQL version/collation without reading production data.
func openAICacheWriteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSQLite, oldMySQL := common.UsingSQLite, common.UsingMySQL
	t.Cleanup(func() { common.UsingSQLite, common.UsingMySQL = oldSQLite, oldMySQL })
	dsn := os.Getenv("TEST_OPENAI_CACHE_MYSQL_DSN")
	if dsn == "" {
		common.UsingSQLite, common.UsingMySQL = true, false
		return useControllerCapacityTestDB(t)
	}
	config, err := mysqldsn.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "openai_cache_test", config.DBName)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldLog := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLog })
	common.UsingSQLite, common.UsingMySQL = false, true
	// Every row belongs to the isolated fixture database. Keep imported schemas.
	for _, row := range []any{&model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}} {
		if db.Migrator().HasTable(row) {
			require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(row).Error)
		}
	}
	return db
}
