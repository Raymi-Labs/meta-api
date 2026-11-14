package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"testing"
)

func TestUserChannelGroupOptionsSwapAndRollback(t *testing.T) {
	db := setupGroupUsageTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}, &Enterprise{}))
	oldUsers, oldChannels, oldOptions := setting.UserGroups2JSONString(), ratio_setting.GroupRatio2JSONString(), common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		setting.UpdateUserGroupsByJSONString(oldUsers)
		ratio_setting.UpdateGroupRatioByJSONString(oldChannels)
		common.OptionMap = oldOptions
	})
	encode := func(users, channels string) string {
		b, err := common.Marshal(UserChannelGroupOptions{UserGroups: users, GroupRatio: channels})
		require.NoError(t, err)
		return string(b)
	}
	before := encode(`{"default":"","vip":""}`, `{"default":1,"premium":2}`)
	after := encode(`{"default":"","premium":""}`, `{"default":1,"vip":2}`)
	require.NoError(t, UpdateUserChannelGroupOptions(before))
	require.NoError(t, UpdateUserChannelGroupOptions(after))
	require.True(t, setting.IsRegisteredUserGroup("premium"))
	require.True(t, ratio_setting.ContainsGroupRatio("vip"))
	require.Error(t, UpdateOption("UserGroups", `{"default":"","vip":""}`))
	require.Error(t, UpdateOption("GroupRatio", `{"default":1,"premium":1}`))
	require.Error(t, UpdateUserChannelGroupOptions(encode(`{"default":"","vip":""}`, `{"vip":1}`)))
	var saved []Option
	require.NoError(t, db.Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).Find(&saved).Error)
	beforeUsers, beforeChannels := setting.UserGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	calls := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fail_second_group_option", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			calls++
			if calls == 2 {
				tx.AddError(errors.New("injected option failure"))
			}
		}
	}))
	require.Error(t, UpdateUserChannelGroupOptions(before))
	var got []Option
	require.NoError(t, db.Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).Find(&got).Error)
	require.Equal(t, saved, got)
	require.JSONEq(t, beforeUsers, setting.UserGroups2JSONString())
	require.JSONEq(t, beforeChannels, ratio_setting.GroupRatio2JSONString())
	require.JSONEq(t, beforeUsers, common.OptionMap["UserGroups"])
	require.JSONEq(t, beforeChannels, common.OptionMap["GroupRatio"])
}
