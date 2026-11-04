package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise real selection, HTTP adapters, refusal retry and wallet settlement,
// rather than treating successful semaphore acquisition as a successful relay.
func TestVMCapacityFullRelayRefusalAndBilling(t *testing.T) {
	if runRelayMatrixInSubprocess(t) {
		return
	}
	db := useRelayMatrixProcessDB(t)
	require.NoError(t, i18n.Init())
	service.InitTokenEncoders()
	service.InitHttpClient()
	enableControllerCapacityForTest(t)
	common.RetryTimes, constant.CountToken, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = 2, false, false, true, true
	operation_setting.GetGeneralSetting().PingIntervalEnabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-capacity-fixture":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"claude-capacity-fixture":1}`))
	const initial = 1000000
	user := &model.User{Id: 778910, Username: "capacity-relay", Group: "default", Role: common.RoleCommonUser, Quota: initial, Status: common.UserStatusEnabled}
	token := &model.Token{Id: 778911, UserId: user.Id, Key: "fixture-token", Group: "default", Status: common.TokenStatusEnabled, RemainQuota: initial}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(token).Error)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/channel/capacity" {
			fmt.Fprintf(w, `{"schema_version":1,"model":"claude-capacity-fixture","observed_at":%q,"max_age_ms":1000,"state_quality":"known","effective_admission_limit":4,"available_now":4,"key_inflight":0}`, time.Now().UTC().Format(time.RFC3339Nano))
			return
		}
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","code":"channel_capacity_busy","message":"fixture capacity busy"}}`)
			return
		}
		fmt.Fprint(w, `{"id":"msg_capacity_fixture","type":"message","role":"assistant","model":"claude-capacity-fixture","content":[{"type":"text","text":"CAPACITY_OK"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer server.Close()
	base, settings := server.URL, `{"max_concurrency":10,"upstream_capacity":{"enabled":true,"overflow_policy":"wait","max_wait_ms":3000}}`
	ch := &model.Channel{Id: 778912, Name: "vm-capacity", Group: "default", Models: "claude-capacity-fixture", Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "fixture-key", BaseURL: &base, Setting: &settings, AutoBan: common.GetPointer(1)}
	require.NoError(t, db.Create(ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: ch.Models, ChannelId: ch.Id, Enabled: true, Weight: 1}).Error)
	r := gin.New()
	r.POST("/v1/messages", func(c *gin.Context) {
		require.NoError(t, middleware.SetupContextForToken(c, token))
		common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
		common.SetContextKey(c, constant.ContextKeyUserRole, user.Role)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, token.Group)
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		c.Set(common.RequestIdKey, "vm-capacity-full-relay")
	}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatClaude) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-capacity-fixture","max_tokens":32,"messages":[{"role":"user","content":"OK"}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "CAPACITY_OK")
	require.EqualValues(t, 2, calls.Load())
	var logs []model.Log
	require.NoError(t, db.Where("request_id = ? AND type = ?", "vm-capacity-full-relay", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Positive(t, logs[0].Quota)
	require.Eventually(t, func() bool {
		var u model.User
		var tok model.Token
		return db.First(&u, user.Id).Error == nil && db.First(&tok, token.Id).Error == nil && u.Quota == initial-logs[0].Quota && tok.RemainQuota == initial-logs[0].Quota
	}, 2*time.Second, 10*time.Millisecond)
	require.Zero(t, service.ChannelRateLimitCooldownRemaining(ch.Id))
	require.False(t, service.IsChannelModelCircuitOpen(ch, ch.Models))
	var current model.Channel
	require.NoError(t, db.First(&current, ch.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, current.Status)
	inflight, err := middleware.QueryChannelConcurrency(ch.Id)
	require.NoError(t, err)
	require.Zero(t, inflight)
}
