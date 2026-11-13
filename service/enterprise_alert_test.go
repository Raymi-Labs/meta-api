package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/require"
)

// spend 发起并结算一次企业请求，成员的提醒线设为 2,000,000。
func (f *enterpriseBilling) spend(estimate int) {
	f.t.Helper()
	c, info := f.accept(estimate, 0)
	info.UserSetting.QuotaWarningThreshold = 2_000_000
	require.NoError(f.t, SettleBilling(c, info, estimate))
}

func TestEnterpriseMemberLowBalanceRemindsOnce(t *testing.T) {
	f := newEnterpriseBilling(t, 5_000_000, 0)

	// 结算后仍不低于提醒线：不发。
	f.spend(3_000_000)
	require.Empty(t, f.mails)
	require.Zero(t, f.memberRow().LowBalanceNotifiedAt)

	// 跌破提醒线：只提醒成员本人，且只提醒一次。
	f.spend(1_000_000)
	require.Len(t, f.mails, 1)
	require.Equal(t, f.user.Email, f.mails[0].receiver)
	require.NotZero(t, f.memberRow().LowBalanceNotifiedAt)
	f.spend(100_000)
	require.Len(t, f.mails, 1)

	// 获得新额度后资格复位，再次跌破时重新提醒。
	require.NoError(t, model.AllocateEnterpriseQuota(model.EnterpriseActor{UserId: f.owner.Id}, f.ent.Id, f.member.Id, 5_000_000, ""))
	require.Zero(t, f.memberRow().LowBalanceNotifiedAt)
	f.spend(5_000_000)
	require.Len(t, f.mails, 2)
	require.Equal(t, f.user.Email, f.mails[1].receiver)
	f.personalUntouched()
}

func TestEnterpriseLowBalanceRemindsManagers(t *testing.T) {
	f := newEnterpriseBilling(t, 5_000_000, 0)
	setThreshold := func(threshold int64) {
		require.NoError(t, model.DB.Model(&model.Enterprise{}).Where("id = ?", f.ent.Id).
			Updates(map[string]any{"low_balance_threshold": threshold}).Error)
	}
	notifiedAt := func() int64 {
		var ent model.Enterprise
		require.NoError(t, model.DB.First(&ent, f.ent.Id).Error)
		return ent.LowBalanceNotifiedAt
	}

	// 余额高于提醒线、或没设提醒线：不发。
	setThreshold(10_000_000)
	require.Zero(t, notifyEnterpriseLowBalances(10))
	setThreshold(0)
	require.Zero(t, notifyEnterpriseLowBalances(10))
	require.Empty(t, f.mails)

	// 低于提醒线：提醒所有者，只一次。
	setThreshold(200_000_000)
	require.Equal(t, 1, notifyEnterpriseLowBalances(10))
	require.Len(t, f.mails, 1)
	require.Equal(t, f.owner.Email, f.mails[0].receiver)
	require.NotZero(t, notifiedAt())
	require.Zero(t, notifyEnterpriseLowBalances(10))
	require.Len(t, f.mails, 1)

	// 余额回到提醒线以上时复位，之后再次跌破会重新提醒。
	setThreshold(10_000_000)
	require.Zero(t, notifyEnterpriseLowBalances(10))
	require.Zero(t, notifiedAt())
	setThreshold(200_000_000)
	require.Equal(t, 1, notifyEnterpriseLowBalances(10))
	require.Len(t, f.mails, 2)

	// 冻结的企业不提醒。
	require.NoError(t, model.DB.Model(&model.Enterprise{}).Where("id = ?", f.ent.Id).
		Updates(map[string]any{"low_balance_notified_at": 0, "status": model.EnterpriseStatusFrozen}).Error)
	require.Zero(t, notifyEnterpriseLowBalances(10))
	require.Len(t, f.mails, 2)
}
