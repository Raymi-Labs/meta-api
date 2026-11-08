package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// External DSNs must name disposable test databases. Each fixture creates and
// removes only its uniquely prefixed options table; no existing tables are used.
// SQLite always runs; unset external DSNs leave those dialects explicitly skipped.
func TestPulseConfigDatabaseIntegration(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupPulseConfigIntegrationDB(t, dialect)
			reset := func(t *testing.T) {
				t.Helper()
				require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Option{}).Error)
				common.ReplacePulseConfig(nil)
				common.LogConsumeEnabled = true
				common.OptionMap = map[string]string{}
			}

			t.Run("partial_save_preserves_environment_and_latest_database_secret", func(t *testing.T) {
				reset(t)
				t.Setenv("PULSE_SERVICE_HMAC_SECRET", "config-test-environment-worker")
				require.NoError(t, db.Create(&Option{Key: "PulseRollbackHMACSecret", Value: "config-test-legacy-admin"}).Error)
				loadOptionsFromDatabase()
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{
					"PulseForumSSOCallbackURL": "https://pulse.internal:8088/api/user-center/login/callback", "PulseServiceHMACSecret": "  ",
				}}))
				require.NotContains(t, pulseSavedOverrides(t), "PulseServiceHMACSecret", "blank secret must not freeze environment fallback")
				require.Equal(t, "config-test-environment-worker", common.GetPulseConfig()["PulseServiceHMACSecret"])
				t.Setenv("PULSE_SERVICE_HMAC_SECRET", "config-test-rotated-environment-worker")
				require.Equal(t, "config-test-rotated-environment-worker", common.GetPulseConfig()["PulseServiceHMACSecret"])
				stale := common.GetPulseConfigOverrides()
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseServiceHMACSecret": "config-test-latest-worker"}}))
				common.ReplacePulseConfig(stale)
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseBenefitMaxGrantQuota": "50"}}))
				require.Equal(t, "config-test-latest-worker", common.GetPulseConfig()["PulseServiceHMACSecret"], "stale instance must merge the committed bundle")
				require.Equal(t, "config-test-legacy-admin", common.GetPulseConfig()["PulseRollbackHMACSecret"])
				require.Equal(t, "50", pulseSavedOverrides(t)["PulseBenefitMaxGrantQuota"])
			})

			t.Run("clear_current_and_previous_secrets_survives_reload", func(t *testing.T) {
				reset(t)
				t.Setenv("PULSE_SERVICE_HMAC_SECRET", "config-test-environment-worker")
				t.Setenv("PULSE_SERVICE_HMAC_SECRET_PREVIOUS", "config-test-environment-old-worker")
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{ClearSecrets: []string{"PulseServiceHMACSecretPrevious"}}))
				require.Empty(t, common.GetPulseConfig()["PulseServiceHMACSecretPrevious"])
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{ClearSecrets: []string{"PulseServiceHMACSecret"}}))
				common.ReplacePulseConfig(nil)
				loadOptionsFromDatabase()
				require.Empty(t, common.GetPulseConfig()["PulseServiceHMACSecret"])
				require.Empty(t, common.GetPulseConfig()["PulseServiceHMACSecretPrevious"])
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseServiceHMACSecret": " "}}))
				require.Contains(t, pulseSavedOverrides(t), "PulseServiceHMACSecret")
				require.Empty(t, pulseSavedOverrides(t)["PulseServiceHMACSecret"], "blank input must retain explicit clearing")
			})

			t.Run("invalid_save_is_atomic", func(t *testing.T) {
				reset(t)
				require.Error(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseBenefitEnabled": "true"}}))
				var count int64
				require.NoError(t, db.Model(&Option{}).Count(&count).Error)
				require.Zero(t, count, "failed initial save must roll back the locking row")
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseForumSSOCallbackURL": "https://original.internal:8088/api/user-center/login/callback"}}))
				before := pulseSavedOverrides(t)
				for _, update := range []PulseConfigUpdate{
					{Config: map[string]string{"PulseForumSSOCallbackURL": "https://changed.internal:8088/api/user-center/login/callback", "PulseBenefitMaxGrantQuota": "1.5"}},
					{Config: map[string]string{"PulseServiceHMACSecret": "config-test-duplicate", "PulseRollbackHMACSecret": "config-test-duplicate"}},
					{Config: map[string]string{"PulseServiceHMACSecret": "config-test-new-worker"}, ClearSecrets: []string{"PulseServiceHMACSecret"}},
				} {
					require.Error(t, UpdatePulseConfig(update))
					require.Equal(t, before, pulseSavedOverrides(t))
					require.Equal(t, before, common.GetPulseConfigOverrides())
				}
			})

			t.Run("retired_usage_log_field_rejection_does_not_change_logs", func(t *testing.T) {
				reset(t)
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseForumSSOCallbackURL": "https://original.internal:8088/api/user-center/login/callback"}}))
				before := pulseSavedOverrides(t)
				common.LogConsumeEnabled = false
				const callback = "test:pulse_config_log_insert_failure"
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if option, ok := tx.Statement.Dest.(*Option); ok && option.Key == "LogConsumeEnabled" {
						tx.AddError(errors.New("injected consume-log write failure"))
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(callback)) })
				require.Error(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{
					"PulseUsageLogRequired": "true", "PulseServiceHMACSecret": "config-test-must-not-commit",
				}}))
				require.Equal(t, before, pulseSavedOverrides(t))
				require.Equal(t, before, common.GetPulseConfigOverrides())
				require.False(t, common.LogConsumeEnabled)
				var count int64
				require.NoError(t, db.Model(&Option{}).Where(&Option{Key: "LogConsumeEnabled"}).Count(&count).Error)
				require.Zero(t, count)
			})

			t.Run("saved_policy_and_log_guard_reload_without_restart", func(t *testing.T) {
				reset(t)
				common.LogConsumeEnabled = false
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{
					"PulseBenefitEnabled":    "true",
					"PulseServiceHMACSecret": "config-test-worker", "PulseRollbackHMACSecret": "config-test-rollback",
					"PulseBenefitMaxGrantQuota": "50", "PulseBenefitUserDailyQuota": "100", "PulseBenefitDailyQuota": "500",
				}}))
				require.False(t, common.LogConsumeEnabled)
				require.NoError(t, UpdateOption("LogConsumeEnabled", "false"))
				policy, err := loadPulseBenefitPolicy()
				require.NoError(t, err)
				require.EqualValues(t, 50, policy.maxGrant)
				common.ReplacePulseConfig(map[string]string{"PulseBenefitEnabled": "false"})
				var logOption Option
				require.NoError(t, db.Where(&Option{Key: "LogConsumeEnabled"}).First(&logOption).Error)
				require.Equal(t, "false", logOption.Value)
				common.ReplacePulseConfig(nil)
				common.LogConsumeEnabled = false
				loadOptionsFromDatabase()
				require.False(t, common.LogConsumeEnabled)
				require.NoError(t, UpdateOption("LogConsumeEnabled", "false"))
				policy, err = loadPulseBenefitPolicy()
				require.NoError(t, err)
				require.EqualValues(t, 500, policy.daily)
				t.Setenv("PULSE_BENEFIT_ENABLED", "true")
				require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseBenefitEnabled": "false"}}))
				_, err = loadPulseBenefitPolicy()
				require.ErrorIs(t, err, ErrPulseBenefitPaused)
			})
		})
	}
}

func setupPulseConfigIntegrationDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	if dialect == "sqlite" {
		driver = sqlite.Open(filepath.Join(t.TempDir(), "pulse-config.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	} else {
		env := "TEST_PULSE_CONFIG_MYSQL_DSN"
		if dialect == "postgres" {
			env = "TEST_PULSE_CONFIG_POSTGRES_DSN"
		}
		dsn := os.Getenv(env)
		if dsn == "" {
			t.Skip("disposable database not configured: " + env)
		}
		driver = mysql.Open(dsn)
		if dialect == "postgres" {
			driver = postgres.Open(dsn)
		}
	}
	db, err := gorm.Open(driver, &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("pulse_cfg_%d_", time.Now().UnixNano())},
	})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&Option{}))
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&Option{})) })

	oldDB, oldOverrides, oldLogs := DB, common.GetPulseConfigOverrides(), common.LogConsumeEnabled
	oldOptions := common.OptionMap
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	t.Cleanup(func() {
		DB = oldDB
		common.ReplacePulseConfig(oldOverrides)
		common.LogConsumeEnabled = oldLogs
		common.OptionMap = oldOptions
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPostgres
		initCol()
	})
	DB = db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = dialect == "sqlite", dialect == "mysql", dialect == "postgres"
	initCol()
	common.ReplacePulseConfig(nil)
	common.OptionMap = map[string]string{}
	for _, env := range []string{
		"PULSE_ENV", "PULSE_INTERNAL_URL", "PULSE_USER_BFF_HMAC_SECRET", "PULSE_ADMIN_HMAC_SECRET",
		"PULSE_SERVICE_HMAC_SECRET", "PULSE_SERVICE_HMAC_SECRET_PREVIOUS", "PULSE_ROLLBACK_HMAC_SECRET", "PULSE_ROLLBACK_HMAC_SECRET_PREVIOUS",
		"PULSE_FORUM_SSO_SECRET", "PULSE_FORUM_SSO_SECRET_PREVIOUS", "PULSE_FORUM_SSO_CALLBACK_URL", "PULSE_BENEFIT_ENABLED",
		"PULSE_BENEFIT_MAX_GRANT_QUOTA", "PULSE_BENEFIT_USER_DAILY_QUOTA", "PULSE_BENEFIT_DAILY_QUOTA",
	} {
		t.Setenv(env, "")
	}
	return db
}
