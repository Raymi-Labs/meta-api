package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestInviteTierPeriodBoundaries(t *testing.T) {
	for _, tc := range []struct {
		date       string
		tier, kind int
		key, end   string
	}{
		{"2026-12-31T18:00:00Z", 0, 2, "2027-01", "2027-02-01"},
		{"2021-01-01T00:00:00Z", 3, 1, "2020-W53", "2021-01-04"},
		{"2026-11-22T16:00:00Z", 3, 1, "2026-W48", "2026-11-30"},
	} {
		at, e := time.Parse(time.RFC3339, tc.date)
		require.NoError(t, e)
		kind, key, end := invitePeriod(at.In(time.FixedZone("other", -7*3600)), tc.tier)
		require.Equal(t, tc.kind, kind)
		require.Equal(t, tc.key, key)
		require.Equal(t, tc.end, end.Format("2006-01-02"))
		l := InviteCommissionLedger{InviterTierSnapshot: tc.tier}
		scheduleInviteLedger(&l, at.Unix())
		require.GreaterOrEqual(t, l.NextRetryAt, at.Unix()+7*86400)
	}
}
func tierPayment(t *testing.T, inviter *User, name string) (*User, *TopUp) {
	buyer := createInviteCommissionTestUser(t, name, inviter.Id)
	require.NoError(t, DB.Model(buyer).Update("created_at", common.GetTimestamp()-10*86400).Error)
	order := createInviteCommissionTopUp(t, buyer.Id, name+"_paid", 100, 100, 100, common.TopUpStatusSuccess)
	require.NoError(t, EnqueueInviteCommissionFromTopUp(order))
	return buyer, order
}
func tierRows(t *testing.T, trade string) []InviteCommissionLedger {
	var rows []InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ? OR source_trade_no = ?", trade, trade).Order("id").Find(&rows).Error)
	return rows
}
func TestInviteTierStatementRegistrationAndFrozenPayout(t *testing.T) {
	setupTierTest(t)
	inviter := createInviteCommissionTestUser(t, "cycle_owner", 0)
	buyer, order := tierPayment(t, inviter, "cycle_buyer")
	rows := tierRows(t, order.TradeNo)
	require.Len(t, rows, 2)
	require.Equal(t, 6, rows[1].CommissionLevel)
	require.Equal(t, fmt.Sprintf("invite-register:%d", buyer.Id), rows[1].TopupTradeNo)
	now := rows[0].NextRetryAt + 1
	require.NoError(t, IssueInviteStatements(now))
	require.NoError(t, IssueInviteStatements(now))
	var st []InviteCommissionStatement
	require.NoError(t, DB.Find(&st).Error)
	require.Len(t, st, 1)
	require.Equal(t, 2, st[0].Status)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: inviter.Id, WithdrawalFrozen: true}).Error)
	p, s, e := settleTierCommission(rows[0].Id, 0, now)
	require.NoError(t, e)
	require.False(t, p)
	require.False(t, s)
	var deferred InviteCommissionLedger
	require.NoError(t, DB.First(&deferred, rows[0].Id).Error)
	require.Zero(t, deferred.RetryCount)
	require.Equal(t, "payout_frozen", deferred.RiskReason)
	require.NoError(t, DB.Model(&PromoterProfile{}).Where("user_id = ?", inviter.Id).Update("withdrawal_frozen", false).Error)
	require.NoError(t, DB.Create(&InviteCommissionDebt{InviterUserId: inviter.Id, DebtAmount: 4000}).Error)
	for _, l := range rows {
		p, _, e := settleTierCommission(l.Id, 0, now+86401)
		require.NoError(t, e)
		require.True(t, p)
		p, _, e = settleTierCommission(l.Id, 0, now+86401)
		require.NoError(t, e)
		require.False(t, p)
	}
	var u User
	require.NoError(t, DB.First(&u, inviter.Id).Error)
	require.Equal(t, int64(1200), u.AffQuota)
	require.NoError(t, DB.First(&st[0], st[0].Id).Error)
	require.Equal(t, 3, st[0].Status)
	require.Equal(t, int64(4000), st[0].DeductedAmount)
	require.Equal(t, int64(1200), st[0].PaidAmount)
}
func TestInviteTierReversalDoesNotOverdrawAndRestoresDebtOffset(t *testing.T) {
	setupTierTest(t)
	owner := createInviteCommissionTestUser(t, "debt_owner", 0)
	_, order := tierPayment(t, owner, "debt_buyer")
	rows := tierRows(t, order.TradeNo)
	now := rows[0].NextRetryAt + 1
	require.NoError(t, IssueInviteStatements(now))
	require.NoError(t, DB.Create(&InviteCommissionDebt{InviterUserId: owner.Id, DebtAmount: 5000}).Error)
	for _, l := range rows {
		_, _, e := settleTierCommission(l.Id, 0, now)
		require.NoError(t, e)
	}
	require.NoError(t, DB.Model(&User{}).Where("id = ?", owner.Id).Updates(map[string]any{"aff_quota": 0, "quota": 10, "transferable_quota": 0}).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { _, e := reverseInviteCommissionsByTradeNoTx(tx, order.TradeNo); return e }))
	}
	var u User
	require.NoError(t, DB.First(&u, owner.Id).Error)
	require.Equal(t, 10, u.Quota)
	var debt InviteCommissionDebt
	require.NoError(t, DB.Where("inviter_user_id = ?", owner.Id).First(&debt).Error)
	require.Equal(t, int64(5200), debt.DebtAmount-debt.PaidAmount)
}
func TestInviteTierValidityAndLoopExclusion(t *testing.T) {
	setupTierTest(t)
	owner := createInviteCommissionTestUser(t, "valid_owner", 0)
	buyer := createInviteCommissionTestUser(t, "valid_buyer", owner.Id)
	now := common.GetTimestamp()
	buyer.CreatedAt = now
	buyer.UsedQuota = 0
	require.Equal(t, "pending_time", inviteValidity(buyer, 100, now))
	require.Equal(t, "valid", inviteValidity(buyer, 100, now+7*86400))
	require.Equal(t, "pending_usage", inviteValidity(buyer, 99, now))
	require.Equal(t, "invalid", inviteValidity(buyer, 99, now+7*86400))
	buyer.UsedQuota = 1
	require.Equal(t, "valid", inviteValidity(buyer, 100, now))
	require.NoError(t, DB.Model(buyer).Update("invite_transferred_quota", 60000).Error)
	order := createInviteCommissionTopUp(t, buyer.Id, "loop_source", 100, 100, 100, common.TopUpStatusSuccess)
	require.NoError(t, EnqueueInviteCommissionFromTopUp(order))
	require.NoError(t, EnqueueInviteCommissionFromTopUp(order))
	var l InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", order.TradeNo).First(&l).Error)
	require.Equal(t, int64(40000), l.BaseQuota)
	require.Equal(t, int64(60000), l.ExcludedBaseQuota)
	var refreshed User
	require.NoError(t, DB.First(&refreshed, buyer.Id).Error)
	require.Equal(t, int64(60000), refreshed.InviteExcludedQuota)
}
func TestInviteTierIdentityPrivacyAndRelatedAccount(t *testing.T) {
	setupTierTest(t)
	t.Setenv("CRYPTO_SECRET", "test-only-persistent-key")
	owner := createInviteCommissionTestUser(t, "identity_owner", 0)
	buyer := createInviteCommissionTestUser(t, "identity_buyer", owner.Id)
	require.NoError(t, SavePromoterIdentity(owner.Id, "Test Owner", "DOC123456"))
	require.NoError(t, ReviewPromoterIdentity(owner.Id, 1, true, "verified"))
	require.NoError(t, SavePromoterIdentity(buyer.Id, "Test Buyer", "DOC123456"))
	require.ErrorIs(t, ReviewPromoterIdentity(buyer.Id, 1, true, ""), ErrInviteEligibility)
	var identity PromoterIdentity
	require.NoError(t, DB.Where("user_id = ?", owner.Id).First(&identity).Error)
	raw, e := common.Marshal(identity)
	require.NoError(t, e)
	require.NotContains(t, string(raw), "DOC123456")
	require.NotContains(t, string(raw), "ciphertext")
	plain, e := InviteIdentityPlaintext(owner.Id)
	require.NoError(t, e)
	require.Equal(t, "DOC123456", plain["document"])
	order := createInviteCommissionTopUp(t, buyer.Id, "duplicate_subject", 100, 100, 100, common.TopUpStatusSuccess)
	require.NoError(t, EnqueueInviteCommissionFromTopUp(order))
	riskRows := tierRows(t, order.TradeNo)
	require.Len(t, riskRows, 1)
	require.Equal(t, InviteCommissionStatusSkipped, riskRows[0].Status)
	var risk PaymentRiskCase
	require.NoError(t, DB.First(&risk).Error)
	require.Equal(t, "invite_related_account", risk.Reason)
}
func TestInviteTierPromotionLeaveAndRebind(t *testing.T) {
	setupTierTest(t)
	owner := createInviteCommissionTestUser(t, "promoter_owner", 0)
	member := createInviteCommissionTestUser(t, "promoter_member", owner.Id)
	dept := Department{LeaderUserId: owner.Id}
	require.NoError(t, DB.Create(&dept).Error)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: member.Id, AdminOverrideTier: 2}).Error)
	a, e := ApplyPromoter(member.Id, 4, dept.Id)
	require.NoError(t, e)
	require.NoError(t, ReviewPromoterApplication(a.Id, 1, true, "approved"))
	require.ErrorIs(t, ReviewPromoterApplication(a.Id, 1, true, "again"), ErrInviteConflict)
	require.NoError(t, LeaveInviteDepartment(member.Id))
	var m DepartmentMember
	require.NoError(t, DB.Where("user_id = ?", member.Id).First(&m).Error)
	require.Zero(t, m.LeftAt)
	require.NoError(t, MaintainInvitePromoters(m.LeaveEffectiveAt))
	require.NoError(t, DB.First(&m, m.Id).Error)
	require.Positive(t, m.LeftAt)
	var p PromoterProfile
	require.NoError(t, DB.Where("user_id = ?", member.Id).First(&p).Error)
	require.True(t, p.UpstreamCut)
	require.Equal(t, 3, p.ManualTier)
	require.ErrorIs(t, RequestInviteRebind(member.Id, owner.Id), ErrInviteEligibility)
	child := createInviteCommissionTestUser(t, "rebind_child", owner.Id)
	newOwner := createInviteCommissionTestUser(t, "new_owner", 0)
	require.NoError(t, RequestInviteRebind(child.Id, newOwner.Id))
	var r InviteRebind
	require.NoError(t, DB.First(&r).Error)
	require.ErrorIs(t, ConfirmInviteRebind(r.Id, owner.Id), ErrInviteEligibility)
	require.NoError(t, ConfirmInviteRebind(r.Id, newOwner.Id))
	require.Error(t, RequestInviteRebind(child.Id, owner.Id))
}
func TestInviteTierPoolAdjustmentIdempotent(t *testing.T) {
	setupTierTest(t)
	owner := createInviteCommissionTestUser(t, "pool_owner", 0)
	buyer := createInviteCommissionTestUser(t, "pool_buyer", owner.Id)
	when := time.Date(2026, 1, 10, 0, 0, 0, 0, inviteBusinessLocation).Unix()
	order := createInviteCommissionTopUp(t, buyer.Id, "pool_order", 100, 1, 1, common.TopUpStatusSuccess)
	require.NoError(t, DB.Model(order).Update("complete_time", when).Error)
	l := InviteCommissionLedger{TopupTradeNo: order.TradeNo, SourceTradeNo: order.TradeNo, InviterUserId: owner.Id, InviteeUserId: buyer.Id, CommissionLevel: 1, AmountVersion: 2, CommissionQuota: 1000, BaseQuota: 1000, Status: InviteCommissionStatusPending, BizDate: "2026-01-10", ValidityState: "valid"}
	scheduleInviteLedger(&l, when)
	require.NoError(t, DB.Create(&l).Error)
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, inviteBusinessLocation).Unix()
	require.NoError(t, AdjustInvitePoolMonth("2026-01", now))
	require.NoError(t, AdjustInvitePoolMonth("2026-01", now))
	require.NoError(t, DB.First(&l, l.Id).Error)
	require.Equal(t, int64(160), l.CommissionQuota)
	require.Equal(t, int64(840), l.PoolDeduction)
	var carry InvitePoolCarry
	require.NoError(t, DB.First(&carry).Error)
	require.Equal(t, int64(840), carry.Amount)
	require.NoError(t, ReleaseInviteCarry(carry.NextAt))
	require.NoError(t, ReleaseInviteCarry(carry.NextAt))
	var n int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Where("topup_trade_no LIKE ?", "invite-carry:%").Count(&n).Error)
	require.Equal(t, int64(1), n)
}
func TestInviteTierEarlyPayoutAndAdminUndo(t *testing.T) {
	setupTierTest(t)
	owner := createInviteCommissionTestUser(t, "early_owner", 0)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: owner.Id, ManualTier: 3}).Error)
	_, order := tierPayment(t, owner, "early_buyer")
	rows := tierRows(t, order.TradeNo)
	now := common.GetTimestamp()
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Where("source_trade_no = ?", order.TradeNo).Updates(map[string]any{"issue_at": now - 1, "observed_until": now - 1}).Error)
	require.NoError(t, IssueInviteStatements(now))
	a, e := RequestInviteEarlyPayout(owner.Id)
	require.NoError(t, e)
	_, e = RequestInviteEarlyPayout(owner.Id)
	require.ErrorIs(t, e, ErrInviteConflict)
	require.NoError(t, ReviewInviteEarlyPayout(a.Id, 1, true, "ok"))
	p, _, e := settleTierCommission(rows[0].Id, 0, now)
	require.NoError(t, e)
	require.True(t, p)
	action, e := ChangeInvitePromoter(owner.Id, 1, "freeze", 0, 0, true, 0, "review")
	require.NoError(t, e)
	require.ErrorIs(t, checkInvitePayoutEligibility(DB, owner.Id), ErrInviteFrozen)
	require.NoError(t, ReverseInviteAdminAction(action.Id, 1))
	require.NoError(t, checkInvitePayoutEligibility(DB, owner.Id))
}
func TestInviteTierCommissionerThirdMonth(t *testing.T) {
	setupTierTest(t)
	owner := createInviteCommissionTestUser(t, "assessed_owner", 0)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: owner.Id, ManualTier: 3, ManualTierEffectiveMonth: "2026-01"}).Error)
	tier, e := getTierForCommission(DB, owner.Id, "2026-02")
	require.NoError(t, e)
	require.Equal(t, 3, tier)
	tier, e = getTierForCommission(DB, owner.Id, "2026-03")
	require.NoError(t, e)
	require.Equal(t, 0, tier)
	for i := 0; i < 30; i++ {
		require.NoError(t, DB.Create(&InviteeQualification{InviteeId: i + 100, InviterId: owner.Id, QualifiedMonth: "2026-02", QualifiedAt: 1, Status: 1}).Error)
	}
	tier, e = getTierForCommission(DB, owner.Id, "2026-03")
	require.NoError(t, e)
	require.Equal(t, 3, tier)
}

func TestInviteTierRollbackSwitchPreservesFreeze(t *testing.T) {
	setupTierTest(t)
	u := createInviteCommissionTestUser(t, "rollback_frozen", 0)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: u.Id, WithdrawalFrozen: true}).Error)
	require.NoError(t, DB.Model(u).Update("aff_quota", 100000000).Error)
	s := common.GetInviteTierSettings()
	s.Enabled = false
	s.ActivatedAt = 1
	require.NoError(t, common.SetInviteTierSettings(s))
	require.ErrorIs(t, u.TransferAffQuotaToQuota(1000), ErrInviteFrozen)
}

func TestInviteTierUsageOnlyQualificationAndAdminUndoIsolation(t *testing.T) {
	setupTierTest(t)
	owner := createInviteCommissionTestUser(t, "usage_owner", 0)
	buyer := createInviteCommissionTestUser(t, "usage_buyer", owner.Id)
	require.NoError(t, DB.Model(buyer).Updates(map[string]any{"created_at": common.GetTimestamp() - 8*86400, "used_quota": 1}).Error)
	require.NoError(t, RefreshInviteQualifications(common.GetTimestamp()))
	require.NoError(t, RefreshInviteQualifications(common.GetTimestamp()))
	var l InviteCommissionLedger
	require.NoError(t, DB.First(&l).Error)
	require.Equal(t, 6, l.CommissionLevel)
	require.NoError(t, IssueInviteStatements(l.NextRetryAt+1))
	p, settled, e := settleTierCommission(l.Id, 0, l.NextRetryAt+1)
	require.NoError(t, e)
	require.True(t, p)
	require.True(t, settled)
	action, e := ChangeInvitePromoter(owner.Id, 1, "freeze", 0, 0, true, 0, "")
	require.NoError(t, e)
	require.NoError(t, DB.Model(&PromoterProfile{}).Where("user_id = ?", owner.Id).Update("manual_tier", 3).Error)
	require.NoError(t, ReverseInviteAdminAction(action.Id, 1))
	var profile PromoterProfile
	require.NoError(t, DB.Where("user_id = ?", owner.Id).First(&profile).Error)
	require.Equal(t, 3, profile.ManualTier)
}

func TestInviteTierWithdrawalMinimumAndFreezeAtReview(t *testing.T) {
	setupTierTest(t)
	require.NoError(t, DB.AutoMigrate(&AffWithdrawal{}))
	user := createInviteCommissionTestUser(t, "withdraw_tier", 0)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: user.Id, ManualTier: 3}).Error)
	require.NoError(t, DB.Model(user).Update("aff_quota", 100000).Error)
	_, e := CreateAffWithdrawal(user.Id, 994, "test-account", "Test User")
	require.ErrorIs(t, e, ErrInviteMinimum)
	withdrawal, e := CreateAffWithdrawal(user.Id, 1000, "test-account", "Test User")
	require.NoError(t, e)
	require.Equal(t, int64(100), withdrawal.AmountCents)
	require.NoError(t, DB.Model(&PromoterProfile{}).Where("user_id = ?", user.Id).Update("withdrawal_frozen", true).Error)
	_, e = ApproveAffWithdrawal(withdrawal.Id, 1, "")
	require.ErrorIs(t, e, ErrInviteFrozen)
	_, e = RejectAffWithdrawal(withdrawal.Id, 1, "cancel frozen request")
	require.NoError(t, e)
	var refreshed User
	require.NoError(t, DB.First(&refreshed, user.Id).Error)
	require.Equal(t, int64(100000), refreshed.AffQuota)
}

func TestInviteTierReversalDebitsTransferableFunds(t *testing.T) {
	setupTierTest(t)
	u := createInviteCommissionTestUser(t, "mixed_wallet", 0)
	require.NoError(t, DB.Model(u).Updates(map[string]any{"quota": 100, "transferable_quota": 40, "aff_quota": 0}).Error)
	require.NoError(t, DB.Create(&InviteCommissionLedger{TopupTradeNo: "mixed-refund", InviterUserId: u.Id, CommissionLevel: 1, AmountVersion: 2, Status: InviteCommissionStatusSettled, CommissionQuota: 100, SettledQuota: 100}).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { _, e := reverseInviteCommissionsByTradeNoTx(tx, "mixed-refund"); return e }))
	require.NoError(t, DB.First(u, u.Id).Error)
	require.Equal(t, 60, u.Quota)
	require.Zero(t, u.TransferableQuota)
	var debt InviteCommissionDebt
	require.NoError(t, DB.First(&debt).Error)
	require.Equal(t, int64(60), debt.DebtAmount)
}

func TestInviteTierStatementAccountsForDailyCap(t *testing.T) {
	setupTierTest(t)
	u := createInviteCommissionTestUser(t, "cap_statement", 0)
	_, order := tierPayment(t, u, "cap_statement_buyer")
	rows := tierRows(t, order.TradeNo)
	now := rows[0].NextRetryAt + 1
	require.NoError(t, IssueInviteStatements(now))
	for _, row := range rows {
		_, _, e := settleTierCommission(row.Id, 1000, now)
		require.NoError(t, e)
	}
	var statement InviteCommissionStatement
	require.NoError(t, DB.First(&statement).Error)
	require.Equal(t, int64(1000), statement.PaidAmount)
	require.Equal(t, statement.DueAmount, statement.PaidAmount+statement.DeductedAmount)
}
