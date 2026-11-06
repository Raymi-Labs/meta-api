package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// TestRouteInsightTablesAcrossDialects exercises the new tables and the
// token route_plan_id column on every configured database: AutoMigrate twice,
// UUID-row upserts, window queries, GROUP BY counts and retention deletes.
func TestRouteInsightTablesAcrossDialects(t *testing.T) {
	oldPG, oldSQLite, oldMySQL, oldDB := common.UsingPostgreSQL, common.UsingSQLite, common.UsingMySQL, DB
	t.Cleanup(func() {
		common.UsingPostgreSQL, common.UsingSQLite, common.UsingMySQL, DB = oldPG, oldSQLite, oldMySQL, oldDB
		initCol()
		InvalidateRoutePlanCache()
	})
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var dsn string
			switch dialect {
			case "sqlite":
				dsn = "local"
				previousPath := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "route.db")
				t.Cleanup(func() { common.SQLitePath = previousPath })
			case "mysql":
				dsn = os.Getenv("TEST_MYSQL_DSN")
			case "postgres":
				dsn = os.Getenv("TEST_POSTGRES_DSN")
			}
			if dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			t.Setenv("ROUTE_INSIGHT_TEST_DSN", dsn)
			common.UsingPostgreSQL, common.UsingSQLite, common.UsingMySQL = false, false, false
			db, err := chooseDB("ROUTE_INSIGHT_TEST_DSN", false)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			DB = db
			initCol()
			InvalidateRoutePlanCache()
			for _, table := range []any{&RouteAttemptMetric{}, &RouteShadowMetric{}, &RoutePlan{}, &Token{}} {
				_ = db.Migrator().DropTable(table)
			}
			t.Cleanup(func() {
				for _, table := range []any{&RouteAttemptMetric{}, &RouteShadowMetric{}, &RoutePlan{}, &Token{}} {
					_ = db.Migrator().DropTable(table)
				}
			})
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(&RouteAttemptMetric{}, &RouteShadowMetric{}, &RoutePlan{}, &Token{}))
			}
			require.True(t, db.Migrator().HasColumn(&Token{}, "route_plan_id"))

			ctx := context.Background()
			now := time.Now()
			ts := now.Unix() / 60 * 60
			rows := []RouteAttemptMetric{
				{ID: "00000000-0000-0000-0000-00000000000a", BucketTs: ts, ChannelID: 1, GroupName: "g", ModelName: "m", AttemptCount: 3, SuccessCount: 2, ServiceFailureCount: 1, TTFTHistogram: "[1,1,0,0,0,0,0,0,0,0,0,0,0]", UpdatedAt: now.Unix()},
				{ID: "00000000-0000-0000-0000-00000000000b", BucketTs: now.Add(-4 * 24 * time.Hour).Unix(), ChannelID: 1, GroupName: "g", ModelName: "m", AttemptCount: 9},
			}
			require.NoError(t, SaveRouteAttemptSnapshots(ctx, rows))
			rows[0].AttemptCount, rows[0].SuccessCount = 5, 4
			require.NoError(t, SaveRouteAttemptSnapshots(ctx, rows), "re-saving the same UUID overwrites totals")
			got, err := QueryRouteAttemptMetrics(ctx, now.Add(-time.Hour).Unix(), now.Unix(), "m")
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.EqualValues(t, 5, got[0].AttemptCount)
			require.Equal(t, rows[0].TTFTHistogram, got[0].TTFTHistogram)

			shadow := []RouteShadowMetric{{ID: "00000000-0000-0000-0000-00000000000c", BucketTs: now.Unix() / 3600 * 3600, ModelName: "m", DecisionCount: 2, DisagreeCount: 1}}
			require.NoError(t, SaveRouteShadowSnapshots(ctx, shadow))
			shadow[0].DecisionCount = 3
			require.NoError(t, SaveRouteShadowSnapshots(ctx, shadow))
			shadows, err := QueryRouteShadowMetrics(ctx, now.Add(-2*time.Hour).Unix(), now.Unix(), "")
			require.NoError(t, err)
			require.Len(t, shadows, 1)
			require.EqualValues(t, 3, shadows[0].DecisionCount)
			require.NoError(t, DeleteExpiredRouteInsightMetrics(ctx, now))
			all, err := QueryRouteAttemptMetrics(ctx, 0, now.Unix(), "")
			require.NoError(t, err)
			require.Len(t, all, 1, "attempt rows older than 3 days are removed")

			plan := &RoutePlan{Name: "跨库方案", Groups: `["g"]`, SaleRatio: 1.25, ModelRatios: `{"m":2}`, UserIds: `[1]`}
			require.NoError(t, plan.Normalize(nil))
			require.NoError(t, CreateRoutePlan(plan))
			plan.Description = "更新"
			require.NoError(t, UpdateRoutePlan(plan))
			stored, err := GetRoutePlanByID(plan.Id)
			require.NoError(t, err)
			require.EqualValues(t, 2, stored.Version)
			require.Equal(t, "跨库方案", stored.Name)
			require.NoError(t, db.Create(&Token{Key: "route-dialect-key-1", Name: "a", RoutePlanId: plan.Id}).Error)
			require.NoError(t, db.Create(&Token{Key: "route-dialect-key-2", Name: "b", RoutePlanId: plan.Id}).Error)
			require.NoError(t, db.Create(&Token{Key: "route-dialect-key-3", Name: "c"}).Error)
			counts, err := CountRoutePlanTokens()
			require.NoError(t, err)
			require.EqualValues(t, 2, counts[plan.Id])
			require.Error(t, DeleteRoutePlan(plan.Id))
			require.NoError(t, db.Model(&Token{}).Where("route_plan_id = ?", plan.Id).Update("route_plan_id", 0).Error)
			require.NoError(t, DeleteRoutePlan(plan.Id))
		})
	}
}
