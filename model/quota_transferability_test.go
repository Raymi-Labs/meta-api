package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGrantUserQuotaTx_SeparatesPaidAndFreeQuota(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)
	user := createInviteCommissionTestUser(t, "quota_source_grants", 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"quota":              50,
		"transferable_quota": 100,
	}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return GrantUserQuotaTx(tx, user.Id, 30, 0)
	}))
	var afterFree User
	require.NoError(t, DB.First(&afterFree, user.Id).Error)
	assert.Equal(t, 80, afterFree.Quota)
	assert.Equal(t, 50, afterFree.TransferableQuota)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return GrantUserQuotaTx(tx, user.Id, 20, 20)
	}))
	var afterPaid User
	require.NoError(t, DB.First(&afterPaid, user.Id).Error)
	assert.Equal(t, 100, afterPaid.Quota)
	assert.Equal(t, 70, afterPaid.TransferableQuota)
}

func TestRevokeTransferableQuotaGrantTx_RemovesTransferEligibility(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)
	user := createInviteCommissionTestUser(t, "quota_source_revoke", 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"quota":              120,
		"transferable_quota": 80,
	}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return RevokeTransferableQuotaGrantTx(tx, user.Id, 50)
	}))
	var refreshed User
	require.NoError(t, DB.First(&refreshed, user.Id).Error)
	assert.Equal(t, 70, refreshed.Quota)
	assert.Equal(t, 30, refreshed.TransferableQuota)
}

func TestRevokeTransferableQuotaGrantTx_PartiallyShrinksActiveWalletCode(t *testing.T) {
	setupWalletRedemptionTest(t)
	creator := createInviteCommissionTestUser(t, "quota_revoke_active_code", 0)
	setWalletQuota(t, creator.Id, 300)
	created, err := CreateWalletFundedRedemption(creator.Id, 300, "quota-revoke-active-code-001")
	require.NoError(t, err)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return RevokeTransferableQuotaGrantTx(tx, creator.Id, 100)
	}))

	var refreshedCreator User
	var refreshedCode Redemption
	require.NoError(t, DB.First(&refreshedCreator, creator.Id).Error)
	require.NoError(t, DB.First(&refreshedCode, created.Redemption.Id).Error)
	assert.Zero(t, refreshedCreator.Quota)
	assert.Zero(t, refreshedCreator.TransferableQuota)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, refreshedCode.Status)
	assert.Equal(t, 200, refreshedCode.Quota)
	assert.Equal(t, 200, refreshedCode.TransferableQuota)
}

func TestRevokeTransferableQuotaGrantTx_DoesNotDoubleChargeSelfRedeem(t *testing.T) {
	setupWalletRedemptionTest(t)
	creator := createInviteCommissionTestUser(t, "quota_revoke_self_redeem", 0)
	setWalletQuota(t, creator.Id, 300)
	created, err := CreateWalletFundedRedemption(creator.Id, 100, "quota-revoke-self-code-001")
	require.NoError(t, err)
	_, err = RedeemWithResult(created.Redemption.Key, creator.Id)
	require.NoError(t, err)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return RevokeTransferableQuotaGrantTx(tx, creator.Id, 300)
	}))

	var refreshedCreator User
	require.NoError(t, DB.First(&refreshedCreator, creator.Id).Error)
	assert.Zero(t, refreshedCreator.Quota)
	assert.Zero(t, refreshedCreator.TransferableQuota)
}

func TestRevokeTransferableQuotaGrantTx_PreservesRecipientsOwnPaidQuota(t *testing.T) {
	setupWalletRedemptionTest(t)
	creator := createInviteCommissionTestUser(t, "quota_revoke_recipient_creator", 0)
	recipient := createInviteCommissionTestUser(t, "quota_revoke_paid_recipient", 0)
	setWalletQuota(t, creator.Id, 100)
	setWalletQuota(t, recipient.Id, 50)
	created, err := CreateWalletFundedRedemption(creator.Id, 100, "quota-revoke-recipient-code-001")
	require.NoError(t, err)
	_, err = RedeemWithResult(created.Redemption.Key, recipient.Id)
	require.NoError(t, err)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return RevokeTransferableQuotaGrantTx(tx, creator.Id, 100)
	}))

	var refreshedRecipient User
	require.NoError(t, DB.First(&refreshedRecipient, recipient.Id).Error)
	assert.Equal(t, 50, refreshedRecipient.Quota)
	assert.Equal(t, 50, refreshedRecipient.TransferableQuota)
}

func TestPaidTopUpEntryPointsGrantTransferableQuota(t *testing.T) {
	tests := []struct {
		name          string
		paymentMethod string
		provider      string
		amount        int64
		money         float64
		expectedQuota int
		complete      func(*TopUp) error
	}{
		{
			name:          "generic payment callback",
			paymentMethod: "alipay",
			provider:      PaymentProviderEpay,
			amount:        20,
			money:         16,
			expectedQuota: 200,
			complete: func(topUp *TopUp) error {
				return CompleteTopUpByTradeNoWithPayment(topUp.TradeNo, "test", "", topUp.PaymentMethod, topUp.PaymentProvider, nil)
			},
		},
		{
			name:          "stripe callback",
			paymentMethod: PaymentMethodStripe,
			provider:      PaymentProviderStripe,
			amount:        20,
			money:         20,
			expectedQuota: 200,
			complete: func(topUp *TopUp) error {
				return Recharge(topUp.TradeNo, "test_customer", "", 16)
			},
		},
		{
			name:          "creem callback",
			paymentMethod: PaymentMethodCreem,
			provider:      PaymentProviderCreem,
			amount:        250,
			money:         20,
			expectedQuota: 250,
			complete: func(topUp *TopUp) error {
				return RechargeCreem(topUp.TradeNo, "", "", "")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupInviteCommissionSubscriptionTest(t)
			originalQuotaPerUnit := common.QuotaPerUnit
			originalCommissionEnabled := common.InviterCommissionEnabled
			common.QuotaPerUnit = 10
			common.InviterCommissionEnabled = false
			t.Cleanup(func() {
				common.QuotaPerUnit = originalQuotaPerUnit
				common.InviterCommissionEnabled = originalCommissionEnabled
			})

			user := createInviteCommissionTestUser(t, "paid_entry_"+test.provider, 0)
			topUp := &TopUp{
				UserId:          user.Id,
				Amount:          test.amount,
				Money:           test.money,
				PaidMoney:       test.money,
				TradeNo:         "paid-entry-" + test.provider,
				PaymentMethod:   test.paymentMethod,
				PaymentProvider: test.provider,
				CreateTime:      common.GetTimestamp(),
				Status:          common.TopUpStatusPending,
			}
			require.NoError(t, DB.Create(topUp).Error)
			require.NoError(t, test.complete(topUp))
			// Payment providers retry callbacks. A replay must be an idempotent
			// success and must not grant quota a second time.
			require.NoError(t, test.complete(topUp))

			var refreshed User
			require.NoError(t, DB.First(&refreshed, user.Id).Error)
			assert.Equal(t, test.expectedQuota, refreshed.Quota)
			assert.Equal(t, test.expectedQuota, refreshed.TransferableQuota)
		})
	}
}

func TestFreeAndReferralQuotaRemainNonTransferable(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 10
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})
	user := createInviteCommissionTestUser(t, "non_transferable_rewards", 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"quota":              50,
		"transferable_quota": 50,
		"aff_quota":          100,
	}).Error)

	adminCode := &Redemption{
		UserId:        0,
		Key:           common.GetUUID(),
		Status:        common.RedemptionCodeStatusEnabled,
		Name:          "admin gift",
		BenefitType:   RedemptionBenefitTypeQuota,
		Quota:         100,
		CreatedTime:   common.GetTimestamp(),
		FundingSource: RedemptionFundingSourceAdmin,
	}
	require.NoError(t, DB.Create(adminCode).Error)
	_, err := RedeemWithResult(adminCode.Key, user.Id)
	require.NoError(t, err)

	var afterAdminGift User
	require.NoError(t, DB.First(&afterAdminGift, user.Id).Error)
	assert.Equal(t, 150, afterAdminGift.Quota)
	assert.Equal(t, 150, afterAdminGift.TransferableQuota)

	require.NoError(t, afterAdminGift.TransferAffQuotaToQuota(100))
	var afterReferralTransfer User
	require.NoError(t, DB.First(&afterReferralTransfer, user.Id).Error)
	assert.Equal(t, 250, afterReferralTransfer.Quota)
	assert.Equal(t, 250, afterReferralTransfer.TransferableQuota)
}

func TestTransferAffQuotaToQuota_PreservesCachedQuotaDelta(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	originalSyncFrequency := common.SyncFrequency
	originalRDB := common.RDB
	originalRedisEnabled := common.RedisEnabled
	common.QuotaPerUnit = 10
	common.SyncFrequency = 60
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.SyncFrequency = originalSyncFrequency
		common.RDB = originalRDB
		common.RedisEnabled = originalRedisEnabled
	})

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, redisClient.Close())
	})
	common.RDB = redisClient
	common.RedisEnabled = true

	user := createInviteCommissionTestUser(t, "affiliate_transfer_cache", 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"quota":              100,
		"transferable_quota": 100,
		"aff_quota":          100,
	}).Error)
	require.NoError(t, DB.First(user, user.Id).Error)
	require.NoError(t, updateUserCache(*user))

	// 模拟一次已写入 Redis、但尚未批量落库的并发调用扣费。
	require.NoError(t, cacheDecrUserQuota(user.Id, 20))
	cacheKey := getUserCacheKey(user.Id)
	ctx := context.Background()
	beforeTTL, err := common.RDB.TTL(ctx, cacheKey).Result()
	require.NoError(t, err)
	require.Greater(t, beforeTTL, time.Duration(0))

	require.NoError(t, user.TransferAffQuotaToQuota(50))

	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 150, persisted.Quota)
	assert.Equal(t, int64(50), persisted.AffQuota)
	assert.Equal(t, 150, persisted.TransferableQuota)

	cachedQuota, err := common.RDB.HGet(ctx, cacheKey, "Quota").Int()
	require.NoError(t, err)
	assert.Equal(t, 130, cachedQuota)
	assert.Equal(t, user.Username, common.RDB.HGet(ctx, cacheKey, "Username").Val())
	afterTTL, err := common.RDB.TTL(ctx, cacheKey).Result()
	require.NoError(t, err)
	assert.Equal(t, beforeTTL, afterTTL)
}

func TestMigrateWalletRedemptionTransferPolicyBackfillsNonCodeBalances(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)
	freeBalance := createInviteCommissionTestUser(t, "transfer_policy_free", 0)
	recipient := createInviteCommissionTestUser(t, "transfer_policy_recipient", 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", freeBalance.Id).Updates(map[string]any{
		"quota": 1000, "transferable_quota": 0,
	}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", recipient.Id).Updates(map[string]any{
		"quota": 500, "transferable_quota": 0,
	}).Error)
	require.NoError(t, DB.Create(&Redemption{
		UserId: freeBalance.Id, UsedUserId: recipient.Id, Key: common.GetUUID(),
		Status: common.RedemptionCodeStatusUsed, Name: "received wallet code",
		BenefitType: RedemptionBenefitTypeQuota, Quota: 300,
		FundingSource: RedemptionFundingSourceWallet, CreatedTime: common.GetTimestamp(), RedeemedTime: common.GetTimestamp(),
	}).Error)

	require.NoError(t, MigrateWalletRedemptionTransferPolicy())
	var migratedFree, migratedRecipient User
	require.NoError(t, DB.First(&migratedFree, freeBalance.Id).Error)
	require.NoError(t, DB.First(&migratedRecipient, recipient.Id).Error)
	assert.Equal(t, 1000, migratedFree.TransferableQuota)
	assert.Equal(t, 200, migratedRecipient.TransferableQuota)

	// The version marker makes repeated startup migrations idempotent.
	require.NoError(t, MigrateWalletRedemptionTransferPolicy())
	require.NoError(t, DB.First(&migratedRecipient, recipient.Id).Error)
	assert.Equal(t, 200, migratedRecipient.TransferableQuota)
}
