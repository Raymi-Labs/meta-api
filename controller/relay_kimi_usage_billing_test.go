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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kimiUsageBillingTransport struct {
	body, contentType string
	calls             int
}

func (transport *kimiUsageBillingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport.calls++
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {transport.contentType}}, Body: io.NopCloser(strings.NewReader(transport.body)), Request: req}, nil
}

func TestKimiUsageAliasesSettleIdenticallyWithoutDoubleCounting(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, fields, fullUsage, priorUsage, status string
			cached, charged, prompt, completion         int
			noTail                                      bool
		}{
			{name: "nested", fields: `"prompt_tokens_details":{"cached_tokens":80},"completion_tokens_details":{"reasoning_tokens":18}`, cached: 80, charged: 48},
			{name: "flat", fields: `"cached_tokens":80,"reasoning_tokens":18`, cached: 80, charged: 48},
			{name: "both", fields: `"cached_tokens":80,"reasoning_tokens":18,"prompt_tokens_details":{"cached_tokens":80},"completion_tokens_details":{"reasoning_tokens":18}`, cached: 80, charged: 48},
			{name: "zero_wins", fields: `"cached_tokens":80,"reasoning_tokens":18,"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":0}`, charged: 120},
			{name: "cache_read_alias", fields: `"cache_read_tokens":80,"cache_write_tokens":20,"completion_tokens_details":{"reasoning_tokens":18}`, cached: 80, charged: 48},
			{name: "cache_read_zero_wins", fields: `"cache_read_tokens":80,"prompt_tokens_details":{"cached_tokens":0}`, charged: 120},
			{
				name: "null_blocks_details_aliases", noTail: true, status: "unknown", charged: 120,
				fields: `"prompt_tokens_details":{"cached_tokens":null},"completion_tokens_details":{"reasoning_tokens":null},"input_tokens_details":{"cached_tokens":80},"output_tokens_details":{"reasoning_tokens":18}`,
			},
			{
				name: "zero_blocks_details_aliases", noTail: true, charged: 120,
				fields: `"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":0},"input_tokens_details":{"cached_tokens":80},"output_tokens_details":{"reasoning_tokens":18}`,
			},
			{
				name: "negative_blocks_details_aliases", noTail: true, status: "invalid", charged: 120,
				fields: `"prompt_tokens_details":{"cached_tokens":-1},"completion_tokens_details":{"reasoning_tokens":-1},"input_tokens_details":{"cached_tokens":80},"output_tokens_details":{"reasoning_tokens":18}`,
			},
			{
				name: "oversized_blocks_details_aliases", noTail: true, status: "invalid", charged: 120,
				fields: `"prompt_tokens_details":{"cached_tokens":101},"completion_tokens_details":{"reasoning_tokens":21},"input_tokens_details":{"cached_tokens":80},"output_tokens_details":{"reasoning_tokens":18}`,
			},
			{
				name: "conflicting_totals_invalidate_prior_counts", noTail: true, status: "missing",
				fields: `"input_tokens":200,"output_tokens":40`, prompt: 200, completion: 40, charged: 240,
				priorUsage: `{"prompt_tokens":100,"completion_tokens":20,"cached_tokens":80,"reasoning_tokens":18}`,
			},
			{
				name: "fresh_counts_with_conflicting_totals", noTail: true,
				fields: `"input_tokens":200,"output_tokens":40,"cached_tokens":80,"reasoning_tokens":18`,
				prompt: 200, completion: 40, cached: 80, charged: 168,
			},
			{
				name: "fresh_counts_exceed_smaller_alias_total", noTail: true, status: "invalid",
				fields: `"input_tokens":50,"output_tokens":10,"cached_tokens":80,"reasoning_tokens":18`,
				prompt: 50, completion: 10, charged: 60,
			},
			{
				name: "responses_style_totals", noTail: true, cached: 80, charged: 48,
				fullUsage: `{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":80},"output_tokens_details":{"reasoning_tokens":18}}`,
			},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				if tc.status == "" {
					tc.status = "reported"
				}
				if tc.prompt == 0 {
					tc.prompt = 100
				}
				if tc.completion == 0 {
					tc.completion = 20
				}
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
				oldModel, oldCache, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CacheRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"kimi-k3":1}`))
				require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"kimi-k3":0.1}`))
				require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"kimi-k3":1}`))
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModel))
					require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(oldCache))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
				})
				const initial = 1000000
				user := model.User{Id: 994701, Username: "usage-fixture", Quota: initial, Status: common.UserStatusEnabled}
				token := model.Token{Id: 994702, UserId: user.Id, Key: "usage-fixture", RemainQuota: initial, Status: common.TokenStatusEnabled}
				channel := model.Channel{Id: 994703, Type: constant.ChannelTypeMoonshot, Status: common.ChannelStatusEnabled, Key: "fixture"}
				for _, row := range []any{&user, &token, &channel} {
					require.NoError(t, db.Create(row).Error)
				}
				usageJSON := tc.fullUsage
				if usageJSON == "" {
					usageJSON = `{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,` + tc.fields + `}`
				}
				body := `{"model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":` + usageJSON + `}`
				transport := &kimiUsageBillingTransport{body: body, contentType: "application/json"}
				if stream {
					transport.contentType = "text/event-stream"
					transport.body = ""
					if tc.priorUsage != "" {
						transport.body += `data: {"choices":[],"usage":` + tc.priorUsage + "}\n\n"
					}
					transport.body += "data: " + strings.Replace(body, `"message":{"role":"assistant","content":"OK"}`, `"delta":{"content":"OK"}`, 1) + "\n\n"
					if !tc.noTail {
						transport.body += `data: {"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}` + "\n\n"
					}
					transport.body += "data: " + `{"choices":[],"metadata":"trailer"}` + "\n\ndata: [DONE]\n\n"
				}
				client := service.GetHttpClient()
				oldTransport := client.Transport
				client.Transport = transport
				t.Cleanup(func() { client.Transport = oldTransport })
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"kimi-k3","messages":[{"role":"user","content":"OK"}],"stream":%t}`, stream)))
				c.Request.Header.Set("Content-Type", "application/json")
				requestID := tc.name + fmt.Sprint(stream)
				c.Set(common.RequestIdKey, requestID)
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "kimi-k3"))
				Relay(c, types.RelayFormatOpenAI)
				common.CleanupBodyStorage(c)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, 1, transport.calls)
				require.Eventually(t, func() bool {
					var u model.User
					var tok model.Token
					if db.First(&u, user.Id).Error != nil || db.First(&tok, token.Id).Error != nil {
						return false
					}
					return u.Quota == initial-tc.charged && tok.RemainQuota == initial-tc.charged && u.UsedQuota == tc.charged
				}, 2*time.Second, 10*time.Millisecond)
				var logs []model.Log
				require.NoError(t, db.Where("request_id = ?", requestID).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, tc.charged, logs[0].Quota)
				var other map[string]any
				require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
				require.EqualValues(t, tc.cached, other["cache_tokens"])
				require.Equal(t, tc.prompt, logs[0].PromptTokens)
				require.Equal(t, tc.completion, logs[0].CompletionTokens, "reasoning already included in output total")
				evidence := other["admin_info"].(map[string]any)["kimi_usage_evidence"].(map[string]any)
				cacheEvidence := evidence["cache"].(map[string]any)
				require.Equal(t, tc.status, cacheEvidence["status"])
				if tc.status == "reported" {
					require.EqualValues(t, tc.cached, cacheEvidence["value"])
				} else {
					require.NotContains(t, cacheEvidence, "value", "unknown/invalid counts are not reported as zero")
				}
				if stream && !tc.noTail {
					require.Equal(t, true, cacheEvidence["restored_from_prior_chunk"])
				} else {
					require.NotContains(t, cacheEvidence, "restored_from_prior_chunk")
				}
				publicLogs, err := model.GetLogByTokenId(token.Id)
				require.NoError(t, err)
				require.Len(t, publicLogs, 1)
				require.NotContains(t, publicLogs[0].Other, "kimi_usage_evidence", "supplier diagnostics are administrator-only")
				require.Equal(t, tc.charged, publicLogs[0].Quota)
			})
		}
	}
}
