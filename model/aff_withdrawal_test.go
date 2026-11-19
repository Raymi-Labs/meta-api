package model

import (
	"errors"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAffWithdrawalTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	originDB := DB
	originLogDB := LOG_DB
	originRedisEnabled := common.RedisEnabled
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price
	originUSDExchangeRate := operation_setting.USDExchangeRate

	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.QuotaPerUnit = 100
	operation_setting.Price = 20
	operation_setting.USDExchangeRate = 999

	t.Cleanup(func() {
		DB = originDB
		LOG_DB = originLogDB
		common.RedisEnabled = originRedisEnabled
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
		operation_setting.USDExchangeRate = originUSDExchangeRate
	})

	require.NoError(t, db.AutoMigrate(&User{}, &AffWithdrawal{}))
}

func createAffWithdrawalTestUser(t *testing.T, username string, affQuota int) *User {
	t.Helper()
	user := &User{
		Username: username,
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  username + "-aff",
		AffQuota: int64(affQuota),
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestCreateAffWithdrawal_FreezesAffQuotaAndSnapshotsAmount(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	user := createAffWithdrawalTestUser(t, "withdraw_create", 500)

	withdrawal, err := CreateAffWithdrawal(user.Id, AffWithdrawalMinimumQuota, "user@example.com", "张三")
	require.NoError(t, err)

	assert.Equal(t, user.Id, withdrawal.UserId)
	assert.Equal(t, AffWithdrawalMinimumQuota, withdrawal.Quota)
	assert.EqualValues(t, AffWithdrawalMinimumAmountCents, withdrawal.AmountCents)
	assert.Equal(t, float64(100), withdrawal.QuotaPerUnitSnapshot)
	assert.Equal(t, float64(20), withdrawal.PriceSnapshot)
	assert.Equal(t, AffWithdrawalStatusPending, withdrawal.Status)

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(250), updated.AffQuota)

	var count int64
	require.NoError(t, DB.Model(&AffWithdrawal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestCreateAffWithdrawal_RejectsQuotaBelowMinimumWithoutMutation(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	user := createAffWithdrawalTestUser(t, "withdraw_below_minimum", 500)

	_, err := CreateAffWithdrawal(user.Id, AffWithdrawalMinimumQuota-1, "user@example.com", "张三")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAffWithdrawalQuotaTooLow)

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(500), updated.AffQuota)

	var count int64
	require.NoError(t, DB.Model(&AffWithdrawal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateAffWithdrawal_RejectsPaymentBelowMinimumWithoutMutation(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	operation_setting.Price = 19.99
	user := createAffWithdrawalTestUser(t, "withdraw_payment_below_minimum", 500)

	_, err := CreateAffWithdrawal(user.Id, AffWithdrawalMinimumQuota, "user@example.com", "张三")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAffWithdrawalAmountTooLow)

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(500), updated.AffQuota)

	var count int64
	require.NoError(t, DB.Model(&AffWithdrawal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateAffWithdrawal_InsufficientQuotaDoesNotCreateRecord(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	user := createAffWithdrawalTestUser(t, "withdraw_insufficient", AffWithdrawalMinimumQuota)

	_, err := CreateAffWithdrawal(user.Id, AffWithdrawalMinimumQuota+1, "user@example.com", "张三")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAffWithdrawalInsufficientQuota))

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(AffWithdrawalMinimumQuota), updated.AffQuota)

	var count int64
	require.NoError(t, DB.Model(&AffWithdrawal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestRejectAffWithdrawal_RefundsFrozenAffQuotaOnce(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	user := createAffWithdrawalTestUser(t, "withdraw_reject", 500)
	withdrawal, err := CreateAffWithdrawal(user.Id, AffWithdrawalMinimumQuota, "user@example.com", "张三")
	require.NoError(t, err)

	reviewed, err := RejectAffWithdrawal(withdrawal.Id, 99, "信息不匹配")
	require.NoError(t, err)
	assert.Equal(t, AffWithdrawalStatusRejected, reviewed.Status)
	assert.Equal(t, 99, reviewed.ReviewerUserId)
	assert.Equal(t, "信息不匹配", reviewed.AdminRemark)

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(500), updated.AffQuota)

	_, err = RejectAffWithdrawal(withdrawal.Id, 99, "重复驳回")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAffWithdrawalAlreadyReviewed))

	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(500), updated.AffQuota)
}

func TestApproveAffWithdrawal_DoesNotDeductAgain(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	user := createAffWithdrawalTestUser(t, "withdraw_approve", 500)
	withdrawal, err := CreateAffWithdrawal(user.Id, AffWithdrawalMinimumQuota, "user@example.com", "张三")
	require.NoError(t, err)

	reviewed, err := ApproveAffWithdrawal(withdrawal.Id, 99, "已支付宝转账")
	require.NoError(t, err)
	assert.Equal(t, AffWithdrawalStatusApproved, reviewed.Status)
	assert.Equal(t, 99, reviewed.ReviewerUserId)

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(250), updated.AffQuota)

	_, err = ApproveAffWithdrawal(withdrawal.Id, 99, "重复通过")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAffWithdrawalAlreadyReviewed))

	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(250), updated.AffQuota)
}

func TestApproveAffWithdrawal_RejectsLegacyRecordBelowMinimum(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	user := createAffWithdrawalTestUser(t, "withdraw_legacy_below_minimum", 500)
	withdrawal := &AffWithdrawal{
		UserId:               user.Id,
		Quota:                AffWithdrawalMinimumQuota - 1,
		AmountCents:          AffWithdrawalMinimumAmountCents,
		QuotaPerUnitSnapshot: common.QuotaPerUnit,
		PriceSnapshot:        operation_setting.Price,
		AlipayAccount:        "user@example.com",
		AlipayName:           "张三",
		Status:               AffWithdrawalStatusPending,
		CreatedAt:            common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(withdrawal).Error)

	_, err := ApproveAffWithdrawal(withdrawal.Id, 99, "尝试通过旧订单")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAffWithdrawalQuotaTooLow)

	var reloaded AffWithdrawal
	require.NoError(t, DB.First(&reloaded, "id = ?", withdrawal.Id).Error)
	assert.Equal(t, AffWithdrawalStatusPending, reloaded.Status)
	assert.Zero(t, reloaded.ReviewerUserId)

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(500), updated.AffQuota)
}

func TestApproveAffWithdrawal_RejectsLegacyRecordBelowMinimumPayment(t *testing.T) {
	setupAffWithdrawalTestDB(t)
	user := createAffWithdrawalTestUser(t, "withdraw_legacy_payment_below_minimum", 500)
	withdrawal := &AffWithdrawal{
		UserId:               user.Id,
		Quota:                AffWithdrawalMinimumQuota,
		AmountCents:          AffWithdrawalMinimumAmountCents - 1,
		QuotaPerUnitSnapshot: common.QuotaPerUnit,
		PriceSnapshot:        operation_setting.Price,
		AlipayAccount:        "user@example.com",
		AlipayName:           "张三",
		Status:               AffWithdrawalStatusPending,
		CreatedAt:            common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(withdrawal).Error)

	_, err := ApproveAffWithdrawal(withdrawal.Id, 99, "尝试通过低金额旧订单")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAffWithdrawalAmountTooLow)

	var reloaded AffWithdrawal
	require.NoError(t, DB.First(&reloaded, "id = ?", withdrawal.Id).Error)
	assert.Equal(t, AffWithdrawalStatusPending, reloaded.Status)
	assert.Zero(t, reloaded.ReviewerUserId)

	var updated User
	require.NoError(t, DB.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, int64(500), updated.AffQuota)
}

func TestCalculateAffWithdrawalAmountCents_UsesTopUpPriceOnly(t *testing.T) {
	setupAffWithdrawalTestDB(t)

	cents, err := CalculateAffWithdrawalAmountCents(50, common.QuotaPerUnit, operation_setting.Price)
	require.NoError(t, err)
	assert.EqualValues(t, 1000, cents)
}

func TestCalculateAffWithdrawalAmountCents_RejectsNonFiniteConfig(t *testing.T) {
	setupAffWithdrawalTestDB(t)

	for _, testCase := range []struct {
		name         string
		quotaPerUnit float64
		price        float64
	}{
		{name: "nan quota per unit", quotaPerUnit: math.NaN(), price: operation_setting.Price},
		{name: "infinite quota per unit", quotaPerUnit: math.Inf(1), price: operation_setting.Price},
		{name: "nan price", quotaPerUnit: common.QuotaPerUnit, price: math.NaN()},
		{name: "infinite price", quotaPerUnit: common.QuotaPerUnit, price: math.Inf(1)},
		{name: "overflowing price", quotaPerUnit: common.QuotaPerUnit, price: math.MaxFloat64},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := CalculateAffWithdrawalAmountCents(AffWithdrawalMinimumQuota, testCase.quotaPerUnit, testCase.price)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrAffWithdrawalInvalidPayment)
		})
	}
}
