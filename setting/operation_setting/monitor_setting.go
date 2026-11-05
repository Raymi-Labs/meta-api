package operation_setting

import (
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes float64 `json:"auto_test_channel_minutes"`
	PreDisableWaitEnabled  bool    `json:"pre_disable_wait_enabled"`
	PreDisableWaitMinutes  float64 `json:"pre_disable_wait_minutes"`
	// 账号池耗尽通知：多 Key 渠道的全部 Key 进入冷却/禁用时提醒管理员。
	// Emails 支持多个邮箱（逗号/分号/换行分隔），留空则回退 root 用户的站内通知渠道。
	PoolExhaustedNotifyEnabled bool   `json:"pool_exhausted_notify_enabled"`
	PoolExhaustedNotifyEmails  string `json:"pool_exhausted_notify_emails"`
}

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:     false,
	AutoTestChannelMinutes:     30,
	PreDisableWaitEnabled:      false,
	PreDisableWaitMinutes:      10,
	PoolExhaustedNotifyEnabled: false,
	PoolExhaustedNotifyEmails:  "",
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
		}
	}
	return &monitorSetting
}
