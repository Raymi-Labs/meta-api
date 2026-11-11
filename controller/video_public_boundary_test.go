package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoPublicHeadersAndOwnership(t *testing.T) {
	db := useControllerCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	service.InitHttpClient()
	setting := system_setting.GetFetchSetting()
	old := setting.EnableSSRFProtection
	setting.EnableSSRFProtection = false
	t.Cleanup(func() { setting.EnableSSRFProtection = old })
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "video/mp4")
		if r.URL.Path == "/json" {
			w.Header().Set("Content-Type", "application/json")
		}
		for _, name := range []string{"Location", "Content-Location", "Link", "Server", "Set-Cookie", "X-Upstream", "Content-Disposition"} {
			w.Header().Set(name, "https://private-provider.example/key=secret")
		}
		io.WriteString(w, "test-video")
	}))
	defer upstream.Close()
	ch := &model.Channel{Id: 993880, Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL}
	require.NoError(t, db.Create(ch).Error)
	task := &model.Task{TaskID: "task_private", UserId: 42, ChannelId: ch.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: upstream.URL + "/content"}}
	require.NoError(t, db.Create(task).Error)
	for _, tc := range []struct {
		name               string
		user, role, status int
	}{{"owner", 42, common.RoleCommonUser, 200}, {"other", 43, common.RoleCommonUser, 404}, {"anonymous", 0, 0, 404}, {"admin", 1, common.RoleAdminUser, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			c, w := videoTestContext()
			c.Set("id", tc.user)
			c.Set("role", tc.role)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			before := calls.Load()
			VideoProxy(c)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.NotContains(t, w.Header(), "Set-Cookie")
			h, _ := json.Marshal(w.Header())
			require.NotContains(t, string(h), "private-provider")
			require.NotContains(t, string(h), "secret")
			if tc.status == 200 {
				require.Equal(t, "test-video", w.Body.String())
				require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
				require.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
			} else {
				require.Equal(t, before, calls.Load())
			}
		})
	}

	// A JSON error returned with HTTP 200 must not be streamed as a video.
	task.PrivateData.ResultURL = upstream.URL + "/json"
	require.NoError(t, db.Save(task).Error)
	c, w := videoTestContext()
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	VideoProxy(c)
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.NotContains(t, w.Body.String(), "test-video")
	require.NotContains(t, w.Header().Get("Location"), "private-provider")
}

func TestUserVideoListProjection(t *testing.T) {
	task := &model.Task{TaskID: "task_public", Platform: constant.TaskPlatform(fmt.Sprint(constant.ChannelTypeDoubaoVideo)), Status: model.TaskStatusSuccess,
		FailReason: "https://private-provider.example/legacy", Data: []byte(`{"metadata":{"url":"https://private-provider.example"},"nested":"secret-key"}`),
		Properties: model.Properties{UpstreamModelName: "private-provider.example", OriginModelName: "public-model"}}
	body, err := json.Marshal(tasksToDto([]*model.Task{task}, false))
	require.NoError(t, err)
	require.NotContains(t, string(body), "private-provider")
	require.NotContains(t, string(body), "secret-key")
	require.Contains(t, string(body), "/v1/videos/task_public/content")
	require.Contains(t, string(task.Data), "private-provider")
}

func TestVideoRequestErrorHidesProvider(t *testing.T) {
	for _, path := range []string{"/v1/videos", "/v1/video/generations", "/kling/v1/videos/text2video", "/jimeng/"} {
		c, w := videoTestContext()
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		err := &dto.TaskError{Code: "task_failed", LocalError: true, Error: fmt.Errorf("private-provider.example secret"), Message: "https://private-provider.example/key=secret", Data: map[string]any{"debug": "secret"}, StatusCode: 502}
		respondTaskError(c, err)
		require.Equal(t, 502, w.Code)
		require.NotContains(t, w.Body.String(), "private-provider")
		require.NotContains(t, w.Body.String(), "secret")
		require.Contains(t, err.Message, "private-provider")
	}
}
