package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInvitePromotionAggregation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	old := DB
	DB = db
	t.Cleanup(func() { DB = old; require.NoError(t, sqlDB.Close()) })
	require.NoError(t, DB.AutoMigrate(&InvitePromotionClick{}))
	ctx := context.Background()
	now := time.Date(2026, 10, 22, 0, 30, 0, 0, time.FixedZone("CST", 8*3600))
	var wg sync.WaitGroup
	failures := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- RecordInvitePromotionClick(ctx, 1, "banner", now) }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.NoError(t, RecordInvitePromotionClick(ctx, 1, "untrusted", now))
	require.NoError(t, RecordInvitePromotionClick(ctx, 2, "banner", now))
	require.NoError(t, RecordInvitePromotionClick(ctx, 1, "text", now.AddDate(0, 0, -29)))
	require.NoError(t, RecordInvitePromotionClick(ctx, 1, "text", now.AddDate(0, 0, -30)))
	require.NoError(t, RecordInvitePromotionClick(ctx, 1, "text", now.AddDate(0, 0, 1)))
	stats, err := GetInvitePromotionStats(ctx, 1, now)
	require.NoError(t, err)
	require.Equal(t, "2026-10-21", stats.To)
	require.Equal(t, "2026-09-22", stats.From)
	require.Equal(t, int64(42), stats.Clicks)
	require.Equal(t, int64(40), stats.Sources["banner"])
	require.Equal(t, int64(1), stats.Sources["link"])
	require.Len(t, stats.Daily, 30)
	require.Equal(t, int64(1), stats.Daily[0].Clicks)
	require.Equal(t, int64(41), stats.Daily[29].Clicks)
	empty, err := GetInvitePromotionStats(ctx, 3, now)
	require.NoError(t, err)
	require.Zero(t, empty.Clicks)
	require.Len(t, empty.Daily, 30)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, RecordInvitePromotionClick(ctx, 1, "button", now))
}

// Dialect rendering verifies portable upserts without claiming live-engine tests.
func TestInvitePromotionUpsertDialects(t *testing.T) {
	base, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	conn, err := base.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	for _, dialect := range []gorm.Dialector{
		mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}),
		postgres.New(postgres.Config{Conn: conn}),
	} {
		t.Run(dialect.Name(), func(t *testing.T) {
			db, err := gorm.Open(dialect, &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
			require.NoError(t, err)
			var sql string
			require.NoError(t, db.Callback().Create().After("gorm:create").Register("capture", func(tx *gorm.DB) { sql = tx.Statement.SQL.String() }))
			old := DB
			DB = db
			defer func() { DB = old }()
			require.NoError(t, RecordInvitePromotionClick(context.Background(), 7, "banner", time.Now()))
			if dialect.Name() == "mysql" {
				require.Contains(t, sql, "ON DUPLICATE KEY UPDATE")
				require.Contains(t, sql, "`invite_promotion_clicks`.`clicks")
			} else {
				require.Contains(t, sql, `ON CONFLICT ("user_id","day","source")`)
				require.Contains(t, sql, `"invite_promotion_clicks"."clicks"`)
			}
		})
	}
}
