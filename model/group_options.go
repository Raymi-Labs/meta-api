package model

import (
	"errors"
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
		entries := []Option{
			{Key: "GroupRatio", Value: ratio_setting.GroupRatio2JSONString()},
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
	if err := ratio_setting.UpdateGroupRatioByJSONString(pair.GroupRatio); err != nil {
		return err
	}
	common.OptionMap["GroupRatio"] = pair.GroupRatio
	return nil
}
