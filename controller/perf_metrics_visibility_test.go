package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perf "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPublicMetricsFilterBeforeAggregation(t *testing.T) {
	db := setupModelSyncTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}, &model.User{}, &model.ModelPermission{}))
	oldUsable, oldRatio := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(oldUsable)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatio)
		model.InvalidateModelPermissionCache()
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"public"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"private":2}`))
	model.InvalidateModelPermissionCache()
	name := "sample-" + t.Name() + time.Now().Format("150405.000000")
	for _, g := range []string{"default", "private"} {
		require.NoError(t, db.Create(&model.PerfMetric{ModelName: name, Group: g, BucketTs: time.Now().Add(-time.Hour).Unix(), RequestCount: 3}).Error)
		perf.Record(perf.Sample{Model: name, Group: g, Success: true})
	}
	view := func(handler gin.HandlerFunc, query string, id int) []byte {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/perf-metrics"+query, nil)
		if id > 0 {
			c.Set("id", id)
		}
		handler(c)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Header().Get("Cache-Control"), "private")
		return w.Body.Bytes()
	}
	summary := func(handler gin.HandlerFunc, id int) int64 {
		var r struct{ Data perf.SummaryAllResult }
		require.NoError(t, common.Unmarshal(view(handler, "", id), &r))
		for _, entry := range r.Data.Models {
			if entry.ModelName == name {
				return entry.RequestCount
			}
		}
		return 0
	}
	require.EqualValues(t, 4, summary(GetPerfMetricsSummary, 0))
	admin := model.User{Username: "metrics-admin", Group: "default", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AffCode: "metrics-admin"}
	require.NoError(t, db.Create(&admin).Error)
	require.EqualValues(t, 8, summary(GetDashboardPerfMetricsSummary, admin.Id))
	require.NoError(t, db.Model(&admin).Update("role", common.RoleCommonUser).Error)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", admin.Id)
	c.Set("role", common.RoleAdminUser)
	GetDashboardPerfMetricsSummary(c)
	require.Equal(t, 403, w.Code)
	require.NotContains(t, w.Body.String(), name)
	user := model.User{Username: "metrics", Group: "private", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	require.EqualValues(t, 8, summary(GetPerfMetricsSummary, user.Id))
	require.NotContains(t, string(view(GetPerfMetrics, "?model="+name, 0)), "private")
	require.Contains(t, string(view(GetPerfMetrics, "?model="+name, user.Id)), "private")
	require.NotContains(t, string(view(GetPerfMetrics, "?model="+name+"&group=private", 0)), "private")
	require.NoError(t, db.Model(&user).Update("status", common.UserStatusDisabled).Error)
	require.EqualValues(t, 4, summary(GetPerfMetricsSummary, user.Id))
}
