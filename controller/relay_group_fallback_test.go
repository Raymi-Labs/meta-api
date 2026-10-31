package controller

import (
	"fmt"
	"io"
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
		routing                                    string
		name                                       string
		on, unavailable, healthy, allFail, limited bool
		affinity, capacity                         bool
		retries, primaryCalls, backupCalls, status int
	}{
		{name: "affinity_highest_priority", affinity: true, retries: 1, primaryCalls: 1, backupCalls: 1, status: 200},
		{name: "affinity_capacity_highest_priority", affinity: true, capacity: true, retries: 1, primaryCalls: 1, backupCalls: 1, status: 200},
		{name: "affinity_no_extra_budget", affinity: true, retries: 0, primaryCalls: 1, status: 503},
		{name: "scoped_model_to_backup", routing: `{"groups":["primary","backup"],"model_groups":{"group-fallback-model":["backup"],"group-other-model":["primary"]}}`, on: true, retries: 1, backupCalls: 1, status: 200},
		{name: "scoped_no_global_expansion", routing: `{"groups":["primary"]}`, on: true, retries: 1, primaryCalls: 1, status: 503},
		{name: "scoped_fallback", routing: `{"groups":["primary","backup"]}`, retries: 1, primaryCalls: 1, backupCalls: 1, status: 200},
		{name: "scoped_all_fail_refund", routing: `{"groups":["primary","backup"]}`, allFail: true, retries: 1, primaryCalls: 1, backupCalls: 1, status: 503},
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
			affinity := operation_setting.GetChannelAffinitySetting()
			concurrency := operation_setting.GetChannelConcurrencySetting()
			oldAffinity, oldConcurrency := *affinity, *concurrency
			t.Setenv("CHANNEL_CONCURRENCY_ENABLED", "")
			concurrency.Enabled = tc.capacity
			concurrency.AffinityStickyEnabled = true
			concurrency.DefaultRpmLimit = 0
			if tc.affinity {
				*affinity = operation_setting.ChannelAffinitySetting{Enabled: true, DefaultTTLSeconds: 60, MaxEntries: 100,
					Rules: []operation_setting.ChannelAffinityRule{{Name: tc.name, ModelRegex: []string{"^group-fallback-model$"}, IncludeRuleName: true,
						KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "context_string", Key: "affinity_fixture"}}}}}
			}
			t.Cleanup(func() {
				if tc.affinity {
					_, err := service.ClearChannelAffinityCacheByRuleName(tc.name)
					require.NoError(t, err)
				}
				*affinity, *concurrency = oldAffinity, oldConcurrency
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
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"group-fallback-model":1,"group-other-model":1}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"primary":1,"backup":3,"last":5}`))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"primary":"","backup":"","last":""}`))
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["backup","last"]`))
			const initialQuota = 1000000
			user := &model.User{Id: 998601, Username: "fallback-fixture", Group: "primary", Role: common.RoleCommonUser, Quota: initialQuota, Status: common.UserStatusEnabled}
			token := &model.Token{Id: 998602, UserId: user.Id, Key: "fallback-fixture-token", Group: "primary", RoutingConfig: tc.routing, AutoGroupFallback: tc.on, Status: common.TokenStatusEnabled, RemainQuota: initialQuota, ChannelLimitsEnabled: tc.limited, ChannelLimits: "998603"}
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
				body, _ := io.ReadAll(r.Body)
				if tc.healthy || strings.Contains(string(body), "group-other-model") {
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
				priority := int64(1)
				if tc.affinity {
					group = "primary"
					priority = []int64{2, 3, 1}[i]
				}
				base := backup.URL
				if i == 0 {
					base = primary.URL
				}
				status := common.ChannelStatusEnabled
				if i == 0 && tc.unavailable {
					status = common.ChannelStatusManuallyDisabled
				}
				ch := &model.Channel{Id: 998603 + i, Name: group, Group: group, Models: "group-fallback-model", Type: constant.ChannelTypeOpenAI, Status: status, Key: "fixture-key", BaseURL: &base, AutoBan: common.GetPointer(0), Priority: &priority}
				require.NoError(t, db.Create(ch).Error)
				require.NoError(t, db.Create(&model.Ability{Group: group, Model: "group-fallback-model", ChannelId: ch.Id, Enabled: status == common.ChannelStatusEnabled, Priority: &priority, Weight: 1}).Error)
			}
			if tc.name == "scoped_model_to_backup" {
				require.NoError(t, db.Create(&model.Ability{Group: "primary", Model: "group-other-model", ChannelId: 998603, Enabled: true, Priority: common.GetPointer(int64(1)), Weight: 1}).Error)
			}
			r := gin.New()
			r.Use(middleware.RequestFailureLog(), middleware.RouteTag("relay"))
			r.POST("/v1/chat/completions", func(c *gin.Context) {
				require.NoError(t, middleware.SetupContextForToken(c, token))
				common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
				common.SetContextKey(c, constant.ContextKeyUserRole, user.Role)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, token.Group)
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				c.Set(common.RequestIdKey, "fallback-"+tc.name+c.GetHeader("X-Fixture-Suffix"))
				if tc.affinity {
					c.Set("affinity_fixture", tc.name)
					_, found := service.GetPreferredChannelByAffinity(c, "group-fallback-model", "primary")
					require.False(t, found)
					service.RecordChannelAffinity(c, 998603)
				}
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
				if tc.healthy || tc.affinity {
					expectedGroup, expectedRatio = "primary", 1
				}
				if tc.affinity {
					require.Equal(t, 998604, log.ChannelId, "choose the highest remaining tier, not the lowest fallback")
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
			if tc.name == "scoped_model_to_backup" {
				w2 := httptest.NewRecorder()
				req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"group-other-model","messages":[{"role":"user","content":"OK"}]}`))
				req2.Header.Set("Content-Type", "application/json")
				req2.Header.Set("X-Fixture-Suffix", "-second")
				r.ServeHTTP(w2, req2)
				require.Equal(t, 200, w2.Code, w2.Body.String())
				require.EqualValues(t, 1, primaryCalls.Load())
				require.EqualValues(t, 1, backupCalls.Load())
				var all []model.Log
				require.NoError(t, db.Where("token_id = ? AND type = ?", token.Id, model.LogTypeConsume).Order("id").Find(&all).Error)
				require.Len(t, all, 2)
				require.Equal(t, "primary", all[1].Group)
				require.Equal(t, 3*all[1].Quota, all[0].Quota, "same usage uses the selected model's 1x or 3x group")
				require.Eventually(t, func() bool {
					var balance model.Token
					var wallet model.User
					return db.First(&balance, token.Id).Error == nil && db.First(&wallet, user.Id).Error == nil && balance.RemainQuota == initialQuota-all[0].Quota-all[1].Quota && wallet.Quota == balance.RemainQuota
				}, 3*time.Second, 10*time.Millisecond)
			}

		})
	}
}
