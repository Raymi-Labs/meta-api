package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoFetchDoesNotExposeProvider(t *testing.T) {
	db := useRelayCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	for _, platform := range []int{constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora} {
		for _, status := range []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure} {
			task := &model.Task{TaskID: fmt.Sprintf("task_public_%d_%s", platform, status), UserId: 42, ChannelId: 9901,
				Platform: constant.TaskPlatform(fmt.Sprint(platform)), Status: status, Progress: "100%",
				FailReason:  "provider-secret.example secret-key provider-task",
				Properties:  model.Properties{OriginModelName: "public-model", UpstreamModelName: "provider-secret.example"},
				PrivateData: model.TaskPrivateData{UpstreamTaskID: "provider-task", ResultURL: "https://provider-secret.example/content?key=secret-key"},
				Data:        json.RawMessage(`{"id":"provider-task","model":"provider-secret.example","status":"completed","seconds":"4","size":"1280x720","content":{"video_url":"https://provider-secret.example/content"},"metadata":{"url":"https://provider-secret.example/content","nested":{"key":"secret-key"}},"error":{"code":"provider-secret.example","message":"secret-key"},"extra":"provider-secret.example"}`)}
			require.NoError(t, db.Create(task).Error)
			for _, prefix := range []string{"/v1/videos/", "/v1/video/generations/"} {
				t.Run(fmt.Sprint(platform, status, prefix), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Set("id", 42)
					c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
					c.Request = httptest.NewRequest(http.MethodGet, prefix+task.TaskID, nil)
					body, err := videoFetchByIDRespBodyBuilder(c)
					require.Nil(t, err)
					for _, secret := range []string{"provider-secret.example", "secret-key", "provider-task"} {
						require.NotContains(t, string(body), secret)
					}
					if status == model.TaskStatusSuccess {
						require.Contains(t, string(body), "/v1/videos/"+task.TaskID+"/content")
					}
					var stored model.Task
					require.NoError(t, db.First(&stored, task.ID).Error)
					require.Equal(t, task.Data, stored.Data)
					require.Equal(t, task.PrivateData, stored.PrivateData)
				})
			}
		}
	}
}

func TestVideoRealtimeResponseDoesNotExposeProvider(t *testing.T) {
	db := useRelayCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	service.InitHttpClient()
	fetch := system_setting.GetFetchSetting()
	old := fetch.EnableSSRFProtection
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() { fetch.EnableSSRFProtection = old })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"name":"provider-task","done":true,"response":{"videos":[{"uri":"https://private-provider.example/result?key=secret-key","mimeType":"video/mp4"}]}}`)
	}))
	defer upstream.Close()
	ch := &model.Channel{Id: 9909, Type: constant.ChannelTypeGemini, Key: "secret-key", BaseURL: &upstream.URL}
	require.NoError(t, db.Create(ch).Error)
	for _, prefix := range []string{"/v1/videos/", "/v1/video/generations/"} {
		task := &model.Task{TaskID: "task_realtime", UserId: 42, ChannelId: ch.Id, Platform: constant.TaskPlatform(fmt.Sprint(ch.Type)), Status: model.TaskStatusSuccess, Progress: "100%", Properties: model.Properties{OriginModelName: "gemini-video"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "provider-task"}, Data: []byte(`{"done":true,"response":{"videos":[{"uri":"https://private-provider.example/result"}]}}`)}
		require.NoError(t, db.Create(task).Error)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("id", 42)
		c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
		c.Request = httptest.NewRequest(http.MethodGet, prefix+task.TaskID, nil)
		body, err := videoFetchByIDRespBodyBuilder(c)
		require.Nil(t, err)
		require.NotContains(t, string(body), "private-provider")
		require.NotContains(t, string(body), "secret-key")
		require.NotContains(t, string(body), "provider-task")
		require.Contains(t, string(body), "/v1/videos/task_realtime/content")
		require.NoError(t, db.Delete(task).Error)
	}
}

func TestSeedanceFailureConsistentAcrossPublicEntries(t *testing.T) {
	db := useRelayCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	for _, tc := range []struct{ status, code, want string }{
		{"failed", "InputImageSensitiveContentDetected.PrivacyInformation", "video_input_rejected"},
		{"failed", "OutputVideoSensitiveContentDetected", "video_output_rejected"},
		{"failed", "InvalidImageURL.InvalidFormat", "video_invalid_input"},
		{"failed", "OperationDenied.ServiceNotOpen", "video_access_unavailable"},
		{"failed", "unknown-secret-code", "video_generation_failed"},
		{"cancelled", "", "video_cancelled"}, {"expired", "", "video_expired"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			data, err := common.Marshal(map[string]any{"status": tc.status, "error": map[string]string{"code": tc.code, "message": "https://private.example/?token=secret bearer hidden"}})
			require.NoError(t, err)
			a := &doubao.TaskAdaptor{}
			parsed, err := a.ParseTaskResult(data)
			require.NoError(t, err)
			require.Contains(t, parsed.Reason, tc.want)
			require.NotContains(t, parsed.Reason, "private.example")
			require.NotContains(t, parsed.Reason, "secret")
			for _, platform := range []int{constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine} {
				task := &model.Task{TaskID: fmt.Sprintf("task_%d_%s", platform, tc.want), UserId: 42, Platform: constant.TaskPlatform(fmt.Sprint(platform)), Status: model.TaskStatusFailure, Data: data}
				require.NoError(t, db.Create(task).Error)
				for _, prefix := range []string{"/v1/videos/", "/v1/video/generations/"} {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Set("id", 42)
					c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
					c.Request = httptest.NewRequest(http.MethodGet, prefix+task.TaskID, nil)
					response, fetchErr := videoFetchByIDRespBodyBuilder(c)
					require.Nil(t, fetchErr)
					require.Contains(t, string(response), tc.want)
					require.NotContains(t, string(response), "secret")
				}
				converted, err := a.ConvertToOpenAIVideo(task)
				require.NoError(t, err)
				var v dto.OpenAIVideo
				require.NoError(t, common.Unmarshal(converted, &v))
				public := service.PublicVideo(&v, task.TaskID, "alias")
				require.Equal(t, tc.want, public.Error.Code)
				listed := PublicTaskModel2Dto(task)
				var listedVideo dto.OpenAIVideo
				require.NoError(t, common.Unmarshal(listed.Data, &listedVideo))
				require.Equal(t, public.Error, listedVideo.Error)
				require.Equal(t, public.Error.Message, listed.FailReason)
				wire, err := common.Marshal(listed)
				require.NoError(t, err)
				require.NotContains(t, string(wire), "private.example")
				require.NotContains(t, string(wire), "secret")
			}
		})
	}
}
