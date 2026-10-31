package model

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestGroupHealthProbeHistoryDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(":memory:")
			switch dialect {
			case "mysql":
				if os.Getenv("TEST_MYSQL_DSN") == "" {
					t.Skip("disposable TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
			case "postgres":
				if os.Getenv("TEST_POSTGRES_DSN") == "" {
					t.Skip("disposable TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.Open(os.Getenv("TEST_POSTGRES_DSN"))
			}
			oldDB, oldLogDB, oldPG := DB, LOG_DB, common.UsingPostgreSQL
			common.UsingPostgreSQL = dialect == "postgres"
			initCol()
			t.Cleanup(func() { DB, LOG_DB, common.UsingPostgreSQL = oldDB, oldLogDB, oldPG; initCol() })
			prefix := fmt.Sprintf("probe_history_%d_", time.Now().UnixNano())
			var err error
			DB, err = gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: prefix + "main_"}})
			require.NoError(t, err)
			LOG_DB, err = gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: prefix + "log_"}})
			require.NoError(t, err)
			mainDB, logDB := DB, LOG_DB
			t.Cleanup(func() {
				require.NoError(t, mainDB.Migrator().DropTable(&Ability{}, &GroupHealthMetric{}))
				require.NoError(t, logDB.Migrator().DropTable(&Log{}))
				for _, db := range []*gorm.DB{mainDB, logDB} {
					conn, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, conn.Close())
				}
			})
			require.NoError(t, DB.AutoMigrate(&Ability{}, &GroupHealthMetric{}))
			require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
			now := time.Unix(1800000000+1800, 0)
			hour := now.Unix() / 3600 * 3600
			abilities := []Ability{
				{Group: "public", Model: "tested", ChannelId: 1},
				{Group: "public", Model: "other", ChannelId: 1},
				{Group: "paid", Model: "other", ChannelId: 1},
				{Group: "public", Model: "other", ChannelId: 2},
				{Group: "private", Model: "other", ChannelId: 3},
				{Group: "auto", Model: "other", ChannelId: 1},
			}
			require.NoError(t, DB.Create(&abilities).Error)
			addLog := func(channel int, ts int64, mutate func(*Log)) {
				row := Log{ChannelId: channel, CreatedAt: ts, Type: LogTypeConsume, ModelName: "tested", TokenName: "模型测试", Content: "模型测试", Group: "administrator"}
				if mutate != nil {
					mutate(&row)
				}
				require.NoError(t, LOG_DB.Create(&row).Error)
			}
			addLog(1, hour-3500, nil)
			addLog(2, hour-3400, nil)    // multiple successes still produce one evidence
			addLog(1, hour-71*3600, nil) // oldest visible bucket included
			addLog(1, hour-72*3600, nil) // expired evidence ignored
			addLog(1, now.Unix()+1, nil) // never read beyond captured cutoff
			addLog(1, hour+1, nil)       // current partial hour is included
			addLog(99, hour-3500, nil)   // deleted channel has no target
			addLog(3, hour-3500, func(row *Log) { row.Type = LogTypeError })
			addLog(3, hour-3500, func(row *Log) { row.TokenId = 42 })
			addLog(3, hour-3500, func(row *Log) { row.Content = "ordinary request" })
			addLog(3, hour-3500, func(row *Log) { row.TokenName = "ordinary token" })
			require.NoError(t, DB.Create(&GroupHealthMetric{ID: "real-failure", GroupName: "public", ModelName: "other", BucketTs: hour - 3600, RequestCount: 1, ServiceFailureCount: 1, ProbeCount: 2}).Error)
			require.NoError(t, DB.Create(&GroupHealthMetric{ID: "live-pass", GroupName: "public", ModelName: "tested", BucketTs: hour - 3600, ProbeCount: 4, ProbeSuccessCount: 2}).Error)
			count, err := BackfillGroupHealthProbeHistory(context.Background(), now)
			require.NoError(t, err)
			require.EqualValues(t, 8, count)
			count, err = BackfillGroupHealthProbeHistory(context.Background(), now)
			require.NoError(t, err)
			require.Zero(t, count, "restarts and retries never duplicate evidence")
			rows, err := QueryGroupHealthMetrics(context.Background(), []string{"public", "paid", "private", "auto", "administrator"}, "", hour-72*3600, now.Unix()+3600)
			require.NoError(t, err)
			require.Len(t, rows, 9)
			for _, row := range rows {
				require.Contains(t, []string{"public", "paid"}, row.GroupName)
				require.GreaterOrEqual(t, row.BucketTs, hour-71*3600)
				require.LessOrEqual(t, row.BucketTs, hour)
				require.Zero(t, row.UsageSampleCount)
				require.Zero(t, row.TTFTCount)
				if row.GroupName == "public" && row.ModelName == "other" && row.BucketTs == hour-3600 {
					require.EqualValues(t, 1, row.RequestCount)
					require.EqualValues(t, 1, row.ServiceFailureCount)
					require.EqualValues(t, 3, row.ProbeCount)
					require.EqualValues(t, 1, row.ProbeSuccessCount)
				} else if row.GroupName == "public" && row.ModelName == "tested" && row.BucketTs == hour-3600 {
					require.EqualValues(t, 4, row.ProbeCount, "already collected passes aren't added again")
					require.EqualValues(t, 2, row.ProbeSuccessCount)
				} else {
					require.Zero(t, row.RequestCount)
					require.EqualValues(t, 1, row.ProbeCount)
					require.EqualValues(t, 1, row.ProbeSuccessCount)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err = BackfillGroupHealthProbeHistory(ctx, now)
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}
