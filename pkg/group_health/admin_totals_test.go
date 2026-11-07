package grouphealth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAdminGroupTotalsAcrossDatabases(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(":memory:")
			if dialect == "mysql" {
				if os.Getenv("TEST_MYSQL_DSN") == "" {
					t.Skip("no disposable MySQL")
				}
				driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
			}
			if dialect == "postgres" {
				if os.Getenv("TEST_POSTGRES_DSN") == "" {
					t.Skip("no disposable PostgreSQL")
				}
				driver = postgres.Open(os.Getenv("TEST_POSTGRES_DSN"))
			}
			previous := model.DB
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			model.DB = db
			t.Cleanup(func() { model.DB = previous })
			require.NoError(t, db.Migrator().DropTable(&model.GroupHealthHalfHourMetric{}))
			require.NoError(t, db.AutoMigrate(&model.GroupHealthHalfHourMetric{}))
			ctx := context.Background()
			now := time.Unix(1800000000, 0).Add(10 * time.Minute)
			ts := now.Unix() / HalfHourBucketSeconds * HalfHourBucketSeconds
			require.NoError(t, model.SaveGroupHealthHalfHourSnapshots(ctx, []model.GroupHealthMetric{
				{ID: "x1", GroupName: "x", ModelName: "m1", BucketTs: ts, RequestCount: 4, SuccessCount: 3, ServiceFailureCount: 1, UpdatedAt: 7},
				{ID: "x2", GroupName: "x", ModelName: "m2", BucketTs: ts - HalfHourBucketSeconds, RequestCount: 6, SuccessCount: 6, UpdatedAt: 9},
				{ID: "x-old", GroupName: "x", ModelName: "m1", BucketTs: ts - 2*HalfHourBucketSeconds, RequestCount: 50},
				{ID: "y", GroupName: "y", ModelName: "m1", BucketTs: ts, RequestCount: 20, SuccessCount: 10},
				{ID: "auto", GroupName: "auto", ModelName: "m1", BucketTs: ts, RequestCount: 99},
			}))
			result, err := AdminGroupTotals(ctx, now, 1)
			require.NoError(t, err)
			require.Equal(t, ts-HalfHourBucketSeconds, result.StartTime, "one hour is the current and previous half hour")
			require.Len(t, result.Groups, 2)
			require.Equal(t, "y", result.Groups[0].Group)
			x := result.Groups[1]
			require.EqualValues(t, 10, x.RequestCount)
			require.EqualValues(t, 9, x.SuccessCount)
			require.EqualValues(t, 1, x.ServiceFailureCount)
			require.EqualValues(t, 9, x.UpdatedAt)
			require.InDelta(t, 90.0, *x.SuccessRate, 0.001)
		})
	}
}
