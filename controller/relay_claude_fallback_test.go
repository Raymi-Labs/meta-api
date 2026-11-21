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
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRelayClaudeOfficialFallback(t *testing.T) {
	const primary = "claude-opus-5-5"
	const target = "claude-opus-4-8"
	const initial = 2000000
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	bundle := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{primary: "tiered_expr", target: "tiered_expr"}, BillingExpr: map[string]string{primary: `tier("base",p*2+c*2)`, target: `tier("base",p*6+c*6)`}})
	t.Cleanup(func() { billing_setting.ReplaceBundle(bundle) })
	for _, tc := range []struct {
		name                              string
		format                            types.RelayFormat
		stream, enabled, allowed, refused bool
		wantQuota                         int
	}{
		{"off", types.RelayFormatClaude, false, false, true, false, 10},
		{"model not allowed", types.RelayFormatClaude, false, true, false, false, 10},
		{"target in another group", types.RelayFormatClaude, false, true, true, false, 10},
		{"complete", types.RelayFormatClaude, false, true, true, false, 36},
		{"converted complete", types.RelayFormatOpenAI, false, true, true, false, 36},
		{"midstream", types.RelayFormatClaude, true, true, true, false, 139},
		{"converted stream", types.RelayFormatOpenAI, true, true, true, false, 139},
		{"alternative refuses", types.RelayFormatOpenAI, false, true, true, true, 0},
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
			token := model.Token{Id: 5102, UserId: user.Id, Key: "fallback-fixture", Group: "default", RemainQuota: initial, ClaudeRefusalFallback: tc.enabled, ModelLimitsEnabled: true, ModelLimits: primary}
			if tc.allowed {
				token.ModelLimits += "," + target
			}
			ch := model.Channel{Id: 5103, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "fixture"}
			group := "default"
			if tc.name == "target in another group" {
				group = "vip"
			}
			for _, row := range []any{&user, &token, &ch, &model.Ability{Group: group, Model: target, ChannelId: ch.Id, Enabled: true}} {
				require.NoError(t, db.Create(row).Error)
			}
			expected := tc.enabled && tc.allowed && group == "default"
			count := 0
			client := service.GetHttpClient()
			old := client.Transport
			t.Cleanup(func() { client.Transport = old })
			client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
				count++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, expected, gjson.GetBytes(body, "fallbacks").Exists())
				require.Equal(t, expected, strings.Contains(req.Header.Get("anthropic-beta"), "server-side-fallback-"))
				require.Contains(t, req.Header.Get("anthropic-beta"), "test-existing")
				data := `{"type":"message","id":"msg_fixture","model":"claude-opus-5-5","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":8,"output_tokens":2}}`
				ct := "application/json"
				if expected {
					require.Equal(t, target, gjson.GetBytes(body, "fallbacks.0.model").String())
					in, out := 100, 0
					if tc.stream {
						out = 3
					}
					block := `{"type":"fallback","from":{"model":"claude-opus-5-5"},"to":{"model":"claude-opus-4-8"},"trigger":{"type":"refusal","category":"general_harms"}}`
					stop, completion := "end_turn", 2
					if tc.refused {
						stop, completion = "refusal", 0
					}
					usage := fmt.Sprintf(`{"input_tokens":10,"output_tokens":%d,"iterations":[{"type":"message","model":"claude-opus-5-5","input_tokens":%d,"output_tokens":%d},{"type":"fallback_message","model":"claude-opus-4-8","input_tokens":10,"output_tokens":%d}]}`, completion, in, out, completion)
					if !tc.refused && !tc.stream {
						block += `,{"type":"text","text":"first "},{"type":"text","text":"second"}`
					}
					data = fmt.Sprintf(`{"type":"message","id":"msg_fixture","model":"claude-opus-4-8","content":[%s],"stop_reason":"%s","usage":%s}`, block, stop, usage)
					if tc.stream {
						ct = "text/event-stream"
						frames := []string{
							`{"type":"message_start","message":{"id":"msg_fixture","model":"claude-opus-5-5","usage":{"input_tokens":100,"output_tokens":0}}}`,
							`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"before "}}`,
							`{"type":"content_block_start","index":1,"content_block":` + block + `}`,
							`{"type":"content_block_stop","index":1}`,
							`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"after"}}`,
							`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":` + usage + `}`,
							`{"type":"message_stop"}`,
						}
						data = "data: " + strings.Join(frames, "\n\ndata: ") + "\n\n"
					}
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
			if tc.name == "converted complete" {
				require.Equal(t, "first second", gjson.Get(w.Body.String(), "choices.0.message.content").String())
			}
			if tc.stream {
				require.Contains(t, w.Body.String(), "before ")
				require.Contains(t, w.Body.String(), "after")
				require.Contains(t, w.Body.String(), target)
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, tc.wantQuota, logs[0].Quota)
			require.Equal(t, expected, gjson.Get(logs[0].Other, "claude_fallback").Exists())
			if expected {
				require.Equal(t, "msg_fixture", gjson.Get(logs[0].Other, "admin_info.claude_message_id").String())
				require.Equal(t, "general_harms", gjson.Get(logs[0].Other, "admin_info.claude_primary_refusal_category").String())
				require.Equal(t, target, gjson.Get(logs[0].Other, "claude_fallback.serving_model").String())
				require.EqualValues(t, 2, gjson.Get(logs[0].Other, "claude_fallback.attempts.#").Int())
			}
			require.Eventually(t, func() bool {
				var u model.User
				var tok model.Token
				return db.First(&u, user.Id).Error == nil && db.First(&tok, token.Id).Error == nil && u.Quota == initial-tc.wantQuota && tok.RemainQuota == initial-tc.wantQuota
			}, 2*time.Second, 10*time.Millisecond)
		})
	}
}
