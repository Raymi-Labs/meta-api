package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// These opt-in DSNs MUST point to disposable test databases: this test rebuilds
// the listed tables. Unset DSNs skip all external database work; ordinary tests
// retain their existing isolated SQLite fixtures.
func TestPulseBenefitExternalDatabaseSafety(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			env := "TEST_PULSE_BENEFIT_MYSQL_DSN"
			if dialect == "postgres" {
				env = "TEST_PULSE_BENEFIT_POSTGRES_DSN"
			}
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skip("disposable database not configured: " + env)
			}
			var driver gorm.Dialector = mysql.Open(dsn)
			if dialect == "postgres" {
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(16)
			pool.SetMaxIdleConns(16)
			previousDB, previousLog := DB, LOG_DB
			previousSQLite, previousMySQL, previousPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
			previousRedis := common.RedisEnabled
			DB, LOG_DB = db, db
			common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = false, dialect == "mysql", dialect == "postgres"
			common.RedisEnabled = false
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLog
				common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = previousSQLite, previousMySQL, previousPostgres
				common.RedisEnabled = previousRedis
				initCol()
				_ = pool.Close()
			})
			t.Setenv("PULSE_BENEFIT_ENABLED", "true")
			t.Setenv("PULSE_BENEFIT_MAX_GRANT_QUOTA", "1000")
			t.Setenv("PULSE_BENEFIT_USER_DAILY_QUOTA", "1000")
			t.Setenv("PULSE_BENEFIT_DAILY_QUOTA", "1000")

			models := []any{&PulseBenefitReceipt{}, &PulseBenefitQuotaCounter{}, &BenefitChangeRecord{}, &PulseBenefitLogReceipt{}, &Log{},
				&BenefitRollbackOperation{}, &PulseFundingLedger{}, &PulseWalletReservation{}, &SubscriptionIssuance{}, &User{}}
			reset := func(t *testing.T) {
				t.Helper()
				require.NoError(t, db.Migrator().DropTable(models...))
				require.NoError(t, db.AutoMigrate(models...))
			}
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(models...)) })

			t.Run("usage_log_concurrent_replay", func(t *testing.T) {
				reset(t)
				user := createPaymentRiskCaseTestUser(t, "receiver-log-replay")
				request := pulseTestRequest("receiver-log-replay", user.Id, 10)
				requests := make([]PulseBenefitGrantRequest, 20)
				for i := range requests {
					requests[i] = request
				}
				for _, result := range runConcurrentPulseGrants(requests) {
					require.NoError(t, result.err)
				}
				_, err := RollbackPulseBenefit(request.SourceRef, "test")
				require.NoError(t, err)
				require.NoError(t, SyncPulseBenefitLogs(""))
				logs, total, err := GetUserLogs(user.Id, LogTypeSystem, 0, 0, "", "", 0, 10, "", "")
				require.NoError(t, err)
				require.EqualValues(t, 2, total)
				require.Equal(t, []int{-10, 10}, []int{logs[0].Quota, logs[1].Quota})
				// Simulate an old schema and verify pending defaults/backfill dedup.
				require.NoError(t, db.Migrator().DropIndex(&BenefitChangeRecord{}, "idx_benefit_pulse_log"))
				require.NoError(t, db.Migrator().DropColumn(&BenefitChangeRecord{}, "PulseLogSynced"))
				require.NoError(t, db.AutoMigrate(&BenefitChangeRecord{}))
				// A real upgrade starts with fresh connections; discard PostgreSQL
				// prepared SELECT * plans from before this in-process schema change.
				pool.SetMaxIdleConns(0)
				pool.SetMaxIdleConns(16)
				require.NoError(t, SyncPulseBenefitLogs(""))
				var count int64
				require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
				require.EqualValues(t, 2, count)
			})

			t.Run("concurrent_global_and_user_limits", func(t *testing.T) {
				reset(t)
				t.Setenv("PULSE_BENEFIT_USER_DAILY_QUOTA", "40")
				t.Setenv("PULSE_BENEFIT_DAILY_QUOTA", "100")
				users := make([]*User, 4)
				for i := range users {
					users[i] = createPaymentRiskCaseTestUser(t, fmt.Sprintf("receiver-limit-%d", i))
				}
				requests := make([]PulseBenefitGrantRequest, 120)
				for i := range requests {
					requests[i] = pulseTestRequest(fmt.Sprintf("concurrent-limit-%d", i), users[i%len(users)].Id, 2)
				}
				results := runConcurrentPulseGrants(requests)
				successes := 0
				for _, result := range results {
					if result.err == nil {
						successes++
						require.Equal(t, PulseBenefitStatusApplied, result.value.Status)
					} else {
						require.ErrorIs(t, result.err, ErrPulseBenefitLimit)
					}
				}
				require.Equal(t, 50, successes)
				total := 0
				for _, user := range users {
					var refreshed User
					require.NoError(t, db.First(&refreshed, user.Id).Error)
					require.LessOrEqual(t, refreshed.Quota, 40)
					total += refreshed.Quota
				}
				require.Equal(t, 100, total)
				var count int64
				require.NoError(t, db.Model(&PulseBenefitReceipt{}).Count(&count).Error)
				require.EqualValues(t, 50, count)
				require.Greater(t, pool.Stats().OpenConnections, 1, "exercise independent database connections")
			})

			t.Run("same_reference_replay_and_paused_rollback", func(t *testing.T) {
				reset(t)
				user := createPaymentRiskCaseTestUser(t, "receiver-replay")
				request := pulseTestRequest("concurrent-one-reference", user.Id, 20)
				requests := make([]PulseBenefitGrantRequest, 100)
				for i := range requests {
					requests[i] = request
				}
				applied := 0
				for _, result := range runConcurrentPulseGrants(requests) {
					require.NoError(t, result.err)
					require.True(t, result.value.Applied)
					if result.value.Status == PulseBenefitStatusApplied {
						applied++
					} else {
						require.Equal(t, PulseBenefitStatusAlreadyApplied, result.value.Status)
					}
				}
				require.Equal(t, 1, applied)
				var refreshed User
				require.NoError(t, db.First(&refreshed, user.Id).Error)
				require.Equal(t, 20, refreshed.Quota)
				var count int64
				require.NoError(t, db.Model(&BenefitChangeRecord{}).Count(&count).Error)
				require.EqualValues(t, 1, count)
				t.Setenv("PULSE_BENEFIT_ENABLED", "false")
				replay, err := GrantPulseBenefit(request)
				require.NoError(t, err)
				require.Equal(t, PulseBenefitStatusAlreadyApplied, replay.Status)
				_, err = GrantPulseBenefit(pulseTestRequest("paused-new-reference", user.Id, 1))
				require.ErrorIs(t, err, ErrPulseBenefitPaused)
				query, err := QueryPulseBenefit(request.SourceRef)
				require.NoError(t, err)
				require.True(t, query.Applied)
				for i := 0; i < 2; i++ {
					reversal, err := RollbackPulseBenefit(request.SourceRef, "integration reversal")
					require.NoError(t, err)
					require.True(t, reversal.RolledBack)
				}
				require.NoError(t, db.First(&refreshed, user.Id).Error)
				require.Zero(t, refreshed.Quota)
				require.NoError(t, db.Model(&BenefitChangeRecord{}).Where("action = ?", BenefitActionRollback).Count(&count).Error)
				require.EqualValues(t, 1, count)
			})

			t.Run("funding_equal_settlement_and_deleted_recipient", func(t *testing.T) {
				reset(t)
				user := createPaymentRiskCaseTestUser(t, "receiver-funding")
				require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
					"quota": 100, "pulse_paid_quota": 100, "pulse_funding_epoch": 1,
				}).Error)
				_, err := AdjustPulseWalletReservation(user.Id, "equal-funding", 50, "reserved", true)
				require.NoError(t, err)
				receipt, err := AdjustPulseWalletReservation(user.Id, "equal-funding", 50, "settled", true)
				require.NoError(t, err, "MySQL unchanged wallet fields must not reject finalization")
				require.EqualValues(t, 50, receipt.PaidQuota)
				require.EqualValues(t, 50, receipt.PaidQuota)
				require.NoError(t, db.Delete(&User{}, user.Id).Error)
				_, err = AdjustPulseWalletReservation(user.Id, "deleted-funding", 10, "settled", true)
				require.Error(t, err)
				var count int64
				require.NoError(t, db.Model(&PulseWalletReservation{}).Where("request_id = ?", "deleted-funding").Count(&count).Error)
				require.Zero(t, count)
			})

			t.Run("spent_reward_cannot_create_debt", func(t *testing.T) {
				reset(t)
				user := createPaymentRiskCaseTestUser(t, "receiver-insufficient")
				request := pulseTestRequest("spent-reference", user.Id, 20)
				_, err := GrantPulseBenefit(request)
				require.NoError(t, err)
				require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", 3).Error)
				t.Setenv("PULSE_BENEFIT_ENABLED", "false")
				_, err = RollbackPulseBenefit(request.SourceRef, "insufficient balance")
				require.ErrorIs(t, err, ErrPulseBenefitInsufficientBalance)
				var refreshed User
				require.NoError(t, db.First(&refreshed, user.Id).Error)
				require.Equal(t, 3, refreshed.Quota)
				query, err := QueryPulseBenefit(request.SourceRef)
				require.NoError(t, err)
				require.True(t, query.Applied)
				var count int64
				require.NoError(t, db.Model(&BenefitChangeRecord{}).Where("action = ?", BenefitActionRollback).Count(&count).Error)
				require.Zero(t, count)
			})
		})
	}
}

type pulseConcurrentGrantResult struct {
	value PulseBenefitResult
	err   error
}

func runConcurrentPulseGrants(requests []PulseBenefitGrantRequest) []pulseConcurrentGrantResult {
	results := make([]pulseConcurrentGrantResult, len(requests))
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i, request := range requests {
		workers.Add(1)
		go func(i int, request PulseBenefitGrantRequest) {
			defer workers.Done()
			<-start
			results[i].value, results[i].err = GrantPulseBenefit(request)
		}(i, request)
	}
	close(start)
	workers.Wait()
	return results
}

// SQLite also serves real installations with a multi-connection pool. This
// checks transaction lock ordering instead of hiding it behind MaxOpenConns(1).
func TestPulseFundingSQLiteMultiConnectionTransactions(t *testing.T) {
	setupPulseBenefitTestDB(t)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "funding.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, db.AutoMigrate(&User{}, &PulseFundingLedger{}, &PulseWalletReservation{}))
	DB = db
	user := createPaymentRiskCaseTestUser(t, "sqlite-paid-funding")
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{"quota": 300, "pulse_paid_quota": 100, "pulse_funding_epoch": 1}).Error)
	type outcome struct {
		receipt PulseWalletReservation
		err     error
	}
	results := make([]outcome, 20)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range results {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			results[i].receipt, results[i].err = AdjustPulseWalletReservation(user.Id, fmt.Sprintf("sqlite-proof-%d", i), 10, "settled", true)
		}(i)
	}
	close(start)
	workers.Wait()
	var paid int64
	for _, result := range results {
		require.NoError(t, result.err)
		paid += result.receipt.PaidQuota
	}
	require.EqualValues(t, 100, paid)
	var refreshed User
	require.NoError(t, db.First(&refreshed, user.Id).Error)
	require.Equal(t, 100, refreshed.Quota)
	require.Zero(t, refreshed.PulsePaidQuota)
	require.Greater(t, pool.Stats().OpenConnections, 1)
}

func TestPulseFundingDeletedRecipientCannotCertifyUnappliedDebit(t *testing.T) {
	setupPaymentRiskCaseTestDB(t)
	user := createPaymentRiskCaseTestUser(t, "deleted-proof-user")
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"quota": 100, "pulse_paid_quota": 100, "pulse_funding_epoch": 1,
	}).Error)
	require.NoError(t, DB.Delete(&User{}, user.Id).Error)
	_, err := AdjustPulseWalletReservation(user.Id, "deleted-wallet-proof", 10, "settled", true)
	require.Error(t, err, "a zero-row wallet update must not produce a settled paid proof")
	var count int64
	require.NoError(t, DB.Model(&PulseWalletReservation{}).Where("request_id = ?", "deleted-wallet-proof").Count(&count).Error)
	require.Zero(t, count)
	var refreshed User
	require.NoError(t, DB.Unscoped().First(&refreshed, user.Id).Error)
	require.Equal(t, 100, refreshed.Quota)
}
