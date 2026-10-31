package controller

import (
	"context"
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
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type claudeStreamAuditTransport struct {
	*responsesBillingTransport
	request dto.ClaudeRequest
}

func (t *claudeStreamAuditTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := common.Unmarshal(body, &t.request); err != nil {
		return nil, err
	}
	return t.responsesBillingTransport.RoundTrip(r)
}

func TestRelayClaudeStreamFailureRefundAndCompletion(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	// Keep one DB/global configuration for the matrix. The production health
	// collector has a background ticker; replacing DB on every case races it.
	db := useControllerCapacityTestDB(t)
	sqlDB, e := db.DB()
	require.NoError(t, e)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.Token{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.GroupHealthMetric{}))
	oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
	constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, 3, false, true, true
	t.Cleanup(func() {
		constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
	})
	oldRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-stream-audit":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	caseIndex := 0
	start := "data: " + `{"type":"message_start","message":{"id":"msg_fixture","model":"claude-stream-audit","usage":{"input_tokens":100,"output_tokens":1,"cache_read_input_tokens":30}}}` + "\n\n"
	text := "data: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}` + "\n\n"
	thinking := "data: " + `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"synthetic thinking"}}` + "\n\n"
	tool := "data: " + `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"synthetic-tool","name":"noop","input":{}}}` + "\n\n"
	stop := "data: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}` + "\n\ndata: " + `{"type":"message_stop"}` + "\n\n"
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		for _, funding := range []string{service.BillingSourceWallet, service.BillingSourceSubscription, service.BillingSourceToken} {
			for _, tc := range []struct {
				name, body               string
				success, cancelAfterStop bool
			}{
				{"partial_text", start + text, false, false},
				{"partial_thinking", start + thinking, false, false},
				{"partial_tool", start + tool, false, false},
				{"empty", "", false, false},
				{"complete", start + text + stop, true, false},
				{"cleanup_cancel", start + text + stop, true, true},
				{"client_cancel_partial", start + text, false, false},
			} {
				t.Run(string(format)+"/"+funding+"/"+tc.name, func(t *testing.T) {
					caseIndex++
					id := 995800 + caseIndex*10
					const initial = 1000000
					user := &model.User{Id: id + 1, Username: fmt.Sprintf("stream-audit-%d", id), AffCode: fmt.Sprint(id), Quota: initial, Status: common.UserStatusEnabled}
					token := &model.Token{Id: id + 2, UserId: user.Id, Key: fmt.Sprintf("stream-audit-%d", id), RemainQuota: initial, Status: common.TokenStatusEnabled}
					plan := &model.SubscriptionPlan{Id: id + 4, Title: "stream-audit", QuotaResetPeriod: "never"}
					sub := &model.UserSubscription{Id: id + 5, UserId: user.Id, PlanId: plan.Id, AmountTotal: initial, Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix()}
					channel := &model.Channel{Id: id + 3, Name: "stream-audit", Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "synthetic-key"}
					for _, row := range []any{user, token, plan, sub, channel} {
						require.NoError(t, db.Create(row).Error)
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					transport := &responsesBillingTransport{body: tc.body, onRequest: func() error {
						var reserved model.Token
						if err := db.First(&reserved, token.Id).Error; err != nil {
							return err
						}
						if reserved.RemainQuota >= initial {
							return fmt.Errorf("request was not precharged")
						}
						return nil
					}}
					if tc.cancelAfterStop {
						transport.onClose = cancel
					}
					canceledPartial := tc.name == "client_cancel_partial"
					if canceledPartial {
						transport.cancel = cancel
					}
					client := service.GetHttpClient()
					oldTransport := client.Transport
					capture := &claudeStreamAuditTransport{responsesBillingTransport: transport}
					client.Transport = capture
					t.Cleanup(func() { client.Transport = oldTransport })
					path := "/v1/messages"
					if format == types.RelayFormatOpenAI {
						path = "/v1/chat/completions"
					}
					requestID := "claude-stream-" + string(format) + funding + tc.name
					r := gin.New()
					r.Use(middleware.RequestFailureLog(), middleware.RouteTag("relay"))
					r.POST(path, func(c *gin.Context) {
						common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
						common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
						common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
						common.SetContextKey(c, constant.ContextKeyUsingGroup, requestID)
						c.Set(common.RequestIdKey, requestID)
						pref := "wallet_only"
						if funding == service.BillingSourceSubscription {
							pref = "subscription_only"
						}
						if funding == service.BillingSourceToken {
							common.SetContextKey(c, constant.ContextKeyTokenBillingMode, model.TokenBillingModeTokenOnly)
						}
						common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: pref})
						require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "claude-stream-audit"))
						Relay(c, format)
						common.CleanupBodyStorage(c)
						require.Equal(t, tc.success, service.RequestSucceeded(c))
					})
					w := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"claude-stream-audit","messages":[{"role":"user","content":"synthetic"}],"stream":true,"max_tokens":2048,"thinking":{"type":"adaptive"},"output_config":{"effort":"max"}}`)).WithContext(ctx)
					req.Header.Set("Content-Type", "application/json")
					r.ServeHTTP(w, req)
					require.Equal(t, 1, transport.calls, "partial text/thinking/tools must never be replayed")
					if format == types.RelayFormatClaude {
						require.Equal(t, "max", gjson.GetBytes(capture.request.OutputConfig, "effort").String())
						require.Equal(t, "adaptive", capture.request.Thinking.Type)
						require.EqualValues(t, 2048, capture.request.MaxTokens)
					}
					require.NoError(t, grouphealth.Flush(context.Background()))
					var health model.GroupHealthMetric
					require.NoError(t, db.Where("group_name = ?", requestID).First(&health).Error)
					require.EqualValues(t, 1, health.RequestCount)
					require.Eventually(t, func() bool {
						var u model.User
						var tok model.Token
						var s model.UserSubscription
						if db.First(&u, user.Id).Error != nil || db.First(&tok, token.Id).Error != nil || db.First(&s, sub.Id).Error != nil {
							return false
						}
						if tc.success || canceledPartial {
							return tok.UsedQuota > 0 && tok.RemainQuota < initial
						}
						return u.Quota == initial && u.UsedQuota == 0 && tok.RemainQuota == initial && tok.UsedQuota == 0 && s.AmountUsed == 0
					}, 3*time.Second, 10*time.Millisecond)
					var logs []model.Log
					require.NoError(t, db.Where("request_id = ?", requestID).Find(&logs).Error)
					require.Len(t, logs, 1)
					if canceledPartial {
						require.Equal(t, model.LogTypeConsume, logs[0].Type)
						require.Positive(t, logs[0].Quota)
						require.Zero(t, health.SuccessCount)
						require.Zero(t, health.UsageSampleCount)
						require.NotContains(t, w.Body.String(), "[DONE]")
						require.NotContains(t, w.Body.String(), "message_stop")
						return
					}
					if tc.success {
						require.Equal(t, model.LogTypeConsume, logs[0].Type)
						require.Positive(t, logs[0].Quota)
						require.EqualValues(t, 1, health.SuccessCount)
					} else {
						require.Equal(t, model.LogTypeError, logs[0].Type)
						require.Zero(t, logs[0].Quota)
						require.Zero(t, health.SuccessCount)
						require.Zero(t, health.UsageSampleCount)
						require.Contains(t, w.Body.String(), "error")
						require.Contains(t, w.Body.String(), requestID)
						require.NotContains(t, w.Body.String(), "event: message_stop")
						require.NotContains(t, w.Body.String(), `"finish_reason":"stop"`)
						require.False(t, gjson.Get(logs[0].Other, "admin_info.unbilled_usage.charged").Bool())
						if format == types.RelayFormatClaude {
							require.Equal(t, "max", gjson.Get(logs[0].Other, "admin_info.claude_requested_settings.effort").String())
						}
						public, _, err := model.GetUserLogs(user.Id, model.LogTypeError, 0, 0, "", "", 0, 10, "", requestID)
						require.NoError(t, err)
						require.Len(t, public, 1)
						require.NotContains(t, public[0].Other, "claude_requested_settings")
						require.NotContains(t, public[0].Other, "claude_stream_diagnostics")
					}
				})
			}
		}
	}
}
