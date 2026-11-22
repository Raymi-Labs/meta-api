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
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestRelayClaudeRefusalRetrySuccess tests that when the first channel refuses, the request retries on a second channel and succeeds
func TestRelayClaudeRefusalRetrySuccess(t *testing.T) {
	const testModel = "claude-sonnet-4"
	const initial = 5000000

	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}

	oldRatio := ratio_setting.ModelRatio2JSONString()
	oldGroup := ratio_setting.GroupRatio2JSONString()
	oldUsable := setting.UserUsableGroups2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"`+testModel+`":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`["default"]`))
	t.Cleanup(func() {
		ratio_setting.UpdateModelRatioByJSONString(oldRatio)
		ratio_setting.UpdateGroupRatioByJSONString(oldGroup)
		setting.UpdateUserUsableGroupsByJSONString(oldUsable)
	})

	bundle := billing_setting.TieredBundle{
		BillingMode: billing_setting.GetBillingModeCopy(),
		BillingExpr: billing_setting.GetBillingExprCopy(),
	}
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{
		BillingMode: map[string]string{testModel: "tiered_expr"},
		BillingExpr: map[string]string{testModel: `tier("base",p*3+c*3)`},
	})
	t.Cleanup(func() { billing_setting.ReplaceBundle(bundle) })

	for _, tc := range []struct {
		name   string
		format types.RelayFormat
		stream bool
	}{
		{"messages non-stream", types.RelayFormatClaude, false},
		{"messages stream", types.RelayFormatClaude, true},
		{"chat non-stream", types.RelayFormatOpenAI, false},
		{"chat stream", types.RelayFormatOpenAI, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := useControllerCapacityTestDB(t)
			sql, err := db.DB()
			require.NoError(t, err)
			sql.SetMaxOpenConns(1)
			t.Cleanup(func() { sql.Close() })

			require.NoError(t, db.AutoMigrate(
				&model.User{}, &model.Token{}, &model.Ability{}, &model.Model{},
				&model.ModelPermission{}, &model.Log{}, &model.ContentSafetyViolation{},
				&model.PulseWalletReservation{}, &model.PulseFundingLedger{},
			))

			redis, batch, logEnabled, errLog, sqlite := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite
			retryTimes := common.RetryTimes
			common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite = false, false, true, true, true
			common.RetryTimes = 1
			t.Cleanup(func() {
				common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite = redis, batch, logEnabled, errLog, sqlite
				common.RetryTimes = retryTimes
			})

			user := model.User{Id: 6101, Username: "refusal-retry-user", Group: "default", Quota: initial, Status: common.UserStatusEnabled}
			token := model.Token{Id: 6102, UserId: user.Id, Key: "refusal-retry-key", Group: "default", RemainQuota: initial}
			ch1 := model.Channel{Id: 6103, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "ch1-refuses"}
			ch2 := model.Channel{Id: 6104, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "ch2-succeeds"}

			for _, row := range []any{
				&user, &token, &ch1, &ch2,
				&model.Ability{Group: "default", Model: testModel, ChannelId: ch1.Id, Enabled: true},
				&model.Ability{Group: "default", Model: testModel, ChannelId: ch2.Id, Enabled: true},
			} {
				require.NoError(t, db.Create(row).Error)
			}

			count := 0
			client := service.GetHttpClient()
			old := client.Transport
			t.Cleanup(func() { client.Transport = old })

			client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
				count++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, testModel, gjson.GetBytes(body, "model").String())

				apiKey := req.Header.Get("x-api-key")
				var data string
				ct := "application/json"

				if apiKey == "ch1-refuses" {
					// First channel refuses
					data = `{"type":"message","id":"msg_refusal","model":"` + testModel + `","content":[],"stop_reason":"refusal","stop_details":{"category":"policy_violation"},"usage":{"input_tokens":10,"output_tokens":0}}`
					if tc.stream {
						ct = "text/event-stream"
						frames := []string{
							`{"type":"message_start","message":{"id":"msg_refusal","model":"` + testModel + `","usage":{"input_tokens":10,"output_tokens":0}}}`,
							`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"category":"policy_violation"}},"usage":{"output_tokens":0}}`,
							`{"type":"message_stop"}`,
						}
						data = "data: " + strings.Join(frames, "\n\ndata: ") + "\n\n"
					}
				} else {
					// Second channel succeeds
					data = `{"type":"message","id":"msg_success","model":"` + testModel + `","content":[{"type":"text","text":"Success response"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
					if tc.stream {
						ct = "text/event-stream"
						frames := []string{
							`{"type":"message_start","message":{"id":"msg_success","model":"` + testModel + `","usage":{"input_tokens":10,"output_tokens":0}}}`,
							`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
							`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Success response"}}`,
							`{"type":"content_block_stop","index":0}`,
							`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
							`{"type":"message_stop"}`,
						}
						data = "data: " + strings.Join(frames, "\n\ndata: ") + "\n\n"
					}
				}

				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": {ct}},
					Body:       io.NopCloser(strings.NewReader(data)),
					Request:    req,
				}, nil
			})

			endpoint := "/v1/messages"
			if tc.format == types.RelayFormatOpenAI {
				endpoint = "/v1/chat/completions"
			}

			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = httptest.NewRequest("POST", endpoint, strings.NewReader(fmt.Sprintf(
				`{"model":"%s","messages":[{"role":"user","content":"test"}],"max_tokens":100,"stream":%t}`,
				testModel, tc.stream,
			)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set(common.RequestIdKey, "refusal-retry-test")
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			require.NoError(t, middleware.SetupContextForToken(ctx, &token))

			Relay(ctx, tc.format)

			// Should have tried both channels
			require.Equal(t, 2, count, "should have tried exactly 2 channels")

			// Should return success (200)
			require.Equal(t, 200, w.Code, "should return 200 after retry succeeds")

			// Response should contain success content
			respBody := w.Body.String()
			if tc.stream {
				require.Contains(t, respBody, "Success response", "streamed response should contain success text")
			} else {
				if tc.format == types.RelayFormatClaude {
					require.Contains(t, respBody, "Success response", "non-stream Claude response should contain success text")
				} else {
					require.Contains(t, respBody, "Success response", "non-stream OpenAI response should contain success text")
				}
			}

			// Check consumption log - only the successful attempt should be charged
			var logs []model.Log
			time.Sleep(100 * time.Millisecond) // Allow async log write
			require.NoError(t, db.Where("user_id = ?", user.Id).Find(&logs).Error)
			require.Len(t, logs, 1, "should have exactly 1 consumption log (only successful attempt)")

			log := logs[0]
			modelName := gjson.Get(log.Other, "model_name").String()
			require.Equal(t, testModel, modelName)
			require.Equal(t, ch2.Id, log.ChannelId, "log should be for the second (successful) channel")
			quotaDelta := gjson.Get(log.Other, "quota").Int()
			require.Greater(t, quotaDelta, int64(0), "should have charged for the successful request")

			// Check admin_info.attempts for refusal retry record
			adminInfo := gjson.Get(log.Other, "admin_info")
			attempts := adminInfo.Get("attempts").Array()
			require.Len(t, attempts, 1, "should have 1 attempt record in admin_info")

			attempt := attempts[0]
			require.Equal(t, "claude_refusal", attempt.Get("retry_kind").String())
			require.Equal(t, "claude_refusal", attempt.Get("error_code").String())
			require.Equal(t, "msg_refusal", attempt.Get("claude_message_id").String())
			require.Equal(t, "policy_violation", attempt.Get("claude_refusal_category").String())

			// Success log should not have claude_message_id or reject_reason at top level
			require.False(t, adminInfo.Get("claude_message_id").Exists())
			require.False(t, adminInfo.Get("reject_reason").Exists() && adminInfo.Get("reject_reason").String() != "")
		})
	}
}

// TestRelayClaudeRefusalRetryAllRefuse tests that when all channels refuse, the final refusal is returned and charged
func TestRelayClaudeRefusalRetryAllRefuse(t *testing.T) {
	const testModel = "claude-sonnet-4"
	const initial = 5000000

	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}

	oldRatio := ratio_setting.ModelRatio2JSONString()
	oldGroup := ratio_setting.GroupRatio2JSONString()
	oldUsable := setting.UserUsableGroups2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"`+testModel+`":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`["default"]`))
	t.Cleanup(func() {
		ratio_setting.UpdateModelRatioByJSONString(oldRatio)
		ratio_setting.UpdateGroupRatioByJSONString(oldGroup)
		setting.UpdateUserUsableGroupsByJSONString(oldUsable)
	})

	bundle := billing_setting.TieredBundle{
		BillingMode: billing_setting.GetBillingModeCopy(),
		BillingExpr: billing_setting.GetBillingExprCopy(),
	}
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{
		BillingMode: map[string]string{testModel: "tiered_expr"},
		BillingExpr: map[string]string{testModel: `tier("base",p*3+c*3)`},
	})
	t.Cleanup(func() { billing_setting.ReplaceBundle(bundle) })

	for _, tc := range []struct {
		name   string
		format types.RelayFormat
		stream bool
	}{
		{"messages non-stream", types.RelayFormatClaude, false},
		{"messages stream", types.RelayFormatClaude, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := useControllerCapacityTestDB(t)
			sql, err := db.DB()
			require.NoError(t, err)
			sql.SetMaxOpenConns(1)
			t.Cleanup(func() { sql.Close() })

			require.NoError(t, db.AutoMigrate(
				&model.User{}, &model.Token{}, &model.Ability{}, &model.Model{},
				&model.ModelPermission{}, &model.Log{}, &model.ContentSafetyViolation{},
				&model.PulseWalletReservation{}, &model.PulseFundingLedger{},
			))

			redis, batch, logEnabled, errLog, sqlite := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite
			retryTimes := common.RetryTimes
			common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite = false, false, true, true, true
			common.RetryTimes = 1
			t.Cleanup(func() {
				common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite = redis, batch, logEnabled, errLog, sqlite
				common.RetryTimes = retryTimes
			})

			user := model.User{Id: 6201, Username: "all-refuse-user", Group: "default", Quota: initial, Status: common.UserStatusEnabled}
			token := model.Token{Id: 6202, UserId: user.Id, Key: "all-refuse-key", Group: "default", RemainQuota: initial}
			ch1 := model.Channel{Id: 6203, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "ch1-refuses"}
			ch2 := model.Channel{Id: 6204, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "ch2-refuses"}

			for _, row := range []any{
				&user, &token, &ch1, &ch2,
				&model.Ability{Group: "default", Model: testModel, ChannelId: ch1.Id, Enabled: true},
				&model.Ability{Group: "default", Model: testModel, ChannelId: ch2.Id, Enabled: true},
			} {
				require.NoError(t, db.Create(row).Error)
			}

			count := 0
			client := service.GetHttpClient()
			old := client.Transport
			t.Cleanup(func() { client.Transport = old })

			client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
				count++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, testModel, gjson.GetBytes(body, "model").String())

				// Both channels refuse
				data := `{"type":"message","id":"msg_refusal_final","model":"` + testModel + `","content":[],"stop_reason":"refusal","stop_details":{"category":"general_harms"},"usage":{"input_tokens":10,"output_tokens":0}}`
				ct := "application/json"
				if tc.stream {
					ct = "text/event-stream"
					frames := []string{
						`{"type":"message_start","message":{"id":"msg_refusal_final","model":"` + testModel + `","usage":{"input_tokens":10,"output_tokens":0}}}`,
						`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"category":"general_harms"}},"usage":{"output_tokens":0}}`,
						`{"type":"message_stop"}`,
					}
					data = "data: " + strings.Join(frames, "\n\ndata: ") + "\n\n"
				}

				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": {ct}},
					Body:       io.NopCloser(strings.NewReader(data)),
					Request:    req,
				}, nil
			})

			endpoint := "/v1/messages"
			if tc.format == types.RelayFormatOpenAI {
				endpoint = "/v1/chat/completions"
			}

			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = httptest.NewRequest("POST", endpoint, strings.NewReader(fmt.Sprintf(
				`{"model":"%s","messages":[{"role":"user","content":"test"}],"max_tokens":100,"stream":%t}`,
				testModel, tc.stream,
			)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set(common.RequestIdKey, "all-refuse-test")
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			require.NoError(t, middleware.SetupContextForToken(ctx, &token))

			Relay(ctx, tc.format)

			// Should have tried both channels
			require.Equal(t, 2, count, "should have tried exactly 2 channels")

			// Final refusal should be returned (200 with refusal content)
			require.Equal(t, 200, w.Code)

			respBody := w.Body.String()
			if tc.format == types.RelayFormatClaude {
				require.Contains(t, respBody, "refusal")
			} else {
				require.Contains(t, respBody, "content_filter")
			}

			// Should have charged for the last attempt
			var logs []model.Log
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, db.Where("user_id = ?", user.Id).Find(&logs).Error)
			require.Len(t, logs, 1, "should have exactly 1 consumption log (final refusal)")

			log := logs[0]
			require.Equal(t, ch2.Id, log.ChannelId, "should be charged for the second (final) channel")
		})
	}
}

// TestRelayClaudeRefusalNoRetryWhenRetryTimesZero tests that refusal is delivered normally when RetryTimes=0
func TestRelayClaudeRefusalNoRetryWhenRetryTimesZero(t *testing.T) {
	const testModel = "claude-sonnet-4"
	const initial = 5000000

	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}

	oldRatio := ratio_setting.ModelRatio2JSONString()
	oldGroup := ratio_setting.GroupRatio2JSONString()
	oldUsable := setting.UserUsableGroups2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"`+testModel+`":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`["default"]`))
	t.Cleanup(func() {
		ratio_setting.UpdateModelRatioByJSONString(oldRatio)
		ratio_setting.UpdateGroupRatioByJSONString(oldGroup)
		setting.UpdateUserUsableGroupsByJSONString(oldUsable)
	})

	bundle := billing_setting.TieredBundle{
		BillingMode: billing_setting.GetBillingModeCopy(),
		BillingExpr: billing_setting.GetBillingExprCopy(),
	}
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{
		BillingMode: map[string]string{testModel: "tiered_expr"},
		BillingExpr: map[string]string{testModel: `tier("base",p*3+c*3)`},
	})
	t.Cleanup(func() { billing_setting.ReplaceBundle(bundle) })

	db := useControllerCapacityTestDB(t)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })

	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Ability{}, &model.Model{},
		&model.ModelPermission{}, &model.Log{}, &model.ContentSafetyViolation{},
		&model.PulseWalletReservation{}, &model.PulseFundingLedger{},
	))

	redis, batch, logEnabled, sqlite := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
	retryTimes := common.RetryTimes
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = false, false, true, true
	common.RetryTimes = 0
	t.Cleanup(func() {
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = redis, batch, logEnabled, sqlite
		common.RetryTimes = retryTimes
	})

	user := model.User{Id: 6301, Username: "no-retry-user", Group: "default", Quota: initial, Status: common.UserStatusEnabled}
	token := model.Token{Id: 6302, UserId: user.Id, Key: "no-retry-key", Group: "default", RemainQuota: initial}
	ch := model.Channel{Id: 6303, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "single-channel"}

	for _, row := range []any{
		&user, &token, &ch,
		&model.Ability{Group: "default", Model: testModel, ChannelId: ch.Id, Enabled: true},
	} {
		require.NoError(t, db.Create(row).Error)
	}

	count := 0
	client := service.GetHttpClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })

	client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
		count++
		data := `{"type":"message","id":"msg_refusal","model":"` + testModel + `","content":[],"stop_reason":"refusal","stop_details":{"category":"policy_violation"},"usage":{"input_tokens":10,"output_tokens":0}}`
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(data)),
			Request:    req,
		}, nil
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", strings.NewReader(fmt.Sprintf(
		`{"model":"%s","messages":[{"role":"user","content":"test"}],"max_tokens":100}`,
		testModel,
	)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, "no-retry-test")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	require.NoError(t, middleware.SetupContextForToken(ctx, &token))

	Relay(ctx, types.RelayFormatClaude)

	// Should only try once (no retry)
	require.Equal(t, 1, count, "should only try once when RetryTimes=0")

	// Should return refusal normally
	require.Equal(t, 200, w.Code)
	respBody := w.Body.String()
	require.Contains(t, respBody, "refusal")

	// Should have one consumption log
	var logs []model.Log
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&logs).Error)
	require.Len(t, logs, 1)
}

// TestRelayClaudeRefusalNoRetryWithSpecificChannel tests that refusal is not retried when a specific channel is requested
func TestRelayClaudeRefusalNoRetryWithSpecificChannel(t *testing.T) {
	const testModel = "claude-sonnet-4"
	const initial = 5000000

	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}

	oldRatio := ratio_setting.ModelRatio2JSONString()
	oldGroup := ratio_setting.GroupRatio2JSONString()
	oldUsable := setting.UserUsableGroups2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"`+testModel+`":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`["default"]`))
	t.Cleanup(func() {
		ratio_setting.UpdateModelRatioByJSONString(oldRatio)
		ratio_setting.UpdateGroupRatioByJSONString(oldGroup)
		setting.UpdateUserUsableGroupsByJSONString(oldUsable)
	})

	bundle := billing_setting.TieredBundle{
		BillingMode: billing_setting.GetBillingModeCopy(),
		BillingExpr: billing_setting.GetBillingExprCopy(),
	}
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{
		BillingMode: map[string]string{testModel: "tiered_expr"},
		BillingExpr: map[string]string{testModel: `tier("base",p*3+c*3)`},
	})
	t.Cleanup(func() { billing_setting.ReplaceBundle(bundle) })

	db := useControllerCapacityTestDB(t)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })

	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Ability{}, &model.Model{},
		&model.ModelPermission{}, &model.Log{}, &model.ContentSafetyViolation{},
		&model.PulseWalletReservation{}, &model.PulseFundingLedger{},
	))

	redis, batch, logEnabled, sqlite := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
	retryTimes := common.RetryTimes
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = false, false, true, true
	common.RetryTimes = 1
	t.Cleanup(func() {
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = redis, batch, logEnabled, sqlite
		common.RetryTimes = retryTimes
	})

	user := model.User{Id: 6401, Username: "specific-ch-user", Group: "default", Quota: initial, Status: common.UserStatusEnabled}
	token := model.Token{Id: 6402, UserId: user.Id, Key: "specific-ch-key", Group: "default", RemainQuota: initial}
	ch1 := model.Channel{Id: 6403, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "specific-channel"}
	ch2 := model.Channel{Id: 6404, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "other-channel"}

	for _, row := range []any{
		&user, &token, &ch1, &ch2,
		&model.Ability{Group: "default", Model: testModel, ChannelId: ch1.Id, Enabled: true},
		&model.Ability{Group: "default", Model: testModel, ChannelId: ch2.Id, Enabled: true},
	} {
		require.NoError(t, db.Create(row).Error)
	}

	count := 0
	client := service.GetHttpClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })

	client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
		count++
		data := `{"type":"message","id":"msg_refusal","model":"` + testModel + `","content":[],"stop_reason":"refusal","stop_details":{"category":"policy_violation"},"usage":{"input_tokens":10,"output_tokens":0}}`
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(data)),
			Request:    req,
		}, nil
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", strings.NewReader(fmt.Sprintf(
		`{"model":"%s","messages":[{"role":"user","content":"test"}],"max_tokens":100}`,
		testModel,
	)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, "specific-ch-test")
	ctx.Set("specific_channel_id", ch1.Id) // Simulate specific channel request
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	require.NoError(t, middleware.SetupContextForToken(ctx, &token))

	Relay(ctx, types.RelayFormatClaude)

	// Should only try the specific channel once
	require.Equal(t, 1, count, "should only try once when specific channel is requested")

	// Should return refusal normally
	require.Equal(t, 200, w.Code)
	respBody := w.Body.String()
	require.Contains(t, respBody, "refusal")

	// Should have one consumption log for the specific channel
	var logs []model.Log
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, ch1.Id, logs[0].ChannelId)
}
