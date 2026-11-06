package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRoutePlanNormalizeAuthorizeAndDeleteGuard(t *testing.T) {
	previous := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = previous
		InvalidateRoutePlanCache()
	})
	require.NoError(t, db.AutoMigrate(&RoutePlan{}, &Token{}))

	exists := func(group string) bool { return group == "pool-a" || group == "pool-b" }
	plan := &RoutePlan{Name: " 企业稳定 ", Groups: `["pool-a"," pool-a ","auto","pool-b"]`, SaleRatio: 1.5,
		ModelRatios: `{"gpt-x":2}`, UserIds: `[0,42]`, UserGroups: `["vip",""]`}
	require.NoError(t, plan.Normalize(exists))
	require.Equal(t, "企业稳定", plan.Name)
	require.Equal(t, `["pool-a","pool-b"]`, plan.Groups, "dedupe, trim and drop auto")
	require.Equal(t, `[42]`, plan.UserIds)

	bad := &RoutePlan{Name: "x", Groups: `["retired"]`, SaleRatio: 1}
	require.Error(t, bad.Normalize(exists), "a retired group cannot enter the pool")
	require.Error(t, (&RoutePlan{Name: "x", Groups: `[]`, SaleRatio: 1}).Normalize(exists))
	require.Error(t, (&RoutePlan{Name: "x", Groups: `["pool-a"]`, SaleRatio: -1}).Normalize(exists))

	require.True(t, plan.AuthorizedFor(42, "default"))
	require.True(t, plan.AuthorizedFor(7, "vip"))
	require.False(t, plan.AuthorizedFor(7, "default"), "no grant, no access")
	require.False(t, (&RoutePlan{Status: RoutePlanStatusEnabled}).AuthorizedFor(1, ""), "empty grants authorize nobody")

	require.NoError(t, CreateRoutePlan(plan))
	require.Equal(t, int64(1), plan.Version)
	cached, err := GetRoutePlanCached(plan.Id)
	require.NoError(t, err)
	snapshot, err := cached.Snapshot()
	require.NoError(t, err)
	require.Equal(t, []string{"pool-a", "pool-b"}, snapshot.Groups)
	require.Equal(t, 2.0, snapshot.RatioFor("gpt-x", ""))
	require.Equal(t, 1.5, snapshot.RatioFor("other", ""))

	plan.Status = RoutePlanStatusDisabled
	require.NoError(t, UpdateRoutePlan(plan))
	require.Equal(t, int64(2), plan.Version, "every edit is a new contract version")
	cached, err = GetRoutePlanCached(plan.Id)
	require.NoError(t, err)
	require.Equal(t, RoutePlanStatusDisabled, cached.Status, "local edits invalidate the cache")
	require.False(t, cached.AuthorizedFor(42, ""), "a disabled plan authorizes nobody")

	require.NoError(t, db.Create(&Token{Key: "k1", Name: "t", RoutePlanId: plan.Id}).Error)
	require.Error(t, DeleteRoutePlan(plan.Id), "tokens still reference the plan")
	counts, err := CountRoutePlanTokens()
	require.NoError(t, err)
	require.Equal(t, int64(1), counts[plan.Id])
	require.NoError(t, db.Model(&Token{}).Where("key = ?", "k1").Update("route_plan_id", 0).Error)
	require.NoError(t, DeleteRoutePlan(plan.Id))
	_, err = GetRoutePlanCached(plan.Id)
	require.ErrorIs(t, err, ErrRoutePlanNotFound)
}
