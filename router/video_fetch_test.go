package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoFetchRoutesReturnOwnedTaskWithoutGeneration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis, oldMemory, oldSQLite := common.RedisEnabled, common.MemoryCacheEnabled, common.UsingSQLite
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, common.UsingSQLite = false, false, true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled, common.MemoryCacheEnabled, common.UsingSQLite = oldRedis, oldMemory, oldSQLite
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}, &model.Channel{}))
	owner := model.User{Id: 97291, Username: "video-fetch-owner", Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 100000, AffCode: "videoowner"}
	require.NoError(t, db.Create(&owner).Error)
	token := model.Token{UserId: owner.Id, Key: "videofetchregressionkey0123456789abcdefghijklmnopqrstuvwxyz", Name: "video-fetch", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100000}
	require.NoError(t, db.Create(&token).Error)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	ch := model.Channel{Id: 97292, Type: constant.ChannelTypeXai, Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL, Key: "private-channel-key"}
	require.NoError(t, db.Create(&ch).Error)
	resultURL := "https://example.invalid/generated-video.mp4"
	for _, status := range []model.TaskStatus{model.TaskStatusQueued, model.TaskStatusInProgress, model.TaskStatusSuccess, model.TaskStatusFailure} {
		task := model.Task{TaskID: "task_" + string(status), UserId: owner.Id, ChannelId: ch.Id,
			Platform: constant.TaskPlatform(strconv.Itoa(ch.Type)), Status: status, Progress: "100%",
			Properties:  model.Properties{OriginModelName: "grok-imagine-video"},
			PrivateData: model.TaskPrivateData{Key: "private-submission-key", UpstreamTaskID: "private-upstream-id"}}
		if status == model.TaskStatusSuccess {
			task.PrivateData.ResultURL = resultURL
		}
		require.NoError(t, db.Create(&task).Error)
	}
	foreign := model.Task{TaskID: "task_foreign", UserId: owner.Id + 1, ChannelId: ch.Id, Platform: constant.TaskPlatform(strconv.Itoa(ch.Type)), Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: resultURL}}
	require.NoError(t, db.Create(&foreign).Error)
	r := gin.New()
	SetVideoRouter(r)
	for _, prefix := range []string{"/v1/videos/", "/v1/video/generations/"} {
		for _, status := range []model.TaskStatus{model.TaskStatusQueued, model.TaskStatusInProgress, model.TaskStatusSuccess, model.TaskStatusFailure} {
			t.Run(prefix+string(status), func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, prefix+"task_"+string(status), nil)
				req.Header.Set("Authorization", "Bearer sk-"+token.Key)
				recorder := httptest.NewRecorder()
				require.NotPanics(t, func() { r.ServeHTTP(recorder, req) })
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				var response map[string]any
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				if prefix == "/v1/videos/" {
					require.Equal(t, "task_"+string(status), response["id"])
					require.Equal(t, status.ToVideoStatus(), response["status"])
					if status == model.TaskStatusSuccess {
						require.Equal(t, resultURL, response["metadata"].(map[string]any)["url"])
					}
				} else {
					require.Equal(t, "success", response["code"])
					data := response["data"].(map[string]any)
					require.Equal(t, "task_"+string(status), data["task_id"])
					require.Equal(t, string(status), data["status"])
					if status == model.TaskStatusSuccess {
						require.Equal(t, resultURL, data["result_url"])
					}
				}
				require.NotContains(t, recorder.Body.String(), "private-submission-key")
				require.NotContains(t, recorder.Body.String(), "private-upstream-id")
			})
		}
		for _, taskID := range []string{"task_foreign", "task_missing"} {
			t.Run(prefix+taskID, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, prefix+taskID, nil)
				req.Header.Set("Authorization", "Bearer sk-"+token.Key)
				recorder := httptest.NewRecorder()
				require.NotPanics(t, func() { r.ServeHTTP(recorder, req) })
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), "task_not_exist")
				require.NotContains(t, recorder.Body.String(), resultURL)
			})
		}
		t.Run(prefix+"unauthenticated", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, prefix+"task_SUCCESS", nil))
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.NotContains(t, recorder.Body.String(), resultURL)
		})
	}
	var savedUser model.User
	var savedToken model.Token
	require.NoError(t, db.First(&savedUser, owner.Id).Error)
	require.NoError(t, db.First(&savedToken, token.Id).Error)
	require.Equal(t, owner.Quota, savedUser.Quota)
	require.Equal(t, token.RemainQuota, savedToken.RemainQuota)
	require.Equal(t, token.UsedQuota, savedToken.UsedQuota)
	var taskCount int64
	require.NoError(t, db.Model(&model.Task{}).Count(&taskCount).Error)
	require.EqualValues(t, 5, taskCount)
	require.Zero(t, upstreamCalls.Load(), "status queries must not create or poll xAI tasks")
}
