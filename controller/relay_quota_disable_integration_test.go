package controller

import (
	"fmt"
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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the actual Relay defer, which appends a request ID before the outer
// final-result observers run. Synthetic MarkRequestFailure calls miss this step.
func TestRelayQuotaDisablePreservesFinalCause(t *testing.T) {
	for _, protocol := range []struct {
		path, body string
		format     types.RelayFormat
	}{
		{"/v1/chat/completions", `{"model":"quota-chain-fixture","messages":[{"role":"user","content":"hello"}],"max_tokens":100000}`, types.RelayFormatOpenAI},
		{"/v1/messages", `{"model":"quota-chain-fixture","messages":[{"role":"user","content":"hello"}],"max_tokens":100000}`, types.RelayFormatClaude},
		{"/v1/responses", `{"model":"quota-chain-fixture","input":"hello","max_output_tokens":100000}`, types.RelayFormatOpenAIResponses},
		{"/v1beta/models/quota-chain-fixture:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":100000}}`, types.RelayFormatGemini},
	} {
		for _, source := range []string{"wallet", "token", "token_package", "subscription"} {
			t.Run(protocol.path+"/"+source, func(t *testing.T) {
				db := useControllerCapacityTestDB(t)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)
				t.Cleanup(func() { sqlDB.Close() })
				require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.UserSubscription{}, &model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
				oldSQLite := common.UsingSQLite
				common.UsingSQLite = true
				t.Cleanup(func() { common.UsingSQLite = oldSQLite })
				oldRedis, oldMemory, oldCount, oldLog := common.RedisEnabled, common.MemoryCacheEnabled, constant.CountToken, constant.ErrorLogEnabled
				common.RedisEnabled, common.MemoryCacheEnabled, constant.CountToken, constant.ErrorLogEnabled = false, false, false, false
				t.Cleanup(func() {
					common.RedisEnabled, common.MemoryCacheEnabled, constant.CountToken, constant.ErrorLogEnabled = oldRedis, oldMemory, oldCount, oldLog
				})
				oldRatio := ratio_setting.ModelRatio2JSONString()
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"quota-chain-fixture":1}`))
				t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
				if service.GetHttpClient() == nil {
					service.InitHttpClient()
				}
				client := service.GetHttpClient()
				oldTransport := client.Transport
				client.Transport = healthTestTransport(func(*http.Request) (*http.Response, error) {
					t.Error("quota rejection must precede upstream call")
					return nil, fmt.Errorf("unexpected upstream")
				})
				t.Cleanup(func() { client.Transport = oldTransport })
				user := model.User{Id: 998901, Username: "quota-chain-user", Group: "default", Quota: 100000000, Status: common.UserStatusEnabled, Setting: `{"billing_preference":"wallet_only","quota_blocked_notify_enabled":false}`}
				token := model.Token{Id: 998902, UserId: user.Id, Key: "quotachainfixture", Status: common.TokenStatusEnabled, RemainQuota: 100000000, UnlimitedQuota: true, ExpiredTime: -1}
				switch source {
				case "wallet":
					user.Quota = 11379
				case "token":
					token.UnlimitedQuota = false
					token.RemainQuota = 1
				case "token_package":
					token.PackageEnabled = true
					token.PackageLimitQuota = 1
					token.PackagePeriod = "daily"
					token.PackageNextResetTime = time.Now().Add(time.Hour).Unix()
				case "subscription":
					user.Setting = `{"billing_preference":"subscription_only","quota_blocked_notify_enabled":false}`
				}
				channel := model.Channel{Id: 998903, Type: constant.ChannelTypeOpenAI, Key: "fixture", Status: common.ChannelStatusEnabled}
				for _, row := range []any{&user, &token, &channel} {
					require.NoError(t, db.Create(row).Error)
				}
				r := gin.New()
				r.Use(middleware.RequestId(), middleware.RequestFailureLog(), middleware.RouteTag("relay"), middleware.TokenAuth())
				var lastRequestID string
				r.POST(protocol.path, func(c *gin.Context) {
					lastRequestID = c.GetString(common.RequestIdKey)
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "quota-chain-fixture"))
					defer common.CleanupBodyStorage(c)
					Relay(c, protocol.format)
				})
				for n := 1; n <= 4; n++ {
					req := httptest.NewRequest("POST", protocol.path, strings.NewReader(protocol.body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer sk-"+token.Key)
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					if n <= 3 {
						require.Equal(t, 403, w.Code, w.Body.String())
						require.Contains(t, w.Body.String(), lastRequestID)
					} else {
						require.Equal(t, 401, w.Code, w.Body.String())
						require.Contains(t, w.Body.String(), "连续三次")
					}
					var current model.Token
					require.NoError(t, db.First(&current, token.Id).Error)
					require.Equal(t, min(n, 3), current.QuotaFailureCount)
					if n < 3 {
						require.Equal(t, common.TokenStatusEnabled, current.Status)
					} else {
						require.Equal(t, common.TokenStatusDisabled, current.Status)
					}
				}
			})
		}
	}
}
