package model

import (
	"fmt"
	"os"
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

func routeOutageTestDB(t *testing.T, dialect string) {
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
	oldDB, oldPg := DB, common.UsingPostgreSQL
	DB, common.UsingPostgreSQL = db, dialect == "postgres"
	initCol()
	t.Cleanup(func() { DB, common.UsingPostgreSQL = oldDB, oldPg; initCol(); _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&RouteOutage{}))
	require.NoError(t, db.Where("group_name LIKE ?", "route-outage-test-%").Delete(&RouteOutage{}).Error)
}

func TestRouteOutageDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			routeOutageTestDB(t, dialect)
			now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC).UnixMilli()
			require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}))
			require.NoError(t, DB.Where("id = ?", 979801).Delete(&Channel{}).Error)
			require.NoError(t, DB.Where(map[string]any{"group": "route-outage-test-lookup"}).Delete(&Ability{}).Error)
			require.NoError(t, DB.Create(&Channel{Id: 979801, Key: "test-key", Status: common.ChannelStatusEnabled}).Error)
			require.NoError(t, DB.Create(&Ability{Group: "route-outage-test-lookup", Model: "gpt-4-gizmo-*", ChannelId: 979801, Enabled: false}).Error)
			channels, canonical, known, err := ConfiguredRouteChannels("route-outage-test-lookup", "gpt-4-gizmo-fixture")
			require.NoError(t, err)
			require.True(t, known)
			require.Equal(t, "gpt-4-gizmo-*", canonical)
			require.Len(t, channels, 1)
			require.Equal(t, common.ChannelStatusManuallyDisabled, channels[0].Status)
			require.NoError(t, DB.Where(map[string]any{"group": "route-outage-test-lookup"}).Delete(&Ability{}).Error)
			require.NoError(t, DB.Where("id = ?", 979801).Delete(&Channel{}).Error)

			batch := RouteOutageBatch{Group: "route-outage-test-core", Model: "fixture", Count: 3, FirstAt: now - 1000, LastAt: now, TokenIDs: []int{8, 9}}
			id := RouteOutageID(batch.Group, batch.Model)
			require.NoError(t, RecordRouteOutage(batch, 2, now))
			var wg sync.WaitGroup
			for i := range 8 {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					b := batch
					b.TokenIDs = []int{i + 1}
					if err := RecordRouteOutage(b, 2, now); err != nil {
						t.Error(err)
					}
				}(i)
			}
			wg.Wait()
			var row RouteOutage
			require.NoError(t, DB.First(&row, "id = ?", id).Error)
			require.EqualValues(t, 27, row.FailureCount)
			var samples []int
			require.NoError(t, common.UnmarshalJsonStr(row.TokenSamples, &samples))
			require.LessOrEqual(t, len(samples), 8)
			var winners atomic.Int32
			var claim *RouteOutage
			var claimMu sync.Mutex
			for i := range 8 {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					c, err := ClaimRouteOutageNotice(id, now, fmt.Sprintf("worker-%d", i))
					if err != nil {
						t.Error(err)
					}
					if c != nil {
						winners.Add(1)
						claimMu.Lock()
						claim = c
						claimMu.Unlock()
					}
				}(i)
			}
			wg.Wait()
			require.EqualValues(t, 1, winners.Load())
			require.NotNil(t, claim)
			require.NoError(t, FinishRouteOutageNotice(claim, now, QuotaNoticeFailed))
			early, err := ClaimRouteOutageNotice(id, now+time.Minute.Milliseconds(), "early")
			require.NoError(t, err)
			require.Nil(t, early)
			retry, err := ClaimRouteOutageNotice(id, now+5*time.Minute.Milliseconds(), "retry")
			require.NoError(t, err)
			require.NotNil(t, retry)
			require.NoError(t, FinishRouteOutageNotice(retry, now+5*time.Minute.Milliseconds(), QuotaNoticeSubmitted))
			require.NoError(t, ResolveRouteOutage(id, now+6*time.Minute.Milliseconds(), true))
			recovery, err := ClaimRouteOutageNotice(id, now+6*time.Minute.Milliseconds(), "recovery")
			require.NoError(t, err)
			require.NotNil(t, recovery)
			require.False(t, recovery.Active)
			require.NoError(t, FinishRouteOutageNotice(recovery, now+6*time.Minute.Milliseconds(), QuotaNoticeSubmitted))
			require.NoError(t, DB.First(&row, "id = ?", id).Error)
			require.False(t, row.Active)
			require.False(t, row.Notified)
			// A stale delivery must not erase the next outage or its counters.
			next := batch
			next.FirstAt = now + 7*time.Minute.Milliseconds()
			next.LastAt = next.FirstAt
			next.Count = 1
			require.NoError(t, RecordRouteOutage(next, 2, next.LastAt))
			require.NoError(t, FinishRouteOutageNotice(recovery, next.LastAt, QuotaNoticeSubmitted))
			require.NoError(t, DB.First(&row, "id = ?", id).Error)
			require.True(t, row.Active)
			require.EqualValues(t, 1, row.FailureCount)
			require.EqualValues(t, 2, row.Generation)
			// Expired leases are recoverable; unknown outcomes use the long cadence.
			abandoned, err := ClaimRouteOutageNotice(id, next.LastAt, "abandoned")
			require.NoError(t, err)
			require.NotNil(t, abandoned)
			recovered, err := ClaimRouteOutageNotice(id, next.LastAt+2*time.Minute.Milliseconds(), "lease-recovery")
			require.NoError(t, err)
			require.NotNil(t, recovered)
			require.NoError(t, FinishRouteOutageNotice(recovered, next.LastAt+2*time.Minute.Milliseconds(), QuotaNoticeUnknown))
			early, err = ClaimRouteOutageNotice(id, next.LastAt+3*time.Minute.Milliseconds(), "too-soon")
			require.NoError(t, err)
			require.Nil(t, early)
			// Removing configuration retires instead of announcing service recovery.
			require.NoError(t, ResolveRouteOutage(id, next.LastAt+4*time.Minute.Milliseconds(), false))
			require.NoError(t, DB.First(&row, "id = ?", id).Error)
			require.False(t, row.Notified)
			require.NoError(t, CleanupRouteOutages(now+8*24*time.Hour.Milliseconds()))
			var count int64
			require.NoError(t, DB.Model(&RouteOutage{}).Where("id = ?", id).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestRouteOutageRecoveryDuringDelivery(t *testing.T) {
	routeOutageTestDB(t, "sqlite")
	now := time.Now().UnixMilli()
	b := RouteOutageBatch{Group: "route-outage-test-inflight", Model: "m", Count: 1, FirstAt: now, LastAt: now}
	require.NoError(t, RecordRouteOutage(b, 1, now))
	id := RouteOutageID(b.Group, b.Model)
	claim, err := ClaimRouteOutageNotice(id, now, "outage")
	require.NoError(t, err)
	require.NoError(t, ResolveRouteOutage(id, now+1, true))
	require.NoError(t, FinishRouteOutageNotice(claim, now+2, QuotaNoticeSubmitted))
	recovery, err := ClaimRouteOutageNotice(id, now+3, "recovery")
	require.NoError(t, err)
	require.NotNil(t, recovery)
	require.False(t, recovery.Active)
}

func TestRouteOutageReminderRequiresNewFailures(t *testing.T) {
	routeOutageTestDB(t, "sqlite")
	now := time.Now().UnixMilli()
	b := RouteOutageBatch{Group: "route-outage-test-reminder", Model: "m", Count: 2, FirstAt: now, LastAt: now}
	require.NoError(t, RecordRouteOutage(b, 1, now))
	id := RouteOutageID(b.Group, b.Model)
	claim, err := ClaimRouteOutageNotice(id, now, "first")
	require.NoError(t, err)
	require.NoError(t, FinishRouteOutageNotice(claim, now, QuotaNoticeSubmitted))
	later := now + time.Hour.Milliseconds()
	idle, err := ClaimRouteOutageNotice(id, later, "idle")
	require.NoError(t, err)
	require.Nil(t, idle)
	notices, err := ListRouteOutageNotices(later)
	require.NoError(t, err)
	require.Empty(t, notices)
	b.FirstAt, b.LastAt = now+1000, now+1000
	require.NoError(t, RecordRouteOutage(b, 1, now+1000))
	early, err := ClaimRouteOutageNotice(id, now+2000, "early")
	require.NoError(t, err)
	require.Nil(t, early)
	reminder, err := ClaimRouteOutageNotice(id, later, "reminder")
	require.NoError(t, err)
	require.NotNil(t, reminder)
	// Concurrent arrivals are not included in the message already being delivered.
	b.FirstAt, b.LastAt = later+1, later+1
	require.NoError(t, RecordRouteOutage(b, 1, later+1))
	require.NoError(t, FinishRouteOutageNotice(reminder, later+2, QuotaNoticeSubmitted))
	var row RouteOutage
	require.NoError(t, DB.First(&row, "id = ?", id).Error)
	require.EqualValues(t, 6, row.FailureCount)
	require.EqualValues(t, 4, row.LastNotifiedFailureCount)
	another, err := ClaimRouteOutageNotice(id, later+time.Hour.Milliseconds(), "next")
	require.NoError(t, err)
	require.NotNil(t, another)
}
