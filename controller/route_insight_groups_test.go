package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type routeInsightGroupsBody struct {
	Data struct {
		Hours         int                    `json:"hours"`
		SnapshotFresh bool                   `json:"snapshot_fresh"`
		Groups        []routeInsightGroupRow `json:"groups"`
	} `json:"data"`
}

func TestRouteInsightGroupsSumAllGroupsAndAttributeUnhealthyChannels(t *testing.T) {
	previousDB := model.DB
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousCache := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousCache
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.GroupHealthMetric{}, &model.GroupHealthHalfHourMetric{}))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"a":1,"b":1,"idle":1,"auto":1}`))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Name: "ch-one", Group: "a,b", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 2, Name: "ch-two", Group: "a", Status: common.ChannelStatusManuallyDisabled}).Error)

	now := time.Now()
	ts := now.Unix() / 1800 * 1800
	require.NoError(t, model.SaveGroupHealthHalfHourSnapshots(context.Background(), []model.GroupHealthMetric{
		{ID: "a-m1", GroupName: "a", ModelName: "m1", BucketTs: ts, RequestCount: 10, SuccessCount: 8, ServiceFailureCount: 1, ExcludedUserErrorCount: 5, TTFTSumMs: 3000, TTFTCount: 3},
		{ID: "a-m2", GroupName: "a", ModelName: "m2", BucketTs: ts, RequestCount: 10, SuccessCount: 10},
		{ID: "a-replica", GroupName: "a", ModelName: "m1", BucketTs: ts, RequestCount: 5, SuccessCount: 5},
		{ID: "a-old", GroupName: "a", ModelName: "m1", BucketTs: ts - 3*3600, RequestCount: 100, SuccessCount: 0},
		{ID: "retired", GroupName: "retired", ModelName: "m1", BucketTs: ts, RequestCount: 2, SuccessCount: 2},
		{ID: "auto", GroupName: "auto", ModelName: "m1", BucketTs: ts, RequestCount: 9, SuccessCount: 9},
	}))
	restore := routeinsight.SetHealthForTesting(
		routeinsight.HealthEntry{ChannelID: 1, Model: "m1", Health: routeinsight.RouteHealth{Status: routeinsight.StatusUnhealthy, Reason: "low_success_rate"}},
		routeinsight.HealthEntry{ChannelID: 1, Model: "m2", Health: routeinsight.RouteHealth{Status: routeinsight.StatusHealthy}},
		routeinsight.HealthEntry{ChannelID: 2, Model: "m1", Health: routeinsight.RouteHealth{Status: routeinsight.StatusUnhealthy, Reason: "slow_ttft"}},
	)
	t.Cleanup(restore)

	request := func(query string) routeInsightGroupsBody {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/route_insight/groups"+query, nil)
		GetRouteInsightGroups(c)
		require.Equal(t, http.StatusOK, w.Code)
		var body routeInsightGroupsBody
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
		return body
	}

	body := request("?hours=1")
	require.Equal(t, 1, body.Data.Hours)
	require.True(t, body.Data.SnapshotFresh)
	byName := map[string]routeInsightGroupRow{}
	order := make([]string, 0)
	for _, row := range body.Data.Groups {
		byName[row.Group] = row
		order = append(order, row.Group)
	}
	require.Equal(t, []string{"a", "retired", "b", "idle"}, order, "busiest first, then idle configured groups; auto never listed")
	a := byName["a"]
	require.True(t, a.Configured)
	require.EqualValues(t, 25, a.RequestCount, "models and replicas are summed; older buckets stay outside the window")
	require.EqualValues(t, 23, a.SuccessCount)
	require.EqualValues(t, 1, a.ServiceFailureCount)
	require.EqualValues(t, 1, a.UnknownFailureCount)
	require.EqualValues(t, 5, a.ExcludedUserErrorCount)
	require.InDelta(t, 92.0, *a.SuccessRate, 0.001)
	require.InDelta(t, 1000.0, *a.AvgTTFTMs, 0.001)
	require.Len(t, a.UnhealthyChannels, 1, "disabled channels and healthy routes are not listed")
	require.Equal(t, routeInsightUnhealthyChannel{ChannelID: 1, ChannelName: "ch-one", Models: []string{"m1"}, Reasons: []string{"low_success_rate"}}, a.UnhealthyChannels[0])
	require.Len(t, byName["b"].UnhealthyChannels, 1, "a channel's unhealthy route is skipped in every group it serves")
	require.EqualValues(t, 0, byName["b"].RequestCount)
	require.False(t, byName["retired"].Configured)
	require.NotNil(t, byName["idle"].UnhealthyChannels)
	require.Nil(t, byName["idle"].SuccessRate)

	require.EqualValues(t, 125, request("?hours=24").Data.Groups[0].RequestCount)
	require.Equal(t, 168, request("?hours=9999").Data.Hours, "the window is capped at group-health retention")
}
