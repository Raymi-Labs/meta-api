package controller

import (
	"context"
	"errors"
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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type responsesBillingTransport struct {
	body      string
	calls     int
	onRequest func() error
	cancel    context.CancelFunc
	onClose   context.CancelFunc
}

func (t *responsesBillingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	if err := t.onRequest(); err != nil {
		return nil, err
	}
	body := io.ReadCloser(io.NopCloser(strings.NewReader(t.body)))
	if t.cancel != nil {
		body = &responsesBillingCanceledBody{reader: strings.NewReader(t.body), cancel: t.cancel}
	}
	if t.onClose != nil {
		body = &responsesBillingCloseCancelBody{ReadCloser: body, cancel: t.onClose}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body, Request: req}, nil
}

type responsesBillingCanceledBody struct {
	reader *strings.Reader
	cancel context.CancelFunc
}

func (b *responsesBillingCanceledBody) Read(p []byte) (int, error) {
	if b.reader.Len() > 0 {
		return b.reader.Read(p)
	}
	b.cancel()
	return 0, context.Canceled
}

func (b *responsesBillingCanceledBody) Close() error { return nil }

type responsesBillingCloseCancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *responsesBillingCloseCancelBody) Close() error {
	b.cancel()
	return b.ReadCloser.Close()
}

type responsesBillingDeliveryWriter struct {
	gin.ResponseWriter
	failure  string
	terminal bool
	cancel   context.CancelFunc
}

func (w *responsesBillingDeliveryWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), `"type":"response.completed"`) {
		w.terminal = true
		if w.failure == "write" {
			return 0, io.ErrClosedPipe
		}
	}
	return w.ResponseWriter.Write(data)
}
func (w *responsesBillingDeliveryWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}
func (w *responsesBillingDeliveryWriter) Flush() { _ = w.FlushError() }
func (w *responsesBillingDeliveryWriter) FlushError() error {
	if w.terminal && w.failure == "flush" {
		return errors.New("downstream flush failed")
	}
	w.ResponseWriter.Flush()
	if w.terminal && w.failure == "hidden-flush-cancel" {
		w.cancel()
	}
	return nil
}

func TestRelayResponsesInterruptedOutputRefundsAllFundingSources(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	delta := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"
	withUsage := `"usage":{"input_tokens":41093,"output_tokens":1,"total_tokens":41094}`
	complete := delta + `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],` + withUsage + "}}\n\n"
	cases := []struct {
		name                 string
		body                 string
		estimated            bool
		canceled             bool
		noOutput             bool
		emptyCompleted       bool
		deliveryFailure      string
		success              bool
		cancelAfterCompleted bool
	}{
		{name: "upstream_failure_with_usage", body: delta + `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"upstream_error","message":"temporarily unavailable"},` + withUsage + "}}\n\n"},
		{name: "upstream_failure_without_usage", body: delta + "data: {\"type\":\"error\",\"error\":{\"code\":\"upstream_error\",\"message\":\"temporarily unavailable\"}}\n\n", estimated: true},
		{name: "incomplete", body: delta + `data: {"type":"response.incomplete","response":{"status":"incomplete",` + withUsage + "}}\n\n"},
		{name: "missing_completed", body: delta, estimated: true},
		{name: "client_canceled", body: delta, estimated: true, canceled: true},
		{name: "no_output", body: `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"upstream_error","message":"temporarily unavailable"},` + withUsage + "}}\n\n", noOutput: true},
		{name: "empty_completed", body: "data: malformed-json\n\n" + `data: {"type":"response.completed","response":{"status":"completed","output":[],` + withUsage + "}}\n\n", noOutput: true, emptyCompleted: true},
		{name: "completed_write_failure", body: complete, deliveryFailure: "write"},
		{name: "completed_flush_failure", body: complete, deliveryFailure: "flush"},
		{name: "completed_hidden_flush_cancel", body: complete, deliveryFailure: "hidden-flush-cancel"},
		{name: "delivered_completed", body: complete, success: true},
		{name: "delivered_then_canceled", body: complete, success: true, cancelAfterCompleted: true},
	}
	for _, funding := range []string{service.BillingSourceWallet, service.BillingSourceSubscription, service.BillingSourceToken} {
		for _, tc := range cases {
			t.Run(funding+"/"+tc.name, func(t *testing.T) {
				db := useControllerCapacityTestDB(t)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)
				t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
				require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.Token{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.GroupHealthMetric{}))
				oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
				constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, 3, false, true, true
				t.Cleanup(func() {
					constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
				})
				oldRatio := ratio_setting.ModelRatio2JSONString()
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-responses-refund":1}`))
				t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
				const initialQuota = 1000000
				user := &model.User{Id: 993501, Username: "responses-refund", Quota: initialQuota, Status: common.UserStatusEnabled}
				token := &model.Token{Id: 993502, UserId: user.Id, Key: "responses-refund-test", Status: common.TokenStatusEnabled, RemainQuota: initialQuota}
				require.NoError(t, db.Create(user).Error)
				require.NoError(t, db.Create(token).Error)
				plan := &model.SubscriptionPlan{Id: 993504, Title: "refund-test", QuotaResetPeriod: "never"}
				sub := &model.UserSubscription{Id: 993505, UserId: user.Id, PlanId: plan.Id, AmountTotal: initialQuota, Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix()}
				require.NoError(t, db.Create(plan).Error)
				require.NoError(t, db.Create(sub).Error)
				requestID := "refund-" + funding + "-" + tc.name
				requestCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				transport := &responsesBillingTransport{body: tc.body, onRequest: func() error {
					var reserved model.Token
					if err := db.First(&reserved, token.Id).Error; err != nil {
						return err
					}
					if reserved.RemainQuota >= initialQuota {
						return fmt.Errorf("test must exercise a real precharge")
					}
					return nil
				}}
				if tc.canceled {
					transport.cancel = cancel
				}
				if tc.cancelAfterCompleted {
					transport.onClose = cancel
				}
				client := service.GetHttpClient()
				oldTransport := client.Transport
				client.Transport = transport
				t.Cleanup(func() { client.Transport = oldTransport })
				channel := &model.Channel{Id: 993503, Name: "responses-refund-test", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "test-key"}
				if tc.emptyCompleted {
					autoBan := 1
					baseURL := "https://empty-output-" + funding + ".invalid"
					channel.AutoBan, channel.BaseURL = &autoBan, &baseURL
					monitor := operation_setting.GetMonitorSetting()
					oldWait, oldMinutes := monitor.PreDisableWaitEnabled, monitor.PreDisableWaitMinutes
					monitor.PreDisableWaitEnabled, monitor.PreDisableWaitMinutes = true, 1
					t.Cleanup(func() { monitor.PreDisableWaitEnabled, monitor.PreDisableWaitMinutes = oldWait, oldMinutes })
				}
				require.NoError(t, db.Create(channel).Error)
				r := gin.New()
				r.Use(middleware.RequestFailureLog(), middleware.RouteTag("relay"))
				r.POST("/v1/responses", func(c *gin.Context) {
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
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "test-responses-refund"))
					if tc.deliveryFailure != "" {
						c.Writer = &responsesBillingDeliveryWriter{ResponseWriter: c.Writer, failure: tc.deliveryFailure, cancel: cancel}
					}
					Relay(c, types.RelayFormatOpenAIResponses)
					require.Equal(t, tc.success, service.RequestSucceeded(c))
				})
				w := httptest.NewRecorder()
				// A tool disables text auto-continuation; no replay is safe after output.
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-responses-refund","input":"test","stream":true,"tools":[{"type":"function","name":"test_tool","parameters":{"type":"object"}}]}`)).WithContext(requestCtx)
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(w, req)
				require.Equal(t, 1, transport.calls, w.Body.String())
				require.NoError(t, grouphealth.Flush(context.Background()))
				var health model.GroupHealthMetric
				require.NoError(t, db.Where("group_name = ?", requestID).First(&health).Error)
				require.EqualValues(t, 1, health.RequestCount)
				if tc.success {
					require.EqualValues(t, 1, health.SuccessCount)
					require.Zero(t, health.ClientCanceledCount)
					require.Eventually(t, func() bool {
						var actualToken model.Token
						return db.First(&actualToken, token.Id).Error == nil && actualToken.UsedQuota > 0 && actualToken.RemainQuota < initialQuota
					}, time.Second, 10*time.Millisecond)
					var consume []model.Log
					require.NoError(t, db.Where("request_id = ?", requestID).Find(&consume).Error)
					require.Len(t, consume, 1)
					require.Equal(t, model.LogTypeConsume, consume[0].Type)
					require.Positive(t, consume[0].Quota)
					return
				}
				require.Zero(t, health.SuccessCount)
				if tc.deliveryFailure != "" {
					require.Zero(t, health.ServiceFailureCount, "downstream failures must not be blamed on the provider")
					require.EqualValues(t, 1, health.RequestCount-health.SuccessCount-health.ServiceFailureCount, "delivery failure remains in the unknown-failure denominator")
					require.Zero(t, health.ExcludedUserErrorCount)
					if tc.deliveryFailure == "hidden-flush-cancel" {
						require.EqualValues(t, 1, health.ClientCanceledCount)
					} else {
						require.Zero(t, health.ClientCanceledCount, "a write error alone does not prove a client cancellation")
					}
					var current model.Channel
					require.NoError(t, db.First(&current, channel.Id).Error)
					require.Equal(t, common.ChannelStatusEnabled, current.Status)
					require.False(t, service.IsChannelUnavailableForRequest(&current))
				}
				require.Eventually(t, func() bool {
					var actualUser model.User
					var actualToken model.Token
					var actualSub model.UserSubscription
					if db.First(&actualUser, user.Id).Error != nil || db.First(&actualToken, token.Id).Error != nil || db.First(&actualSub, sub.Id).Error != nil {
						return false
					}
					return actualUser.Quota == initialQuota && actualUser.UsedQuota == 0 && actualToken.RemainQuota == initialQuota && actualToken.UsedQuota == 0 && actualSub.AmountUsed == 0
				}, 3*time.Second, 10*time.Millisecond, "all precharged balances must be restored")
				var logs []model.Log
				require.NoError(t, db.Where("request_id = ?", requestID).Find(&logs).Error)
				require.Len(t, logs, 1, "failure must not create a consumption row")
				require.Equal(t, model.LogTypeError, logs[0].Type)
				require.Zero(t, logs[0].Quota)
				details, err := common.StrToMap(logs[0].Other)
				require.NoError(t, err)
				admin := details["admin_info"].(map[string]interface{})
				if tc.emptyCompleted {
					require.Contains(t, w.Body.String(), "event: response.failed")
					require.NotContains(t, w.Body.String(), "event: response.completed")
					require.False(t, service.ShouldDisableChannel(channel.Type, types.NewOpenAIError(fmt.Errorf("%s", service.ResponsesStreamEmptyCompletedReason), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())))
					var current model.Channel
					require.NoError(t, db.First(&current, channel.Id).Error)
					require.Equal(t, common.ChannelStatusEnabled, current.Status)
					require.False(t, service.IsChannelUnavailableForRequest(&current))
					sibling := &model.Channel{Id: channel.Id + 1, Status: common.ChannelStatusEnabled, BaseURL: channel.BaseURL}
					require.False(t, service.IsChannelUnavailableForRequest(sibling), "the same upstream address must remain selectable")
					diagnostics := admin["responses_stream_diagnostics"].(map[string]interface{})
					require.EqualValues(t, 1, diagnostics["json_decode_errors"])
					require.Equal(t, true, diagnostics["completed_response_present"])
					completed := diagnostics["responses_completed_summary"].(map[string]interface{})
					require.Equal(t, "completed", completed["status"])
					require.EqualValues(t, 0, completed["output_count"])
					public, _, e := model.GetUserLogs(user.Id, model.LogTypeError, 0, 0, "", "", 0, 10, "", requestID)
					require.NoError(t, e)
					require.Len(t, public, 1)
					require.NotContains(t, public[0].Other, "responses_stream_diagnostics")
				}
				if tc.noOutput {
					require.NotContains(t, admin, "unbilled_usage")
				} else {
					usage, ok := admin["unbilled_usage"].(map[string]interface{})
					require.True(t, ok, fmt.Sprint(admin))
					require.Equal(t, false, usage["charged"])
					require.Equal(t, true, usage["interrupted_output"])
					require.Equal(t, tc.estimated, usage["input_tokens_estimated"])
					require.Positive(t, usage["completion_tokens"])
					if !tc.estimated {
						require.EqualValues(t, 41093, usage["prompt_tokens"])
					}
				}
				if funding == service.BillingSourceWallet {
					var reservation model.PulseWalletReservation
					require.NoError(t, db.Where("request_id = ?", requestID).First(&reservation).Error)
					require.Equal(t, "refunded", reservation.Status)
				}
				if funding == service.BillingSourceSubscription {
					var reservation model.SubscriptionPreConsumeRecord
					require.NoError(t, db.Where("request_id = ?", requestID).First(&reservation).Error)
					require.Equal(t, "refunded", reservation.Status)
				}
			})
		}
	}
}
