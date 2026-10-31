package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenQuotaAutoDisableHTTP(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldRedis, oldMemory, oldLog := model.DB, common.RedisEnabled, common.MemoryCacheEnabled, constant.ErrorLogEnabled
	model.DB, common.RedisEnabled, common.MemoryCacheEnabled, constant.ErrorLogEnabled = db, false, false, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.MemoryCacheEnabled, constant.ErrorLogEnabled = oldDB, oldRedis, oldMemory, oldLog
		sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.UserSubscription{}, &model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	user := model.User{Id: 901001, Username: "auto-disable", Status: common.UserStatusEnabled, Group: "default", Quota: 100000, Setting: `{"quota_blocked_notify_enabled":false}`}
	require.NoError(t, db.Create(&user).Error)
	for i, kind := range []string{"auth_token", "preconsume_token", "package", "wallet", "subscription", "upstream", "database", "success_break", "other_break", "query", "ip_denied"} {
		t.Run(kind, func(t *testing.T) {
			require.NoError(t, db.Model(&user).Update("quota", 100000).Error)
			token := model.Token{Id: 901100 + i, UserId: user.Id, Key: fmt.Sprintf("quotahttp%d", i), RemainQuota: 100, Status: common.TokenStatusEnabled, ExpiredTime: -1}
			if kind == "auth_token" || kind == "ip_denied" {
				token.RemainQuota = 0
			}
			if kind == "ip_denied" {
				ips := "198.51.100.0/24"
				token.AllowIps = &ips
			}
			if kind == "package" {
				token.PackageEnabled = true
				token.PackageLimitQuota = 10
				token.PackageUsedQuota = 10
				token.PackagePeriod = "daily"
				token.PackageNextResetTime = time.Now().Add(time.Hour).Unix()
			}
			if kind == "wallet" {
				require.NoError(t, db.Model(&user).Update("quota", 0).Error)
			}
			require.NoError(t, token.Insert())
			attempt := 0
			router := gin.New()
			router.Use(RequestFailureLog(), RouteTag("relay"), TokenAuth())
			handler := func(c *gin.Context) {
				attempt++
				var apiErr *types.NewAPIError
				switch kind {
				case "preconsume_token":
					e := service.PreConsumeTokenQuota(&relaycommon.RelayInfo{TokenId: token.Id, TokenKey: token.Key}, 200)
					require.Error(t, e)
					apiErr = types.NewErrorWithStatusCode(e, types.ErrorCodePreConsumeTokenQuotaFailed, 403)
				case "wallet", "subscription":
					pref := "wallet_only"
					if kind == "subscription" {
						pref = "subscription_only"
					}
					_, apiErr = service.NewBillingSession(c, &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: pref}, RequestId: fmt.Sprint(attempt)}, 10)
					require.NotNil(t, apiErr)
				case "upstream":
					apiErr = types.NewOpenAIError(errors.New("quota insufficient"), types.ErrorCodeInsufficientUserQuota, 403)
				case "database":
					apiErr = types.NewErrorWithStatusCode(errors.New("database unavailable"), types.ErrorCodePreConsumeTokenQuotaFailed, 403)
				default:
					if kind == "success_break" && attempt == 3 {
						c.Status(200)
						return
					}
					if kind == "other_break" && attempt == 3 {
						c.Status(429)
						return
					}
					apiErr = types.NewErrorWithStatusCode(&types.QuotaRejection{Source: "wallet", Message: "insufficient"}, types.ErrorCodeInsufficientUserQuota, 403)
				}
				service.MarkRequestFailure(c, apiErr)
				c.Status(apiErr.StatusCode)
			}
			router.POST("/v1/chat/completions", handler)
			router.GET("/v1/models", handler)
			request := func(method, path string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(method, path, strings.NewReader(`{"model":"fixture"}`))
				req.Header.Set("Authorization", "Bearer sk-"+token.Key)
				req.RemoteAddr = "192.0.2.1:1234"
				router.ServeHTTP(w, req)
				return w
			}
			read := func() model.Token {
				var got model.Token
				require.NoError(t, db.First(&got, token.Id).Error)
				return got
			}
			for n := 1; n <= 3; n++ {
				method, path := "POST", "/v1/chat/completions"
				if kind == "query" {
					method, path = "GET", "/v1/models"
				}
				request(method, path)
				if n < 3 {
					require.NotEqual(t, common.TokenStatusDisabled, read().Status)
				}
			}
			switch kind {
			case "upstream", "database", "success_break", "other_break", "query", "ip_denied":
				require.Zero(t, read().QuotaFailureCount)
				require.NotEqual(t, common.TokenStatusDisabled, read().Status)
				if kind == "success_break" || kind == "other_break" {
					request("POST", "/v1/chat/completions")
					require.Equal(t, 1, read().QuotaFailureCount)
				}
			default:
				require.Equal(t, 3, read().QuotaFailureCount)
				require.Equal(t, common.TokenStatusDisabled, read().Status)
				response := request("POST", "/v1/chat/completions")
				require.Equal(t, http.StatusUnauthorized, response.Code)
				require.Contains(t, response.Body.String(), "连续三次")
				fresh := read()
				fresh.Status = common.TokenStatusEnabled
				fresh.RemainQuota = 100
				fresh.PackageUsedQuota = 0
				require.NoError(t, fresh.Update())
				request("POST", "/v1/chat/completions")
				require.Equal(t, 1, read().QuotaFailureCount)
				require.NotEqual(t, common.TokenStatusDisabled, read().Status)
			}
		})
	}
}
