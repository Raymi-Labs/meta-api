package model

import (
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func setupEnterpriseGroupMaintenance(t *testing.T, dialect string) {
	t.Helper()
	enterpriseTestDB(t, dialect)
	oldUsers, oldGroups, oldOptions := setting.UserGroups2JSONString(), ratio_setting.GroupRatio2JSONString(), common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserGroupsByJSONString(oldUsers))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
		common.OptionMap = oldOptions
	})
	require.NoError(t, setting.UpdateUserGroupsByJSONString(`{"default":""}`))
}

func TestEnterpriseGroupMaintenanceRejectsReferencedDeletion(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			setupEnterpriseGroupMaintenance(t, dialect)
			group := "contract-" + common.GetUUID()[:8]
			base := ratio_setting.GetGroupRatioCopy()
			base["default"], base[group], base[group+"-unused"] = 1, 0.8, 0.7
			raw, err := common.Marshal(base)
			require.NoError(t, err)
			require.NoError(t, UpdateOption("GroupRatio", string(raw)))
			owner := enterpriseTestUser(t, 0)
			ent, err := CreateEnterprise(1, "分组维护验收", group, "UTC", owner.Id)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, DB.Where("id = ?", ent.Id).Delete(&Enterprise{}).Error) })
			// Frozen enterprises and a disabled feature still retain their configured price.
			require.NoError(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusFrozen, "test"))
			oldEnabled := common.EnterpriseEnabled
			common.EnterpriseEnabled = false
			t.Cleanup(func() { common.EnterpriseEnabled = oldEnabled })
			delete(base, group)
			base[group+"-renamed"] = 0.8
			removal, err := common.Marshal(base)
			require.NoError(t, err)
			pair, err := common.Marshal(UserChannelGroupOptions{UserGroups: `{"default":""}`, GroupRatio: string(removal)})
			require.NoError(t, err)
			for _, update := range []func() error{
				func() error { return UpdateOption("GroupRatio", string(removal)) },
				func() error { return UpdateOption("group_ratio_setting.group_ratio", string(removal)) },
				func() error { return UpdateUserChannelGroupOptions(string(pair)) },
			} {
				err := update()
				require.Error(t, err)
				require.Contains(t, err.Error(), group)
				require.Contains(t, err.Error(), "企业管理")
				var saved Option
				require.NoError(t, DB.First(&saved, Option{Key: "GroupRatio"}).Error)
				require.JSONEq(t, string(raw), saved.Value)
				require.JSONEq(t, string(raw), ratio_setting.GroupRatio2JSONString())
			}
			// Case-only renames are removals even on MySQL's case-insensitive collation.
			delete(base, group+"-renamed")
			base[strings.ToUpper(group)] = 0.8
			upper, err := common.Marshal(base)
			require.NoError(t, err)
			require.Error(t, UpdateOption("GroupRatio", string(upper)))
			delete(base, strings.ToUpper(group))
			base[group] = 0
			delete(base, group+"-unused")
			free, err := common.Marshal(base)
			require.NoError(t, err)
			require.NoError(t, UpdateOption("group_ratio_setting.group_ratio", string(free)))
			ratio, ok := ratio_setting.GetValidGroupRatio(group)
			require.True(t, ok)
			require.Zero(t, ratio)
			require.NoError(t, UpdateEnterpriseByPlatform(1, ent.Id, ent.Name, "default"))
			require.NoError(t, UpdateOption("GroupRatio", string(removal)))
			require.False(t, ratio_setting.ContainsGroupRatio(group))
			// Stale process cache cannot revive a deleted group when binding a new enterprise.
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(raw)))
			require.ErrorIs(t, UpdateEnterpriseByPlatform(1, ent.Id, ent.Name, group), ErrEnterpriseBillingGroup)
			another := enterpriseTestUser(t, 0)
			_, err = CreateEnterprise(1, "旧缓存验收", group, "UTC", another.Id)
			require.ErrorIs(t, err, ErrEnterpriseBillingGroup)
		})
	}
}

func TestEnterpriseGroupConcurrentBindingAndDeletion(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			setupEnterpriseGroupMaintenance(t, dialect)
			ent, _, _ := enterpriseTestOrg(t)
			t.Cleanup(func() { require.NoError(t, DB.Where("id = ?", ent.Id).Delete(&Enterprise{}).Error) })
			for i := 0; i < 10; i++ {
				group := "race-" + common.GetUUID()[:8]
				base := ratio_setting.GetGroupRatioCopy()
				base["default"] = 1
				base[group] = 0.8
				raw, err := common.Marshal(base)
				require.NoError(t, err)
				require.NoError(t, UpdateOption("GroupRatio", string(raw)))
				delete(base, group)
				removed, err := common.Marshal(base)
				require.NoError(t, err)
				start := make(chan struct{})
				var bindErr, deleteErr error
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); <-start; bindErr = UpdateEnterpriseByPlatform(1, ent.Id, ent.Name, group) }()
				go func() { defer wg.Done(); <-start; deleteErr = UpdateOption("GroupRatio", string(removed)) }()
				close(start)
				wg.Wait()
				require.NotEqual(t, bindErr == nil, deleteErr == nil, "exactly one conflicting operation must win")
				got, err := GetEnterpriseByID(ent.Id)
				require.NoError(t, err)
				_, valid := ratio_setting.GetValidGroupRatio(got.BillingGroup)
				require.True(t, valid)
				require.NoError(t, UpdateEnterpriseByPlatform(1, ent.Id, ent.Name, "default"))
				require.NoError(t, UpdateOption("GroupRatio", string(removed)))
			}
		})
	}
}

func TestEnterpriseGroupLegacyAliasCanonicalization(t *testing.T) {
	setupEnterpriseGroupMaintenance(t, "sqlite")
	require.NoError(t, DB.Create(&Option{Key: "group_ratio_setting.group_ratio", Value: `{"default":1,"legacy-contract":0.6}`}).Error)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	owner := enterpriseTestUser(t, 0)
	ent, err := CreateEnterprise(1, "旧配置验收", "legacy-contract", "UTC", owner.Id)
	require.NoError(t, err)
	require.NoError(t, UpdateOption("group_ratio_setting.group_ratio", `{"default":1,"legacy-contract":0.4}`))
	var count int64
	require.NoError(t, DB.Model(&Option{}).Where(Option{Key: "group_ratio_setting.group_ratio"}).Count(&count).Error)
	require.Zero(t, count)
	loadOptionsFromDatabase()
	ratio, valid := ratio_setting.GetValidGroupRatio(ent.BillingGroup)
	require.True(t, valid)
	require.Equal(t, 0.4, ratio)
	require.JSONEq(t, common.OptionMap["GroupRatio"], common.OptionMap["group_ratio_setting.group_ratio"])
}

func TestEnterpriseGroupConflictingLegacyPricesRequireExplicitSave(t *testing.T) {
	setupEnterpriseGroupMaintenance(t, "sqlite")
	require.NoError(t, DB.Create(&Option{Key: "GroupRatio", Value: `{"default":1,"contract":1}`}).Error)
	require.NoError(t, DB.Create(&Option{Key: "group_ratio_setting.group_ratio", Value: `{"default":1,"contract":0.8,"legacy-only":0.6}`}).Error)
	// Preserve the deployed loader's behavior; never choose a different price on upgrade.
	loadOptionsFromDatabase()
	before := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, UpdateOption("UserGroups", `{"default":"","vip":""}`))
	require.JSONEq(t, before, ratio_setting.GroupRatio2JSONString())
	require.Error(t, UpdateOption("usergroups", `{"default":"","legacy-only":""}`))
	require.False(t, setting.IsRegisteredUserGroup("legacy-only"))
	require.JSONEq(t, before, ratio_setting.GroupRatio2JSONString())
	var alias Option
	require.NoError(t, DB.First(&alias, Option{Key: "group_ratio_setting.group_ratio"}).Error)
	require.JSONEq(t, `{"default":1,"contract":0.8,"legacy-only":0.6}`, alias.Value)
	owner := enterpriseTestUser(t, 0)
	_, err := CreateEnterprise(1, "冲突配置", "contract", "UTC", owner.Id)
	require.Error(t, err)
	require.Contains(t, err.Error(), "新旧配置不一致")
	require.True(t, IsEnterpriseUserError(err))
	require.NoError(t, UpdateOption("GroupRatio", `{"default":1,"contract":0.7}`))
	loadOptionsFromDatabase()
	ratio, valid := ratio_setting.GetValidGroupRatio("contract")
	require.True(t, valid)
	require.Equal(t, 0.7, ratio)
	_, err = CreateEnterprise(1, "确认配置", "contract", "UTC", owner.Id)
	require.NoError(t, err)
}

func TestEnterpriseGroupOptionKeyAliasesCannotBypassProtection(t *testing.T) {
	setupEnterpriseGroupMaintenance(t, "sqlite")
	require.NoError(t, UpdateOption("GroupRatio", `{"default":1,"contract":0.8}`))
	owner := enterpriseTestUser(t, 0)
	_, err := CreateEnterprise(1, "配置键验收", "contract", "UTC", owner.Id)
	require.NoError(t, err)
	for _, key := range []string{"groupratio", "GROUPRATIO", "GroupRatio ", "GROUP_RATIO_SETTING.GROUP_RATIO", "group_ratio_setting.group_ratio "} {
		err := UpdateOption(key, `{"default":1}`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "企业管理")
		ratio, ok := ratio_setting.GetValidGroupRatio("contract")
		require.True(t, ok)
		require.Equal(t, 0.8, ratio)
	}
}
