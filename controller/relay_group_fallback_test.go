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
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayPreferredGroupFallback(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitTokenEncoders()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	for _, tc := range []struct {
		name                                       string
		on, unavailable, healthy, allFail, limited bool
		retries, primaryCalls, backupCalls, status int
	}{
		{name: "preferred_healthy", on: true, healthy: true, retries: 1, primaryCalls: 1, status: 200},
		{name: "unavailable_on", on: true, unavailable: true, retries: 1, backupCalls: 1, status: 200},
		{name: "unavailable_off", unavailable: true, retries: 1, status: 503},
		{name: "upstream_failure_on", on: true, retries: 1, primaryCalls: 1, backupCalls: 1, status: 200},
		{name: "upstream_failure_off", retries: 1, primaryCalls: 1, status: 503},
		{name: "budget_exhausted", on: true, allFail: true, retries: 1, primaryCalls: 1, backupCalls: 1, status: 503},
		{name: "retries_disabled", on: true, retries: 0, primaryCalls: 1, status: 503},
		{name: "channel_restricted", on: true, limited: true, retries: 1, primaryCalls: 1, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := useControllerCapacityTestDB(t)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.ModelPermission{}))
			oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite
			constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = true, false, false, tc.retries, false, true, true
			t.Cleanup(func() {
				constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite = oldLog, oldRedis, oldCount, oldRetry, oldBatch, oldConsume, oldSQLite
			})
			general := operation_setting.GetGeneralSetting()
			oldPing := general.PingIntervalEnabled
			general.PingIntervalEnabled = false
			t.Cleanup(func() { general.PingIntervalEnabled = oldPing })
			oldModel, oldGroup, oldAuto, oldUsable := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString(), setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString()
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModel))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
			})
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"group-fallback-model":1}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"primary":1,"backup":3,"last":5}`))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"primary":"","backup":"","last":""}`))
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["backup","last"]`))
			const initialQuota = 1000000
			user := &model.User{Id: 998601, Username: "fallback-fixture", Group: "primary", Role: common.RoleCommonUser, Quota: initialQuota, Status: common.UserStatusEnabled}
			token := &model.Token{Id: 998602, UserId: user.Id, Key: "fallback-fixture-token", Group: "primary", AutoGroupFallback: tc.on, Status: common.TokenStatusEnabled, RemainQuota: initialQuota, ChannelLimitsEnabled: tc.limited, ChannelLimits: "998603"}
			require.NoError(t, db.Create(user).Error)
			require.NoError(t, db.Create(token).Error)
			var primaryCalls, backupCalls atomic.Int32
			success := func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"chatcmpl-fixture","object":"chat.completion","model":"group-fallback-model","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
			}
			failure := func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(503)
				fmt.Fprint(w, `{"error":{"type":"service_unavailable_error","message":"fixture unavailable"}}`)
			}
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				primaryCalls.Add(1)
				if tc.healthy {
					success(w)
				} else {
					failure(w)
				}
			}))
			t.Cleanup(primary.Close)
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				backupCalls.Add(1)
				if tc.allFail {
					failure(w)
				} else {
					success(w)
				}
			}))
			t.Cleanup(backup.Close)
			for i, group := range []string{"primary", "backup", "last"} {
				base := backup.URL
				if i == 0 {
					base = primary.URL
				}
				status := common.ChannelStatusEnabled
				if i == 0 && tc.unavailable {
					status = common.ChannelStatusManuallyDisabled
				}
				ch := &model.Channel{Id: 998603 + i, Name: group, Group: group, Models: "group-fallback-model", Type: constant.ChannelTypeOpenAI, Status: status, Key: "fixture-key", BaseURL: &base, AutoBan: common.GetPointer(0)}
				require.NoError(t, db.Create(ch).Error)
				require.NoError(t, db.Create(&model.Ability{Group: group, Model: "group-fallback-model", ChannelId: ch.Id, Enabled: status == common.ChannelStatusEnabled, Priority: common.GetPointer(int64(1)), Weight: 1}).Error)
			}
			r := gin.New()
			r.Use(middleware.RequestFailureLog(), middleware.RouteTag("relay"))
			r.POST("/v1/chat/completions", func(c *gin.Context) {
				require.NoError(t, middleware.SetupContextForToken(c, token))
				common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
				common.SetContextKey(c, constant.ContextKeyUserRole, user.Role)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, token.Group)
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				c.Set(common.RequestIdKey, "fallback-"+tc.name)
			}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"group-fallback-model","messages":[{"role":"user","content":"OK"}]}`))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.EqualValues(t, tc.primaryCalls, primaryCalls.Load())
			require.EqualValues(t, tc.backupCalls, backupCalls.Load())
			var logs []model.Log
			require.NoError(t, db.Where("request_id = ?", "fallback-"+tc.name).Find(&logs).Error)
			require.Len(t, logs, 1)
			log := logs[0]
			if tc.status == 200 {
				require.Equal(t, model.LogTypeConsume, log.Type)
				expectedGroup, expectedRatio := "backup", float64(3)
				if tc.healthy {
					expectedGroup, expectedRatio = "primary", 1
				}
				require.Equal(t, expectedGroup, log.Group)
				var other map[string]interface{}
				require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
				require.Equal(t, expectedRatio, other["group_ratio"])
				require.Positive(t, log.Quota)
			} else {
				require.Equal(t, model.LogTypeError, log.Type)
				require.Zero(t, log.Quota)
			}
			require.Eventually(t, func() bool {
				var gotUser model.User
				var gotToken model.Token
				return db.First(&gotUser, user.Id).Error == nil && db.First(&gotToken, token.Id).Error == nil && gotUser.Quota == initialQuota-log.Quota && gotToken.RemainQuota == initialQuota-log.Quota
			}, 3*time.Second, 10*time.Millisecond, "charge once on final group; failed requests refund")
		})
	}
}
