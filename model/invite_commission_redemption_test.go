package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var (
	inviteCommissionRedemptionMigrateOnce sync.Once
	inviteCommissionRedemptionMigrateErr  error
)

func setupInviteCommissionRedemptionTest(t *testing.T) {
	t.Helper()
	inviteCommissionRedemptionMigrateOnce.Do(func() {
		inviteCommissionRedemptionMigrateErr = DB.AutoMigrate(&User{}, &Redemption{}, &InviteCommissionLedger{}, &InviteCommissionDailyCapState{})
	})
	require.NoError(t, inviteCommissionRedemptionMigrateErr)
	require.NoError(t, DB.Exec("DELETE FROM invite_commission_ledgers").Error)
	require.NoError(t, DB.Exec("DELETE FROM invite_commission_daily_cap_states").Error)
	require.NoError(t, DB.Exec("DELETE FROM redemptions").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
}

func createInviteCommissionTestUser(t *testing.T, username string, inviterID int) *User {
	t.Helper()
	user := &User{
		Username:    username,
		Password:    "test-password",
		DisplayName: username,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		InviterId:   inviterID,
		AffCode:     fmt.Sprintf("aff_%s", username),
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestRedeemQuotaCode_DoesNotCreateInviteCommissionLedger(t *testing.T) {
	setupInviteCommissionRedemptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originRate := common.InviterRechargeCommissionRate
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originRate
	})
	common.InviterCommissionEnabled = true
	common.InviterRechargeCommissionRate = 0.1

	inviter := createInviteCommissionTestUser(t, "inviter_redemption", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_redemption", inviter.Id)

	redemption := &Redemption{
		Key:         common.GetUUID(),
		Status:      common.RedemptionCodeStatusEnabled,
		Name:        "余额兑换码",
		BenefitType: RedemptionBenefitTypeQuota,
		Quota:       300,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, redemption.Insert())

	quota, err := Redeem(redemption.Key, invitee.Id)
	require.NoError(t, err)
	assert.Equal(t, 300, quota)

	var refreshed User
	require.NoError(t, DB.First(&refreshed, invitee.Id).Error)
	assert.Equal(t, 300, refreshed.Quota)

	var count int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestAdminDirectQuotaUpdate_DoesNotCreateInviteCommissionLedger(t *testing.T) {
	setupInviteCommissionRedemptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originRate := common.InviterRechargeCommissionRate
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originRate
	})
	common.InviterCommissionEnabled = true
	common.InviterRechargeCommissionRate = 0.1

	inviter := createInviteCommissionTestUser(t, "inviter_direct_add", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_direct_add", inviter.Id)

	// 模拟管理员直接修改余额（非充值、非兑换码），应不触发返佣入池。
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Update("quota", gorm.Expr("quota + ?", 500)).Error)

	var count int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestSettleInviteCommission_SkipsLegacyRedemptionLedger(t *testing.T) {
	setupInviteCommissionRedemptionTest(t)

	originDailyCap := common.InviterCommissionDailyCap
	common.InviterCommissionDailyCap = 0
	t.Cleanup(func() {
		common.InviterCommissionDailyCap = originDailyCap
	})

	inviter := createInviteCommissionTestUser(t, "inviter_legacy_redemption", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_legacy_redemption", inviter.Id)
	ledger := &InviteCommissionLedger{
		InviteeUserId:   invitee.Id,
		InviterUserId:   inviter.Id,
		TopupTradeNo:    "redeem:101",
		BizDate:         "2026-01-01",
		BaseQuota:       300,
		CommissionRate:  0.1,
		CommissionQuota: 30,
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
	assert.Equal(t, InviteCommissionRiskReasonRedemptionNotCommissionable, refreshedLedger.RiskReason)
	assert.Equal(t, int64(0), refreshedLedger.SettledQuota)

	var refreshedInviter User
	require.NoError(t, DB.First(&refreshedInviter, inviter.Id).Error)
	assert.Equal(t, int64(0), refreshedInviter.AffQuota)
	assert.Equal(t, int64(0), refreshedInviter.AffHistoryQuota)
}
