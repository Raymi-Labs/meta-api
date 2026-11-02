package model

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Opt-in integration against isolated cash_test databases. Production schema
// (without rows) can be imported before this test to exercise additive upgrades.
func TestCashDatabaseCompatibility(t *testing.T) {
	for _, engine := range []string{"MYSQL", "POSTGRES"} {
		t.Run(engine, func(t *testing.T) {
			dsn := os.Getenv("META_CASH_TEST_" + engine + "_DSN")
			if dsn == "" {
				t.Skip("isolated database DSN not supplied")
			}
			if !strings.Contains(dsn, "cash_test") || !strings.Contains(dsn, "127.0.0.1") {
				t.Fatal("only local isolated cash_test databases are allowed")
			}
			setupCashTestDB(t)
			var dialector gorm.Dialector
			if engine == "MYSQL" {
				dialector = mysql.Open(dsn)
			} else {
				dialector = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			DB, LOG_DB = db, db
			common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = false, engine == "MYSQL", engine == "POSTGRES"
			initCol()
			t.Cleanup(initCol)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&User{}, &UpstreamAccount{}, &PulseFundingLedger{}, &PulseWalletReservation{}, &WalletTransferLock{}))
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(cashAccountingModels()...))
			}
			require.NoError(t, db.Clauses(clause.OnConflict{DoNothing: true}).Create(&CashAccountingConfig{ID: 1, EnabledAt: 1, EnabledBy: 1}).Error)
			u := createPaymentRiskCaseTestUser(t, "c"+common.GetUUID()[:8])
			lot := cashGrantTest(t, u, 100, "topup")
			r := cashReceiptTest(t, lot, "9.90", common.GetTimestamp()-1)
			require.EqualValues(t, 9900000, r.NetCNYMicros)
			var wg sync.WaitGroup
			results := make(chan error, 10)
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, e := AdjustPulseWalletReservation(u.Id, common.GetUUID(), 10, "settled", true)
					results <- e
				}()
			}
			wg.Wait()
			close(results)
			for e := range results {
				require.NoError(t, e)
			}
			require.Zero(t, readFundingUser(t, u.Id).Quota)
			require.NoError(t, db.First(&lot, lot.ID).Error)
			require.Zero(t, lot.RemainingQuota)
			var allocated int64
			require.NoError(t, db.Model(&CashFundingAllocation{}).Where("lot_id = ?", lot.ID).Select("SUM(quota)").Scan(&allocated).Error)
			require.EqualValues(t, 100, allocated)
		})
	}
}
