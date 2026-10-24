package model

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func quotaNoticeTestDB(t *testing.T, dialect string) {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "mysql":
		dsn := os.Getenv("TEST_COMPAT_MYSQL_DSN")
		if dsn == "" {
			t.Skip("disposable MySQL not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_COMPAT_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("disposable PostgreSQL not configured")
		}
		driver = postgres.Open(dsn)
	default:
		driver = sqlite.Open(":memory:")
	}
	db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	if dialect == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
	}
	old := DB
	DB = db
	t.Cleanup(func() { DB = old; _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&QuotaNoticeAccount{}, &QuotaNoticeSignal{}, &QuotaNotice{}))
	for _, m := range []interface{}{&QuotaNotice{}, &QuotaNoticeSignal{}, &QuotaNoticeAccount{}} {
		require.NoError(t, db.Where("user_id >= ? AND user_id <= ?", 900001, 900099).Delete(m).Error)
	}
}

func quotaFailure(t *testing.T, user int, scope string, at int64, request string) {
	t.Helper()
	ok, _, err := RecordQuotaNoticeSignal(user, QuotaNoticeResource{Scope: scope, Source: "wallet", LastAt: at, RequestID: request, Available: 3, Required: 9, Known: true}, false)
	require.NoError(t, err)
	require.True(t, ok)
}
func quotaBurst(t *testing.T, user int, at int64) {
	t.Helper()
	for i := 0; i < 3; i++ {
		quotaFailure(t, user, "wallet:0", at-int64(2-i)*31000, fmt.Sprintf("%d-%d-%d", user, at, i))
	}
}
func quotaSubmit(t *testing.T, user int, at int64, status string) *QuotaNotice {
	t.Helper()
	quotaBurst(t, user, at)
	n, err := PrepareQuotaNotice(user, at, nil)
	require.NoError(t, err)
	require.NotNil(t, n)
	claimed, err := ClaimQuotaNotice(n.ID, at)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.NoError(t, FinishQuotaNotice(n.ID, at, status, "fixture", "email", "te***@example.invalid"))
	return n
}

func TestQuotaNoticeDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			quotaNoticeTestDB(t, dialect)
			// 23:59 Beijing: the second send crosses midnight without resetting cap.
			base := time.Date(2026, 10, 24, 15, 59, 0, 0, time.UTC).UnixMilli()
			t.Run("rolling24h_and_cadence", func(t *testing.T) {
				user := 900001
				quotaSubmit(t, user, base, QuotaNoticeSubmitted)
				quotaBurst(t, user, base+29*quotaNoticeMinute)
				n, err := PrepareQuotaNotice(user, base+29*quotaNoticeMinute, nil)
				require.NoError(t, err)
				require.Nil(t, n)
				quotaSubmit(t, user, base+30*quotaNoticeMinute, QuotaNoticeSubmitted)
				quotaSubmit(t, user, base+(6*60+30)*quotaNoticeMinute, QuotaNoticeSubmitted)
				quotaSubmit(t, user, base+(12*60+30)*quotaNoticeMinute, QuotaNoticeSubmitted)
				at := base + (18*60+30)*quotaNoticeMinute
				quotaBurst(t, user, at)
				n, err = PrepareQuotaNotice(user, at, nil)
				require.NoError(t, err)
				require.Nil(t, n)
				var suppressed int64
				require.NoError(t, DB.Model(&QuotaNotice{}).Where("user_id = ? AND status = ?", user, QuotaNoticeSuppressed).Count(&suppressed).Error)
				require.EqualValues(t, 1, suppressed)
				// Recreate process state / cache: only SQL records enforce the cap.
				n, err = PrepareQuotaNotice(user, base+QuotaNoticeWindow-1, nil)
				require.NoError(t, err)
				require.Nil(t, n)
				quotaSubmit(t, user, base+QuotaNoticeWindow, QuotaNoticeSubmitted)
				rows, total, err := ListQuotaNotices(user, 0, 10)
				require.NoError(t, err)
				require.EqualValues(t, 6, total)
				require.Equal(t, base+QuotaNoticeWindow, rows[0].UpdatedAt)
			})
			t.Run("burst_dedup_and_concurrent_claim", func(t *testing.T) {
				user := 900002
				for i := 0; i < 154; i++ {
					quotaFailure(t, user, "wallet:0", base+int64(i)*380, fmt.Sprintf("burst-%d", i))
				}
				n, err := PrepareQuotaNotice(user, base+59000, nil)
				require.NoError(t, err)
				require.Nil(t, n)
				ok, _, err := RecordQuotaNoticeSignal(user, QuotaNoticeResource{Scope: "wallet:0", RequestID: "burst-0", LastAt: base}, false)
				require.NoError(t, err)
				require.False(t, ok)
				quotaFailure(t, user, "wallet:0", base+75000, "after-minute")
				var created atomic.Int32
				var wg sync.WaitGroup
				var id atomic.Int64
				for range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						row, e := PrepareQuotaNotice(user, base+75000, nil)
						require.NoError(t, e)
						if row != nil {
							created.Add(1)
							id.Store(int64(row.ID))
						}
					}()
				}
				wg.Wait()
				require.EqualValues(t, 1, created.Load())
				var claims atomic.Int32
				for range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						row, e := ClaimQuotaNotice(int(id.Load()), base+75000)
						require.NoError(t, e)
						if row != nil {
							claims.Add(1)
						}
					}()
				}
				wg.Wait()
				require.EqualValues(t, 1, claims.Load())
				// A crash after claim is unknown, consumes one slot, never requeues.
				require.NoError(t, RecoverQuotaNoticeLeases(base+75000+2*quotaNoticeMinute))
				var row QuotaNotice
				require.NoError(t, DB.First(&row, int(id.Load())).Error)
				require.Equal(t, QuotaNoticeUnknown, row.Status)
				require.NoError(t, RecoverQuotaNoticeLeases(base+75000+3*quotaNoticeMinute))
				var account QuotaNoticeAccount
				require.NoError(t, DB.First(&account, "user_id = ?", user).Error)
				require.Equal(t, 1, account.Stage)
			})
			t.Run("recovery_does_not_reset_rolling_cap", func(t *testing.T) {
				user := 900003
				for i := 0; i < 4; i++ {
					at := base + int64(i)*20*quotaNoticeMinute
					quotaSubmit(t, user, at, QuotaNoticeSubmitted)
					_, _, err := RecordQuotaNoticeSignal(user, QuotaNoticeResource{Scope: "wallet:0", LastAt: at + 1, RequestID: fmt.Sprintf("paid-%d", i)}, true)
					require.NoError(t, err)
				}
				at := base + 80*quotaNoticeMinute
				quotaBurst(t, user, at)
				n, err := PrepareQuotaNotice(user, at, nil)
				require.NoError(t, err)
				require.Nil(t, n)
			})
			t.Run("other_resource_success_and_silence", func(t *testing.T) {
				user := 900004
				quotaSubmit(t, user, base, QuotaNoticeSubmitted)
				_, _, err := RecordQuotaNoticeSignal(user, QuotaNoticeResource{Scope: "token:99", LastAt: base + 1, RequestID: "unrelated"}, true)
				require.NoError(t, err)
				n, err := PrepareQuotaNotice(user, base+30*quotaNoticeMinute, nil)
				require.NoError(t, err)
				require.Nil(t, n)
				var a QuotaNoticeAccount
				require.NoError(t, DB.First(&a, "user_id = ?", user).Error)
				resources, err := quotaNoticeResources(&a)
				require.NoError(t, err)
				require.Contains(t, resources, "wallet:0")
				require.Equal(t, 1, a.Stage)
			})
			t.Run("bounded_retry_same_event_and_release", func(t *testing.T) {
				user := 900005
				n := quotaSubmit(t, user, base, QuotaNoticeFailed)
				for i, at := range []int64{base + quotaNoticeMinute, base + 6*quotaNoticeMinute} {
					quotaBurst(t, user, at)
					retry, err := PrepareQuotaNotice(user, at, nil)
					require.NoError(t, err)
					require.NotNil(t, retry)
					require.Equal(t, n.ID, retry.ID)
					claimed, err := ClaimQuotaNotice(retry.ID, at)
					require.NoError(t, err)
					require.Equal(t, i+2, claimed.Attempts)
					require.NoError(t, FinishQuotaNotice(retry.ID, at, QuotaNoticeFailed, "fixture", "email", "masked"))
				}
				var row QuotaNotice
				require.NoError(t, DB.First(&row, n.ID).Error)
				require.Equal(t, QuotaNoticeFailed, row.Status)
				require.Equal(t, 3, row.Attempts)
				count, _, err := quotaNoticeSlots(DB, user, base+7*quotaNoticeMinute)
				require.NoError(t, err)
				require.Zero(t, count)
				next, err := PrepareQuotaNotice(user, base+7*quotaNoticeMinute, nil)
				require.NoError(t, err)
				require.Nil(t, next)
			})
			t.Run("inactive_retry_is_cancelled", func(t *testing.T) {
				user := 900006
				n := quotaSubmit(t, user, base, QuotaNoticeFailed)
				_, err := PrepareQuotaNotice(user, base+11*quotaNoticeMinute, nil)
				require.NoError(t, err)
				var row QuotaNotice
				require.NoError(t, DB.First(&row, n.ID).Error)
				require.Equal(t, QuotaNoticeSkipped, row.Status)
			})
			t.Run("merged_scope_and_audit_isolation", func(t *testing.T) {
				user := 900007
				quotaBurst(t, user, base)
				quotaFailure(t, user, "token:11", base-1000, "other-token")
				n, err := PrepareQuotaNotice(user, base, nil)
				require.NoError(t, err)
				require.NotNil(t, n)
				var summary []QuotaNoticeResource
				require.NoError(t, common.UnmarshalJsonStr(n.Summary, &summary))
				require.Len(t, summary, 2)
				rows, total, err := ListQuotaNotices(900099, 0, 10)
				require.NoError(t, err)
				require.Empty(t, rows)
				require.Zero(t, total)
			})
		})
	}
}

func TestQuotaNoticeEvidenceByteBudget(t *testing.T) {
	resources := map[string]QuotaNoticeResource{}
	for i := 0; i < 64; i++ {
		names := make([]string, 64)
		for j := range names {
			names[j] = strings.Repeat("<", 120)
		}
		resources[fmt.Sprint(i)] = QuotaNoticeResource{Scope: fmt.Sprint(i), TokenName: strings.Repeat("<", 80), ModelName: strings.Repeat("<", 120), ModelNames: names, LastAt: int64(i + 1), Count: 3}
	}
	a := QuotaNoticeAccount{}
	require.NoError(t, saveQuotaNoticeResources(&a, resources))
	require.LessOrEqual(t, len(a.Resources), 56*1024)
	require.Contains(t, resources, "63")
}
