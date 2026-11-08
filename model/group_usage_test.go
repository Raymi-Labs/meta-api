package model

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupGroupUsageTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	var dialector gorm.Dialector = sqlite.Open(":memory:")
	driver := os.Getenv("GROUP_USAGE_TEST_DRIVER")
	switch driver {
	case "mysql":
		dialector = mysql.Open(os.Getenv("GROUP_USAGE_TEST_DSN"))
	case "postgres":
		dialector = postgres.Open(os.Getenv("GROUP_USAGE_TEST_DSN"))
	case "":
	default:
		t.Fatalf("unsupported group usage test driver %q", driver)
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	originDB := DB
	originLogDB := LOG_DB
	originRedis := common.RedisEnabled
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL

	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = driver == ""
	common.UsingMySQL = driver == "mysql"
	common.UsingPostgreSQL = driver == "postgres"
	initCol()

	t.Cleanup(func() {
		DB = originDB
		LOG_DB = originLogDB
		common.RedisEnabled = originRedis
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		initCol()
	})

	if driver != "" {
		// Only a disposable database with this exact name may be reset by this suite.
		require.Equal(t, "meta_group_test", db.Migrator().CurrentDatabase())
		require.NoError(t, db.Migrator().DropTable(&User{}, &Token{}, &SubscriptionPlan{}, &Option{}))
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &SubscriptionPlan{}))
	return db
}

func seedGroupUsageUser(t *testing.T, db *gorm.DB, id int, group string) {
	t.Helper()
	require.NoError(t, db.Create(&User{
		Id:       id,
		Username: fmt.Sprintf("user_%d", id),
		Password: "password123",
		Group:    group,
		Status:   common.UserStatusEnabled,
		AffCode:  fmt.Sprintf("aff_%d", id),
	}).Error)
}

func TestCountUsersByGroupSkipsDeletedUsers(t *testing.T) {
	db := setupGroupUsageTestDB(t)
	seedGroupUsageUser(t, db, 1, "default")
	seedGroupUsageUser(t, db, 2, "default")
	seedGroupUsageUser(t, db, 3, "vip")
	seedGroupUsageUser(t, db, 4, "OpenAI · 蒸馏")
	require.NoError(t, db.Delete(&User{}, 2).Error)

	counts, err := CountUsersByGroup()
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"default": 1, "vip": 1, "OpenAI · 蒸馏": 1}, counts)
}

func TestCountSubscriptionPlansByUpgradeGroupSkipsEmpty(t *testing.T) {
	db := setupGroupUsageTestDB(t)
	for i, group := range []string{"vip", "vip", "svip", ""} {
		require.NoError(t, db.Create(&SubscriptionPlan{Id: i + 1, Title: fmt.Sprintf("plan %d", i), UpgradeGroup: group}).Error)
	}

	counts, err := CountSubscriptionPlansByUpgradeGroup()
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"vip": 2, "svip": 1}, counts)
}

func TestCountEnabledTokensByGroupUsesRoutingGroups(t *testing.T) {
	db := setupGroupUsageTestDB(t)
	tokens := []Token{
		{Id: 1, Key: "k1", Group: "OpenAI · 优质", Status: common.TokenStatusEnabled},
		{Id: 2, Key: "k2", Group: "vip", Status: common.TokenStatusEnabled},
		{Id: 3, Key: "k3", Group: "vip", Status: common.TokenStatusDisabled},
		{Id: 4, Key: "k4", Group: "", Status: common.TokenStatusEnabled},
		{Id: 5, Key: "k5", Group: "auto", Status: common.TokenStatusEnabled},
		// Multi-group tokens are counted by their routing groups, once each.
		{Id: 6, Key: "k6", Group: "OpenAI · 优质", Status: common.TokenStatusEnabled,
			RoutingConfig: `{"groups":["OpenAI · 优质","土豪组"],"model_groups":{"gpt-6-luna":["土豪组"]}}`},
	}
	for i := range tokens {
		require.NoError(t, db.Create(&tokens[i]).Error)
	}

	counts, err := CountEnabledTokensByGroup()
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"OpenAI · 优质": 2, "vip": 1, "土豪组": 1}, counts)
}

func TestReassignUserGroupMovesOnlyMatchingUsers(t *testing.T) {
	db := setupGroupUsageTestDB(t)
	seedGroupUsageUser(t, db, 1, "OpenAI · 蒸馏")
	seedGroupUsageUser(t, db, 2, "OpenAI · 蒸馏")
	seedGroupUsageUser(t, db, 3, "vip")

	invalidated := []int{}
	originInvalidator := userCacheInvalidator
	userCacheInvalidator = func(id int) error {
		invalidated = append(invalidated, id)
		return nil
	}
	t.Cleanup(func() { userCacheInvalidator = originInvalidator })

	ids, err := ReassignUserGroup("OpenAI · 蒸馏", "default")
	require.NoError(t, err)
	require.ElementsMatch(t, []int{1, 2}, ids)
	require.ElementsMatch(t, []int{1, 2}, invalidated)

	counts, err := CountUsersByGroup()
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"default": 2, "vip": 1}, counts)

	ids, err = ReassignUserGroup("missing", "default")
	require.NoError(t, err)
	require.Empty(t, ids)

	_, err = ReassignUserGroup("vip", "vip")
	require.Error(t, err)
}

func TestReassignUserGroupHandlesMoreUsersThanOneChunk(t *testing.T) {
	db := setupGroupUsageTestDB(t)
	for id := 1; id <= 1203; id++ {
		seedGroupUsageUser(t, db, id, "text")
	}

	ids, err := ReassignUserGroup("text", "default")
	require.NoError(t, err)
	require.Len(t, ids, 1203)

	counts, err := CountUsersByGroup()
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"default": 1203}, counts)
}

func TestReassignUserGroupRollsBackAllChunksOnFailure(t *testing.T) {
	db := setupGroupUsageTestDB(t)
	for id := 1; id <= 501; id++ {
		seedGroupUsageUser(t, db, id, "legacy")
	}
	old := userCacheInvalidator
	invalidations := 0
	userCacheInvalidator = func(int) error { invalidations++; return nil }
	t.Cleanup(func() { userCacheInvalidator = old })
	updates := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("audit_fail_second", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			updates++
			if updates == 2 {
				tx.AddError(errors.New("injected second chunk failure"))
			}
		}
	}))
	ids, err := ReassignUserGroup("legacy", "default")
	require.Error(t, err)
	counts, e := CountUsersByGroup()
	require.NoError(t, e)
	t.Logf("error=%v returned_ids=%d counts=%v cache_invalidations=%d", err, len(ids), counts, invalidations)
	require.Equal(t, int64(501), counts["legacy"], "failed bulk move should not leave committed partial changes")
	require.Empty(t, ids)
	require.Zero(t, invalidations)
}
