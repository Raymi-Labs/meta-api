package model

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTokenRoutingWhitespaceIsNotLegacy(t *testing.T) {
	config, err := ParseTokenRoutingConfig("")
	require.NoError(t, err)
	require.Nil(t, config)
	for _, raw := range []string{" ", "\t\n", "null"} {
		config, err = ParseTokenRoutingConfig(raw)
		require.Error(t, err)
		require.Nil(t, config)
	}
}

func TestTokenRoutingMigration(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var dialector gorm.Dialector
			switch engine {
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				dialector = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				dialector = postgres.Open(dsn)
			default:
				dialector = sqlite.Open(":memory:")
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			table := fmt.Sprintf("token_route_test_%d", time.Now().UnixNano())
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(table)) })
			// A private table with the exact Token schema, minus the new column,
			// models the old version without touching application tables.
			require.NoError(t, db.Table(table).AutoMigrate(&Token{}))
			require.NoError(t, db.Table(table).Migrator().DropColumn(&Token{}, "routing_config"))
			old := Token{Key: "legacy-test-key", Group: "default", RemainQuota: 12345, UsedQuota: 678, AutoGroupFallback: true}
			require.NoError(t, db.Table(table).Omit("routing_config").Create(&old).Error)
			for range 2 {
				require.NoError(t, db.Table(table).AutoMigrate(&Token{}))
			}
			var restored Token
			require.NoError(t, db.Table(table).First(&restored, old.Id).Error)
			require.Empty(t, restored.RoutingConfig)
			require.Equal(t, old.RemainQuota, restored.RemainQuota)
			require.Equal(t, old.UsedQuota, restored.UsedQuota)
			require.True(t, restored.AutoGroupFallback)
			config := `{"groups":["default","vip"],"model_groups":{"模型-A":["vip"]}}`
			require.NoError(t, db.Table(table).Where("id = ?", old.Id).Update("routing_config", config).Error)
			require.NoError(t, db.Table(table).First(&restored, old.Id).Error)
			require.Equal(t, config, restored.RoutingConfig)
			require.NoError(t, db.Table(table).Where("id = ?", old.Id).Update("routing_config", "").Error)
			require.NoError(t, db.Table(table).First(&restored, old.Id).Error)
			require.Empty(t, restored.RoutingConfig)
		})
	}
}
