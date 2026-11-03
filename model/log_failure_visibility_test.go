package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFailureLogsKeepPublicDiagnosticsAndIsolateUsers(t *testing.T) {
	db := setupLogQueryTestDB(t)
	for _, uid := range []int{1, 2} {
		require.NoError(t, db.Create(&Log{UserId: uid, Type: LogTypeError, ChannelId: 19, RequestId: "req-failure", Content: "raw upstream secret", Other: `{"request_failure":true,"failure_reason":"请求处理失败","failure_hint":"请稍后重试","http_status":200,"status_code":400,"error_code":"content_policy","channel_name":"private-channel","channel_id":19,"upstream":{"url":"https://private.example"},"admin_info":{"error_message":"private detail","preconsume":{"estimated_quota":536742306,"policy":"bounded_chat_wallet"}}}`}).Error)
	}
	logs, total, err := GetUserLogs(1, LogTypeError, 0, 0, "", "", 0, 10, "", "req-failure")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, 1, logs[0].UserId)
	require.Zero(t, logs[0].ChannelId)
	require.Equal(t, "请求处理失败", logs[0].Content)
	require.NotContains(t, logs[0].Other, "private")
	require.NotContains(t, logs[0].Other, "preconsume")
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.EqualValues(t, 200, other["http_status"])
	require.EqualValues(t, 400, other["status_code"])
	var stored Log
	require.NoError(t, db.Where("user_id = ?", 1).First(&stored).Error)
	require.Contains(t, stored.Other, "preconsume")
	require.Contains(t, stored.Other, "private detail", "user redaction must not change administrator diagnostics")
	require.NoError(t, db.Create(&Log{UserId: 1, Type: LogTypeError, Content: "legacy credential", Other: `{"channel_name":"private-channel","upstream":{"url":"https://private.example"},"status_code":502}`}).Error)
	logs, _, err = GetUserLogs(1, LogTypeError, 0, 0, "", "", 0, 10, "", "")
	require.NoError(t, err)
	for _, log := range logs {
		require.NotContains(t, log.Content, "credential")
		require.NotContains(t, log.Other, "private")
	}
}
