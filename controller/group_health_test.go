package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGroupHealthUsesCurrentMembershipAndModelPermissions(t *testing.T) {
	previousDB := model.DB
	previousGroups := setting.UserUsableGroups2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() {
		model.DB = previousDB
		model.InvalidateModelPermissionCache()
		_ = setting.UpdateUserUsableGroupsByJSONString(previousGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ModelPermission{}, &model.GroupHealthMetric{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public","auto":"Auto","removed":"Removed"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"public":1,"own":1,"private":1,"auto":1}`))
	user := model.User{Id: 1, Username: "health-reader", Group: "own", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	permission := model.ModelPermission{ModelName: "hidden", NameRule: model.NameRuleExact, VisibilityScope: model.ModelPermissionScopeAdminOnly, CallScope: model.ModelPermissionScopeAll}
	require.NoError(t, permission.Insert())
	now := time.Now()
	ts := now.Unix() / 3600 * 3600
	require.NoError(t, model.SaveGroupHealthSnapshots(context.Background(), []model.GroupHealthMetric{
		{ID: "visible", GroupName: "public", ModelName: "normal", BucketTs: ts, RequestCount: 10, SuccessCount: 7, ServiceFailureCount: 1, ClientCanceledCount: 1, ExcludedUserErrorCount: 2},
		{ID: "excluded-only", GroupName: "own", ModelName: "normal", BucketTs: ts, ExcludedUserErrorCount: 3},
		{ID: "hidden", GroupName: "public", ModelName: "hidden", BucketTs: ts, RequestCount: 1000, SuccessCount: 500, ServiceFailureCount: 200, ClientCanceledCount: 100, ExcludedUserErrorCount: 100},
		{ID: "private", GroupName: "private", ModelName: "normal", BucketTs: ts, RequestCount: 500, SuccessCount: 200, ServiceFailureCount: 100, ClientCanceledCount: 100, ExcludedUserErrorCount: 100},
	}))
	request := func(query string, id int) (*httptest.ResponseRecorder, grouphealth.Result) {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/health/groups"+query, nil)
		c.Set("id", id)
		c.Set("role", common.RoleRootUser) // stale elevated session must not widen visibility
		GetGroupHealth(c)
		var body struct {
			Data grouphealth.Result `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
		return w, body.Data
	}
	w, data := request("", 1)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	require.Len(t, data.Groups, 2)
	require.Equal(t, "own", data.Groups[0].Group)
	require.Equal(t, "public", data.Groups[1].Group)
	require.EqualValues(t, 10, data.Groups[1].RequestCount, "all-model aggregate must exclude hidden model")
	require.Equal(t, grouphealth.WindowScope, data.WindowScope)
	public := data.Groups[1]
	require.EqualValues(t, 7, public.SuccessCount)
	require.EqualValues(t, 3, public.FailureCount)
	require.EqualValues(t, 1, public.ServiceFailureCount)
	require.EqualValues(t, 2, public.UnknownFailureCount)
	require.EqualValues(t, 1, public.ClientCanceledCount)
	require.EqualValues(t, 2, public.ExcludedUserErrorCount)
	require.EqualValues(t, 2, public.ExcludedCount)
	require.EqualValues(t, 12, public.ObservedCount)
	require.InDelta(t, 70, *public.SuccessRate, 0.001, "excluded user errors must not enter the denominator")
	for _, bucket := range public.Series {
		require.Equal(t, bucket.RequestCount, bucket.SuccessCount+bucket.ServiceFailureCount+bucket.UnknownFailureCount)
		require.Equal(t, bucket.ObservedCount, bucket.RequestCount+bucket.ExcludedCount)
		require.LessOrEqual(t, bucket.ClientCanceledCount, bucket.UnknownFailureCount)
	}
	require.Zero(t, data.Groups[0].RequestCount)
	require.EqualValues(t, 3, data.Groups[0].ObservedCount)
	require.EqualValues(t, 3, data.Groups[0].ExcludedCount)
	require.Nil(t, data.Groups[0].SuccessRate)
	require.Len(t, data.Groups[1].Series, 24)
	_, exact := request("?model=normal&group=public", 1)
	require.Len(t, exact.Groups, 1)
	require.Equal(t, public.Metrics, exact.Groups[0].Metrics, "exact model and permitted aggregate must agree")
	_, data = request("?group=private", 1)
	require.Empty(t, data.Groups)
	_, data = request("?model=hidden", 1)
	require.Empty(t, data.Groups)
	w, _ = request("", 0)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("status", common.UserStatusDisabled).Error)
	w, _ = request("", 1)
	require.Equal(t, http.StatusForbidden, w.Code)
}
