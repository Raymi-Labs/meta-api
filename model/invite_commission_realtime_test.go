package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInviteRealtimePaymentAtomicity(t *testing.T) {
	for _, provider := range []string{"epay", "manual", "stripe", "creem", "subscription"} {
		t.Run(provider, func(t *testing.T) {
			setupInviteCommissionTopUpTest(t)
			inviter := createInviteCommissionTestUser(t, "atomic_inviter", 0)
			invitee := createInviteCommissionTestUser(t, "atomic_invitee", inviter.Id)
			trade := "atomic_" + provider
			var topUp *TopUp
			if provider == "subscription" {
				plan := createSubscriptionPlanForInviteCommissionTest(t, "atomic plan", 88, 1000)
				createSubscriptionOrderForInviteCommissionTest(t, invitee.Id, plan.Id, trade, 88, SubscriptionPurchaseModeStack, 0)
			} else {
				topUp = createInviteCommissionTopUp(t, invitee.Id, trade, 100, 100, 100, common.TopUpStatusPending)
				method := provider
				if provider == "manual" {
					method = "alipay"
				}
				require.NoError(t, DB.Model(topUp).Updates(map[string]interface{}{"payment_method": method, "payment_provider": InferPaymentProvider("", method), "paid_currency": "CNY"}).Error)
			}
			complete := func() error {
				switch provider {
				case "stripe":
					return RechargeWithPaymentDetails(trade, "", "", TopUpPaymentDetails{PaidMoney: 100, PaidCurrency: "CNY"})
				case "creem":
					return RechargeCreem(trade, "", "", "")
				case "subscription":
					return CompleteSubscriptionOrder(trade, "")
				default:
					return CompleteTopUpByTradeNo(trade, provider, "", nil)
				}
			}
			// A failed durable write cannot leave a successful local payment without its job.
			require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("fail_commission", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "invite_commission_ledgers" {
					tx.AddError(errors.New("injected durable write failure"))
				}
			}))
			require.Error(t, complete())
			var count int64
			require.NoError(t, DB.Model(&InviteCommissionLedger{}).Count(&count).Error)
			require.Zero(t, count)
			if provider == "subscription" {
				var order SubscriptionOrder
				require.NoError(t, DB.Where("trade_no = ?", trade).First(&order).Error)
				require.Equal(t, common.TopUpStatusPending, order.Status)
			} else {
				require.NoError(t, DB.First(topUp, topUp.Id).Error)
				require.Equal(t, common.TopUpStatusPending, topUp.Status)
				var user User
				require.NoError(t, DB.First(&user, invitee.Id).Error)
				require.Zero(t, user.Quota)
			}
			require.NoError(t, DB.Callback().Create().Remove("fail_commission"))
			require.NoError(t, complete())
			require.NoError(t, complete())
			var ledger InviteCommissionLedger
			require.NoError(t, DB.First(&ledger).Error)
			require.Equal(t, InviteCommissionStatusPending, ledger.Status)
			require.Equal(t, 1, ledger.AmountVersion)
			require.NoError(t, DB.Model(&InviteCommissionLedger{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
			settled, _, _, err := SettleInviteCommissionByBizDate(time.Now().Format("2006-01-02"), 300)
			require.NoError(t, err)
			require.Equal(t, 1, settled)
		})
	}
}

func TestInviteRealtimeSnapshotAndRetryRecovery(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	inviter := createInviteCommissionTestUser(t, "retry_inviter", 0)
	invitee := createInviteCommissionTestUser(t, "retry_invitee", inviter.Id)
	for i := 0; i < 2; i++ {
		order := createInviteCommissionTopUp(t, invitee.Id, fmt.Sprintf("retry_%d", i), 100, 100, 100, common.TopUpStatusSuccess)
		require.NoError(t, EnqueueInviteCommissionFromTopUp(order))
	}
	var ledgers []InviteCommissionLedger
	require.NoError(t, DB.Order("id").Find(&ledgers).Error)
	operation_setting.Price = 10 // already accepted commissions must not shrink on retry
	common.InviterRechargeCommissionRate = 0.5
	// Fail only the first ledger update; the second order must still settle.
	fail := true
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("fail_first_ledger", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "invite_commission_ledgers" {
			updates, ok := tx.Statement.Dest.(map[string]interface{})
			if ok && updates["status"] == InviteCommissionStatusSettled && fail {
				fail = false
				tx.AddError(errors.New("injected payout failure"))
			}
		}
	}))
	settled, skipped, attempted, err := SettleInviteCommissionByBizDate("2099-01-01", 300)
	require.NoError(t, err)
	require.Equal(t, 1, settled)
	require.Zero(t, skipped)
	require.Equal(t, 2, attempted)
	require.NoError(t, DB.First(&ledgers[0], ledgers[0].Id).Error)
	require.Equal(t, InviteCommissionStatusPending, ledgers[0].Status)
	require.Equal(t, 1, ledgers[0].RetryCount)
	require.GreaterOrEqual(t, ledgers[0].NextRetryAt, common.GetTimestamp()+29)
	require.NoError(t, DB.Callback().Update().Remove("fail_first_ledger"))
	settled, _, _, err = SettleInviteCommissionByBizDate("2099-01-01", 300)
	require.NoError(t, err)
	require.Zero(t, settled)
	require.NoError(t, DB.Model(&ledgers[0]).Update("next_retry_at", 0).Error)
	settled, _, _, err = SettleInviteCommissionByBizDate("2099-01-01", 300)
	require.NoError(t, err)
	require.Equal(t, 1, settled)
	settled, _, _, err = SettleInviteCommissionByBizDate("2099-01-01", 300)
	require.NoError(t, err)
	require.Zero(t, settled)
	var user User
	require.NoError(t, DB.First(&user, inviter.Id).Error)
	require.Equal(t, 20000, user.AffQuota)
	require.NoError(t, DB.First(&ledgers[0], ledgers[0].Id).Error)
	require.Zero(t, ledgers[0].NextRetryAt)
}

func TestInviteRealtimeConcurrentSettlementAndReversal(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupInviteCommissionTopUpTest(t)
			if dialect != "sqlite" {
				db := capabilitySyncTestDB(t, dialect)
				require.NoError(t, db.AutoMigrate(&SubscriptionOrder{}, &User{}, &TopUp{}, &InviteCommissionLedger{}, &InviteCommissionDailyCapState{}, &Log{}, &PulseFundingLedger{}, &PulseWalletReservation{}))
				LOG_DB = db
				common.UsingSQLite = false
				common.UsingMySQL = dialect == "mysql"
				common.UsingPostgreSQL = dialect == "postgres"
				initCol()
			}
			// Exercise additive migration with a legacy terminal row in every engine.
			legacy := &InviteCommissionLedger{TopupTradeNo: "legacy_terminal", InviterUserId: 999, Status: InviteCommissionStatusSkipped, BizDate: "2026-01-01", CommissionQuota: 25}
			require.NoError(t, DB.Create(legacy).Error)
			for _, field := range []string{"AmountVersion", "RetryCount", "NextRetryAt"} {
				require.NoError(t, DB.Migrator().DropColumn(&InviteCommissionLedger{}, field))
			}
			require.NoError(t, DB.AutoMigrate(&InviteCommissionLedger{}))
			require.NoError(t, DB.First(legacy, legacy.Id).Error)
			require.Zero(t, legacy.AmountVersion)
			require.Zero(t, legacy.NextRetryAt)
			require.Equal(t, 25, legacy.CommissionQuota)
			oldCap := common.InviterCommissionDailyCap
			common.InviterCommissionDailyCap = 15000
			t.Cleanup(func() { common.InviterCommissionDailyCap = oldCap })
			inviter := createInviteCommissionTestUser(t, "race_inviter", 0)
			invitee := createInviteCommissionTestUser(t, "race_invitee", inviter.Id)
			orders := make([]*TopUp, 2)
			for i := range orders {
				orders[i] = createInviteCommissionTopUp(t, invitee.Id, fmt.Sprintf("concurrent_%d", i), 100, 100, 100, common.TopUpStatusSuccess)
				require.NoError(t, EnqueueInviteCommissionFromTopUp(orders[i]))
			}
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _, _, err := SettleInviteCommissionByBizDate("2099-01-01", 300)
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}
			var user User
			require.NoError(t, DB.First(&user, inviter.Id).Error)
			require.Equal(t, 15000, user.AffQuota)
			var cap InviteCommissionDailyCapState
			require.NoError(t, DB.First(&cap).Error)
			require.Equal(t, 15000, cap.SettledQuota)
			// Both orderings must end with no payable commission for a reversed source.
			for i := 0; i < 12; i++ {
				order := createInviteCommissionTopUp(t, invitee.Id, fmt.Sprintf("reversal_%d", i), 100, 100, 100, common.TopUpStatusSuccess)
				require.NoError(t, EnqueueInviteCommissionFromTopUp(order))
				var ledger InviteCommissionLedger
				require.NoError(t, DB.Where("topup_trade_no = ?", order.TradeNo).First(&ledger).Error)
				sourceType := BenefitSourceTopUpOrder
				if i%2 == 1 {
					sourceType = BenefitSourceSubscriptionOrder
					require.NoError(t, DB.Create(&SubscriptionOrder{TradeNo: order.TradeNo, UserId: invitee.Id, Money: 100, Status: common.TopUpStatusSuccess}).Error)
				}
				result := make(chan error, 2)
				go func() { _, _, err := settleSingleInviteCommissionLedger(&ledger, 0); result <- err }()
				go func() {
					result <- DB.Transaction(func(tx *gorm.DB) error {
						if err := lockPaymentSourceForRollbackTx(tx, sourceType, order.TradeNo); err != nil {
							return err
						}
						if _, err := reverseInviteCommissionsByTradeNoTx(tx, order.TradeNo); err != nil {
							return err
						}
						return markReversedPaymentSourceTx(tx, sourceType, order.TradeNo)
					})
				}()
				require.NoError(t, <-result)
				require.NoError(t, <-result)
				require.NoError(t, DB.First(&ledger, ledger.Id).Error)
				require.Equal(t, InviteCommissionStatusSkipped, ledger.Status)
				require.NoError(t, DB.First(&user, inviter.Id).Error)
				require.Equal(t, 15000, user.AffQuota)
			}
		})
	}
}

func TestInviteRealtimeRewardCanBeUsedImmediately(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	require.NoError(t, DB.AutoMigrate(&AffWithdrawal{}))
	inviter := createInviteCommissionTestUser(t, "use_inviter", 0)
	invitee := createInviteCommissionTestUser(t, "use_invitee", inviter.Id)
	order := createInviteCommissionTopUp(t, invitee.Id, "usable_today", 1000, 1000, 1000, common.TopUpStatusSuccess)
	require.NoError(t, EnqueueInviteCommissionFromTopUp(order))
	_, _, _, err := SettleInviteCommissionByBizDate(time.Now().Format("2006-01-02"), 300)
	require.NoError(t, err)
	items, _, summary, err := GetUserInviteRechargeCommissions(inviter.Id, 0, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 100000, summary.RechargeTotalQuota)
	require.NoError(t, inviter.TransferAffQuotaToQuota(1000))
	withdrawal, err := CreateAffWithdrawal(inviter.Id, 50000, "test@example.invalid", "fixture")
	require.NoError(t, err)
	require.Equal(t, AffWithdrawalStatusPending, withdrawal.Status)
	require.NoError(t, DB.First(inviter, inviter.Id).Error)
	require.Equal(t, 49000, inviter.AffQuota)
	require.Equal(t, 1000, inviter.Quota)
}

func TestInviteRealtimeLegacySubscriptionWithoutPaymentMirror(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	order := &SubscriptionOrder{TradeNo: "legacy_subscription_no_mirror", UserId: 1, Status: common.TopUpStatusSuccess}
	require.NoError(t, DB.Create(order).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		if err := lockPaymentSourceForRollbackTx(tx, BenefitSourceSubscriptionOrder, order.TradeNo); err != nil {
			return err
		}
		return markReversedPaymentSourceTx(tx, BenefitSourceSubscriptionOrder, order.TradeNo)
	}))
	require.NoError(t, DB.First(order, order.Id).Error)
	require.Equal(t, common.TopUpStatusFailed, order.Status)
}
