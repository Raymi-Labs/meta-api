package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGroupUsageTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
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
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
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
