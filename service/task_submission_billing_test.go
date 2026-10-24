package service

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func taskBillingInfo(user, token int, key string, quota int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{UserId: user, TokenId: token, TokenKey: key,
		ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		PriceData: types.PriceData{Quota: quota}}
}

func TestTaskSubmissionReserveWalletRetryRollbackAndRefund(t *testing.T) {
	truncate(t)
	seedUser(t, 7301, 100)
	seedToken(t, 8301, 7301, "task-wallet", 80)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := taskBillingInfo(7301, 8301, "task-wallet", 40)
	require.Nil(t, PrepareTaskBilling(c, info))
	original := info.Billing
	require.Equal(t, 60, getUserQuota(t, 7301))
	info.PriceData.Quota = 90 // wallet succeeds, token rejects; both retain only 40
	require.NotNil(t, PrepareTaskBilling(c, info))
	require.Equal(t, 60, getUserQuota(t, 7301))
	require.Equal(t, 40, getTokenRemainQuota(t, 8301))
	info.PriceData.Quota = 70
	require.Nil(t, PrepareTaskBilling(c, info))
	require.Same(t, original, info.Billing)
	require.Equal(t, 30, getUserQuota(t, 7301))
	info.PriceData.Quota = 120
	require.NotNil(t, PrepareTaskBilling(c, info))
	require.Equal(t, 30, getUserQuota(t, 7301))
	info.PriceData.Quota = 10
	require.Nil(t, PrepareTaskBilling(c, info))
	require.Equal(t, 70, info.Billing.GetPreConsumedQuota())
	info.Billing.Refund(c)
	info.Billing.Refund(c)
	require.Eventually(t, func() bool { return getUserQuota(t, 7301) == 100 && getTokenRemainQuota(t, 8301) == 80 }, time.Second, time.Millisecond)
}

func TestTaskSubmissionFirstWalletReservationIsAtomic(t *testing.T) {
	truncate(t)
	seedUser(t, 7302, 100)
	// Two separate first-attempt sessions compete for the same wallet. The
	// initial cached balance check cannot authorize both conditional debits.
	var wg sync.WaitGroup
	results := make(chan *types.NewAPIError, 2)
	infos := []*relaycommon.RelayInfo{taskBillingInfo(7302, 0, "", 70), taskBillingInfo(7302, 0, "", 70)}
	for _, info := range infos {
		info.IsPlayground = true
		wg.Add(1)
		go func(info *relaycommon.RelayInfo) {
			defer wg.Done()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			results <- PrepareTaskBilling(c, info)
		}(info)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	require.Equal(t, 1, wins)
	require.Equal(t, 30, getUserQuota(t, 7302))
	for _, info := range infos {
		if info.Billing != nil {
			s := info.Billing.(*BillingSession)
			require.True(t, s.funding.(*WalletFunding).requireAvailableQuota)
			require.False(t, s.trusted)
		}
	}
}

func TestTaskSubmissionReservesPreviouslyTrustedSession(t *testing.T) {
	truncate(t)
	seedUser(t, 7303, 200)
	seedToken(t, 8303, 7303, "task-trusted", 100)
	info := taskBillingInfo(7303, 8303, "task-trusted", 50)
	info.Billing = &BillingSession{relayInfo: info, funding: &WalletFunding{userId: 7303}, trusted: true}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Nil(t, PrepareTaskBilling(c, info))
	require.Equal(t, 50, info.Billing.GetPreConsumedQuota())
	require.Equal(t, 150, getUserQuota(t, 7303))
	require.Equal(t, 50, getTokenRemainQuota(t, 8303))
}

func TestTaskSubmissionTokenOnlyAndSubscriptionReservations(t *testing.T) {
	t.Run("token-only", func(t *testing.T) {
		truncate(t)
		seedUser(t, 7304, 0)
		seedBillingPackageToken(t, 8304, 7304, "task-package", 100, 80, 0)
		c := newPackageBillingContext(true)
		info := taskBillingInfo(7304, 8304, "task-package", 40)
		require.Nil(t, PrepareTaskBilling(c, info))
		info.PriceData.Quota = 70
		require.Nil(t, PrepareTaskBilling(c, info))
		info.PriceData.Quota = 90
		require.NotNil(t, PrepareTaskBilling(c, info))
		require.Equal(t, 0, getUserQuota(t, 7304))
		require.Equal(t, 30, getTokenRemainQuota(t, 8304))
		info.Billing.Refund(c)
		require.Eventually(t, func() bool { return getTokenRemainQuota(t, 8304) == 100 }, time.Second, time.Millisecond)
	})
	t.Run("subscription", func(t *testing.T) {
		truncate(t)
		seedUser(t, 7305, 0)
		seedToken(t, 8305, 7305, "task-sub", 80)
		seedSubscription(t, 9305, 7305, 100, 0)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := taskBillingInfo(7305, 8305, "task-sub", 40)
		info.SubscriptionId = 9305
		info.Billing = &BillingSession{relayInfo: info, funding: &SubscriptionFunding{subscriptionId: 9305}}
		require.Nil(t, PrepareTaskBilling(c, info))
		info.PriceData.Quota = 90
		require.NotNil(t, PrepareTaskBilling(c, info)) // token rejection rolls funding back
		require.EqualValues(t, 40, getSubscriptionUsed(t, 9305))
		info.PriceData.Quota = 120
		require.NotNil(t, PrepareTaskBilling(c, info))
		info.PriceData.Quota = 70
		require.Nil(t, PrepareTaskBilling(c, info))
		info.Billing.Refund(c)
		info.Billing.Refund(c)
		require.Eventually(t, func() bool { return getSubscriptionUsed(t, 9305) == 0 && getTokenRemainQuota(t, 8305) == 80 }, time.Second, time.Millisecond)
	})
}

func TestTaskSubmissionRejectsFinalizedSession(t *testing.T) {
	info := taskBillingInfo(0, 0, "", 40)
	info.Billing = &BillingSession{relayInfo: info, settled: true}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.NotNil(t, PrepareTaskBilling(c, info))
}
