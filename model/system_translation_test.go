package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSystemTranslationTokenValidation(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&Token{}, &User{}))
			owner := User{Id: 101, Username: "translation-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&owner).Error)
			token := Token{Id: 101, UserId: 101, Key: "test-key-not-for-production", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100, ModelLimitsEnabled: true, ModelLimits: SystemTranslationModel}
			require.NoError(t, db.Create(&token).Error)
			_, err := ValidateSystemTranslationToken(context.Background(), 101, 101)
			require.NoError(t, err)
			_, err = ValidateSystemTranslationToken(context.Background(), 101, 102)
			require.Error(t, err)
			for _, changes := range []map[string]any{
				{"status": common.TokenStatusDisabled}, {"expired_time": time.Now().Unix() - 1}, {"remain_quota": 0}, {"model_limits_enabled": false}, {"model_limits": "qwen-mt-flash,qwen-plus"}, {"routing_config": `{"groups":["default"]}`},
			} {
				require.NoError(t, db.Model(&Token{}).Where("id = ?", 101).Updates(changes).Error)
				_, err = ValidateSystemTranslationToken(context.Background(), 101, 101)
				require.Error(t, err)
				require.NoError(t, db.Save(&token).Error)
			}
			require.NoError(t, db.Model(&User{}).Where("id = ?", 101).Update("role", common.RoleAdminUser).Error)
			_, err = ValidateSystemTranslationToken(context.Background(), 101, 101)
			require.Error(t, err)
		})
	}
}
