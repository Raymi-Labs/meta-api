package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDisabledTokenFailureLogLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLogs := model.DB, model.LOG_DB
	oldRedis, oldMemory, oldEnabled := common.RedisEnabled, common.MemoryCacheEnabled, constant.ErrorLogEnabled
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, constant.ErrorLogEnabled = false, false, true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogs
		common.RedisEnabled, common.MemoryCacheEnabled, constant.ErrorLogEnabled = oldRedis, oldMemory, oldEnabled
		sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))
	user := model.User{Id: 990801, Username: "log-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 100000, Setting: `{"quota_blocked_notify_enabled":false}`}
	require.NoError(t, db.Create(&user).Error)
	for i, kind := range []string{"quota", "manual_disabled", "expired", "ip_denied", "upstream_auth", "inflight_disable"} {
		t.Run(kind, func(t *testing.T) {
			require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Log{}).Error)
			token := model.Token{Id: 990810 + i, UserId: user.Id, Name: kind, Key: fmt.Sprintf("disabledlogfixture%d", i), Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
			switch kind {
			case "quota":
				token.UnlimitedQuota = false
			case "manual_disabled":
				token.Status = common.TokenStatusDisabled
			case "expired":
				token.ExpiredTime = time.Now().Add(-time.Hour).Unix()
			case "ip_denied":
				ips := "198.51.100.0/24"
				token.AllowIps = &ips
			}
			require.NoError(t, token.Insert())
			downstreamCalls := 0
			r := gin.New()
			r.Use(RequestId(), RequestFailureLog(), RouteTag("relay"), TokenAuth())
			r.POST("/v1/responses", func(c *gin.Context) {
				downstreamCalls++
				if kind == "inflight_disable" {
					require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("status", common.TokenStatusDisabled).Error)
				}
				c.Set("original_model", "fixture")
				service.MarkRequestFailure(c, types.NewOpenAIError(errors.New("upstream authentication failed"), types.ErrorCode("upstream_authentication"), 401))
				c.Status(401)
			})
			request := func() string {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"fixture"}`))
				req.Header.Set("Authorization", "Bearer sk-"+token.Key)
				req.RemoteAddr = "192.0.2.1:1234"
				r.ServeHTTP(w, req)
				if kind == "ip_denied" {
					require.Equal(t, 403, w.Code)
				} else {
					require.Equal(t, 401, w.Code)
				}
				requestID := w.Header().Get("X-Oneapi-Request-Id")
				require.NotEmpty(t, requestID)
				if downstreamCalls == 0 {
					require.Contains(t, w.Body.String(), requestID)
				}
				return requestID
			}
			attempts, wantLogs := 1, 1
			if kind == "quota" {
				attempts, wantLogs = 8, 3
			}
			if kind == "manual_disabled" {
				attempts, wantLogs = 5, 0
			}
			var lastID string
			for n := 0; n < attempts; n++ {
				lastID = request()
			}
			var rows []model.Log
			require.NoError(t, db.Order("id").Find(&rows).Error)
			require.Len(t, rows, wantLogs)
			if kind == "upstream_auth" || kind == "inflight_disable" {
				require.Equal(t, 1, downstreamCalls)
			} else {
				require.Zero(t, downstreamCalls)
			}
			for _, row := range rows {
				require.Equal(t, user.Username, row.Username)
				require.Equal(t, token.Id, row.TokenId)
				require.Equal(t, user.Id, row.UserId)
				require.Zero(t, row.Quota)
				details, err := common.StrToMap(row.Other)
				require.NoError(t, err)
				if kind == "quota" {
					require.Equal(t, "quota", details["failure_category"])
				}
				if kind == "upstream_auth" || kind == "inflight_disable" {
					require.Equal(t, "upstream_auth", details["failure_category"])
				}
			}
			if kind != "quota" {
				return
			}
			var current model.Token
			require.NoError(t, db.First(&current, token.Id).Error)
			require.Equal(t, common.TokenStatusDisabled, current.Status)
			require.Equal(t, 3, current.QuotaFailureCount)
			require.Zero(t, current.RemainQuota)
			// List, pagination, exact request lookup and export use the same persisted facts.
			list, total, err := model.GetUserLogs(user.Id, 5, 0, 0, "", token.Name, 1, 1, "", "")
			require.NoError(t, err)
			require.EqualValues(t, 3, total)
			require.Len(t, list, 1)
			require.NotContains(t, list[0].Other, "admin_info")
			_, total, err = model.GetUserLogs(user.Id+1, 5, 0, 0, "", "", 0, 10, "", "")
			require.NoError(t, err)
			require.Zero(t, total)
			_, total, err = model.GetUserLogs(user.Id, 5, 0, 0, "", "", 0, 10, "", lastID)
			require.NoError(t, err)
			require.Zero(t, total)
			exported := 0
			require.NoError(t, model.WalkExportLogs(context.Background(), model.AdminLogQueryFilters{LogType: 5, TokenName: token.Name}, user.Id, false, func(row *model.Log) error {
				exported++
				require.NotContains(t, row.Other, "admin_info")
				return nil
			}))
			require.Equal(t, 3, exported)
			current.Status = common.TokenStatusEnabled
			require.NoError(t, current.Update())
			request()
			require.NoError(t, db.First(&current, token.Id).Error)
			require.Equal(t, 1, current.QuotaFailureCount)
			var count int64
			require.NoError(t, db.Model(&model.Log{}).Count(&count).Error)
			require.EqualValues(t, 4, count)
		})
	}
}
