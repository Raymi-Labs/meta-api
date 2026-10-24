package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskRealtimeTerminalUsesOneRequestAndCapacitySlot(t *testing.T) {
	enableLocalChannelCapacityForTest(t)
	db := useRelayCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.User{}, &model.Token{}, &model.Log{}, &model.PulseFundingLedger{}))
	service.InitHttpClient()
	seedRelayUser(t, 7510, 100)
	seedRelayPackageToken(t, 8510, 7510, "realtime-task-token", 100, 200)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "saved-key", r.Header.Get("X-Goog-Api-Key"))
		require.Equal(t, "saved-key", r.Header.Get("X-Key"))
		require.Equal(t, "7510", r.Header.Get("X-Owner"))
		require.Empty(t, r.Header.Get("X-Client"))
		inflight, err := middleware.QueryChannelConcurrency(9510)
		require.NoError(t, err)
		require.EqualValues(t, 1, inflight)
		io.WriteString(w, `{"done":true,"error":{"message":"blocked"}}`)
	}))
	defer server.Close()
	settings := `{"max_concurrency":1,"rpm_limit":0}`
	overrides := `{"X-Owner":"{authenticated_user_id}","X-Key":"{api_key}","X-Client":"{client_header:X-Client}"}`
	ch := &model.Channel{Id: 9510, Type: constant.ChannelTypeGemini, Key: "wrong-current-key", BaseURL: &server.URL, Setting: &settings, HeaderOverride: &overrides, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(ch).Error)
	task := &model.Task{TaskID: "task_realtime_consistency", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)), UserId: 7510, ChannelId: 9510, Quota: 50, Status: model.TaskStatusInProgress,
		PrivateData: model.TaskPrivateData{Key: "saved-key", UpstreamTaskID: taskcommon.EncodeLocalTaskID("operations/test"), TokenId: 8510, BillingSource: service.BillingSourceWallet}}
	require.NoError(t, db.Create(task).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/videos/task_realtime_consistency", nil)
	c.Request.Header.Set("X-Client", "do-not-forward")
	c.Set("id", 9999) // downloaded task ownership, not this ad-hoc context, is authoritative
	common.SetContextKey(c, common.RequestIdKey, "realtime-consistency")
	require.NotNil(t, tryRealtimeFetch(c, task, false))
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	var saved model.Task
	require.NoError(t, db.First(&saved, task.ID).Error)
	require.Equal(t, "blocked", saved.FailReason)
	var user model.User
	require.NoError(t, db.First(&user, 7510).Error)
	require.Equal(t, 150, user.Quota)
	var token model.Token
	require.NoError(t, db.First(&token, 8510).Error)
	require.Equal(t, 150, token.RemainQuota)
	inflight, err := middleware.QueryChannelConcurrency(ch.Id)
	require.NoError(t, err)
	require.Zero(t, inflight)
	tryRealtimeFetch(c, task, false)
	require.NoError(t, db.First(&user, 7510).Error)
	require.Equal(t, 150, user.Quota)
}
