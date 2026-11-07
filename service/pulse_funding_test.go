package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func seedPaidWallet(t *testing.T, id int) {
	t.Helper()
	seedUser(t, id, 200)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{"pulse_paid_quota": 100, "pulse_funding_epoch": 1}).Error)
	t.Cleanup(func() {
		model.DB.Where("user_id = ?", id).Delete(&model.PulseWalletReservation{})
		model.DB.Where("user_id = ?", id).Delete(&model.PulseFundingLedger{})
	})
}

func TestWalletFundingSessionSettlementDoesNotAddPulseLogMetadata(t *testing.T) {
	truncate(t)
	seedPaidWallet(t, 7401)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set(common.RequestIdKey, "paid-session-equal")
	wallet := &WalletFunding{userId: 7401, requestID: "paid-session-equal"}
	require.NoError(t, wallet.PreConsume(50))
	info := &relaycommon.RelayInfo{UserId: 7401, IsPlayground: true, RequestId: "paid-session-equal", BillingSource: BillingSourceWallet, ChannelMeta: &relaycommon.ChannelMeta{}}
	session := &BillingSession{relayInfo: info, funding: wallet, preConsumedQuota: 50, tokenConsumed: 50}
	info.Billing = session
	require.NoError(t, SettleBilling(ctx, info, 50))
	for i := 0; i < 10; i++ {
		require.NoError(t, SettleBilling(ctx, info, 50))
	}
	require.Equal(t, 150, getUserQuota(t, 7401))
	model.RecordConsumeLog(ctx, 7401, model.RecordConsumeLogParams{Quota: 50, ModelName: "paid-test", Other: map[string]interface{}{"pulse_funding": map[string]any{"paid_quota": 999}}})
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ?", 7401).First(&log).Error)
	var decoded map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &decoded))
	_, exists := decoded["pulse_funding"]
	require.False(t, exists)
}

func TestPulseFundingTokenFailureCannotRefundCommittedWallet(t *testing.T) {
	truncate(t)
	seedPaidWallet(t, 7402)
	wallet := &WalletFunding{userId: 7402, requestID: "paid-token-failure"}
	require.NoError(t, wallet.PreConsume(50))
	info := &relaycommon.RelayInfo{UserId: 7402, TokenId: 9999999, TokenKey: "missing-token", RequestId: "paid-token-failure"}
	session := &BillingSession{relayInfo: info, funding: wallet, preConsumedQuota: 50, tokenConsumed: 50}
	err := session.Settle(60)
	require.Error(t, err)
	require.True(t, session.fundingSettled)
	require.False(t, session.NeedsRefund())
	session.Refund(nil)
	require.Equal(t, 140, getUserQuota(t, 7402))
	require.Equal(t, "settled", wallet.reservation.Status)
	require.EqualValues(t, 60, wallet.reservation.PaidQuota)
}

func TestPulseFundingUnsupportedLifecyclesNeverEmitPaidProof(t *testing.T) {
	for _, info := range []*relaycommon.RelayInfo{
		{RequestId: "async", TaskRelayInfo: &relaycommon.TaskRelayInfo{}},
		{RequestId: "image", ForcePreConsume: true},
		{RequestId: "realtime", RelayFormat: types.RelayFormatOpenAIRealtime},
	} {
		require.Empty(t, synchronousFundingRequestID(info))
	}
	truncate(t)
	seedUser(t, 7403, 100)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{UserId: 7403, Quota: 50, LogType: model.LogTypeConsume, Other: map[string]interface{}{"pulse_funding": map[string]any{"status": "verified", "paid_quota": 50}}})
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{UserId: 7403, Quota: 50, LogType: model.LogTypeRefund, Other: map[string]interface{}{"pulse_funding": map[string]any{"status": "verified", "paid_quota": 50}}})
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ?", 7403).Find(&logs).Error)
	require.Len(t, logs, 2)
	for _, log := range logs {
		var decoded map[string]any
		require.NoError(t, common.UnmarshalJsonStr(log.Other, &decoded))
		_, exists := decoded["pulse_funding"]
		require.False(t, exists)
	}
}
