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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRelayBoundedWalletReservation(t *testing.T) {
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	db := useControllerCapacityTestDB(t)
	sqlDB, e := db.DB()
	require.NoError(t, e)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.UserSubscription{}, &model.GroupHealthMetric{}, &model.GroupHealthHalfHourMetric{}))
	oldRedis, oldMemory, oldSQLite, oldCount, oldRetry, oldLog, oldConsume := common.RedisEnabled, common.MemoryCacheEnabled, common.UsingSQLite, constant.CountToken, common.RetryTimes, constant.ErrorLogEnabled, common.LogConsumeEnabled
	common.RedisEnabled, common.MemoryCacheEnabled, common.UsingSQLite, constant.CountToken, common.RetryTimes, constant.ErrorLogEnabled, common.LogConsumeEnabled = false, false, true, true, 0, true, true
	t.Cleanup(func() {
		common.RedisEnabled, common.MemoryCacheEnabled, common.UsingSQLite, constant.CountToken, common.RetryTimes, constant.ErrorLogEnabled, common.LogConsumeEnabled = oldRedis, oldMemory, oldSQLite, oldCount, oldRetry, oldLog, oldConsume
	})
	oldRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"reservation-fixture":6600}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	cases := []struct {
		path, body, response, maxPath string
		format                        types.RelayFormat
		channel                       int
	}{
		{"/v1/chat/completions", `{"model":"reservation-fixture","messages":[{"role":"user","content":"hello"}],"max_tokens":128000}`, `{"id":"chat","object":"chat.completion","model":"reservation-fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`, "max_tokens", types.RelayFormatOpenAI, constant.ChannelTypeOpenAI},
		{"/v1/messages", `{"model":"reservation-fixture","messages":[{"role":"user","content":"hello"}],"max_tokens":128000}`, `{"id":"msg","type":"message","role":"assistant","model":"reservation-fixture","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`, "max_tokens", types.RelayFormatClaude, constant.ChannelTypeAnthropic},
		{"/v1/responses", `{"model":"reservation-fixture","input":"hello","max_output_tokens":128000}`, `{"id":"resp","object":"response","status":"completed","model":"reservation-fixture","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`, "max_output_tokens", types.RelayFormatOpenAIResponses, constant.ChannelTypeOpenAI},
		{"/v1beta/models/reservation-fixture:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":128000}}`, `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`, "generationConfig.maxOutputTokens", types.RelayFormatGemini, constant.ChannelTypeGemini},
	}
	index := 0
	for _, tc := range cases {
		for _, outcome := range []string{"success", "upstream_error", "canceled"} {
			t.Run(string(tc.format)+"/"+outcome, func(t *testing.T) {
				index++
				id := 989000 + index*10
				initial := 260937323
				user := model.User{Id: id, Username: fmt.Sprint(id), AffCode: fmt.Sprint(id), Status: common.UserStatusEnabled, Group: "default", Quota: initial, Setting: `{"billing_preference":"wallet_only","quota_blocked_notify_enabled":false}`}
				token := model.Token{Id: id + 1, UserId: id, Key: fmt.Sprint(id), Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
				channel := model.Channel{Id: id + 2, Type: tc.channel, Name: "fixture", Key: "synthetic", Status: common.ChannelStatusEnabled}
				for _, row := range []any{&user, &token, &channel} {
					require.NoError(t, db.Create(row).Error)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				transport := healthTestTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					b, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.EqualValues(t, 128000, gjson.GetBytes(b, tc.maxPath).Int(), "upstream max unchanged")
					var heldUser model.User
					require.NoError(t, db.First(&heldUser, id).Error)
					require.Equal(t, initial-int(common.QuotaPerUnit*10), heldUser.Quota)
					if outcome == "canceled" {
						cancel()
						return nil, context.Canceled
					}
					status, body := 200, tc.response
					if outcome == "upstream_error" {
						status = 500
						body = `{"error":{"type":"server_error","message":"fixture"}}`
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				client := service.GetHttpClient()
				oldTransport := client.Transport
				client.Transport = transport
				t.Cleanup(func() { client.Transport = oldTransport })
				router := gin.New()
				router.Use(middleware.RequestId(), middleware.RequestFailureLog(), middleware.RouteTag("relay"), middleware.TokenAuth())
				router.POST(tc.path, func(c *gin.Context) {
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "reservation-fixture"))
					defer common.CleanupBodyStorage(c)
					Relay(c, tc.format)
				})
				req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer sk-"+token.Key)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, 1, calls, w.Body.String())
				var got model.User
				var gotToken model.Token
				if outcome == "success" {
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), "ok")
					var entry model.Log
					require.NoError(t, db.Where("user_id = ? AND type = ?", id, model.LogTypeConsume).First(&entry).Error)
					require.Greater(t, entry.Quota, 0)
					require.NoError(t, db.First(&got, id).Error)
					require.Equal(t, initial-entry.Quota, got.Quota)
					require.NoError(t, db.First(&gotToken, id+1).Error)
					require.Equal(t, entry.Quota, gotToken.UsedQuota)
					require.Zero(t, gotToken.RemainQuota)
					require.Equal(t, "bounded_chat_wallet", gjson.Get(entry.Other, "admin_info.preconsume.policy").String())
				} else {
					require.Eventually(t, func() bool { db.First(&got, id); return got.Quota == initial }, time.Second, 5*time.Millisecond)
					var count int64
					require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", id, model.LogTypeConsume).Count(&count).Error)
					require.Zero(t, count)
					require.NoError(t, db.First(&gotToken, id+1).Error)
					require.Zero(t, gotToken.UsedQuota)
					require.Zero(t, gotToken.QuotaFailureCount)
				}
			})
		}
	}
}

func TestFastPricingRetainsGeminiOutputLimit(t *testing.T) {
	request := &dto.GeminiChatRequest{GenerationConfig: dto.GeminiChatGenerationConfig{MaxOutputTokens: 128000}}
	require.Equal(t, 128000, fastTokenCountMetaForPricing(request).MaxTokens)
}
