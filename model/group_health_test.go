package model

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGroupHealthPersistenceAcrossProcessesAndExactFilters(t *testing.T) {
	previous := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previous })
	require.NoError(t, db.AutoMigrate(&GroupHealthMetric{}, &Ability{}))
	ctx := context.Background()
	now := time.Now()
	ts := now.Unix() / 3600 * 3600
	rows := []GroupHealthMetric{
		{ID: "process-a", GroupName: "public", ModelName: "model-a", BucketTs: ts, RequestCount: 10, SuccessCount: 8, TTFTSumMs: 800, TTFTCount: 8, CacheReadTokens: 80, CacheInputTokens: 100, CacheSampleCount: 1, UpdatedAt: now.Unix(), InputTokens: 100, OutputTokens: 20, UsageSampleCount: 1, ThroughputOutputTokens: 20, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{ID: "process-b", GroupName: "public", ModelName: "model-a", BucketTs: ts, RequestCount: 5, SuccessCount: 5, CacheReadTokens: 30, CacheInputTokens: 170, CacheSampleCount: 2, InputTokens: 200, OutputTokens: 30, UsageSampleCount: 1, ThroughputOutputTokens: 30, GenerationSumMs: 2000, ThroughputSampleCount: 1},
		{ID: "other-model", GroupName: "public", ModelName: "model-b", BucketTs: ts, RequestCount: 100, SuccessCount: 100},
		{ID: "private", GroupName: "private", ModelName: "model-a", BucketTs: ts, RequestCount: 999, SuccessCount: 999},
		{ID: "expired", GroupName: "public", ModelName: "model-a", BucketTs: now.Add(-8 * 24 * time.Hour).Unix(), RequestCount: 1},
	}
	require.NoError(t, SaveGroupHealthSnapshots(ctx, rows))
	require.NoError(t, SaveGroupHealthSnapshots(ctx, rows), "retrying snapshots must not add counts")
	rows[0].RequestCount = 11
	rows[0].CacheInputTokens = 200
	rows[0].CacheSampleCount = 2
	rows[0].ServiceFailureCount = 1
	rows[0].ClientCanceledCount = 1
	rows[0].ExcludedUserErrorCount = 4
	require.NoError(t, SaveGroupHealthSnapshots(ctx, rows[:1]))
	got, err := QueryGroupHealthMetrics(ctx, []string{"public"}, "model-a", ts, now.Unix())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.EqualValues(t, 16, got[0].RequestCount)
	require.EqualValues(t, 13, got[0].SuccessCount)
	require.EqualValues(t, 800, got[0].TTFTSumMs)
	require.EqualValues(t, 110, got[0].CacheReadTokens)
	require.EqualValues(t, 370, got[0].CacheInputTokens)
	require.EqualValues(t, 4, got[0].CacheSampleCount)
	require.EqualValues(t, 1, got[0].ServiceFailureCount)
	require.EqualValues(t, 1, got[0].ClientCanceledCount)
	require.EqualValues(t, 4, got[0].ExcludedUserErrorCount)
	require.EqualValues(t, 300, got[0].InputTokens)
	require.EqualValues(t, 50, got[0].OutputTokens)
	require.EqualValues(t, 2, got[0].UsageSampleCount)
	require.EqualValues(t, 50, got[0].ThroughputOutputTokens)
	require.EqualValues(t, 3000, got[0].GenerationSumMs)
	require.EqualValues(t, 2, got[0].ThroughputSampleCount)
	batch, err := QueryGroupHealthMetricsForModels(ctx, []string{"public"}, []string{"model-a", "model-b"}, ts, now.Unix())
	require.NoError(t, err)
	require.Len(t, batch, 2)
	denied, err := QueryGroupHealthMetricsForModels(ctx, []string{"public"}, []string{}, ts, now.Unix())
	require.NoError(t, err)
	require.Empty(t, denied)
	got, err = QueryGroupHealthMetrics(ctx, nil, "", ts, now.Unix())
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, DeleteExpiredGroupHealthMetrics(ctx, now))
	var count int64
	require.NoError(t, db.Model(&GroupHealthMetric{}).Where("id = ?", "expired").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Create(&Ability{Group: "public", Model: "model-a", ChannelId: 1, Enabled: false}).Error)
	require.True(t, HasConfiguredGroupModel("public", "model-a", nil), "disabled channel is a real outage")
	require.False(t, HasConfiguredGroupModel("public", "typo", nil))
	require.False(t, HasConfiguredGroupModel("public", "model-a", []int{}), "empty channel allowlist denies all")
	require.False(t, HasConfiguredGroupModel("public", "model-a", []int{2}))
	require.True(t, HasConfiguredGroupModel("public", "model-a", []int{1}))
}

func TestGroupHealthCacheMigrationKeepsLegacyRowsUnknown(t *testing.T) {
	previous := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previous })
	require.NoError(t, db.AutoMigrate(&GroupHealthMetric{}))
	// Recreate the pre-cache schema, then exercise the same additive migration
	// used at startup. Existing health outcomes must not turn into cache samples.
	for _, field := range []string{"CacheReadTokens", "CacheInputTokens", "CacheSampleCount"} {
		require.NoError(t, db.Migrator().DropColumn(&GroupHealthMetric{}, field))
	}
	require.NoError(t, db.Table("group_health_metrics").Create(map[string]interface{}{
		"id": "legacy", "group_name": "g", "model_name": "m", "bucket_ts": 3600,
		"request_count": 10, "success_count": 9,
	}).Error)
	require.NoError(t, db.AutoMigrate(&GroupHealthMetric{}))
	rows, err := QueryGroupHealthMetrics(context.Background(), []string{"g"}, "m", 3600, 7200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 10, rows[0].RequestCount)
	require.Zero(t, rows[0].CacheSampleCount)
	require.Zero(t, rows[0].CacheInputTokens)
}

func TestGroupHealthUsageMigrationPreservesExistingAndOldWriterRows(t *testing.T) {
	previous := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previous })
	require.NoError(t, db.AutoMigrate(&GroupHealthMetric{}))
	for _, field := range []string{"InputTokens", "OutputTokens", "UsageSampleCount", "ThroughputOutputTokens", "GenerationSumMs", "ThroughputSampleCount"} {
		require.NoError(t, db.Migrator().DropColumn(&GroupHealthMetric{}, field))
	}
	oldRow := map[string]interface{}{"id": "before-upgrade", "group_name": "g", "model_name": "m", "bucket_ts": 3600, "request_count": 10, "success_count": 9, "service_failure_count": 1}
	require.NoError(t, db.Table("group_health_metrics").Create(oldRow).Error)
	require.NoError(t, db.AutoMigrate(&GroupHealthMetric{}))
	oldRow = map[string]interface{}{"id": "old-replica-after-upgrade", "group_name": "g", "model_name": "m", "bucket_ts": 3600, "request_count": 10, "success_count": 9, "service_failure_count": 1}
	require.NoError(t, db.Table("group_health_metrics").Create(oldRow).Error)
	require.NoError(t, SaveGroupHealthSnapshots(context.Background(), []GroupHealthMetric{{ID: "new-replica", GroupName: "g", ModelName: "m", BucketTs: 3600, RequestCount: 1, SuccessCount: 1, InputTokens: 10, OutputTokens: 5, UsageSampleCount: 1, ThroughputOutputTokens: 5, GenerationSumMs: 1000, ThroughputSampleCount: 1}}))
	rows, err := QueryGroupHealthMetrics(context.Background(), []string{"g"}, "m", 3600, 7200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 21, rows[0].RequestCount)
	require.EqualValues(t, 19, rows[0].SuccessCount)
	require.EqualValues(t, 1, rows[0].UsageSampleCount, "old counts never become measured samples")
	require.EqualValues(t, 10, rows[0].InputTokens)
	require.EqualValues(t, 5, rows[0].OutputTokens)
	require.EqualValues(t, 1, rows[0].ThroughputSampleCount)
}

// Keep the pre-attribution schema explicit: migration must add columns without
// rewriting or reclassifying historical outcomes, including during a rollout
// in which an older process continues to write its original columns.
type groupHealthMetricLegacy struct {
	ID               string `gorm:"type:varchar(36);primaryKey"`
	GroupName        string `gorm:"size:64;index:idx_group_health_window,priority:1"`
	ModelName        string `gorm:"size:255;index:idx_group_health_model,priority:1"`
	BucketTs         int64  `gorm:"index:idx_group_health_window,priority:2;index:idx_group_health_model,priority:2;index"`
	RequestCount     int64
	SuccessCount     int64
	TTFTSumMs        int64
	TTFTCount        int64
	CompletionSumMs  int64
	CompletionCount  int64
	CacheReadTokens  int64 `gorm:"default:0"`
	CacheInputTokens int64 `gorm:"default:0"`
	CacheSampleCount int64 `gorm:"default:0"`
	UpdatedAt        int64 `gorm:"autoUpdateTime:false"`
}

func TestGroupHealthOutcomeMigrationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(":memory:")
			switch dialect {
			case "mysql":
				dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
				if dsn == "" {
					t.Skip("disposable TEST_MYSQL_DSN is not configured")
				}
				dialector = mysqlMigrationDialector{mysql.Dialector{Config: &mysql.Config{DSN: dsn}}}
			case "postgres":
				dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
				if dsn == "" {
					t.Skip("disposable TEST_POSTGRES_DSN is not configured")
				}
				dialector = postgresMigrationDialector{postgres.Dialector{Config: &postgres.Config{DSN: dsn, PreferSimpleProtocol: true}}}
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			table := fmt.Sprintf("group_health_outcome_%d", time.Now().UnixNano())
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(table)) })
			tableDB := db.Table(table).Session(&gorm.Session{})
			require.NoError(t, tableDB.AutoMigrate(&groupHealthMetricLegacy{}))
			require.NoError(t, tableDB.Create(&groupHealthMetricLegacy{ID: "legacy", GroupName: "g", ModelName: "m", BucketTs: 3600, RequestCount: 10, SuccessCount: 6}).Error)
			recorder := &migrationSQLRecorder{}
			require.NoError(t, tableDB.Session(&gorm.Session{Logger: recorder}).AutoMigrate(&GroupHealthMetric{}))
			mutations := recorder.schemaMutations()
			require.Len(t, mutations, 9, "migration only adds attribution and measured usage counters")
			for _, statement := range mutations {
				require.Contains(t, strings.ToUpper(statement), "ADD")
				require.NotContains(t, strings.ToUpper(statement), "DROP")
			}
			recorder.reset()
			require.NoError(t, tableDB.Session(&gorm.Session{Logger: recorder}).AutoMigrate(&GroupHealthMetric{}))
			require.Empty(t, recorder.schemaMutations(), "repeated startup must not change the schema")

			var legacy GroupHealthMetric
			require.NoError(t, tableDB.Where("id = ?", "legacy").First(&legacy).Error)
			require.EqualValues(t, 10, legacy.RequestCount)
			require.EqualValues(t, 6, legacy.SuccessCount)
			require.Zero(t, legacy.ServiceFailureCount)
			require.Zero(t, legacy.ExcludedUserErrorCount)
			require.Zero(t, legacy.ClientCanceledCount)
			require.Zero(t, legacy.UsageSampleCount)
			require.Zero(t, legacy.ThroughputSampleCount)
			require.Zero(t, legacy.InputTokens)
			require.Zero(t, legacy.OutputTokens)
			require.NoError(t, tableDB.Create(&groupHealthMetricLegacy{ID: "old-writer", GroupName: "g", ModelName: "old-after", BucketTs: 3600, RequestCount: 3, SuccessCount: 2}).Error)
			var oldWriter GroupHealthMetric
			require.NoError(t, tableDB.Where("id = ?", "old-writer").First(&oldWriter).Error)
			require.Zero(t, oldWriter.UsageSampleCount)
			require.Zero(t, oldWriter.ThroughputSampleCount)
			require.Zero(t, oldWriter.InputTokens)

			previous := DB
			DB = tableDB
			t.Cleanup(func() { DB = previous })
			ctx := context.Background()
			fresh := GroupHealthMetric{ID: "new", GroupName: "g", ModelName: "m", BucketTs: 3600, RequestCount: 5, SuccessCount: 2, ServiceFailureCount: 1, ClientCanceledCount: 1, ExcludedUserErrorCount: 3, InputTokens: 50, OutputTokens: 20, UsageSampleCount: 1, ThroughputOutputTokens: 20, GenerationSumMs: 1000, ThroughputSampleCount: 1}
			require.NoError(t, SaveGroupHealthSnapshots(ctx, []GroupHealthMetric{fresh}))
			require.NoError(t, SaveGroupHealthSnapshots(ctx, []GroupHealthMetric{fresh}), "ambiguous commit retries must be idempotent")
			// Only the excluded count changes in this later absolute snapshot.
			fresh.ExcludedUserErrorCount = 4
			require.NoError(t, SaveGroupHealthSnapshots(ctx, []GroupHealthMetric{fresh}))
			require.NoError(t, tableDB.Model(&groupHealthMetricLegacy{}).Where("id = ?", "legacy").Update("request_count", 11).Error)
			rows, err := QueryGroupHealthMetrics(ctx, []string{"g"}, "m", 3600, 7200)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.EqualValues(t, 16, rows[0].RequestCount)
			require.EqualValues(t, 8, rows[0].SuccessCount)
			require.EqualValues(t, 1, rows[0].ServiceFailureCount)
			require.EqualValues(t, 1, rows[0].ClientCanceledCount)
			require.EqualValues(t, 4, rows[0].ExcludedUserErrorCount)
			require.EqualValues(t, 50, rows[0].InputTokens)
			require.EqualValues(t, 20, rows[0].OutputTokens)
			require.EqualValues(t, 1, rows[0].UsageSampleCount)
			require.EqualValues(t, 20, rows[0].ThroughputOutputTokens)
			require.EqualValues(t, 1000, rows[0].GenerationSumMs)
			require.EqualValues(t, 1, rows[0].ThroughputSampleCount)
			// Legacy residual 5 + newly unknown 2, without claiming a cause.
			require.EqualValues(t, 7, rows[0].RequestCount-rows[0].SuccessCount-rows[0].ServiceFailureCount)
		})
	}
}
