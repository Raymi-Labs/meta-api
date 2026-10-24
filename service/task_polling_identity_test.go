package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

type taskBatchIdentityAdaptor struct {
	mockAdaptor
	info     *relaycommon.RelayInfo
	check    func(*relaycommon.RelayInfo, string, map[string]any)
	response string
}

func (a *taskBatchIdentityAdaptor) Init(info *relaycommon.RelayInfo) { a.info = info }
func (a *taskBatchIdentityAdaptor) FetchTask(_ string, key string, body map[string]any, _ string) (*http.Response, error) {
	a.check(a.info, key, body)
	response := a.response
	if response == "" {
		response = `{"code":"success","data":[]}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
}

func TestSunoPollingBatchesByPersistedOwnerAndKey(t *testing.T) {
	truncate(t)
	oldFactory := GetTaskAdaptorFunc
	oldMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { GetTaskAdaptorFunc = oldFactory; common.MemoryCacheEnabled = oldMemory })
	base := "http://example.invalid"
	headers := `{"X-Owner":"{authenticated_user_id}"}`
	ch := &model.Channel{Id: 9610, Key: "rotated-channel-key", BaseURL: &base, HeaderOverride: &headers}
	require.NoError(t, model.DB.Create(ch).Error)
	tasks := map[string]*model.Task{
		"a": {UserId: 1, PrivateData: model.TaskPrivateData{Key: "key-a"}},
		"b": {UserId: 2, PrivateData: model.TaskPrivateData{Key: "key-a"}},
		"c": {UserId: 1, PrivateData: model.TaskPrivateData{Key: "key-b"}},
		"d": {UserId: 1, PrivateData: model.TaskPrivateData{Key: "key-a"}},
	}
	seen := map[string]bool{}
	calls := 0
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatformSuno, platform)
		return &taskBatchIdentityAdaptor{check: func(info *relaycommon.RelayInfo, key string, body map[string]any) {
			calls++
			require.Equal(t, ch.GetHeaderOverride()["X-Owner"], info.HeadersOverride["X-Owner"])
			for _, id := range body["ids"].([]string) {
				require.Equal(t, tasks[id].UserId, info.UserId)
				require.Equal(t, tasks[id].PrivateData.Key, key)
				require.Equal(t, key, info.ApiKey)
				seen[id] = true
			}
		}}
	}
	require.NoError(t, updateSunoTasks(context.Background(), ch.Id, []string{"a", "b", "c", "d"}, tasks))
	require.Equal(t, 3, calls)
	require.Len(t, seen, 4)
}

func TestSunoPollingIgnoresResultsOutsideCredentialBatch(t *testing.T) {
	truncate(t)
	oldFactory := GetTaskAdaptorFunc
	t.Cleanup(func() { GetTaskAdaptorFunc = oldFactory })
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor {
		return &taskBatchIdentityAdaptor{
			check:    func(*relaycommon.RelayInfo, string, map[string]any) {},
			response: `{"code":"success","data":[{"task_id":"other","status":"FAILURE","fail_reason":"not in this credential batch"}]}`,
		}
	}
	ch := &model.Channel{Id: 9611}
	tasks := map[string]*model.Task{
		"requested": {TaskID: "requested", UserId: 1, Status: model.TaskStatusInProgress},
		"other":     {TaskID: "other", UserId: 2, Status: model.TaskStatusInProgress},
	}
	require.NoError(t, updateSunoBatch(context.Background(), ch, []string{"requested"}, tasks, 1, "key-one"))
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), tasks["other"].Status)
	require.Empty(t, tasks["other"].FailReason)
}
