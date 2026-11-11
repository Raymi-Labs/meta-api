package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	taskvertex "github.com/QuantumNous/new-api/relay/channel/task/vertex"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type videoLookupAdaptor struct {
	taskvertex.TaskAdaptor
	fetch func(string, string, map[string]any, string) (*http.Response, error)
}

func (a *videoLookupAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	return a.fetch(baseURL, key, body, proxy)
}

func videoTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 42)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_vertex/content", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_vertex"}}
	return ctx, recorder
}

func TestVertexVideoProxyServesStoredBase64(t *testing.T) {
	db := useControllerCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	channel := &model.Channel{Id: 993501, Type: constant.ChannelTypeVertexAi, Key: `{"project_id":"test"}`, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	for _, payload := range []string{
		`{"response":{"videos":[{"bytesBase64Encoded":"dmlkZW8=","mimeType":"video/mp4"}]}}`,
		`{"response":{"bytesBase64Encoded":"dmlkZW8","encoding":"webm"}}`,
		`{"response":{"video":"data:video/mp4;base64,dmlkZW8="}}`,
	} {
		task := &model.Task{TaskID: "task_vertex", UserId: 42, ChannelId: channel.Id, Status: model.TaskStatusSuccess, Data: []byte(payload)}
		require.NoError(t, db.Create(task).Error)
		ctx, recorder := videoTestContext()
		VideoProxy(ctx)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, "video", recorder.Body.String())
		require.Contains(t, recorder.Header().Get("Content-Type"), "video/")
		require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
		require.NoError(t, db.Delete(task).Error)
	}
}

func TestVertexVideoLookupRefetchesRedactedResultWithOriginalKey(t *testing.T) {
	enableControllerCapacityForTest(t)
	channel := &model.Channel{Id: 993502, Type: constant.ChannelTypeVertexAi, Key: `[{"project_id":"wrong"},{"project_id":"right"}]`}
	task := &model.Task{TaskID: "task_vertex", Status: model.TaskStatusSuccess, Data: []byte(`{"done":true,"response":{"video":"truncated..."}}`), PrivateData: model.TaskPrivateData{Key: `{"project_id":"right"}`, UpstreamTaskID: "upstream-op", ResultURL: "https://example.test/v1/videos/task_vertex/content?download=1"}}
	calls := 0
	adaptor := &videoLookupAdaptor{fetch: func(_ string, key string, body map[string]any, _ string) (*http.Response, error) {
		calls++
		require.Equal(t, task.PrivateData.Key, key)
		require.Equal(t, "upstream-op", body["task_id"])
		inflight, err := middleware.QueryChannelConcurrency(channel.Id)
		require.NoError(t, err)
		require.EqualValues(t, 1, inflight)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"done":true,"response":{"videos":[{"bytesBase64Encoded":"dmlkZW8="}]}}`))}, nil
	}}
	ctx, _ := videoTestContext()
	result, err := getVertexVideoURL(ctx, channel, task, adaptor)
	require.NoError(t, err)
	require.Equal(t, "data:video/mp4;base64,dmlkZW8=", result)
	require.Equal(t, 1, calls)
	inflight, err := middleware.QueryChannelConcurrency(channel.Id)
	require.NoError(t, err)
	require.Zero(t, inflight)
}

func TestVertexVideoLookupRejectsErrorsAndRecordsCooldown(t *testing.T) {
	for i, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate limited", 429, `{}`}, {"server error", 500, `{}`},
		{"still running", 200, `{"done":false,"response":{"video":"dmlkZW8="}}`},
		{"failed", 200, `{"done":true,"error":{"message":"blocked"},"response":{"video":"dmlkZW8="}}`},
		{"malformed", 200, `{"done":true,"response":{"video":"invalid..."}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := videoTestContext()
			channel := &model.Channel{Id: 993510 + i, Type: constant.ChannelTypeVertexAi, Key: `{"project_id":"test"}`}
			adaptor := &videoLookupAdaptor{fetch: func(string, string, map[string]any, string) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": []string{"7"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}}
			result, err := getVertexVideoURL(ctx, channel, &model.Task{TaskID: "task_vertex"}, adaptor)
			require.Error(t, err)
			require.Empty(t, result)
			if tc.status == 429 {
				var apiErr *types.NewAPIError
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
				require.GreaterOrEqual(t, service.ChannelRateLimitCooldownRemaining(channel.Id), 6*time.Second)
			}
		})
	}
}

func TestVertexVideoKeyAndURLFallbacks(t *testing.T) {
	prettyCredential := "{\n  \"project_id\": \"single-project\"\n}"
	require.Equal(t, prettyCredential, getVertexTaskKey(&model.Channel{Key: prettyCredential}, &model.Task{}))
	require.Equal(t, `{"project_id":"first"}`, getVertexTaskKey(&model.Channel{Key: `[{"project_id":"first"},{"project_id":"second"}]`}, &model.Task{}))
	require.True(t, isTaskProxyContentURL("/v1/videos/task_vertex/content/", "task_vertex"))
	require.False(t, isTaskProxyContentURL("https://cdn.test/video.mp4?next=/v1/videos/task_vertex/content", "task_vertex"))
	ctx, _ := videoTestContext()
	channel := &model.Channel{Type: constant.ChannelTypeVertexAi, Key: "key"}
	adaptor := &videoLookupAdaptor{fetch: func(string, string, map[string]any, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"done":true,"response":{"video":"https://cdn.test/video.mp4"}}`))}, nil
	}}
	result, err := getVertexVideoURL(ctx, channel, &model.Task{TaskID: "task_vertex"}, adaptor)
	require.NoError(t, err)
	require.Equal(t, "https://cdn.test/video.mp4", result)
	for _, raw := range []string{"data:video/mp4;base64,broken...", "data:text/html;base64,PGh0bWw+", "data:video/mp4,video", "data:video/mp4;base64,"} {
		_, _, err := decodeVideoDataURL(raw)
		require.Error(t, err, fmt.Sprintf("should reject %q", raw))
	}
}
