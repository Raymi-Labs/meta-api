package grouphealth

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Display permissions may be configured for a concrete model ID even when
// routing/ratio lookup normalizes it to a wildcard model family.
func TestHealthReviewExactHiddenModelSurvivesRoutingNormalization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelPermission{}, &model.GroupHealthMetric{}))
	previousDB := model.DB
	model.DB = db
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() {
		model.DB = previousDB
		model.InvalidateModelPermissionCache()
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	name := "gpt-4-gizmo-private"
	require.NoError(t, (&model.ModelPermission{ModelName: name, NameRule: model.NameRuleExact,
		VisibilityScope: model.ModelPermissionScopeAdminOnly, CallScope: model.ModelPermissionScopeAll}).Insert())
	now := time.Now()
	require.NoError(t, model.SaveGroupHealthSnapshots(context.Background(), []model.GroupHealthMetric{{
		ID: "private-metric", GroupName: "public", ModelName: name,
		BucketTs: now.Unix() / BucketSeconds * BucketSeconds, RequestCount: 999, SuccessCount: 999,
	}}))
	require.Empty(t, model.FilterPricingByVisibility([]model.Pricing{{ModelName: name}}, common.RoleCommonUser))
	t.Run("batch", func(t *testing.T) {
		result, err := QueryModels(context.Background(), []string{"public"}, []string{name}, common.RoleCommonUser, 72, now)
		require.NoError(t, err)
		require.Empty(t, result.Models, "raw model visibility must not be bypassed by routing-name normalization")
	})
	t.Run("all-model aggregate", func(t *testing.T) {
		result, err := Query(context.Background(), []string{"public"}, "", common.RoleCommonUser, now, 72)
		require.NoError(t, err)
		require.Len(t, result.Groups, 1)
		require.Zero(t, result.Groups[0].RequestCount, "hidden concrete model must not leak through an aggregate")
	})
}
