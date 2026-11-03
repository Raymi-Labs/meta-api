package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestChatWalletReservationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			quotaNoticeTestDB(t, dialect)
			oldSQLite, oldRedis := common.UsingSQLite, common.RedisEnabled
			common.UsingSQLite, common.RedisEnabled = dialect == "sqlite", false
			t.Cleanup(func() { common.UsingSQLite, common.RedisEnabled = oldSQLite, oldRedis })
			require.NoError(t, DB.AutoMigrate(&User{}, &PulseFundingLedger{}, &PulseWalletReservation{}))
			user := User{Username: "chat-" + common.GetUUID(), AffCode: common.GetUUID()[:12], Quota: 55, Status: common.UserStatusEnabled}
			require.NoError(t, DB.Create(&user).Error)
			prefix := common.GetUUID()
			var wg sync.WaitGroup
			results := make(chan PulseWalletReservation, 20)
			errs := make(chan error, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					r, e := ReserveChatWalletQuota(user.Id, fmt.Sprintf("%s-%d", prefix, i), 100, 10, 10)
					if e != nil {
						errs <- e
					} else {
						results <- r
					}
				}(i)
			}
			wg.Wait()
			close(results)
			close(errs)
			var accepted []PulseWalletReservation
			for r := range results {
				accepted = append(accepted, r)
			}
			require.Len(t, accepted, 5, "only funded holds may enter upstream")
			for e := range errs {
				var rejection *types.QuotaRejection
				require.ErrorAs(t, e, &rejection)
				require.Equal(t, types.QuotaReasonInFlight, rejection.RejectionReason())
			}
			var got User
			require.NoError(t, DB.First(&got, user.Id).Error)
			require.Equal(t, 5, got.Quota)
			for _, r := range accepted {
				again, e := ReserveChatWalletQuota(user.Id, r.RequestID, 100, 10, 10)
				require.NoError(t, e)
				require.Equal(t, r.Quota, again.Quota)
				_, e = AdjustPulseWalletReservation(user.Id, r.RequestID, 0, "refunded", false)
				require.NoError(t, e)
				_, e = AdjustPulseWalletReservation(user.Id, r.RequestID, 0, "refunded", false)
				require.NoError(t, e)
			}
			require.NoError(t, DB.First(&got, user.Id).Error)
			require.Equal(t, 55, got.Quota)
			r, e := ReserveChatWalletQuota(user.Id, prefix+"-settle", 100, 10, 10)
			require.NoError(t, e)
			require.EqualValues(t, 10, r.Quota)
			_, e = AdjustPulseWalletReservation(user.Id, r.RequestID, 70, "settled", false)
			require.NoError(t, e)
			_, e = AdjustPulseWalletReservation(user.Id, r.RequestID, 70, "settled", false)
			require.NoError(t, e)
			require.NoError(t, DB.First(&got, user.Id).Error)
			require.Equal(t, -15, got.Quota, "actual usage remains billable even above initial hold")
			_, e = ReserveChatWalletQuota(user.Id, prefix+"-empty", 100, 10, 10)
			var rejection *types.QuotaRejection
			require.ErrorAs(t, e, &rejection)
			require.Equal(t, types.QuotaReasonExhausted, rejection.RejectionReason())
			require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 5).Error)
			_, e = ReserveChatWalletQuota(user.Id, prefix+"-small", 100, 10, 10)
			require.ErrorAs(t, e, &rejection)
			require.Equal(t, types.QuotaReasonRequestTooLarge, rejection.RejectionReason())
			var rejectedRows int64
			require.NoError(t, DB.Model(&PulseWalletReservation{}).Where("request_id IN ?", []string{prefix + "-empty", prefix + "-small"}).Count(&rejectedRows).Error)
			require.Zero(t, rejectedRows)
		})
	}
}

func TestChatWalletReservationPreservesCashAllocation(t *testing.T) {
	setupCashTestDB(t)
	user := createPaymentRiskCaseTestUser(t, "bounded-cash")
	lot := cashGrantTest(t, user, 100, "topup")
	receipt, err := ReserveChatWalletQuota(user.Id, "bounded-cash-settle", 1000, 10, 10)
	require.NoError(t, err)
	require.EqualValues(t, 10, receipt.Quota)
	for i := 0; i < 2; i++ {
		_, err = AdjustPulseWalletReservation(user.Id, receipt.RequestID, 5, "settled", false)
		require.NoError(t, err)
	}
	require.NoError(t, DB.First(&lot, lot.ID).Error)
	require.EqualValues(t, 95, lot.RemainingQuota)
	require.Equal(t, 95, readFundingUser(t, user.Id).Quota)
	_, err = ReserveChatWalletQuota(user.Id, "bounded-cash-refund", 1000, 10, 10)
	require.NoError(t, err)
	_, err = AdjustPulseWalletReservation(user.Id, "bounded-cash-refund", 0, "refunded", false)
	require.NoError(t, err)
	require.NoError(t, DB.First(&lot, lot.ID).Error)
	require.EqualValues(t, 95, lot.RemainingQuota)
	var allocations []CashFundingAllocation
	require.NoError(t, DB.Where("user_id = ? AND quota > 0", user.Id).Find(&allocations).Error)
	require.Len(t, allocations, 1)
	require.EqualValues(t, 5, allocations[0].Quota)
	require.Equal(t, "settled", allocations[0].Status)
}
