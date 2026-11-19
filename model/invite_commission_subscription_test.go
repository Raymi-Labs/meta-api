package model

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupInviteCommissionSubscriptionTest(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	originDB := DB
	originLogDB := LOG_DB
	originRedisEnabled := common.RedisEnabled
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()
	t.Cleanup(func() {
		DB = originDB
		LOG_DB = originLogDB
		common.RedisEnabled = originRedisEnabled
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		initCol()
	})

	require.NoError(t, db.AutoMigrate(
		&User{},
		&InviteCommissionLedger{},
		&InviteCommissionDailyCapState{},
		&SubscriptionPlan{},
		&SubscriptionOrder{},
		&SubscriptionIssuance{},
		&UserSubscription{},
		&BenefitChangeRecord{},
		&PulseFundingLedger{},
		&PulseWalletReservation{},
		&BenefitRollbackOperation{},
		&TopUp{},
		&Log{},
		&Redemption{},
		&RedemptionReviewCase{},
		&WalletTransferLock{},
		&Option{},
		&SellableTokenProduct{},
		&SellableTokenOrder{},
		&SellableTokenIssuance{},
		&Token{},
	))

	clear := func(model any) {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(model).Error)
	}
	clear(&TopUp{})
	clear(&SubscriptionOrder{})
	clear(&SubscriptionIssuance{})
	clear(&UserSubscription{})
	clear(&BenefitChangeRecord{})
	clear(&BenefitRollbackOperation{})
	clear(&SubscriptionPlan{})
	clear(&InviteCommissionLedger{})
	clear(&InviteCommissionDailyCapState{})
	clear(&User{})
	_ = getSubscriptionPlanCache().Purge()
	_ = getSubscriptionPlanInfoCache().Purge()
}

func createSubscriptionPlanForInviteCommissionTest(t *testing.T, title string, priceAmount float64, totalAmount int64) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Title:         title,
		Subtitle:      "",
		PriceAmount:   priceAmount,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   totalAmount,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func createSubscriptionOrderForInviteCommissionTest(t *testing.T, userID, planID int, tradeNo string, money float64, purchaseMode string, renewTargetSubID int) *SubscriptionOrder {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:                    userID,
		PlanId:                    planID,
		Money:                     money,
		TradeNo:                   tradeNo,
		PaymentMethod:             "epay",
		PurchaseMode:              purchaseMode,
		RenewTargetSubscriptionId: renewTargetSubID,
		CreateTime:                common.GetTimestamp(),
		Status:                    common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)
	return order
}

func requirePendingSubscriptionIssuanceBySourceRef(t *testing.T, sourceType string, sourceRef string) *SubscriptionIssuance {
	t.Helper()
	var issuance SubscriptionIssuance
	require.NoError(t, DB.Where("source_type = ? AND source_ref = ?", sourceType, sourceRef).First(&issuance).Error)
	require.Equal(t, SubscriptionIssuanceStatusPending, issuance.Status)
	return &issuance
}

func requireIssuedSubscriptionIssuanceBySourceRef(t *testing.T, sourceType string, sourceRef string) *SubscriptionIssuance {
	t.Helper()
	var issuance SubscriptionIssuance
	require.NoError(t, DB.Where("source_type = ? AND source_ref = ?", sourceType, sourceRef).First(&issuance).Error)
	require.Equal(t, SubscriptionIssuanceStatusIssued, issuance.Status)
	return &issuance
}

func TestConfirmSubscriptionIssuanceTx_StackDoesNotPolluteIssuanceSaveStatement(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "issue_tx_clean_stack", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "直接发放月卡", 88, 1000)
	issuance := &SubscriptionIssuance{
		UserId:           user.Id,
		PlanId:           plan.Id,
		PlanTitle:        plan.Title,
		SourceType:       SubscriptionIssuanceSourceOrder,
		SourceRef:        "issue_tx_clean_stack_001",
		Status:           SubscriptionIssuanceStatusPending,
		PurchaseMode:     SubscriptionPurchaseModeStack,
		PurchaseQuantity: 1,
	}
	require.NoError(t, DB.Create(issuance).Error)

	var sqlLogs bytes.Buffer
	originDB := DB
	originLogDB := LOG_DB
	testLogger := gormlogger.New(log.New(&sqlLogs, "", 0), gormlogger.Config{
		LogLevel: gormlogger.Info,
		Colorful: false,
	})
	DB = DB.Session(&gorm.Session{Logger: testLogger})
	LOG_DB = LOG_DB.Session(&gorm.Session{Logger: testLogger})
	t.Cleanup(func() {
		DB = originDB
		LOG_DB = originLogDB
	})

	err := DB.Transaction(func(tx *gorm.DB) error {
		sqlLogs.Reset()
		_, _, err := ConfirmSubscriptionIssuanceTx(tx, issuance.Id, user.Id, SubscriptionPurchaseModeStack, 0)
		return err
	})
	require.NoError(t, err)

	logText := sqlLogs.String()
	assert.False(t, strings.Contains(logText, "INSERT INTO `subscription_plans`"), logText)
	assert.False(t, strings.Contains(logText, "UPDATE `subscription_plans`"), logText)

	var refreshedIssuance SubscriptionIssuance
	require.NoError(t, DB.First(&refreshedIssuance, issuance.Id).Error)
	assert.Equal(t, SubscriptionIssuanceStatusIssued, refreshedIssuance.Status)

	var subCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 1, subCount)
}

func TestConfirmSubscriptionIssuanceTx_StackUsesCleanUserQueriesForUpgradeGroup(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "issue_tx_clean_user_query", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "升级分组月卡", 88, 1000)
	plan.UpgradeGroup = "vip"
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Update("upgrade_group", plan.UpgradeGroup).Error)

	issuance := &SubscriptionIssuance{
		UserId:           user.Id,
		PlanId:           plan.Id,
		PlanTitle:        plan.Title,
		SourceType:       SubscriptionIssuanceSourceOrder,
		SourceRef:        "issue_tx_clean_user_query_001",
		Status:           SubscriptionIssuanceStatusPending,
		PurchaseMode:     SubscriptionPurchaseModeStack,
		PurchaseQuantity: 1,
	}
	require.NoError(t, DB.Create(issuance).Error)

	var sqlLogs bytes.Buffer
	originDB := DB
	originLogDB := LOG_DB
	testLogger := gormlogger.New(log.New(&sqlLogs, "", 0), gormlogger.Config{
		LogLevel: gormlogger.Info,
		Colorful: false,
	})
	DB = DB.Session(&gorm.Session{Logger: testLogger})
	LOG_DB = LOG_DB.Session(&gorm.Session{Logger: testLogger})
	t.Cleanup(func() {
		DB = originDB
		LOG_DB = originLogDB
	})

	err := DB.Transaction(func(tx *gorm.DB) error {
		sqlLogs.Reset()
		_, _, err := ConfirmSubscriptionIssuanceTx(tx, issuance.Id, user.Id, SubscriptionPurchaseModeStack, 0)
		return err
	})
	require.NoError(t, err)

	userSQLLines := make([]string, 0)
	for _, line := range strings.Split(sqlLogs.String(), "\n") {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, " from `users`") ||
			strings.Contains(lowerLine, " update `users`") ||
			strings.Contains(lowerLine, " into `users`") ||
			strings.Contains(lowerLine, " from users ") ||
			strings.Contains(lowerLine, " update users ") ||
			strings.Contains(lowerLine, " into users ") {
			userSQLLines = append(userSQLLines, line)
		}
	}
	require.NotEmpty(t, userSQLLines, sqlLogs.String())

	for _, line := range userSQLLines {
		assert.NotContains(t, strings.ToLower(line), "user_id", line)
		assert.LessOrEqual(t, strings.Count(line, fmt.Sprintf("id = %d", user.Id)), 1, line)
	}
}

func TestCompleteSubscriptionOrder_AutoIssueUsesCleanUserQueriesForUpgradeGroup(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "sub_order_clean_user_query", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "自动发放升级月卡", 88, 1000)
	plan.UpgradeGroup = "vip"
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Update("upgrade_group", plan.UpgradeGroup).Error)

	tradeNo := "sub_order_clean_user_query_001"
	createSubscriptionOrderForInviteCommissionTest(t, user.Id, plan.Id, tradeNo, 88, SubscriptionPurchaseModeStack, 0)

	var sqlLogs bytes.Buffer
	originDB := DB
	originLogDB := LOG_DB
	testLogger := gormlogger.New(log.New(&sqlLogs, "", 0), gormlogger.Config{
		LogLevel: gormlogger.Info,
		Colorful: false,
	})
	DB = DB.Session(&gorm.Session{Logger: testLogger})
	LOG_DB = LOG_DB.Session(&gorm.Session{Logger: testLogger})
	t.Cleanup(func() {
		DB = originDB
		LOG_DB = originLogDB
	})

	sqlLogs.Reset()
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	userSQLLines := make([]string, 0)
	for _, line := range strings.Split(sqlLogs.String(), "\n") {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, " from `users`") ||
			strings.Contains(lowerLine, " update `users`") ||
			strings.Contains(lowerLine, " into `users`") ||
			strings.Contains(lowerLine, " from users ") ||
			strings.Contains(lowerLine, " update users ") ||
			strings.Contains(lowerLine, " into users ") {
			userSQLLines = append(userSQLLines, line)
		}
	}
	require.NotEmpty(t, userSQLLines, sqlLogs.String())

	for _, line := range userSQLLines {
		assert.NotContains(t, strings.ToLower(line), "user_id", line)
		assert.LessOrEqual(t, strings.Count(line, fmt.Sprintf("id = %d", user.Id)), 1, line)
	}
}

func TestCompleteSubscriptionOrder_EnqueueInviteCommissionByPaidAmount_StackAndIdempotent(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originRate := common.InviterRechargeCommissionRate
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originRate
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
	})
	common.InviterCommissionEnabled = true
	common.InviterRechargeCommissionRate = 0.1
	common.QuotaPerUnit = 1000
	operation_setting.Price = 8

	inviter := createInviteCommissionTestUser(t, "inviter_sub_stack", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_sub_stack", inviter.Id)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "大月卡", 88, 3000000000)

	tradeNo := "sub_invite_stack_001"
	createSubscriptionOrderForInviteCommissionTest(t, invitee.Id, plan.Id, tradeNo, 88, SubscriptionPurchaseModeStack, 0)

	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))
	// 重复回调幂等：不应重复入返佣台账。
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	var ledger InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ? AND inviter_user_id = ?", tradeNo, inviter.Id).First(&ledger).Error)

	expectedBaseQuota := 11000 // floor(88 * 1000 / 8)
	assert.Equal(t, int64(expectedBaseQuota), ledger.BaseQuota)
	assert.Equal(t, int64(1100), ledger.CommissionQuota)
	assert.Equal(t, InviteCommissionStatusPending, ledger.Status)
	assert.NotEqual(t, int(plan.TotalAmount), ledger.BaseQuota, "返佣基数必须来自实付金额折算，不能按套餐总额度")

	var count int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Where("topup_trade_no = ? AND inviter_user_id = ?", tradeNo, inviter.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	issuance := requireIssuedSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, SubscriptionPurchaseModeStack, issuance.PurchaseMode)
	assert.Equal(t, 1, issuance.PurchaseQuantity)

	var subCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", invitee.Id, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 1, subCount)
}

func TestCompleteSubscriptionOrder_EnqueueInviteCommissionTwoLevels(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originFirstRate := common.InviterRechargeCommissionRate
	originSecondRate := common.InviterRechargeSecondLevelCommissionRate
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originFirstRate
		common.InviterRechargeSecondLevelCommissionRate = originSecondRate
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
	})
	common.InviterCommissionEnabled = true
	common.InviterRechargeCommissionRate = 0.1
	common.InviterRechargeSecondLevelCommissionRate = 0.05
	common.QuotaPerUnit = 1000
	operation_setting.Price = 8

	grandparent := createInviteCommissionTestUser(t, "grandparent_sub_two_level", 0)
	parent := createInviteCommissionTestUser(t, "parent_sub_two_level", grandparent.Id)
	invitee := createInviteCommissionTestUser(t, "invitee_sub_two_level", parent.Id)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "两级返佣月卡", 88, 3000000000)

	tradeNo := "sub_invite_two_level_001"
	createSubscriptionOrderForInviteCommissionTest(t, invitee.Id, plan.Id, tradeNo, 88, SubscriptionPurchaseModeStack, 0)
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	var ledgers []*InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ?", tradeNo).Order("commission_level asc").Find(&ledgers).Error)
	require.Len(t, ledgers, 2)
	assert.Equal(t, parent.Id, ledgers[0].InviterUserId)
	assert.Equal(t, int64(1100), ledgers[0].CommissionQuota)
	assert.Equal(t, grandparent.Id, ledgers[1].InviterUserId)
	assert.Equal(t, int64(550), ledgers[1].CommissionQuota)
}

func TestCompleteSubscriptionOrder_EnqueueInviteCommission_Renew(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originRate := common.InviterRechargeCommissionRate
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originRate
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
	})
	common.InviterCommissionEnabled = true
	common.InviterRechargeCommissionRate = 0.1
	common.QuotaPerUnit = 1000
	operation_setting.Price = 8

	inviter := createInviteCommissionTestUser(t, "inviter_sub_renew", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_sub_renew", inviter.Id)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "续费月卡", 88, 1000)

	now := time.Now().Unix()
	existingSub := &UserSubscription{
		UserId:      invitee.Id,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 86400,
		Status:      "active",
		Source:      "order",
	}
	require.NoError(t, DB.Create(existingSub).Error)

	tradeNo := "sub_invite_renew_001"
	createSubscriptionOrderForInviteCommissionTest(t, invitee.Id, plan.Id, tradeNo, 88, SubscriptionPurchaseModeRenew, existingSub.Id)
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	var ledger InviteCommissionLedger
	require.NoError(t, DB.Where("topup_trade_no = ? AND inviter_user_id = ?", tradeNo, inviter.Id).First(&ledger).Error)
	assert.Equal(t, int64(11000), ledger.BaseQuota)
	assert.Equal(t, int64(1100), ledger.CommissionQuota)
	assert.Equal(t, InviteCommissionStatusPending, ledger.Status)

	issuance := requireIssuedSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, SubscriptionPurchaseModeRenew, issuance.PurchaseMode)
	assert.Equal(t, existingSub.Id, issuance.RenewTargetSubscriptionId)

	var refreshedSub UserSubscription
	require.NoError(t, DB.First(&refreshedSub, "id = ?", existingSub.Id).Error)
	assert.Greater(t, refreshedSub.EndTime, existingSub.EndTime)
}

func TestCompleteSubscriptionOrder_InvalidPrice_DoesNotRollbackCorePaymentState(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originRate := common.InviterRechargeCommissionRate
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originRate
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
	})
	common.InviterCommissionEnabled = true
	common.InviterRechargeCommissionRate = 0.1
	common.QuotaPerUnit = 1000
	operation_setting.Price = 0

	inviter := createInviteCommissionTestUser(t, "inviter_sub_invalid_price", 0)
	invitee := createInviteCommissionTestUser(t, "invitee_sub_invalid_price", inviter.Id)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "异常价格套餐", 88, 1000)

	tradeNo := "sub_invite_invalid_price_001"
	createSubscriptionOrderForInviteCommissionTest(t, invitee.Id, plan.Id, tradeNo, 88, SubscriptionPurchaseModeStack, 0)

	err := CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`)
	require.NoError(t, err)

	order := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.NotZero(t, order.CompleteTime)

	var ledgerCount int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Where("topup_trade_no = ?", tradeNo).Count(&ledgerCount).Error)
	assert.EqualValues(t, 0, ledgerCount)

	var topup TopUp
	require.NoError(t, DB.Where("trade_no = ?", tradeNo).First(&topup).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topup.Status)
	assert.Equal(t, order.Money, topup.Money)

	issuance := requireIssuedSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, SubscriptionPurchaseModeStack, issuance.PurchaseMode)
	assert.Equal(t, 1, issuance.PurchaseQuantity)

	var subCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", invitee.Id, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 1, subCount)
}

func TestCompleteSubscriptionOrder_AutoIssueFailure_PreservesCorePaymentState(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	originEnabled := common.InviterCommissionEnabled
	originRate := common.InviterRechargeCommissionRate
	originQuotaPerUnit := common.QuotaPerUnit
	originPrice := operation_setting.Price
	t.Cleanup(func() {
		common.InviterCommissionEnabled = originEnabled
		common.InviterRechargeCommissionRate = originRate
		common.QuotaPerUnit = originQuotaPerUnit
		operation_setting.Price = originPrice
	})
	common.InviterCommissionEnabled = false
	common.InviterRechargeCommissionRate = 0
	common.QuotaPerUnit = 1000
	operation_setting.Price = 8

	user := createInviteCommissionTestUser(t, "invitee_sub_auto_issue_fail", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "异常时长套餐", 88, 1000)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Update("duration_value", 0).Error)
	_ = getSubscriptionPlanCache().Purge()
	_ = getSubscriptionPlanInfoCache().Purge()

	tradeNo := "sub_auto_issue_fail_001"
	createSubscriptionOrderForInviteCommissionTest(t, user.Id, plan.Id, tradeNo, 88, SubscriptionPurchaseModeStack, 0)

	err := CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`)
	require.NoError(t, err)

	order := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.NotZero(t, order.CompleteTime)

	var topup TopUp
	require.NoError(t, DB.Where("trade_no = ?", tradeNo).First(&topup).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topup.Status)

	issuance := requirePendingSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Contains(t, issuance.IssueSummary, "duration_value must be > 0")
	assert.EqualValues(t, 0, issuance.IssuedTime)

	var subCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 0, subCount)
}

func TestCompleteSubscriptionOrder_RenewQuantityUseSameTargetSubscription(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "invitee_sub_renew_multi_target", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "多份续费定向月卡", 88, 1000)

	now := time.Now().Unix()
	earliestSub := &UserSubscription{
		UserId:      user.Id,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 24*3600,
		Status:      "active",
		Source:      "order",
	}
	require.NoError(t, DB.Create(earliestSub).Error)
	targetSub := &UserSubscription{
		UserId:      user.Id,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  0,
		StartTime:   now - 1800,
		EndTime:     now + 2*24*3600,
		Status:      "active",
		Source:      "order",
	}
	require.NoError(t, DB.Create(targetSub).Error)

	targetOldEndTime := targetSub.EndTime
	earliestOldEndTime := earliestSub.EndTime

	tradeNo := "sub_renew_target_multi_quantity_001"
	order := &SubscriptionOrder{
		UserId:                    user.Id,
		PlanId:                    plan.Id,
		Money:                     176,
		PurchaseQuantity:          2,
		TradeNo:                   tradeNo,
		PaymentMethod:             "epay",
		PurchaseMode:              SubscriptionPurchaseModeRenew,
		RenewTargetSubscriptionId: targetSub.Id,
		CreateTime:                common.GetTimestamp(),
		Status:                    common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	issuance := requireIssuedSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, user.Id, issuance.UserId)
	assert.Equal(t, plan.Id, issuance.PlanId)
	assert.Equal(t, plan.Title, issuance.PlanTitle)
	assert.Equal(t, SubscriptionPurchaseModeRenew, issuance.PurchaseMode)
	assert.Equal(t, 2, issuance.PurchaseQuantity)
	assert.Equal(t, targetSub.Id, issuance.RenewTargetSubscriptionId)

	require.NoError(t, DB.First(targetSub, "id = ?", targetSub.Id).Error)
	require.NoError(t, DB.First(earliestSub, "id = ?", earliestSub.Id).Error)
	assert.Greater(t, targetSub.EndTime, targetOldEndTime)
	assert.Equal(t, earliestOldEndTime, earliestSub.EndTime)
}

func TestCompleteSubscriptionOrder_RenewWithoutTargetWithMultipleActiveFallbackToPendingIssuance(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "invitee_sub_renew_fallback_earliest", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "默认最早到期续费月卡", 88, 1000)

	now := time.Now().Unix()
	earliestSub := &UserSubscription{
		UserId:      user.Id,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 24*3600,
		Status:      "active",
		Source:      "order",
	}
	require.NoError(t, DB.Create(earliestSub).Error)
	laterSub := &UserSubscription{
		UserId:      user.Id,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  0,
		StartTime:   now - 1800,
		EndTime:     now + 3*24*3600,
		Status:      "active",
		Source:      "order",
	}
	require.NoError(t, DB.Create(laterSub).Error)

	earliestOldEndTime := earliestSub.EndTime
	laterOldEndTime := laterSub.EndTime

	tradeNo := "sub_renew_fallback_earliest_001"
	order := &SubscriptionOrder{
		UserId:                    user.Id,
		PlanId:                    plan.Id,
		Money:                     88,
		PurchaseQuantity:          1,
		TradeNo:                   tradeNo,
		PaymentMethod:             "epay",
		PurchaseMode:              SubscriptionPurchaseModeRenew,
		RenewTargetSubscriptionId: 0,
		CreateTime:                common.GetTimestamp(),
		Status:                    common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	issuance := requirePendingSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, user.Id, issuance.UserId)
	assert.Equal(t, plan.Id, issuance.PlanId)
	assert.Equal(t, SubscriptionPurchaseModeRenew, issuance.PurchaseMode)
	assert.Equal(t, 1, issuance.PurchaseQuantity)
	assert.Equal(t, 0, issuance.RenewTargetSubscriptionId)

	// 多个候选但下单时未锁定目标，自动发放应回退为待用户确认。
	require.NoError(t, DB.First(earliestSub, "id = ?", earliestSub.Id).Error)
	require.NoError(t, DB.First(laterSub, "id = ?", laterSub.Id).Error)
	assert.Equal(t, earliestOldEndTime, earliestSub.EndTime)
	assert.Equal(t, laterOldEndTime, laterSub.EndTime)

	orderByTradeNo := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, orderByTradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, orderByTradeNo.Status)
}

func TestCompleteSubscriptionOrder_StackQuantityWithoutActive_CreateMultipleSubscriptions(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "invitee_sub_stack_multi_create", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "新购叠加多条月卡", 88, 1000)

	tradeNo := "sub_stack_multi_create_001"
	order := &SubscriptionOrder{
		UserId:                    user.Id,
		PlanId:                    plan.Id,
		Money:                     176,
		PurchaseQuantity:          2,
		TradeNo:                   tradeNo,
		PaymentMethod:             "epay",
		PurchaseMode:              SubscriptionPurchaseModeStack,
		RenewTargetSubscriptionId: 0,
		CreateTime:                common.GetTimestamp(),
		Status:                    common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	issuance := requireIssuedSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, user.Id, issuance.UserId)
	assert.Equal(t, plan.Id, issuance.PlanId)
	assert.Equal(t, SubscriptionPurchaseModeStack, issuance.PurchaseMode)
	assert.Equal(t, 2, issuance.PurchaseQuantity)
	assert.Equal(t, 0, issuance.RenewTargetSubscriptionId)

	var subCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 2, subCount)
}

func TestCompleteSubscriptionOrder_RenewExtendWithoutActive_ExtendSingleSubscription(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "invitee_sub_renew_extend_single", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "续费式顺延月卡", 88, 1000)

	tradeNo := "sub_renew_extend_single_001"
	order := &SubscriptionOrder{
		UserId:                    user.Id,
		PlanId:                    plan.Id,
		Money:                     176,
		PurchaseQuantity:          2,
		TradeNo:                   tradeNo,
		PaymentMethod:             "epay",
		PurchaseMode:              SubscriptionPurchaseModeRenewExtend,
		RenewTargetSubscriptionId: 0,
		CreateTime:                common.GetTimestamp(),
		Status:                    common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	issuance := requireIssuedSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, user.Id, issuance.UserId)
	assert.Equal(t, plan.Id, issuance.PlanId)
	assert.Equal(t, SubscriptionPurchaseModeRenewExtend, issuance.PurchaseMode)
	assert.Equal(t, 2, issuance.PurchaseQuantity)
	assert.Equal(t, 0, issuance.RenewTargetSubscriptionId)

	var subCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 1, subCount)
}

func TestCompleteSubscriptionOrder_RenewInvalidTargetFallbackToPendingIssuance(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "invitee_sub_renew_invalid_target", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "续费失效目标月卡", 88, 1000)

	now := time.Now().Unix()
	activeSub := &UserSubscription{
		UserId:      user.Id,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 24*3600,
		Status:      "active",
		Source:      "order",
	}
	require.NoError(t, DB.Create(activeSub).Error)

	tradeNo := "sub_renew_invalid_target_001"
	order := &SubscriptionOrder{
		UserId:                    user.Id,
		PlanId:                    plan.Id,
		Money:                     88,
		PurchaseQuantity:          1,
		TradeNo:                   tradeNo,
		PaymentMethod:             "epay",
		PurchaseMode:              SubscriptionPurchaseModeRenew,
		RenewTargetSubscriptionId: activeSub.Id + 9999,
		CreateTime:                common.GetTimestamp(),
		Status:                    common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)

	originalEndTime := activeSub.EndTime
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	issuance := requirePendingSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, SubscriptionPurchaseModeRenew, issuance.PurchaseMode)
	assert.Equal(t, activeSub.Id+9999, issuance.RenewTargetSubscriptionId)

	var refreshedSub UserSubscription
	require.NoError(t, DB.First(&refreshedSub, "id = ?", activeSub.Id).Error)
	assert.Equal(t, originalEndTime, refreshedSub.EndTime)

	orderByTradeNo := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, orderByTradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, orderByTradeNo.Status)
}

func TestCompleteSubscriptionOrder_RenewWithoutActiveFallbackToPendingIssuance(t *testing.T) {
	setupInviteCommissionSubscriptionTest(t)

	user := createInviteCommissionTestUser(t, "invitee_sub_renew_no_active", 0)
	plan := createSubscriptionPlanForInviteCommissionTest(t, "续费无生效月卡", 88, 1000)

	tradeNo := "sub_renew_no_active_001"
	order := &SubscriptionOrder{
		UserId:                    user.Id,
		PlanId:                    plan.Id,
		Money:                     88,
		PurchaseQuantity:          1,
		TradeNo:                   tradeNo,
		PaymentMethod:             "epay",
		PurchaseMode:              SubscriptionPurchaseModeRenew,
		RenewTargetSubscriptionId: 0,
		CreateTime:                common.GetTimestamp(),
		Status:                    common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)
	require.NoError(t, CompleteSubscriptionOrder(tradeNo, `{"status":"success"}`))

	issuance := requirePendingSubscriptionIssuanceBySourceRef(t, SubscriptionIssuanceSourceOrder, tradeNo)
	assert.Equal(t, SubscriptionPurchaseModeRenew, issuance.PurchaseMode)

	var subCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 0, subCount)

	orderByTradeNo := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, orderByTradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, orderByTradeNo.Status)
}
