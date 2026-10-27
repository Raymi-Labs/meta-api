package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/task/hailuo"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestH3RelayNormalizedPricingAndTerminalLifecycle(t *testing.T) {
	db := useRelayCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}, &model.Log{}, &model.PulseFundingLedger{}, &model.UserSubscription{}))
	service.InitHttpClient()
	saved := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
	t.Cleanup(func() { billing_setting.ReplaceBundle(saved) })
	const name = "h3-relay-alias-test"
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{name: "tiered_expr"}, BillingExpr: map[string]string{name: `param("resolution") == "2K" ? tier("video",param("duration") * 100000) : tier("unsupported",-1)`}})
	groups, _ := common.Marshal(ratio_setting.GetGroupRatioCopy())
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(groups))) })
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"h3-test":2}`))
	var submitted []byte
	var paths []string
	status := "running"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		require.Equal(t, "Bearer persisted-h3-key", r.Header.Get("Authorization"))
		if r.Method == "POST" {
			submitted, _ = io.ReadAll(r.Body)
			io.WriteString(w, `{"task_id":"provider-h3-task"}`)
		} else {
			io.WriteString(w, `{"task":{"id":"provider-h3-task","status":"`+status+`","content":{"url":"https://cdn.example.test/video.mp4"},"usage":{"total_tokens":9000000}}}`)
		}
	}))
	defer server.Close()
	ch := &model.Channel{Id: 9563, Type: constant.ChannelTypeMiniMax, Key: "wrong-current-key", BaseURL: &server.URL, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(ch).Error)
	for i, terminal := range []string{"succeeded", "failed", "cancelled"} {
		t.Run(terminal, func(t *testing.T) {
			userID := 7560 + i
			initial := int(10 * common.QuotaPerUnit)
			require.NoError(t, db.Create(&model.User{Id: userID, Username: "h3-user-" + strconv.Itoa(userID), AffCode: "h3-" + strconv.Itoa(userID), Quota: initial, Status: common.UserStatusEnabled, Group: "h3-test"}).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"h3-relay-alias-test","prompt":"test","seconds":"5","size":"2k","aspect_ratio":"16:9"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("model_mapping", `{"h3-relay-alias-test":"MiniMax-H3"}`)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeMiniMax)
			common.SetContextKey(c, constant.ContextKeyChannelId, ch.Id)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "persisted-h3-key")
			info := &relaycommon.RelayInfo{UserId: userID, IsPlayground: true, UsingGroup: "h3-test", UserGroup: "h3-test", OriginModelName: name, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			result, taskErr := RelayTaskSubmit(c, info)
			require.Nil(t, taskErr)
			require.NotNil(t, result)
			require.Equal(t, int(common.QuotaPerUnit), result.Quota) // 5 seconds * $0.10 * group 2
			require.JSONEq(t, string(info.BillingRequestInput.Body), string(submitted))
			require.NoError(t, service.SettleBilling(c, info, result.Quota))
			task := model.InitTask(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMax)), info)
			task.Status = model.TaskStatus(model.TaskStatusInProgress)
			task.Quota = result.Quota
			require.Equal(t, "persisted-h3-key", task.PrivateData.Key)
			task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
			task.PrivateData.BillingSource = service.BillingSourceWallet
			task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: name, PerCallBilling: true}
			task.Data = result.TaskData
			require.NoError(t, db.Create(task).Error)
			stale := *task
			status = terminal
			query, _ := gin.CreateTestContext(httptest.NewRecorder())
			query.Request = httptest.NewRequest("GET", "/v1/videos/"+task.TaskID, nil)
			require.NotNil(t, tryRealtimeFetch(query, task, false))
			wantStatus := model.TaskStatusFailure
			if terminal == "succeeded" {
				wantStatus = model.TaskStatusSuccess
				require.Equal(t, "https://cdn.example.test/video.mp4", task.PrivateData.ResultURL)
			}
			require.Equal(t, model.TaskStatus(wantStatus), task.Status)
			// Background polling of a stale snapshot uses the same terminal CAS.
			adaptor := &hailuo.TaskAdaptor{}
			adaptor.Init(service.TaskPollingRelayInfo(ch, userID, "persisted-h3-key"))
			resp, err := adaptor.FetchTask(server.URL, "persisted-h3-key", map[string]any{"task_id": task.GetUpstreamTaskID(), "upstream_model": task.Properties.UpstreamModelName}, "")
			require.NoError(t, err)
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			parsed, err := adaptor.ParseTaskResult(data)
			require.NoError(t, err)
			require.NoError(t, service.ApplyVideoTaskResult(context.Background(), adaptor, &stale, parsed, data))
			var user model.User
			require.NoError(t, db.First(&user, userID).Error)
			wantQuota := initial
			if terminal == "succeeded" {
				wantQuota -= result.Quota
			}
			require.Equal(t, wantQuota, user.Quota)
			require.Equal(t, "/v2/query/video_generation/provider-h3-task", paths[len(paths)-1])
		})
	}
}

func TestH3InvalidSubmissionNeverReservesOrCallsProvider(t *testing.T) {
	for _, body := range []string{
		`{"prompt":"test","resolution":"2K","ratio":"16:9"}`,
		`{"prompt":"test","duration":5,"resolution":"2K","ratio":"16:9","metadata":{"duration":15}}`,
		`{"prompt":"test","duration":5,"resolution":"2K","ratio":"16:9","image":"https://example.test/a.png"}`,
		`{"prompt":"test","duration":5,"resolution":"2K","ratio":"16:9"}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("model_mapping", `{"unpriced-h3-integration-test":"MiniMax-H3"}`)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeMiniMax)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "http://127.0.0.1:1")
		info := &relaycommon.RelayInfo{OriginModelName: "unpriced-h3-integration-test", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"duration":15}`)}}
		result, err := RelayTaskSubmit(c, info)
		require.Nil(t, result)
		require.NotNil(t, err)
		require.True(t, err.LocalError)
		require.Equal(t, "invalid_task_parameters", err.Code)
		require.Nil(t, info.Billing)
		require.Nil(t, info.BillingRequestInput)
	}
}
