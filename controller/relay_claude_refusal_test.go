package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRelayClaudeRefusalWithRetiredTokenSetting(t *testing.T) {
	const primary = "claude-opus-5-5"

	const initial = 2000000
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	bundle := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{primary: "tiered_expr"}, BillingExpr: map[string]string{primary: `tier("base",p*2+c*2)`}})
	t.Cleanup(func() { billing_setting.ReplaceBundle(bundle) })
	for _, tc := range []struct {
		name   string
		format types.RelayFormat
		stream bool
	}{
		{"messages", types.RelayFormatClaude, false}, {"messages stream", types.RelayFormatClaude, true},
		{"chat", types.RelayFormatOpenAI, false}, {"chat stream", types.RelayFormatOpenAI, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := useControllerCapacityTestDB(t)
			sql, err := db.DB()
			require.NoError(t, err)
			sql.SetMaxOpenConns(1)
			t.Cleanup(func() { sql.Close() })
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}, &model.Model{}, &model.ModelPermission{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
			redis, batch, log, sqlite := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
			common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = false, false, true, true
			t.Cleanup(func() {
				common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = redis, batch, log, sqlite
			})
			user := model.User{Id: 5101, Username: "fallback-fixture", Group: "default", Quota: initial, Status: common.UserStatusEnabled}
			token := model.Token{Id: 5102, UserId: user.Id, Key: "refusal-fixture", Group: "default", RemainQuota: initial}
			ch := model.Channel{Id: 5103, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "fixture"}
			// Recreate the opted-in v1.42.0 row and load it through the current model.
			require.NoError(t, db.Exec("ALTER TABLE tokens ADD COLUMN claude_refusal_fallback BOOLEAN DEFAULT TRUE").Error)
			for _, row := range []any{&user, &token, &ch, &model.Ability{Group: "default", Model: "claude-opus-4-8", ChannelId: ch.Id, Enabled: true}} {
				require.NoError(t, db.Create(row).Error)
			}
			require.NoError(t, db.First(&token, token.Id).Error)
			count := 0
			client := service.GetHttpClient()
			old := client.Transport
			t.Cleanup(func() { client.Transport = old })
			client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
				count++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, primary, gjson.GetBytes(body, "model").String())
				require.False(t, gjson.GetBytes(body, "fallbacks").Exists())
				require.NotContains(t, req.Header.Get("anthropic-beta"), "server-side-fallback-")
				require.Contains(t, req.Header.Get("anthropic-beta"), "test-existing")
				data := `{"type":"message","id":"msg_fixture","model":"claude-opus-5-5","content":[],"stop_reason":"refusal","stop_details":{"category":"general_harms"},"usage":{"input_tokens":8,"output_tokens":0}}`
				ct := "application/json"
				if tc.stream {
					ct = "text/event-stream"
					frames := []string{
						`{"type":"message_start","message":{"id":"msg_fixture","model":"claude-opus-5-5","usage":{"input_tokens":8,"output_tokens":0}}}`,
						`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"category":"general_harms"}},"usage":{"output_tokens":0}}`,
						`{"type":"message_stop"}`,
					}
					data = "data: " + strings.Join(frames, "\n\ndata: ") + "\n\n"
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {ct}}, Body: io.NopCloser(strings.NewReader(data)), Request: req}, nil
			})
			endpoint := "/v1/messages"
			if tc.format == types.RelayFormatOpenAI {
				endpoint = "/v1/chat/completions"
			}
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = httptest.NewRequest("POST", endpoint, strings.NewReader(fmt.Sprintf(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"OK"}],"max_tokens":128,"stream":%t}`, tc.stream)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Request.Header.Set("anthropic-beta", "test-existing")
			ctx.Set(common.RequestIdKey, "fallback-fixture")
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			require.NoError(t, middleware.SetupContextForToken(ctx, &token))
			common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
			require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, &ch, primary))
			Relay(ctx, tc.format)
			common.CleanupBodyStorage(ctx)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Equal(t, 1, count)
			if tc.format == types.RelayFormatClaude {
				require.Contains(t, w.Body.String(), "refusal")
			} else {
				require.Contains(t, w.Body.String(), "content_filter")
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.False(t, gjson.Get(logs[0].Other, "claude_fallback").Exists())
			require.Equal(t, "msg_fixture", gjson.Get(logs[0].Other, "admin_info.claude_message_id").String())
			require.Equal(t, "general_harms", gjson.Get(logs[0].Other, "admin_info.claude_refusal_category").String())
		})
	}
}
