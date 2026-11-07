package model

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var pulseConfigOptionMutex sync.Mutex

type PulseConfigUpdate struct {
	Config       map[string]string `json:"config"`
	ClearSecrets []string          `json:"clear_secrets,omitempty"`
}

func loadPulseConfigOptions(options []*Option) {
	values := map[string]string{}
	var bundle string
	for _, option := range options {
		if option.Key == common.PulseSettingsOptionKey {
			bundle = option.Value
		} else if common.IsPulseConfigKey(option.Key) && option.Value != "" {
			// Preserve the original three console options. Historically an empty
			// legacy option meant environment fallback, unlike the new bundle.
			values[option.Key] = option.Value
		}
	}
	if bundle != "" {
		var persisted map[string]string
		if err := common.UnmarshalJsonStr(bundle, &persisted); err != nil {
			common.SysLog("failed to load Meta Pulse settings: invalid configuration")
			return
		}
		for key, value := range persisted {
			if key == "PulseUsageLogRequired" {
				continue
			}
			if !common.IsPulseConfigKey(key) || key == common.PulseSettingsOptionKey {
				common.SysLog("failed to load Meta Pulse settings: unknown configuration field")
				return
			}
			values[key] = value
		}
	}
	publishPulseConfig(values)
}

func publishPulseConfig(values map[string]string) {
	common.ReplacePulseConfig(values)
}

func validatePulseConfig(cfg common.PulseConfig) error {
	switch cfg["PulseEnv"] {
	case "production", "development", "test":
	default:
		return errors.New("Pulse 运行环境必须为 production、development 或 test")
	}
	for _, key := range []string{"PulseBenefitEnabled"} {
		if cfg[key] != "true" && cfg[key] != "false" {
			return fmt.Errorf("%s 必须为 true 或 false", key)
		}
	}
	for _, key := range []string{"PulseBenefitMaxGrantQuota", "PulseBenefitUserDailyQuota", "PulseBenefitDailyQuota"} {
		raw := cfg[key]
		if raw == "" && cfg["PulseBenefitEnabled"] != "true" {
			continue
		}
		value, err := strconv.ParseInt(raw, 10, strconv.IntSize)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
			return fmt.Errorf("%s 必须为不超过系统整数范围的正整数额度", key)
		}
	}
	if raw := cfg["PulseInternalURL"]; raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("Pulse 内网地址必须为不含用户名、密码、查询参数或片段的 HTTP(S) 地址")
		}
	}
	if raw := cfg["PulseForumSSOCallbackURL"]; raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/api/user-center/login/callback" {
			return errors.New("社区 SSO 回调必须为 HTTPS 地址，路径为 /api/user-center/login/callback，且不含凭据、查询参数或片段")
		}
	}
	// All roles, including both rotation slots, remain distinct. Otherwise a
	// compromised user/SSO key could grant credit or authorize a rollback.
	seen := make(map[string]string)
	for key, value := range cfg {
		value = strings.TrimSpace(value)
		if !common.IsPulseSecretKey(key) || value == "" {
			continue
		}
		if !cfg.SecretUsable(value) {
			return fmt.Errorf("%s 无效：生产环境密钥至少需要 32 个字符", key)
		}
		if other, exists := seen[value]; exists {
			return fmt.Errorf("%s 与 %s 必须使用不同密钥", key, other)
		}
		seen[value] = key
	}
	for _, pair := range [][2]string{{"PulseServiceHMACSecret", "PulseServiceHMACSecretPrevious"}, {"PulseRollbackHMACSecret", "PulseRollbackHMACSecretPrevious"}, {"PulseForumSSOSecret", "PulseForumSSOSecretPrevious"}} {
		if strings.TrimSpace(cfg[pair[0]]) == "" && strings.TrimSpace(cfg[pair[1]]) != "" {
			return fmt.Errorf("设置 %s 前必须配置当前密钥", pair[1])
		}
	}
	if cfg["PulseBenefitEnabled"] == "true" {
		if strings.TrimSpace(cfg["PulseServiceHMACSecret"]) == "" || strings.TrimSpace(cfg["PulseRollbackHMACSecret"]) == "" {
			return errors.New("开启自动发奖前必须配置独立的发奖密钥和撤销密钥")
		}
		if cfg.Production() {
			if !common.RedisEnabled || common.RDB == nil {
				return errors.New("生产环境开启自动发奖需要配置可用的 Redis 防重放存储")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := common.RDB.Ping(ctx).Err(); err != nil {
				return errors.New("Redis 不可用，无法开启生产环境自动发奖")
			}
		}
	}
	return nil
}

// UpdatePulseConfig persists all fields in one row and publishes only after the
// transaction commits. It shares a mutex with option sync and consume-log writes.
func UpdatePulseConfig(update PulseConfigUpdate) error {
	pulseConfigOptionMutex.Lock()
	defer pulseConfigOptionMutex.Unlock()
	initial := common.GetPulseConfigOverrides()
	initialRaw, err := common.Marshal(initial)
	if err != nil {
		return errors.New("无法保存 Pulse 配置")
	}
	var overrides map[string]string
	var validationErr error
	err = DB.Transaction(func(tx *gorm.DB) error {
		// The bundle can contain keys in bound parameters. Suppress GORM SQL
		// logging on this transaction, including driver errors.
		tx = tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(logger.Silent)})
		row := Option{Key: common.PulseSettingsOptionKey, Value: string(initialRaw)}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		// Write before reading: this serializes processes in PostgreSQL/MySQL
		// and reserves SQLite's writer before its snapshot, avoiding lost keys
		// when two nodes save partially edited forms concurrently.
		if err := tx.Model(&Option{}).Where(&Option{Key: common.PulseSettingsOptionKey}).UpdateColumn("value", gorm.Expr("?", clause.Column{Name: "value"})).Error; err != nil {
			return err
		}
		if err := tx.Where(&Option{Key: common.PulseSettingsOptionKey}).First(&row).Error; err != nil {
			return err
		}
		overrides = initial
		var persisted map[string]string
		if err := common.UnmarshalJsonStr(row.Value, &persisted); err != nil {
			return err
		}
		for key, value := range persisted {
			overrides[key] = value
		}
		if err := applyPulseConfigUpdate(overrides, update); err != nil {
			validationErr = err
			return err
		}
		raw, err := common.Marshal(overrides)
		if err != nil {
			return err
		}
		if err := tx.Model(&Option{}).Where(&Option{Key: common.PulseSettingsOptionKey}).Update("value", string(raw)).Error; err != nil {
			return err
		}
		return nil
	})
	if validationErr != nil {
		return validationErr
	}
	if err != nil {
		return errors.New("保存 Pulse 配置失败，请检查数据库状态")
	}
	publishPulseConfig(overrides)
	return nil
}

func applyPulseConfigUpdate(overrides map[string]string, update PulseConfigUpdate) error {
	for key, value := range update.Config {
		if !common.IsPulseConfigKey(key) || key == common.PulseSettingsOptionKey {
			return fmt.Errorf("未知的 Pulse 配置项：%s", key)
		}
		value = strings.TrimSpace(value)
		if common.IsPulseSecretKey(key) && value == "" {
			continue
		}
		overrides[key] = value
	}
	for _, key := range update.ClearSecrets {
		if !common.IsPulseSecretKey(key) {
			return fmt.Errorf("%s 不是可清除的 Pulse 密钥", key)
		}
		if strings.TrimSpace(update.Config[key]) != "" {
			return fmt.Errorf("不能同时修改和清除 %s", key)
		}
		overrides[key] = ""
	}
	return validatePulseConfig(common.ResolvePulseConfig(overrides))
}
