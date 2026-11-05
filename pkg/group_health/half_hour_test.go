package grouphealth

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
	"time"
)

func TestHalfHourCollectionAndQuery(t *testing.T) {
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
			testHalfHourCollectionAndQuery(t, driver)
		})
	}
}

func testHalfHourCollectionAndQuery(t *testing.T, driver gorm.Dialector) {
	previous := model.DB
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() { model.DB = previous; model.InvalidateModelPermissionCache() })
	require.NoError(t, db.AutoMigrate(&model.GroupHealthMetric{}, &model.GroupHealthHalfHourMetric{}, &model.ModelPermission{}))
	ctx := context.Background()
	start := time.Unix(1800000000, 0).Truncate(time.Hour)
	hourly := newCollector(model.SaveGroupHealthSnapshots)
	half := newCollector(model.SaveGroupHealthHalfHourSnapshots, HalfHourBucketSeconds)
	for _, sample := range []Sample{
		{Group: "g", Model: "m", At: start.Add(29*time.Minute + 59*time.Second), Outcome: OutcomeSuccess},
		{Group: "g", Model: "m", At: start.Add(30 * time.Minute), Outcome: OutcomeServiceFailure},
		{Group: "g", Model: "m", At: start.Add(30 * time.Minute), Probe: true, Outcome: OutcomeSuccess},
		{Group: "g", Model: "m", At: start.Add(31 * time.Minute), Probe: true, Outcome: OutcomeServiceFailure},
	} {
		hourly.record(sample)
		half.record(sample)
	}
	require.Len(t, hourly.buckets, 1)
	require.Len(t, half.buckets, 2)
	now := start.Add(45 * time.Minute)
	for i := 0; i < 2; i++ {
		require.NoError(t, hourly.flush(ctx, now))
		require.NoError(t, half.flush(ctx, now))
	}
	for _, hours := range []int{24, 72} {
		result, err := QueryWithBucket(ctx, []string{"g"}, "m", common.RoleCommonUser, now, hours, HalfHourBucketSeconds)
		require.NoError(t, err)
		require.Equal(t, hours, result.WindowHours)
		require.EqualValues(t, 1800, result.BucketSeconds)
		series := result.Groups[0].Series
		require.Len(t, series, hours*2)
		for i := 1; i < len(series); i++ {
			require.EqualValues(t, 1800, series[i].Ts-series[i-1].Ts)
		}
		require.EqualValues(t, 2, result.Groups[0].RequestCount)
		require.EqualValues(t, 1, series[len(series)-2].SuccessCount)
		require.EqualValues(t, 1, series[len(series)-1].FailureCount)
		require.EqualValues(t, 1, series[len(series)-1].ProbeBucketCount)
		require.Equal(t, 50.0, *series[len(series)-1].UptimeRate)
		require.Nil(t, series[0].UptimeRate)
	}
	legacy, err := Query(ctx, []string{"g"}, "m", common.RoleCommonUser, now)
	require.NoError(t, err)
	require.Len(t, legacy.Groups[0].Series, 24)
	require.EqualValues(t, 2, legacy.Groups[0].RequestCount)
	require.NoError(t, model.SaveGroupHealthSnapshots(ctx, []model.GroupHealthMetric{{ID: "old", GroupName: "g", ModelName: "m", BucketTs: start.Add(-time.Hour).Unix(), RequestCount: 100, SuccessCount: 100}}))
	halfResult, err := QueryWithBucket(ctx, []string{"g"}, "m", common.RoleCommonUser, now, 24, 1800)
	require.NoError(t, err)
	require.EqualValues(t, 2, halfResult.Groups[0].RequestCount)
	_, err = QueryWithBucket(ctx, nil, "m", common.RoleCommonUser, now, 24, 900)
	require.Error(t, err)
	require.NoError(t, model.DeleteExpiredGroupHealthHalfHourMetrics(ctx, now.Add(8*24*time.Hour)))
	var count int64
	require.NoError(t, db.Model(&model.GroupHealthHalfHourMetric{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&model.GroupHealthMetric{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}
