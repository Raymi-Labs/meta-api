package model

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UserChannelGroupsOptionKey = "UserChannelGroups"

var groupOptionsMutex sync.Mutex

type UserChannelGroupOptions struct {
	UserGroups string `json:"UserGroups"`
	GroupRatio string `json:"GroupRatio"`
}

func UpdateUserChannelGroupOptions(raw string) error {
	var pair UserChannelGroupOptions
	if err := common.UnmarshalJsonStr(raw, &pair); err != nil {
		return err
	}
	groupOptionsMutex.Lock()
	defer groupOptionsMutex.Unlock()
	return saveUserChannelGroupOptions(pair, "")
}

func updateSingleGroupOption(key, value string) error {
	groupOptionsMutex.Lock()
	defer groupOptionsMutex.Unlock()
	pair := UserChannelGroupOptions{UserGroups: setting.UserGroups2JSONString(), GroupRatio: ratio_setting.GroupRatio2JSONString()}
	if key == "UserGroups" {
		pair.UserGroups = value
	} else {
		pair.GroupRatio = value
	}
	return saveUserChannelGroupOptions(pair, key)
}

// lockGroupRatioOptionTx is shared by group changes and enterprise assignment.
// The insert/no-op also obtains SQLite's writer lock before dependent reads.
func lockGroupRatioOptionTx(tx *gorm.DB) (*Option, error) {
	option := Option{Key: "GroupRatio", Value: ratio_setting.GroupRatio2JSONString()}
	// A legacy alias may be the only persisted value on an upgraded installation.
	// Prefer it over the instance cache when initializing the canonical row.
	var legacy Option
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(Option{Key: "group_ratio_setting.group_ratio"}).Find(&legacy).Error; err != nil {
		return nil, err
	}
	if legacy.Key != "" {
		option.Value = legacy.Value
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&option, Option{Key: "GroupRatio"}).Error; err != nil {
		return nil, err
	}
	return &option, nil
}

func validateEnterpriseBillingGroupTx(tx *gorm.DB, group string) error {
	option, err := lockGroupRatioOptionTx(tx)
	if err != nil {
		return err
	}
	var groups map[string]float64
	if err := common.UnmarshalJsonStr(option.Value, &groups); err != nil {
		return err
	}
	var legacy Option
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(Option{Key: "group_ratio_setting.group_ratio"}).Find(&legacy).Error; err != nil {
		return err
	}
	if legacy.Key != "" {
		var legacyGroups map[string]float64
		if err := common.UnmarshalJsonStr(legacy.Value, &legacyGroups); err != nil {
			return err
		}
		if !maps.Equal(groups, legacyGroups) {
			return ErrEnterpriseBillingConfigConflict
		}
	}
	if ratio, ok := groups[group]; !ok || group == "" || group == "auto" || ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return ErrEnterpriseBillingGroup
	}
	return nil
}

func validateEnterpriseGroupReferencesTx(tx *gorm.DB, nextJSON string) error {
	var groups map[string]float64
	if err := common.UnmarshalJsonStr(nextJSON, &groups); err != nil {
		return err
	}
	// Compare in Go: case-insensitive SQL collations must not equate distinct group keys.
	var enterprises []Enterprise
	if err := tx.Select("id", "billing_group").Find(&enterprises).Error; err != nil {
		return err
	}
	missing := make(map[string]bool)
	for _, enterprise := range enterprises {
		ratio, ok := groups[enterprise.BillingGroup]
		if !ok || ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			missing[enterprise.BillingGroup] = true
		}
	}
	if len(missing) == 0 {
		return nil
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Errorf("分组「%s」仍被企业使用，请先在企业管理中调整计费分组，再删除或改名", strings.Join(names, "、"))
}

// Validate the final pair before writing either option. Publish only after commit;
// the single-option API shares this lock so it cannot bypass the pair invariant.
func saveUserChannelGroupOptions(pair UserChannelGroupOptions, singleKey string) error {
	validate := func() error {
		users, err := setting.ParseUserGroups(pair.UserGroups)
		if err != nil {
			return err
		}
		if err := ratio_setting.CheckGroupRatio(pair.GroupRatio); err != nil {
			return err
		}
		var channels map[string]float64
		if err := common.UnmarshalJsonStr(pair.GroupRatio, &channels); err != nil {
			return err
		}
		if channels == nil {
			return errors.New("channel groups must be a JSON object")
		}
		return setting.CheckUserChannelGroupConflict(users, channels)
	}
	if singleKey == "" {
		if err := validate(); err != nil {
			return err
		}
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		// All writers lock the same two rows in the same order, including across
		// application instances. Missing defaults are created inside this transaction.
		groupOption, err := lockGroupRatioOptionTx(tx)
		if err != nil {
			return err
		}
		entries := []Option{
			*groupOption,
			{Key: "UserGroups", Value: setting.UserGroups2JSONString()},
		}
		for i := range entries {
			key := entries[i].Key
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).FirstOrCreate(&entries[i], Option{Key: key}).Error; err != nil {
				return err
			}
			if singleKey != "" && key != singleKey {
				if key == "UserGroups" {
					pair.UserGroups = entries[i].Value
				} else {
					pair.GroupRatio = entries[i].Value
				}
			}
		}
		if err := validate(); err != nil {
			return err
		}
		if singleKey == "UserGroups" {
			// Keep identity names disjoint from both persisted catalogs until an
			// explicit price save reconciles legacy aliases.
			var legacy Option
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(Option{Key: "group_ratio_setting.group_ratio"}).Find(&legacy).Error; err != nil {
				return err
			}
			if legacy.Key != "" {
				var channels map[string]float64
				if err := common.UnmarshalJsonStr(legacy.Value, &channels); err != nil {
					return err
				}
				users, err := setting.ParseUserGroups(pair.UserGroups)
				if err != nil {
					return err
				}
				if err := setting.CheckUserChannelGroupConflict(users, channels); err != nil {
					return err
				}
			}
		}
		if singleKey != "UserGroups" {
			if err := validateEnterpriseGroupReferencesTx(tx, pair.GroupRatio); err != nil {
				return err
			}
		}
		for _, entry := range entries {
			if singleKey != "" && entry.Key != singleKey {
				continue
			}
			if entry.Key == "UserGroups" {
				entry.Value = pair.UserGroups
			} else {
				entry.Value = pair.GroupRatio
			}
			if err := tx.Save(&entry).Error; err != nil {
				return err
			}
		}
		// Only an explicit multiplier save resolves conflicting legacy prices.
		// Changing identity groups alone must not rewrite anyone's prices.
		if singleKey != "UserGroups" {
			return tx.Where(Option{Key: "group_ratio_setting.group_ratio"}).Delete(&Option{}).Error
		}
		return nil
	}); err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	// Both values have already been parsed and validated before the transaction.
	if err := setting.UpdateUserGroupsByJSONString(pair.UserGroups); err != nil {
		return err
	}
	common.OptionMap["UserGroups"] = pair.UserGroups
	if singleKey == "UserGroups" {
		return nil
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(pair.GroupRatio); err != nil {
		return err
	}
	common.OptionMap["GroupRatio"] = pair.GroupRatio
	common.OptionMap["group_ratio_setting.group_ratio"] = pair.GroupRatio
	return nil
}
