package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// PaidServiceTierRegistration is keyed by the exact public model ID. Disabled
// channels retain registration so an outage cannot silently remove the policy.
var managedPaidServiceTierModels = map[string]bool{}
var invalidPaidServiceTierRegistry bool

// Caller holds channelSyncLock. Build once on metadata refresh, not per request.
func refreshPaidServiceTierRegistryLocked() {
	next := map[string]bool{}
	invalid := false
	for _, ch := range channelsIDM {
		if ch == nil || !strings.Contains(ch.OtherSettings, "paid_service_tiers") {
			continue
		}
		settings, err := readPaidTierSettings(ch)
		if err != nil {
			invalid = true
			continue
		}
		for name, tiers := range settings.PaidServiceTiers {
			if len(tiers) > 0 {
				next[name] = true
			}
		}
	}
	managedPaidServiceTierModels = next
	invalidPaidServiceTierRegistry = invalid
}
func PaidServiceTierManaged(ctx context.Context, name string) (bool, error) {
	// Compaction is not an escape from the original model's paid-tier policy.
	name = strings.TrimSuffix(name, ratio_setting.CompactModelSuffix)
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		if invalidPaidServiceTierRegistry {
			return false, fmt.Errorf("invalid channel service tier settings")
		}
		return managedPaidServiceTierModels[name], nil
	}
	if DB == nil {
		return false, fmt.Errorf("channel registry unavailable")
	}
	var channels []Channel
	if err := DB.WithContext(ctx).Select("id, settings").Where("settings LIKE ?", "%paid_service_tiers%").Find(&channels).Error; err != nil {
		return false, err
	}
	for i := range channels {
		settings, err := readPaidTierSettings(&channels[i])
		if err != nil {
			return false, err
		}
		if len(settings.PaidServiceTiers[name]) > 0 {
			return true, nil
		}
	}
	return false, nil
}
func readPaidTierSettings(ch *Channel) (dto.ChannelOtherSettings, error) {
	var settings dto.ChannelOtherSettings
	if ch != nil && ch.OtherSettings != "" {
		if err := common.UnmarshalJsonStr(ch.OtherSettings, &settings); err != nil {
			return settings, fmt.Errorf("invalid channel service tier settings")
		}
	}
	return settings, nil
}
func ChannelSupportsPaidServiceTier(ch *Channel, name, tier string) bool {
	if ch == nil || ch.Type != constant.ChannelTypeOpenAI {
		return false
	}
	settings, err := readPaidTierSettings(ch)
	if err != nil || !settings.AllowServiceTier {
		return false
	}
	for _, allowed := range settings.PaidServiceTiers[name] {
		if allowed == tier {
			return true
		}
	}
	return false
}
func ValidatePaidServiceTierSettings(ch *Channel) error {
	if ch == nil {
		return fmt.Errorf("channel is nil")
	}
	if !strings.Contains(ch.OtherSettings, "paid_service_tiers") {
		return nil
	}
	settings, err := readPaidTierSettings(ch)
	if err != nil {
		return err
	}
	if len(settings.PaidServiceTiers) == 0 {
		return nil
	}
	if ch.Type != constant.ChannelTypeOpenAI || !settings.AllowServiceTier {
		return fmt.Errorf("paid service tiers require an OpenAI channel with service_tier enabled")
	}
	if len(settings.PaidServiceTiers) > 100 {
		return fmt.Errorf("too many paid service tier models")
	}
	for name, tiers := range settings.PaidServiceTiers {
		if name == "" || strings.TrimSpace(name) != name || !common.StringsContains(ch.GetModels(), name) {
			return fmt.Errorf("paid service tier model must be an exact channel model")
		}
		if len(tiers) == 0 || len(tiers) > 2 {
			return fmt.Errorf("select priority and/or ultrafast")
		}
		seen := map[string]bool{}
		for _, tier := range tiers {
			if (tier != "priority" && tier != "ultrafast") || seen[tier] {
				return fmt.Errorf("invalid paid service tier")
			}
			seen[tier] = true
		}
	}
	return nil
}
