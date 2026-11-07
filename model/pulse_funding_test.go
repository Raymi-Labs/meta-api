package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func paidFundingUser(t *testing.T, unknown, paid int) *User {
	t.Helper()
	u := createPaymentRiskCaseTestUser(t, "paid-funding-"+common.GetUUID())
	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Update("quota", unknown).Error)
	if paid > 0 {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			if err := GrantUserQuotaTx(tx, u.Id, paid, paid); err != nil {
				return err
			}
			return creditOnlinePaidFundingTx(tx, &TopUp{UserId: u.Id, TradeNo: common.GetUUID(), Money: 1, Status: common.TopUpStatusSuccess}, paid, "stripe")
		}))
	}
	return u
}

func readFundingUser(t *testing.T, id int) User {
	t.Helper()
	var u User
	require.NoError(t, DB.First(&u, id).Error)
	return u
}

func TestPaidFundingUnknownAndBonusNeverMintAllowance(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	u := paidFundingUser(t, 100, 0)
	require.NoError(t, GrantUserQuota(u.Id, 500, 500)) // transferable is not proof
	for _, source := range []string{"manual", "test", "redemption", "pulse_reward"} {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return creditOnlinePaidFundingTx(tx, &TopUp{UserId: u.Id, TradeNo: source, Money: 1, Status: common.TopUpStatusSuccess}, 100, source)
		}))
	}
	r, err := AdjustPulseWalletReservation(u.Id, "unknown-history", 100, "settled", true)
	require.NoError(t, err)
	require.Zero(t, r.PaidQuota)
	require.Zero(t, r.PaidQuota)
	require.Zero(t, readFundingUser(t, u.Id).PulsePaidQuota)
}

func TestPaidFundingPartialSettlementAndRepeatedTransitions(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	u := paidFundingUser(t, 100, 100)
	r, err := AdjustPulseWalletReservation(u.Id, "partial", 150, "reserved", true)
	require.NoError(t, err)
	require.EqualValues(t, 100, r.PaidQuota)
	for i := 0; i < 100; i++ {
		r, err = AdjustPulseWalletReservation(u.Id, "partial", 75, "settled", true)
		require.NoError(t, err)
	}
	require.EqualValues(t, 75, r.PaidQuota)
	require.EqualValues(t, 75, r.PaidQuota)
	after := readFundingUser(t, u.Id)
	require.Equal(t, 125, after.Quota)
	require.EqualValues(t, 25, after.PulsePaidQuota)
	_, err = AdjustPulseWalletReservation(u.Id, "partial", 76, "settled", true)
	require.Error(t, err)
	_, err = AdjustPulseWalletReservation(u.Id, "partial", 0, "refunded", true)
	require.Error(t, err, "final spend cannot be silently refunded without reversal provenance")
}

func TestPaidFundingRefundRestoresExactlyReservedPaidOnce(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	u := paidFundingUser(t, 40, 60)
	_, err := AdjustPulseWalletReservation(u.Id, "refund", 80, "reserved", true)
	require.NoError(t, err)
	for i := 0; i < 100; i++ {
		_, err = AdjustPulseWalletReservation(u.Id, "refund", 0, "refunded", true)
		require.NoError(t, err)
	}
	after := readFundingUser(t, u.Id)
	require.Equal(t, 100, after.Quota)
	require.EqualValues(t, 60, after.PulsePaidQuota)
}

func TestPaidFundingUnknownDebitInvalidatesInFlightRefund(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	u := paidFundingUser(t, 100, 100)
	_, err := AdjustPulseWalletReservation(u.Id, "inflight", 70, "reserved", true)
	require.NoError(t, err)
	originalBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = originalBatch })
	require.NoError(t, DecreaseUserQuota(u.Id, 10))
	require.Equal(t, 120, readFundingUser(t, u.Id).Quota, "unattributed debit must commit even with batch updates enabled")
	_, err = AdjustPulseWalletReservation(u.Id, "inflight", 0, "refunded", true)
	require.NoError(t, err)
	after := readFundingUser(t, u.Id)
	require.Equal(t, 190, after.Quota)
	require.Zero(t, after.PulsePaidQuota, "refund cannot resurrect invalidated allowance")
	var count int64
	require.NoError(t, DB.Model(&PulseFundingLedger{}).Where("kind = ? AND user_id = ?", "invalidate", u.Id).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestPaidFundingConcurrentReservationsDoNotReuseAllowance(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	u := paidFundingUser(t, 200, 100)
	type outcome struct {
		receipt PulseWalletReservation
		err     error
	}
	out := make(chan outcome, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, e := AdjustPulseWalletReservation(u.Id, fmt.Sprintf("parallel-%d", i), 10, "settled", true)
			out <- outcome{r, e}
		}(i)
	}
	wg.Wait()
	close(out)
	var total int64
	for x := range out {
		require.NoError(t, x.err)
		total += x.receipt.PaidQuota
	}
	require.EqualValues(t, 100, total)
	after := readFundingUser(t, u.Id)
	require.Equal(t, 100, after.Quota)
	require.Zero(t, after.PulsePaidQuota)
}

func TestPaidFundingRejectsCrossUserReceiptAndInsufficientReserveAtomically(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	u := paidFundingUser(t, 0, 30)
	other := paidFundingUser(t, 0, 30)
	_, err := AdjustPulseWalletReservation(u.Id, "same-id", 10, "reserved", true)
	require.NoError(t, err)
	_, err = AdjustPulseWalletReservation(other.Id, "same-id", 10, "reserved", true)
	require.Error(t, err)
	_, err = AdjustPulseWalletReservation(u.Id, "too-large", 100, "reserved", true)
	require.ErrorIs(t, err, ErrInsufficientImageQuota)
	require.Equal(t, 20, readFundingUser(t, u.Id).Quota)
	require.Equal(t, 30, readFundingUser(t, other.Id).Quota)
	var count int64
	require.NoError(t, DB.Model(&PulseWalletReservation{}).Where("request_id = ?", "too-large").Count(&count).Error)
	require.Zero(t, count)
}

func TestRetiredPulseFundingMetadataIsStrippedFromNewLogs(t *testing.T) {
	for _, quota := range []int{0, 1, 123456789} {
		other := map[string]interface{}{"pulse_funding": map[string]any{"status": "verified", "paid_quota": quota, "proof_ref": "forged"}, "keep": "value"}
		clean := withoutRetiredPulseFunding(other)
		_, ok := clean["pulse_funding"]
		require.False(t, ok)
		require.Equal(t, "value", clean["keep"])
		require.IsType(t, map[string]any{}, other["pulse_funding"], "caller's map must not be mutated")
	}
}

func TestPaidFundingRealPaymentReplayAndBonusConsumption(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	originalUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 10
	t.Cleanup(func() { common.QuotaPerUnit = originalUnit })
	u := paidFundingUser(t, 20, 0)
	order := &TopUp{UserId: u.Id, Amount: 10, Money: 10, PaidMoney: 10, TradeNo: "paid-verified-replay", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(order).Error)
	for i := 0; i < 10; i++ {
		require.NoError(t, Recharge(order.TradeNo, "", "", 10))
	}
	after := readFundingUser(t, u.Id)
	require.Equal(t, 120, after.Quota)
	require.EqualValues(t, 100, after.PulsePaidQuota)
	var count int64
	require.NoError(t, DB.Model(&PulseFundingLedger{}).Where("kind = ? AND source_ref = ?", "online_payment", order.TradeNo).Count(&count).Error)
	require.EqualValues(t, 1, count)
	r, err := AdjustPulseWalletReservation(u.Id, "paid-then-gift", 100, "settled", true)
	require.NoError(t, err)
	require.EqualValues(t, 100, r.PaidQuota)
	require.NoError(t, GrantUserQuota(u.Id, 50, 0))
	r, err = AdjustPulseWalletReservation(u.Id, "gift-cannot-loop", 50, "settled", true)
	require.NoError(t, err)
	require.Zero(t, r.PaidQuota)
	require.Zero(t, r.PaidQuota)
}

func TestPaidFundingWalletCodeAndPaymentClawbackCloseEligibility(t *testing.T) {
	setupWalletRedemptionTest(t)
	u := paidFundingUser(t, 100, 200)
	_, err := CreateWalletFundedRedemption(u.Id, 100, "paid-code-request-0001")
	require.NoError(t, err)
	after := readFundingUser(t, u.Id)
	require.Zero(t, after.PulsePaidQuota)
	require.False(t, after.PulseRewardHold)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return RevokeTransferableQuotaGrantTx(tx, u.Id, 100) }))
	require.True(t, readFundingUser(t, u.Id).PulseRewardHold)
}

func TestPaidFundingMutationsInvalidateCacheWithoutReapplyingDelta(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	u := paidFundingUser(t, 100, 100)
	server := miniredis.RunT(t)
	previous := common.RDB
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() { _ = common.RDB.Close(); common.RDB = previous })
	key := getUserCacheKey(u.Id)
	server.HSet(key, "Quota", "200")
	_, err := AdjustPulseWalletReservation(u.Id, "cache-reserve", 60, "reserved", true)
	require.NoError(t, err)
	require.False(t, server.Exists(key))
	server.HSet(key, "Quota", "140")
	_, err = AdjustPulseWalletReservation(u.Id, "cache-reserve", 0, "refunded", true)
	require.NoError(t, err)
	require.False(t, server.Exists(key))
	server.HSet(key, "Quota", "200")
	require.NoError(t, IncreaseUserQuota(u.Id, 10, false))
	require.False(t, server.Exists(key))
	require.Equal(t, 210, readFundingUser(t, u.Id).Quota)
}
