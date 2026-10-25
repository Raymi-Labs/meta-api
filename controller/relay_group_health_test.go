package controller

import (
	"context"
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
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type healthTestTransport func(*http.Request) (*http.Response, error)

func (f healthTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type healthCancelAfterWrite struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *healthCancelAfterWrite) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	w.cancel()
	return n, err
}

func (w *healthCancelAfterWrite) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func TestRelayGroupHealthRecordsCancellationStagesAndDelivery(t *testing.T) {
	db := useControllerCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.ContentSafetyViolation{}, &model.GroupHealthMetric{}))
	oldRedis, oldCount, oldRetry := common.RedisEnabled, constant.CountToken, common.RetryTimes
	common.RedisEnabled, constant.CountToken, common.RetryTimes = false, false, 0
	t.Cleanup(func() { common.RedisEnabled, constant.CountToken, common.RetryTimes = oldRedis, oldCount, oldRetry })
	oldSensitive, oldPromptSensitive, oldWords := setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWords
	setting.CheckSensitiveOnPromptEnabled = true
	setting.SensitiveWords = []string{"test"}
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWords = oldSensitive, oldPromptSensitive, oldWords
	})
	oldRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-health-relay":0}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	quotaSetting := operation_setting.GetQuotaSetting()
	oldFree := quotaSetting.EnableFreeModelPreConsume
	quotaSetting.EnableFreeModelPreConsume = false
	t.Cleanup(func() { quotaSetting.EnableFreeModelPreConsume = oldFree })
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	channel := &model.Channel{Id: 993498, Name: "health-relay", Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.User{Id: 7312, Username: "health-relay-user", Quota: 0}).Error)

	for _, stage := range []string{"before-attempt", "during-attempt", "after-write", "write-failure", "user-quota", "local-sensitive", "upstream-rate-limit", "local-cooldown"} {
		t.Run(stage, func(t *testing.T) {
			quotaSetting.EnableFreeModelPreConsume = stage == "user-quota"
			setting.CheckSensitiveEnabled = stage == "local-sensitive"
			if stage == "local-cooldown" {
				require.NoError(t, db.Create(&model.ContentSafetyViolation{UserId: 7312, EventKey: "health-cooldown", CooldownUntil: time.Now().Add(time.Hour).Unix()}).Error)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			client.Transport = healthTestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if stage == "upstream-rate-limit" {
					return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"message":"provider quota","type":"rate_limit_error","code":"content_safety_cooldown"}}`)), Request: req}, nil
				}
				var body io.ReadCloser = io.NopCloser(strings.NewReader(`{"id":"health","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
				if stage == "during-attempt" {
					body = &cancelResponseTransport{cancel: cancel}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
			})
			group := "health-" + stage
			r := gin.New()
			r.POST("/v1/chat/completions", func(c *gin.Context) {
				c.Set("id", 7312)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "test-health-relay"))
				Relay(c, types.RelayFormatOpenAI)
			})
			recorder := httptest.NewRecorder()
			var writer http.ResponseWriter = recorder
			switch stage {
			case "before-attempt":
				cancel()
			case "after-write":
				writer = &healthCancelAfterWrite{ResponseRecorder: recorder, cancel: cancel}
			case "write-failure":
				writer = &healthTestResponseWriter{ResponseRecorder: recorder, writeErr: io.ErrClosedPipe}
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-health-relay","messages":[{"role":"user","content":"test"}],"stream":false}`)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(writer, req)
			require.NoError(t, grouphealth.Flush(context.Background()))
			var row model.GroupHealthMetric
			require.NoError(t, db.Where("group_name = ?", group).First(&row).Error)
			if stage == "after-write" {
				require.EqualValues(t, 1, row.UsageSampleCount)
				require.EqualValues(t, 1, row.InputTokens)
				require.EqualValues(t, 1, row.OutputTokens)
				require.Zero(t, row.ThroughputSampleCount, "non-streaming output has no measured generation interval")
			} else {
				require.Zero(t, row.UsageSampleCount, "undelivered and excluded outcomes cannot contribute usage")
			}
			if stage == "upstream-rate-limit" {
				require.EqualValues(t, 1, row.RequestCount)
				require.EqualValues(t, 1, row.ServiceFailureCount)
				require.Zero(t, row.ExcludedUserErrorCount)
				require.Equal(t, 1, calls)
				return
			}
			require.Zero(t, row.ServiceFailureCount)
			if stage == "user-quota" || stage == "local-sensitive" || stage == "local-cooldown" {
				require.Zero(t, row.RequestCount)
				require.EqualValues(t, 1, row.ExcludedUserErrorCount)
				require.Zero(t, row.SuccessCount)
				require.Zero(t, calls)
				return
			}
			require.EqualValues(t, 1, row.RequestCount)
			require.Zero(t, row.ExcludedUserErrorCount)
			switch stage {
			case "before-attempt", "during-attempt":
				require.EqualValues(t, 1, row.ClientCanceledCount)
				require.Zero(t, row.SuccessCount)
				if stage == "before-attempt" {
					require.Zero(t, calls)
				} else {
					require.Equal(t, 1, calls)
				}
			case "after-write":
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				require.EqualValues(t, 1, row.SuccessCount)
				require.Zero(t, row.ClientCanceledCount)
			case "write-failure":
				require.NoError(t, ctx.Err())
				require.Zero(t, row.SuccessCount)
				require.Zero(t, row.ClientCanceledCount)
			}
		})
	}
}
