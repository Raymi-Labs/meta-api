package controller

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskFinalQuotaOutcome(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	old, oldRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() { model.DB, common.RedisEnabled = old, oldRedis; sqlDB, _ := db.DB(); sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	for i, kind := range []string{"local", "upstream", "database"} {
		t.Run(kind, func(t *testing.T) {
			token := model.Token{Id: 901200 + i, Key: kind, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}
			require.NoError(t, token.Insert())
			for n := 0; n < 3; n++ {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
				service.MarkTokenQuotaIdentity(c, &token)
				var cause error = &types.QuotaRejection{Source: "wallet", Message: "local quota"}
				if kind == "database" {
					cause = errors.New("database unavailable")
				}
				apiErr := types.NewErrorWithStatusCode(cause, types.ErrorCodeInsufficientUserQuota, 403)
				taskErr := service.TaskErrorFromAPIError(apiErr)
				taskErr.LocalError = kind != "upstream"
				respondTaskError(c, taskErr)
				service.ObserveTokenQuotaOutcome(c)
				service.ObserveTokenQuotaOutcome(c)
			}
			var got model.Token
			require.NoError(t, db.First(&got, token.Id).Error)
			if kind == "local" {
				require.Equal(t, common.TokenStatusDisabled, got.Status)
				require.Equal(t, 3, got.QuotaFailureCount)
			} else {
				require.Zero(t, got.QuotaFailureCount)
			}
		})
	}
}
