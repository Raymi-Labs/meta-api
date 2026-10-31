package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestModelHealthBatchIsBoundedPrivateAndUsesCurrentPermissions(t *testing.T) {
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
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"public":1,"own":1,"private":1}`))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "metrics-reader", Group: "own", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, (&model.ModelPermission{ModelName: "hidden", NameRule: model.NameRuleExact, VisibilityScope: model.ModelPermissionScopeAdminOnly, CallScope: model.ModelPermissionScopeAll}).Insert())
	now := time.Now().Unix() / 3600 * 3600
	name := "org/model+edition"
	require.NoError(t, model.SaveGroupHealthSnapshots(context.Background(), []model.GroupHealthMetric{
		{ID: "public", GroupName: "public", ModelName: name, BucketTs: now, ProbeCount: 2, ProbeSuccessCount: 1, RequestCount: 1, SuccessCount: 1, TTFTCount: 1, TTFTSumMs: 500, InputTokens: 100, OutputTokens: 30, UsageSampleCount: 1, ThroughputOutputTokens: 30, GenerationSumMs: 1000, ThroughputSampleCount: 1},
		{ID: "probe-now", GroupName: "own", ModelName: name, BucketTs: now, ProbeCount: 10, ProbeSuccessCount: 1},
		{ID: "probe-old", GroupName: "own", ModelName: name, BucketTs: now - 47*3600, ProbeCount: 9},
		{ID: "earlier", GroupName: "own", ModelName: name, BucketTs: now - 48*3600, RequestCount: 2, SuccessCount: 2, TTFTCount: 2, TTFTSumMs: 2500},
		{ID: "private", ProbeCount: 999, ProbeSuccessCount: 999, GroupName: "private", ModelName: name, BucketTs: now, RequestCount: 999, SuccessCount: 999, TTFTCount: 999, TTFTSumMs: 999000, UsageSampleCount: 999, InputTokens: 999999},
		{ID: "hidden", ProbeCount: 999, ProbeSuccessCount: 999, GroupName: "public", ModelName: "hidden", BucketTs: now, RequestCount: 100, SuccessCount: 100},
	}))
	request := func(query string, id int, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/health/models?"+query, nil)
		c.Set("id", id)
		c.Set("role", common.RoleRootUser)
		handler(c)
		return w
	}
	decode := func(w *httptest.ResponseRecorder) grouphealth.ModelResult {
		t.Helper()
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		var body struct {
			Data grouphealth.ModelResult `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
		return body.Data
	}
	query := url.Values{"model": {name, "hidden", name, "missing"}, "hours": {"24"}, "group": {"private"}, "user_id": {"999"}}.Encode()
	result := decode(request(query, 1, GetModelHealthSummaries))
	require.Len(t, result.Models, 2)
	require.Equal(t, name, result.Models[0].Model)
	require.EqualValues(t, 1, result.Models[0].RequestCount)
	require.EqualValues(t, 12, result.Models[0].ProbeCount)
	require.EqualValues(t, 2, result.Models[0].ProbeBucketCount)
	require.Equal(t, 100.0, *result.Models[0].UptimeRate)
	require.Equal(t, 500.0, *result.Models[0].AvgTTFTMs)
	require.Equal(t, 30.0, *result.Models[0].AvgOutputTPS)
	require.EqualValues(t, 130, *result.Models[0].TotalTokens)
	require.Equal(t, "missing", result.Models[1].Model)
	require.Nil(t, result.Models[1].AvgTTFTMs)
	require.Nil(t, result.Models[1].TotalTokens)
	result = decode(request(strings.Replace(query, "hours=24", "hours=72", 1), 1, GetModelHealthSummaries))
	require.Equal(t, 72, result.WindowHours)
	require.EqualValues(t, 3, result.Models[0].RequestCount)
	require.EqualValues(t, 21, result.Models[0].ProbeCount)
	require.EqualValues(t, 3, result.Models[0].ProbeBucketCount)
	require.InDelta(t, 500.0/6, *result.Models[0].UptimeRate, 1e-9)
	require.Equal(t, 1000.0, *result.Models[0].AvgTTFTMs)
	for _, invalid := range []string{"", "model=", "model=" + strings.Repeat("x", 256), "model=m&hours=1", "model=m&hours=720", "model=m&hours=abc", strings.TrimSuffix(strings.Repeat("model=m&", 101), "&")} {
		require.Equal(t, http.StatusBadRequest, request(invalid, 1, GetModelHealthSummaries).Code, invalid)
	}
	require.Equal(t, http.StatusUnauthorized, request(query, 0, GetModelHealthSummaries).Code)
	require.Equal(t, http.StatusBadRequest, request("hours=48", 1, GetGroupHealth).Code)
	group := request("model="+url.QueryEscape(name)+"&hours=72", 1, GetGroupHealth)
	require.Equal(t, http.StatusOK, group.Code)
	var groupBody struct {
		Data grouphealth.Result `json:"data"`
	}
	require.NoError(t, common.Unmarshal(group.Body.Bytes(), &groupBody))
	require.Equal(t, 72, groupBody.Data.WindowHours)
	var samples, probes int64
	for _, g := range groupBody.Data.Groups {
		require.Len(t, g.Series, 72)
		samples += g.UptimeSampleCount
		probes += g.ProbeBucketCount
	}
	require.Equal(t, result.Models[0].UptimeSampleCount, samples)
	require.Equal(t, result.Models[0].ProbeBucketCount, probes)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("role", common.RoleAdminUser).Error)
	admin := decode(request(query, 1, GetModelHealthSummaries))
	require.Len(t, admin.Models, 3, "current admin can view the hidden model but not private groups")
	require.EqualValues(t, 1, admin.Models[0].RequestCount)
	require.Equal(t, "hidden", admin.Models[1].Model)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("status", common.UserStatusDisabled).Error)
	require.Equal(t, http.StatusForbidden, request(query, 1, GetModelHealthSummaries).Code)
}
