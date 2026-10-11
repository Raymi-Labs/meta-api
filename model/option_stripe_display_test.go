package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStripeDisplayOptionsPersistReloadAndRejectInvalidValues(t *testing.T) {
	oldDB, oldOptions := DB, common.OptionMap
	oldName, oldIcon := setting.StripeDisplayName, setting.StripeDisplayIcon
	t.Cleanup(func() {
		DB, common.OptionMap = oldDB, oldOptions
		setting.StripeDisplayName, setting.StripeDisplayIcon = oldName, oldIcon
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	DB, common.OptionMap = db, map[string]string{}
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, UpdateOption("StripeDisplayName", "微信支付"))
	require.NoError(t, UpdateOption("StripeDisplayIcon", "wxpay"))
	require.Equal(t, "微信支付", setting.StripeDisplayName)
	require.Equal(t, "wxpay", setting.StripeDisplayIcon)
	for _, bad := range []struct{ key, value string }{
		{"StripeDisplayName", strings.Repeat("微", 41)},
		{"StripeDisplayName", "微信\n支付"},
		{"StripeDisplayIcon", "https://example.com/icon.svg"},
		{"StripeDisplayIcon", "wechat_pay"},
	} {
		require.Error(t, UpdateOption(bad.key, bad.value))
		require.Error(t, updateOptionMap(bad.key, bad.value))
	}
	var saved []Option
	require.NoError(t, DB.Find(&saved).Error)
	require.Len(t, saved, 2)
	setting.StripeDisplayName, setting.StripeDisplayIcon = "", ""
	for _, option := range saved {
		require.NoError(t, updateOptionMap(option.Key, option.Value))
	}
	require.Equal(t, "微信支付", setting.StripeDisplayName)
	require.Equal(t, "wxpay", setting.StripeDisplayIcon)
	for _, icon := range []string{"stripe", "alipay", "card", ""} {
		require.NoError(t, UpdateOption("StripeDisplayIcon", icon))
		require.Equal(t, icon, setting.StripeDisplayIcon)
	}
	require.NoError(t, UpdateOption("StripeDisplayName", ""))
	require.Empty(t, setting.StripeDisplayName)
}
