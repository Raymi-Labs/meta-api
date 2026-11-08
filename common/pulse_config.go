package common

import (
	"os"
	"strings"
	"sync"
)

// PulseSettingsOptionKey contains an atomic configuration, including secrets.
// It must never be exposed by the generic option listing or updated one field
// at a time. Older deployment environment variables remain the fallback.
const PulseSettingsOptionKey = "PulseSettingsSecret"

var pulseConfigEnv = map[string]string{
	"PulseEnv":                        "PULSE_ENV",
	"PulseServiceHMACSecret":          "PULSE_SERVICE_HMAC_SECRET",
	"PulseServiceHMACSecretPrevious":  "PULSE_SERVICE_HMAC_SECRET_PREVIOUS",
	"PulseRollbackHMACSecret":         "PULSE_ROLLBACK_HMAC_SECRET",
	"PulseRollbackHMACSecretPrevious": "PULSE_ROLLBACK_HMAC_SECRET_PREVIOUS",
	"PulseForumSSOSecret":             "PULSE_FORUM_SSO_SECRET",
	"PulseForumSSOSecretPrevious":     "PULSE_FORUM_SSO_SECRET_PREVIOUS",
	"PulseForumSSOCallbackURL":        "PULSE_FORUM_SSO_CALLBACK_URL",
	"PulseBenefitEnabled":             "PULSE_BENEFIT_ENABLED",
	"PulseBenefitMaxGrantQuota":       "PULSE_BENEFIT_MAX_GRANT_QUOTA",
	"PulseBenefitUserDailyQuota":      "PULSE_BENEFIT_USER_DAILY_QUOTA",
	"PulseBenefitDailyQuota":          "PULSE_BENEFIT_DAILY_QUOTA",
}

var pulseConfigState = struct {
	sync.RWMutex
	values map[string]string
}{}

type PulseConfig map[string]string

func IsPulseConfigKey(key string) bool {
	_, ok := pulseConfigEnv[key]
	return ok || key == PulseSettingsOptionKey
}

func IsPulseSecretKey(key string) bool {
	_, ok := pulseConfigEnv[key]
	return ok && strings.Contains(key, "Secret")
}

// GetPulseConfig returns one immutable-by-convention request snapshot. Explicit
// empty database values suppress environment fallback, which makes clearing a
// retired key and disabling an environment-enabled feature unambiguous.
func GetPulseConfig() PulseConfig {
	return ResolvePulseConfig(GetPulseConfigOverrides())
}

func GetPulseConfigOverrides() map[string]string {
	pulseConfigState.RLock()
	defer pulseConfigState.RUnlock()
	values := make(map[string]string, len(pulseConfigState.values))
	for key, value := range pulseConfigState.values {
		values[key] = value
	}
	return values
}

// ResolvePulseConfig applies an explicit database override set to the current
// environment. Unedited fields keep following their existing deployment values.
func ResolvePulseConfig(overrides map[string]string) PulseConfig {
	values := make(PulseConfig, len(pulseConfigEnv))
	for key, env := range pulseConfigEnv {
		value, set := overrides[key]
		if !set {
			value = os.Getenv(env)
			if value == "" {
				switch key {
				case "PulseEnv":
					value = "development"
				case "PulseBenefitEnabled":
					value = "false"
				}
			}
		}
		values[key] = value
	}
	return values
}

// ReplacePulseConfig copies the map before publishing it; callers may safely
// reuse their input. Passing nil resets the snapshot to environment fallback.
func ReplacePulseConfig(values map[string]string) {
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	pulseConfigState.Lock()
	pulseConfigState.values = copy
	pulseConfigState.Unlock()
}

func (cfg PulseConfig) Production() bool {
	return strings.EqualFold(strings.TrimSpace(cfg["PulseEnv"]), "production")
}

func (cfg PulseConfig) SecretUsable(secret string) bool {
	secret = strings.TrimSpace(secret)
	return secret != "" && (!cfg.Production() || (len(secret) >= 32 && secret != "replace-me"))
}

// Retired settings are accepted only when reading historical configuration.
func IsRetiredPulseConfigKey(key string) bool {
	switch key {
	case "PulseUsageLogRequired", "PulseInternalURL", "PulseUserBFFHMACSecret", "PulseAdminHMACSecret":
		return true
	}
	return false
}
