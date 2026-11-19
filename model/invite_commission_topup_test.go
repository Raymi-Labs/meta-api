package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupInviteCommissionTopUpTest(t *testing.T) {
	t.Helper()
	setupInviteCommissionSubscriptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originRate := common.InviterRechargeCommissionRate
	originSecondLevelRate := common.InviterRechargeSecondLevelCommissionRate
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originRate
		common.InviterRechargeSecondLevelCommissionRate = originSecondLevelRate
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
	})

	common.InviterCommissionEnabled = true
	common.InviterRechargeCommissionRate = 0.1
	common.InviterRechargeSecondLevelCommissionRate = 0
	common.QuotaPerUnit = 1000
	operation_setting.Price = 1
}

func TestEnqueueInviteCommissionFromTopUp_CreatesAndSettlesTwoLevels(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	originDailyCap := common.InviterCommissionDailyCap
	common.InviterCommissionDailyCap = 0
	common.InviterRechargeSecondLevelCommissionRate = 0.05
	t.Cleanup(func() {
		common.InviterCommissionDailyCap = originDailyCap
	})

	grandparent := createInviteCommissionTestUser(t, "grandparent_topup_two_level", 0)
	parent := createInviteCommissionTestUser(t, "parent_topup_two_level", grandparent.Id)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_two_level", parent.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_two_level_001", 100, 100, 100, common.TopUpStatusSuccess)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))
	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))

	var ledgers []*InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).Order("commission_level asc").Find(&ledgers).Error)
	require.Len(t, ledgers, 2)

	direct := ledgers[0]
	assert.Equal(t, InviteCommissionLevelDirect, direct.CommissionLevel)
	assert.Equal(t, parent.Id, direct.InviterUserId)
	assert.Equal(t, invitee.Id, direct.DirectInviteeUserId)
	assert.Equal(t, 0.1, direct.CommissionRate)
	assert.Equal(t, int64(10000), direct.CommissionQuota)

	indirect := ledgers[1]
	assert.Equal(t, InviteCommissionLevelIndirect, indirect.CommissionLevel)
	assert.Equal(t, grandparent.Id, indirect.InviterUserId)
	assert.Equal(t, parent.Id, indirect.DirectInviteeUserId)
	assert.Equal(t, 0.05, indirect.CommissionRate)
	assert.Equal(t, int64(5000), indirect.CommissionQuota)

	settled, skipped, processed, err := SettleInviteCommissionByBizDate("2099-01-01", 10)
	require.NoError(t, err)
	assert.Equal(t, 2, settled)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, 2, processed)

	var refreshedParent User
	require.NoError(t, DB.First(&refreshedParent, parent.Id).Error)
	assert.Equal(t, int64(10000), refreshedParent.AffQuota)
	assert.Equal(t, int64(10000), refreshedParent.AffHistoryQuota)

	var refreshedGrandparent User
	require.NoError(t, DB.First(&refreshedGrandparent, grandparent.Id).Error)
	assert.Equal(t, int64(5000), refreshedGrandparent.AffQuota)
	assert.Equal(t, int64(5000), refreshedGrandparent.AffHistoryQuota)
}

func TestEnqueueInviteCommissionFromTopUp_AllowsSecondLevelOnly(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	common.InviterRechargeCommissionRate = 0
	common.InviterRechargeSecondLevelCommissionRate = 0.05

	grandparent := createInviteCommissionTestUser(t, "grandparent_topup_second_only", 0)
	parent := createInviteCommissionTestUser(t, "parent_topup_second_only", grandparent.Id)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_second_only", parent.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_second_only_001", 100, 100, 100, common.TopUpStatusSuccess)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))

	var ledgers []*InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
	assert.Equal(t, InviteCommissionLevelIndirect, ledgers[0].CommissionLevel)
	assert.Equal(t, grandparent.Id, ledgers[0].InviterUserId)
}

func TestEnqueueInviteCommissionFromTopUp_SkipsSecondLevelWithoutGrandparent(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	common.InviterRechargeSecondLevelCommissionRate = 0.05

	parent := createInviteCommissionTestUser(t, "parent_topup_no_grandparent", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_no_grandparent", parent.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_no_grandparent_001", 100, 100, 100, common.TopUpStatusSuccess)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))

	var ledgers []*InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
	assert.Equal(t, InviteCommissionLevelDirect, ledgers[0].CommissionLevel)
	assert.Equal(t, parent.Id, ledgers[0].InviterUserId)
}

func TestSettleInviteCommission_AppliesDailyCapPerBeneficiary(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	common.InviterRechargeSecondLevelCommissionRate = 0.05

	originDailyCap := common.InviterCommissionDailyCap
	common.InviterCommissionDailyCap = 7000
	t.Cleanup(func() {
		common.InviterCommissionDailyCap = originDailyCap
	})

	grandparent := createInviteCommissionTestUser(t, "grandparent_topup_cap", 0)
	parent := createInviteCommissionTestUser(t, "parent_topup_cap", grandparent.Id)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_cap", parent.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_cap_two_level_001", 100, 100, 100, common.TopUpStatusSuccess)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))
	settled, skipped, processed, err := SettleInviteCommissionByBizDate(time.Unix(topUp.CompleteTime, 0).Format("2006-01-02"), 10)
	require.NoError(t, err)
	assert.Equal(t, 2, settled)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, 2, processed)

	var refreshedParent User
	require.NoError(t, DB.First(&refreshedParent, parent.Id).Error)
	assert.Equal(t, int64(7000), refreshedParent.AffQuota)

	var refreshedGrandparent User
	require.NoError(t, DB.First(&refreshedGrandparent, grandparent.Id).Error)
	assert.Equal(t, int64(5000), refreshedGrandparent.AffQuota)

	var capStates []*InviteCommissionDailyCapState
	require.NoError(t, DB.Order("inviter_user_id asc").Find(&capStates).Error)
	require.Len(t, capStates, 2)
	settledByInviter := map[int]int64{}
	for _, state := range capStates {
		settledByInviter[state.InviterUserId] = state.SettledQuota
	}
	assert.Equal(t, int64(7000), settledByInviter[parent.Id])
	assert.Equal(t, int64(5000), settledByInviter[grandparent.Id])
}

func TestEnqueueInviteCommissionFromTopUp_SkipsCyclicGrandparent(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	common.InviterRechargeSecondLevelCommissionRate = 0.05

	parent := createInviteCommissionTestUser(t, "parent_topup_cycle", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_cycle", parent.Id)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", parent.Id).Update("inviter_id", invitee.Id).Error)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_cycle_001", 100, 100, 100, common.TopUpStatusSuccess)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))

	var ledgers []*InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
	assert.Equal(t, InviteCommissionLevelDirect, ledgers[0].CommissionLevel)
	assert.Equal(t, parent.Id, ledgers[0].InviterUserId)
}

func TestSettleInviteCommission_SkipsUnavailableInviter(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	inviter := createInviteCommissionTestUser(t, "inviter_unavailable_settlement", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_unavailable_settlement", inviter.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_unavailable_inviter_001", 100, 100, 100, common.TopUpStatusSuccess)
	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Update("status", common.UserStatusDisabled).Error)

	settled, skipped, processed, err := SettleInviteCommissionByBizDate("2099-01-01", 10)
	require.NoError(t, err)
	assert.Zero(t, settled)
	assert.Equal(t, 1, skipped)
	assert.Equal(t, 1, processed)

	var ledger InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).First(&ledger).Error)
	assert.Equal(t, InviteCommissionStatusSkipped, ledger.Status)
	assert.Equal(t, InviteCommissionRiskReasonInviterUnavailable, ledger.RiskReason)
	assert.Zero(t, ledger.SettledQuota)
}

func createInviteCommissionTopUp(t *testing.T, userID int, tradeNo string, amount int64, money, paidMoney float64, status string) *TopUp {
	t.Helper()
	topUp := &TopUp{
		UserId:       userID,
		Amount:       amount,
		Money:        money,
		PaidMoney:    paidMoney,
		TradeNo:      tradeNo,
		CreateTime:   common.GetTimestamp(),
		CompleteTime: common.GetTimestamp(),
		Status:       status,
	}
	require.NoError(t, DB.Create(topUp).Error)
	return topUp
}

func TestEnqueueInviteCommissionFromTopUp_UsesPaidMoneyAndDatabaseOrder(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	inviter := createInviteCommissionTestUser(t, "inviter_topup_paid", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_paid", inviter.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_paid_001", 100, 80, 80, common.TopUpStatusSuccess)

	tampered := &TopUp{
		Id:           topUp.Id,
		UserId:       999999,
		TradeNo:      "tampered_trade_no",
		PaidMoney:    1,
		CompleteTime: 1,
		Status:       common.TopUpStatusSuccess,
	}
	require.NoError(t, EnqueueInviteCommissionFromTopUp(tampered))
	require.NoError(t, EnqueueInviteCommissionFromTopUp(tampered))

	var ledger InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).First(&ledger).Error)
	assert.Equal(t, invitee.Id, ledger.InviteeUserId)
	assert.Equal(t, inviter.Id, ledger.InviterUserId)
	assert.Equal(t, int64(80000), ledger.BaseQuota)
	assert.Equal(t, int64(8000), ledger.CommissionQuota)
	assert.NotEqual(t, int(topUp.Amount*int64(common.QuotaPerUnit)), ledger.BaseQuota)

	var count int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Where("topup_trade_no = ?", topUp.TradeNo).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestEnqueueInviteCommissionFromTopUp_StripeUsesCNYPresentmentInsteadOfGrantedMoney(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	inviter := createInviteCommissionTestUser(t, "inviter_topup_stripe", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_stripe", inviter.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_stripe_001", 100, 100, 80, common.TopUpStatusSuccess)
	topUp.PaymentMethod = PaymentMethodStripe
	topUp.PaymentProvider = PaymentProviderStripe
	topUp.PaidCurrency = "USD"
	topUp.PresentmentMoney = 80
	topUp.PresentmentCurrency = "CNY"
	require.NoError(t, DB.Save(topUp).Error)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))

	var ledger InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).First(&ledger).Error)
	assert.Equal(t, int64(80000), ledger.BaseQuota)
	assert.Equal(t, int64(8000), ledger.CommissionQuota)
}

func TestEnqueueInviteCommissionFromTopUp_RejectsUnsuccessfulOrders(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	inviter := createInviteCommissionTestUser(t, "inviter_topup_status", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_status", inviter.Id)
	pending := createInviteCommissionTopUp(t, invitee.Id, "topup_pending_001", 100, 80, 80, common.TopUpStatusPending)
	failed := createInviteCommissionTopUp(t, invitee.Id, "topup_failed_001", 100, 80, 80, common.TopUpStatusFailed)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(pending))
	require.NoError(t, EnqueueInviteCommissionFromTopUp(failed))

	var count int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestEnqueueInviteCommissionFromTopUp_LegacyOrderFallsBackToMoney(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	inviter := createInviteCommissionTestUser(t, "inviter_topup_legacy", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_legacy", inviter.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_legacy_001", 100, 75, 0, common.TopUpStatusSuccess)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))

	var ledger InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", topUp.TradeNo).First(&ledger).Error)
	assert.Equal(t, int64(75000), ledger.BaseQuota)
	assert.Equal(t, int64(7500), ledger.CommissionQuota)
}

func TestSettleInviteCommission_ReconcilesLegacyPendingTopUpLedger(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	originDailyCap := common.InviterCommissionDailyCap
	common.InviterCommissionDailyCap = 0
	t.Cleanup(func() {
		common.InviterCommissionDailyCap = originDailyCap
	})

	inviter := createInviteCommissionTestUser(t, "inviter_topup_reconcile", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_topup_reconcile", inviter.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_reconcile_001", 100, 80, 0, common.TopUpStatusSuccess)
	ledger := &InviteCommissionLedger{
		InviteeUserId:   invitee.Id,
		InviterUserId:   inviter.Id,
		TopupTradeNo:    topUp.TradeNo,
		BizDate:         "2026-01-01",
		BaseQuota:       100000,
		CommissionRate:  0.1,
		CommissionQuota: 10000,
		Status:          InviteCommissionStatusPending,
		CreatedAt:       common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(ledger).Error)

	settled, skipped, processed, err := SettleInviteCommissionByBizDate("2026-01-02", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, settled)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, 1, processed)

	var refreshedLedger InviteCommissionLedger
	require.NoError(t, DB.First(&refreshedLedger, ledger.Id).Error)
	assert.Equal(t, int64(80000), refreshedLedger.BaseQuota)
	assert.Equal(t, int64(8000), refreshedLedger.CommissionQuota)
	assert.Equal(t, int64(8000), refreshedLedger.SettledQuota)

	var refreshedInviter User
	require.NoError(t, DB.First(&refreshedInviter, inviter.Id).Error)
	assert.Equal(t, int64(8000), refreshedInviter.AffQuota)
	assert.Equal(t, int64(8000), refreshedInviter.AffHistoryQuota)
}

func TestSettleInviteCommission_SkipsLegacyStripeWithoutPaidMoney(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	originDailyCap := common.InviterCommissionDailyCap
	common.InviterCommissionDailyCap = 0
	t.Cleanup(func() {
		common.InviterCommissionDailyCap = originDailyCap
	})

	inviter := createInviteCommissionTestUser(t, "inviter_stripe_missing_paid", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_stripe_missing_paid", inviter.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "stripe_missing_paid_001", 100, 100, 0, common.TopUpStatusSuccess)
	topUp.PaymentMethod = PaymentMethodStripe
	topUp.PaymentProvider = PaymentProviderStripe
	require.NoError(t, DB.Save(topUp).Error)
	ledger := &InviteCommissionLedger{
		InviteeUserId:   invitee.Id,
		InviterUserId:   inviter.Id,
		TopupTradeNo:    topUp.TradeNo,
		BizDate:         "2026-01-01",
		BaseQuota:       100000,
		CommissionRate:  0.1,
		CommissionQuota: 10000,
		Status:          InviteCommissionStatusPending,
		CreatedAt:       common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(ledger).Error)

	settled, skipped, processed, err := SettleInviteCommissionByBizDate("2026-01-02", 10)
	require.NoError(t, err)
	assert.Equal(t, 0, settled)
	assert.Equal(t, 1, skipped)
	assert.Equal(t, 1, processed)

	var refreshedLedger InviteCommissionLedger
	require.NoError(t, DB.First(&refreshedLedger, ledger.Id).Error)
	assert.Equal(t, InviteCommissionStatusSkipped, refreshedLedger.Status)
	assert.Equal(t, InviteCommissionRiskReasonPaidMoneyMissing, refreshedLedger.RiskReason)
	assert.Equal(t, int64(0), refreshedLedger.SettledQuota)
}

func TestEnqueueInviteCommissionFromTopUp_StripeUSDWithoutCNYPresentmentIsSkipped(t *testing.T) {
	setupInviteCommissionTopUpTest(t)

	inviter := createInviteCommissionTestUser(t, "inviter_stripe_usd", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_stripe_usd", inviter.Id)
	topUp := createInviteCommissionTopUp(t, invitee.Id, "topup_stripe_usd_001", 100, 100, 3, common.TopUpStatusSuccess)
	topUp.PaymentMethod = PaymentMethodStripe
	topUp.PaymentProvider = PaymentProviderStripe
	topUp.PaidCurrency = "USD"
	require.NoError(t, DB.Save(topUp).Error)

	require.NoError(t, EnqueueInviteCommissionFromTopUp(topUp))

	var count int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Where("topup_trade_no = ?", topUp.TradeNo).Count(&count).Error)
	require.Zero(t, count)
}
