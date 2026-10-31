package controller

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestChannelHealthProbeActualExecution(t *testing.T) {
	if runRelayMatrixInSubprocess(t) {
		return
	}
	db := useRelayMatrixProcessDB(t)
	common.RedisEnabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"probe-model":0}`))
	require.NoError(t, db.Create(&model.User{Id: 8001, Username: "probe-tester", Group: "admin-group", Status: common.UserStatusEnabled}).Error)
	ch := &model.Channel{Id: 8002, Type: constant.ChannelTypeOpenAI, Name: "probe", Group: "public,paid", Models: "probe-model,untested-model", Key: "fixture-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer("https://probe.invalid"), ModelMapping: common.GetPointer(`{"probe-model":"upstream-model"}`)}
	require.NoError(t, db.Create(ch).Error)
	for _, g := range []string{"public", "paid", "auto"} {
		require.NoError(t, db.Create(&model.Ability{Group: g, Model: "probe-model", ChannelId: ch.Id, Enabled: true}).Error)
	}
	require.NoError(t, db.Create(&model.Ability{Group: "public", Model: "untested-model", ChannelId: ch.Id, Enabled: true}).Error)
	service.InitHttpClient()
	calls := 0
	for _, tc := range []struct {
		status  int
		body    string
		success bool
	}{
		{500, `{"error":{"message":"unavailable"}}`, false},
		{200, `{"id":"probe","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, true},
		{200, `{"error":{"message":"invalid response"}}`, false},
	} {
		service.GetHttpClient().Transport = healthTestTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Contains(t, string(body), "upstream-model")
			return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
		})
		result := testChannel(ch, 8001, "probe-model", "", common.GetPointer(false), nil)
		require.Equal(t, tc.success, result.localErr == nil && result.newAPIError == nil)
	}
	require.Equal(t, 3, calls)
	// A local video preflight and failed user lookup do not add probe evidence.
	require.NotNil(t, testChannel(ch, 0, "sora-2", "", nil, nil).preflight)
	require.Error(t, testChannel(ch, 0, "probe-model", "", nil, nil).localErr)
	require.NoError(t, grouphealth.Flush(context.Background()))
	rows, err := model.QueryGroupHealthMetrics(context.Background(), []string{"public", "paid", "auto", "admin-group"}, "", time.Now().Add(-time.Hour).Unix(), time.Now().Unix())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, "probe-model", row.ModelName)
		require.Contains(t, []string{"public", "paid"}, row.GroupName)
		require.EqualValues(t, 3, row.ProbeCount)
		require.EqualValues(t, 1, row.ProbeSuccessCount)
		require.Zero(t, row.RequestCount)
		require.Zero(t, row.UsageSampleCount)
		require.Zero(t, row.TTFTCount)
	}
}
