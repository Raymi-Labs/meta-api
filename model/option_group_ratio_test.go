package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestSpecialGroupRatioOptionPersistence(t *testing.T) {
	oldDB, oldOptions := DB, common.OptionMap
	oldRules := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		DB, common.OptionMap = oldDB, oldOptions
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldRules))
	})
	var dialector gorm.Dialector = sqlite.Open(":memory:")
	if dsn := os.Getenv("META_API_GROUP_RATIO_TEST_MYSQL_DSN"); dsn != "" {
		dialector = mysql.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	DB, common.OptionMap = db, map[string]string{}
	require.NoError(t, DB.AutoMigrate(&Option{}))
	for _, key := range []string{"GroupGroupRatio", "group_ratio_setting.group_group_ratio"} {
		t.Run(key, func(t *testing.T) {
			require.NoError(t, DB.Where(Option{Key: key}).Delete(&Option{}).Error)
			require.Error(t, UpdateOption(key, `null`))
			var count int64
			require.NoError(t, DB.Model(&Option{}).Where(Option{Key: key}).Count(&count).Error)
			require.Zero(t, count)
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("group_ratio_create_failure", func(tx *gorm.DB) { tx.AddError(errors.New("fixture create then save failure")) }))
			require.Error(t, UpdateOption(key, `{}`))
			require.NoError(t, DB.Callback().Update().Remove("group_ratio_create_failure"))
			require.NoError(t, DB.Model(&Option{}).Where(Option{Key: key}).Count(&count).Error)
			require.Zero(t, count, "failed creation must roll back the empty option row")
			const valid = `{"vip":{"paid":0.25,"free":0}}`
			require.NoError(t, UpdateOption(key, valid))
			prior := common.OptionMap[key]
			for _, bad := range []string{`null`, `[]`, `{"vip":null}`, `{"vip":{"paid":null}}`, `{"vip":{"paid":-1}}`, `{"vip":{"paid":0.5},"bad":3}`, `{"vip":`} {
				require.Error(t, UpdateOption(key, bad))
				require.Error(t, updateOptionMap(key, bad))
				var stored Option
				require.NoError(t, DB.First(&stored, Option{Key: key}).Error)
				require.Equal(t, prior, stored.Value)
				require.Equal(t, prior, common.OptionMap[key])
				ratio, ok := ratio_setting.GetGroupGroupRatio("vip", "paid")
				require.True(t, ok)
				require.Equal(t, 0.25, ratio)
			}
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("group_ratio_reject_write", func(tx *gorm.DB) { tx.AddError(errors.New("fixture write failure")) }))
			require.Error(t, UpdateOption(key, `{}`))
			require.Equal(t, prior, common.OptionMap[key])
			ratio, ok := ratio_setting.GetGroupGroupRatio("vip", "paid")
			require.True(t, ok)
			require.Equal(t, 0.25, ratio)
			require.NoError(t, DB.Callback().Update().Remove("group_ratio_reject_write"))
			require.NoError(t, UpdateOption(key, " \n"))
			var stored Option
			require.NoError(t, DB.First(&stored, Option{Key: key}).Error)
			require.Equal(t, "{}", stored.Value)
			require.Equal(t, "{}", common.OptionMap[key])
			require.Equal(t, "{}", ratio_setting.GroupGroupRatio2JSONString())
			// Legacy persisted blanks are tolerated during periodic reload, without a DB migration.
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(valid))
			require.NoError(t, updateOptionMap(key, ""))
			require.Equal(t, "{}", common.OptionMap[key])
			require.Equal(t, "{}", ratio_setting.GroupGroupRatio2JSONString())
		})
	}
}
