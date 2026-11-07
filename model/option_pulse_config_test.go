package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPulseOptionTestDB(t *testing.T) {
	t.Helper()
	oldDB, oldOverrides, oldLogs := DB, common.GetPulseConfigOverrides(), common.LogConsumeEnabled
	oldRedisEnabled, oldRedis := common.RedisEnabled, common.RDB
	oldOptions := common.OptionMap
	t.Cleanup(func() {
		DB = oldDB
		common.ReplacePulseConfig(oldOverrides)
		common.LogConsumeEnabled = oldLogs
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRedis
		common.OptionMap = oldOptions
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	DB = db
	require.NoError(t, DB.AutoMigrate(&Option{}))
	common.ReplacePulseConfig(nil)
	common.OptionMap = map[string]string{}
	common.RedisEnabled, common.RDB = false, nil
	t.Setenv("PULSE_ENV", "development")
	t.Setenv("PULSE_BENEFIT_ENABLED", "false")
}

func pulseSavedOverrides(t *testing.T) map[string]string {
	t.Helper()
	var row Option
	require.NoError(t, DB.Where(&Option{Key: common.PulseSettingsOptionKey}).First(&row).Error)
	var values map[string]string
	require.NoError(t, common.UnmarshalJsonStr(row.Value, &values))
	return values
}

func TestPulseOptionsPreserveUntouchedEnvironmentAndClearExplicitly(t *testing.T) {
	setupPulseOptionTestDB(t)
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "existing-worker-secret")
	t.Setenv("PULSE_ROLLBACK_HMAC_SECRET", "existing-rollback-secret")
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{
		"PulseInternalURL": "http://pulse.internal:8088", "PulseServiceHMACSecret": "  ",
	}}))
	require.Equal(t, map[string]string{"PulseInternalURL": "http://pulse.internal:8088"}, pulseSavedOverrides(t))
	require.Equal(t, "existing-worker-secret", common.GetPulseConfig()["PulseServiceHMACSecret"])
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "environment-rotation")
	require.Equal(t, "environment-rotation", common.GetPulseConfig()["PulseServiceHMACSecret"])
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{ClearSecrets: []string{"PulseServiceHMACSecret"}}))
	require.Empty(t, common.GetPulseConfig()["PulseServiceHMACSecret"])
	common.ReplacePulseConfig(nil)
	loadOptionsFromDatabase()
	require.Empty(t, common.GetPulseConfig()["PulseServiceHMACSecret"], "clear must survive restart despite the environment")
}

func TestPulseOptionsLoadLegacyValuesAndSaveUsesLatestDatabaseKeys(t *testing.T) {
	setupPulseOptionTestDB(t)
	require.NoError(t, DB.Create(&[]Option{{Key: "PulseInternalURL", Value: "http://legacy:8088"}, {Key: "PulseAdminHMACSecret", Value: "legacy-admin-secret"}}).Error)
	loadOptionsFromDatabase()
	require.Equal(t, "legacy-admin-secret", common.GetPulseConfig()["PulseAdminHMACSecret"])
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseServiceHMACSecret": "first-worker-secret"}}))
	stale := common.GetPulseConfigOverrides()
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseServiceHMACSecret": "latest-worker-secret"}}))
	// Simulate another application's stale option-sync snapshot. The transaction
	// must merge against the committed database value, never overwrite its key.
	common.ReplacePulseConfig(stale)
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseBenefitMaxGrantQuota": "500"}}))
	require.Equal(t, "latest-worker-secret", common.GetPulseConfig()["PulseServiceHMACSecret"])
	require.Equal(t, "legacy-admin-secret", common.GetPulseConfig()["PulseAdminHMACSecret"])
}

func TestPulseOptionsValidationIsAtomicAndCannotUseGenericOptionEndpoint(t *testing.T) {
	setupPulseOptionTestDB(t)
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseInternalURL": "http://original:8088"}}))
	before := pulseSavedOverrides(t)
	for _, update := range []PulseConfigUpdate{
		{Config: map[string]string{"PulseBenefitEnabled": "true", "PulseServiceHMACSecret": "new-secret"}},
		{Config: map[string]string{"PulseBenefitMaxGrantQuota": "1.5"}},
		{Config: map[string]string{"PulseServiceHMACSecret": "same-key", "PulseRollbackHMACSecret": "same-key"}},
		{Config: map[string]string{"PulseEnv": "production", "PulseServiceHMACSecret": "short"}},
		{Config: map[string]string{"PulseForumSSOCallbackURL": "http://forum/api/user-center/login/callback"}},
		{Config: map[string]string{"PulseServiceHMACSecretPrevious": "retired-without-current"}},
	} {
		require.Error(t, UpdatePulseConfig(update))
		require.Equal(t, before, pulseSavedOverrides(t))
		require.Equal(t, before, common.GetPulseConfigOverrides())
	}
	for _, key := range []string{"PulseBenefitEnabled", "PulseInternalURL", "PulseServiceHMACSecretPrevious", common.PulseSettingsOptionKey} {
		require.Error(t, UpdateOption(key, "true"))
	}
}

func TestPulseBenefitDoesNotRequireOrEnableConsumeLogs(t *testing.T) {
	setupPulseOptionTestDB(t)
	update := PulseConfigUpdate{Config: map[string]string{
		"PulseEnv": "production", "PulseBenefitEnabled": "true",
		"PulseServiceHMACSecret": strings.Repeat("a", 32), "PulseRollbackHMACSecret": strings.Repeat("b", 32),
		"PulseBenefitMaxGrantQuota": "50", "PulseBenefitUserDailyQuota": "100", "PulseBenefitDailyQuota": "500",
	}}
	require.ErrorContains(t, UpdatePulseConfig(update), "Redis")
	var count int64
	require.NoError(t, DB.Model(&Option{}).Count(&count).Error)
	require.Zero(t, count, "failed first save must not leave a partial configuration")
	server := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = common.RDB.Close() })
	common.RedisEnabled = true
	common.LogConsumeEnabled = false
	require.NoError(t, UpdatePulseConfig(update))
	require.False(t, common.LogConsumeEnabled)
	require.NoError(t, UpdateOption("LogConsumeEnabled", "false"))
	var logOption Option
	require.NoError(t, DB.Where(&Option{Key: "LogConsumeEnabled"}).First(&logOption).Error)
	require.Equal(t, "false", logOption.Value)
	policy, err := loadPulseBenefitPolicy()
	require.NoError(t, err)
	require.Equal(t, int64(50), policy.maxGrant)
	common.ReplacePulseConfig(nil)
	loadOptionsFromDatabase()
	policy, err = loadPulseBenefitPolicy()
	require.NoError(t, err)
	require.Equal(t, int64(500), policy.daily)
	t.Setenv("PULSE_BENEFIT_ENABLED", "true")
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseBenefitEnabled": "false"}}))
	_, err = loadPulseBenefitPolicy()
	require.ErrorIs(t, err, ErrPulseBenefitPaused)
}

func TestPulseOptionSyncFailureRetainsLastConfiguration(t *testing.T) {
	setupPulseOptionTestDB(t)
	t.Setenv("PULSE_BENEFIT_ENABLED", "true")
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseBenefitEnabled": "false"}}))
	before := common.GetPulseConfigOverrides()
	require.NoError(t, DB.Migrator().DropTable(&Option{}))
	loadOptionsFromDatabase()
	require.Equal(t, before, common.GetPulseConfigOverrides())
	_, err := loadPulseBenefitPolicy()
	require.ErrorIs(t, err, ErrPulseBenefitPaused)
}

func TestPulseOptionsRejectRetiredUsageLogSetting(t *testing.T) {
	setupPulseOptionTestDB(t)
	require.Error(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseUsageLogRequired": "true"}}))
}

func TestPulseOptionsRejectSecretReuseAfterWhitespaceNormalization(t *testing.T) {
	setupPulseOptionTestDB(t)
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "  existing-worker-secret\n")
	require.ErrorContains(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{
		"PulseRollbackHMACSecret": "existing-worker-secret",
	}}), "必须使用不同密钥")
}

func TestPulseConsumeLogToggleIsIndependentOfPulseConfiguration(t *testing.T) {
	setupPulseOptionTestDB(t)
	require.NoError(t, UpdatePulseConfig(PulseConfigUpdate{Config: map[string]string{"PulseBenefitEnabled": "false"}}))
	require.NoError(t, UpdateOption("LogConsumeEnabled", "false"))
	var row Option
	require.NoError(t, DB.Where(&Option{Key: "LogConsumeEnabled"}).First(&row).Error)
	require.Equal(t, "false", row.Value)
}

func TestPulseSettingsIgnoreRetiredUsageLogFieldInSavedBundle(t *testing.T) {
	setupPulseOptionTestDB(t)
	require.NoError(t, DB.Create(&Option{Key: common.PulseSettingsOptionKey, Value: `{"PulseEnv":"development","PulseUsageLogRequired":"true","PulseInternalURL":"http://pulse.internal"}`}).Error)
	common.LogConsumeEnabled = false
	loadOptionsFromDatabase()
	require.Equal(t, "http://pulse.internal", common.GetPulseConfig()["PulseInternalURL"])
	if _, ok := common.GetPulseConfig()["PulseUsageLogRequired"]; ok {
		t.Fatal("retired usage-log setting was loaded")
	}
	require.False(t, common.LogConsumeEnabled)
}
