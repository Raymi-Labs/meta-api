package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestQuotaNoticeFinalOutcome(t *testing.T) {
	oldQueue, oldRunning, oldLog := quotaNoticeQueue, quotaNoticeRunning.Load(), constant.ErrorLogEnabled
	quotaNoticeQueue = make(chan quotaNoticeInput, 100)
	quotaNoticeRunning.Store(true)
	constant.ErrorLogEnabled = false
	t.Cleanup(func() {
		quotaNoticeQueue = oldQueue
		quotaNoticeRunning.Store(oldRunning)
		constant.ErrorLogEnabled = oldLog
	})
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		for _, kind := range []string{"local", "upstream", "db_error", "auth", "fallback_success", "free", "query"} {
			t.Run(path+kind, func(t *testing.T) {
				for len(quotaNoticeQueue) > 0 {
					<-quotaNoticeQueue
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"fixture"}`))
				c.Set("id", 7)
				c.Set("token_id", 4)
				c.Set(common.RequestIdKey, "fixture-request")
				c.Set("original_model", "fixture")
				c.Set("token_name", "private-name")
				quota := &types.QuotaRejection{Source: "wallet", Known: true, Available: 1, Required: 9, Message: "local refusal"}
				switch kind {
				case "local":
					MarkRequestFailure(c, types.NewErrorWithStatusCode(quota, types.ErrorCodeInsufficientUserQuota, 403))
				case "upstream":
					MarkRequestFailure(c, types.NewOpenAIError(quota, types.ErrorCodeInsufficientUserQuota, 403))
				case "db_error":
					MarkRequestFailure(c, types.NewErrorWithStatusCode(errors.New("database failed"), types.ErrorCodeInsufficientUserQuota, 403))
				case "auth":
					c.Status(401)
				case "fallback_success":
					MarkQuotaNoticeSettlement(c, &relaycommon.RelayInfo{BillingSource: BillingSourceSubscription, SubscriptionId: 8, TokenId: 4, OriginModelName: "fixture"})
				case "query":
					c.Request.Method = http.MethodGet
					MarkRequestFailure(c, types.NewErrorWithStatusCode(quota, types.ErrorCodeInsufficientUserQuota, 403))
				}
				ObserveQuotaNoticeOutcome(c)
				ObserveQuotaNoticeOutcome(c)
				switch kind {
				case "local":
					require.Len(t, quotaNoticeQueue, 1)
					got := <-quotaNoticeQueue
					require.False(t, got.Success)
					require.Equal(t, "wallet:0", got.Resource.Scope)
				case "fallback_success":
					require.Len(t, quotaNoticeQueue, 4)
					for len(quotaNoticeQueue) > 0 {
						got := <-quotaNoticeQueue
						require.True(t, got.Success)
						require.NotEqual(t, "wallet", got.Resource.Source)
					}
				default:
					require.Empty(t, quotaNoticeQueue)
				}
			})
		}
	}
}

func TestQuotaNoticeRedisFallback(t *testing.T) {
	addr := os.Getenv("TEST_QUOTA_REDIS_ADDR")
	if addr == "" {
		addr = miniredis.RunT(t).Addr()
	}
	old, enabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: addr})
	common.RedisEnabled = true
	t.Cleanup(func() { _ = common.RDB.Close(); common.RDB = old; common.RedisEnabled = enabled })
	now := time.Now().UnixMilli()
	r := model.QuotaNoticeResource{Scope: fmt.Sprintf("test:%d", now), RequestID: "one", LastAt: now - 600000}
	keys := quotaNoticeCacheKeys(900090, r.Scope)
	t.Cleanup(func() { common.RDB.Del(context.Background(), keys...) })
	_, ok := readQuotaNoticeCache(900090, r, now-300000, now)
	require.False(t, ok)
	cacheQuotaNoticeSignal(900090, r, 1)
	for i := 2; i <= 4; i++ {
		r.RequestID = fmt.Sprint(i)
		r.LastAt = now - int64(4-i)*31000
		cacheQuotaNoticeSignal(900090, r, int64(i))
	}
	r.Count = 4
	stats, ok := readQuotaNoticeCache(900090, r, now-300000, now)
	require.True(t, ok)
	require.EqualValues(t, 3, stats.Count)
	require.EqualValues(t, 62000, stats.LastAt-stats.FirstAt)
	// A lost write or out-of-order writer must fall back to durable SQL.
	r.Count = 5
	_, ok = readQuotaNoticeCache(900090, r, now-300000, now)
	require.False(t, ok)
	r.LastAt = now
	cacheQuotaNoticeSignal(900090, r, 6)
	r.Count = 6
	_, ok = readQuotaNoticeCache(900090, r, now-300000, now)
	require.False(t, ok)
	require.NoError(t, common.RDB.Del(context.Background(), keys...).Err())
	_, ok = readQuotaNoticeCache(900090, r, now-300000, now)
	require.False(t, ok)
	require.NoError(t, common.RDB.Close())
	_, ok = readQuotaNoticeCache(900090, r, now-300000, now)
	require.False(t, ok)
}

func TestQuotaNoticeTrackedHTTP(t *testing.T) {
	fetch := system_setting.GetFetchSetting()
	oldFetch := *fetch
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() { *fetch = oldFetch })
	old := httpClient
	httpClient = &http.Client{}
	t.Cleanup(func() { httpClient = old })
	for _, kind := range []string{"accepted", "rejected", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "unknown" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					_ = conn.Close()
					return
				}
				if kind == "rejected" {
					w.WriteHeader(503)
				}
			}))
			defer srv.Close()
			// Local fixtures explicitly disable SSRF only for this test.
			got := SendUserNotificationTracked("", dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, WebhookUrl: srv.URL}, dto.NewNotify(dto.NotifyTypeQuotaBlocked, "fixture", "fixture", nil))
			want := map[string]string{"accepted": model.QuotaNoticeSubmitted, "rejected": model.QuotaNoticeFailed, "unknown": model.QuotaNoticeUnknown}[kind]
			require.Equal(t, want, got.Status)
		})
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	closedURL := "http://" + listener.Addr().String()
	require.NoError(t, listener.Close())
	disconnected := SendUserNotificationTracked("", dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, WebhookUrl: closedURL}, dto.Notify{})
	require.Equal(t, model.QuotaNoticeFailed, disconnected.Status)
	got := SendUserNotificationTracked("", dto.UserSetting{}, dto.Notify{})
	require.Equal(t, model.QuotaNoticeSkipped, got.Status)
}

func TestQuotaNoticeMessageEscapes(t *testing.T) {
	r := model.QuotaNoticeResource{TokenName: `<img src=x onerror=alert(1)>`, ModelName: "fixture", Source: "wallet", Count: 584, Available: 3, Required: 9, Known: true, RequestID: "request-fixture"}
	msg := quotaNoticeMessage(1, []model.QuotaNoticeResource{r}, dto.NotifyTypeEmail)
	require.NotContains(t, msg.Content, "<img")
	require.Contains(t, msg.Content, "&lt;img")
	require.Contains(t, msg.Content, "584")
	require.Contains(t, msg.Content, "预扣额不等于实际费用")
	require.Equal(t, dto.NotifyTypeQuotaBlocked, msg.Type)
	off := false
	require.True(t, (dto.UserSetting{}).QuotaBlockedNotificationsEnabled())
	require.False(t, (dto.UserSetting{QuotaBlockedNotifyEnabled: &off}).QuotaBlockedNotificationsEnabled())
}

func TestQuotaNoticeAuthRestrictionsAndDisabledDelivery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldRedis := model.DB, common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() { model.DB = oldDB; common.RedisEnabled = oldRedis; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Id: 900090, Username: "quota-auth", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "quota-auth"}
	require.NoError(t, db.Create(&user).Error)
	for _, kind := range []string{"valid", "expired", "disabled_token", "ip", "model", "channel", "user_disabled", "opt_out", "not_quota"} {
		t.Run(kind, func(t *testing.T) {
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{"status": common.UserStatusEnabled, "setting": "{}"}).Error)
			token := model.Token{Id: 4, UserId: user.Id, Status: common.TokenStatusExhausted, ExpiredTime: -1}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"allowed"}`))
			c.Request.RemoteAddr = "192.0.2.1:1234"
			var rejected error = &types.QuotaRejection{Source: "token", ResourceID: 4, Message: "quota"}
			switch kind {
			case "expired":
				token.ExpiredTime = 1
			case "disabled_token":
				token.Status = common.TokenStatusDisabled
			case "ip":
				ips := "198.51.100.0/24"
				token.AllowIps = &ips
			case "model":
				token.ModelLimitsEnabled = true
				token.ModelLimits = "different"
			case "channel":
				c.Set("quota_notice_specific_channel", "8")
			case "user_disabled":
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error)
			case "opt_out":
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("setting", `{"quota_blocked_notify_enabled":false}`).Error)
			case "not_quota":
				rejected = errors.New("invalid token")
			}
			MarkQuotaNoticeAuthRejection(c, &token, rejected)
			_, ok := c.Get("quota_notice_auth_rejection")
			require.Equal(t, kind == "valid", ok)
		})
	}
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("setting", `{"quota_blocked_notify_enabled":false}`).Error)
	status, reason, _, _ := deliverQuotaNotice(&model.QuotaNotice{UserID: user.Id})
	require.Equal(t, model.QuotaNoticeSkipped, status)
	require.Equal(t, "notifications_disabled", reason)
}

func TestQuotaNoticeAuthoritativeBillingSources(t *testing.T) {
	truncate(t)
	seedUser(t, 93001, 3)
	seedToken(t, 93002, 93001, "quota-notice-billing", 3)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{UserId: 93001, TokenId: 93002, TokenKey: "quota-notice-billing", ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	_, apiErr := NewBillingSession(c, info, 9)
	var rejected *types.QuotaRejection
	require.ErrorAs(t, apiErr, &rejected)
	require.Equal(t, "wallet", rejected.Source)
	require.EqualValues(t, 3, rejected.Available)
	require.EqualValues(t, 9, rejected.Required)
	rejected = nil
	require.ErrorAs(t, PreConsumeTokenQuota(info, 9), &rejected)
	require.Equal(t, "token", rejected.Source)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 93002).Updates(map[string]interface{}{"remain_quota": 100, "package_enabled": true, "package_limit_quota": 5, "package_period": "hourly", "package_next_reset_time": time.Now().Add(time.Hour).Unix()}).Error)
	rejected = nil
	require.ErrorAs(t, PreConsumeTokenQuota(info, 9), &rejected)
	require.Equal(t, "token_package", rejected.Source)
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	info.IsPlayground = true
	info.RequestId = "quota-notice-no-sub"
	info.UserSetting.BillingPreference = "subscription_only"
	_, apiErr = NewBillingSession(c, info, 9)
	rejected = nil
	require.ErrorAs(t, apiErr, &rejected)
	require.Equal(t, "subscription", rejected.Source)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 93003, Title: "quota-fixture", QuotaResetPeriod: "never"}).Error)
	t.Cleanup(func() {
		model.DB.Delete(&model.SubscriptionPlan{}, 93003)
		model.DB.Where("user_id = ?", 93001).Delete(&model.SubscriptionPreConsumeRecord{})
	})
	seedSubscription(t, 93004, 93001, 100, 0)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", 93004).Update("plan_id", 93003).Error)
	info.RequestId = "quota-notice-fallback"
	info.UserSetting.BillingPreference = "wallet_first"
	session, apiErr := NewBillingSession(c, info, 9)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	require.Equal(t, BillingSourceSubscription, info.BillingSource)
}
