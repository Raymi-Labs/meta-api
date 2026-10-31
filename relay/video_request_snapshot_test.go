package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoRequestSnapshotMatchesAdapters(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       int
		body       string
		seconds    float64
		resolution string
	}{
		{"grok-imagine-video", constant.ChannelTypeXai, `{"seconds":"6","size":"720p"}`, 6, "720p"},
		{"sora-2", constant.ChannelTypeSora, `{}`, 4, "720x1280"},
		{"wan2.5-t2v-preview", constant.ChannelTypeAli, `{"duration":5,"size":"1280*720"}`, 5, "1280*720"},
		{"doubao-seedance-1-0-pro-250528", constant.ChannelTypeDoubaoVideo, `{"duration":5,"resolution":"720p"}`, 5, "720p"},
		{"viduq2", constant.ChannelTypeVidu, `{"duration":5,"size":"720p"}`, 5, "720p"},
		{"kling-v2-master", constant.ChannelTypeKling, `{"duration":5,"mode":"pro"}`, 5, "pro"},
		{"MiniMax-Hailuo-2.3", constant.ChannelTypeMiniMax, `{"duration":6,"size":"1080P"}`, 6, "1080P"},
		{"veo-3.0-generate-001", constant.ChannelTypeGemini, `{"duration":8,"resolution":"720p"}`, 8, "720p"},
		{"veo-3.0-generate-001", constant.ChannelTypeVertexAi, `{"duration":8,"resolution":"1080p","metadata":{"sampleCount":2}}`, 8, "1080p"},
	} {
		t.Run(tc.name+strconv.Itoa(tc.kind), func(t *testing.T) {
			var payload map[string]any
			require.NoError(t, common.Unmarshal([]byte(tc.body), &payload))
			payload["prompt"] = "test"
			payload["model"] = tc.name
			data, _ := common.Marshal(payload)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(string(data)))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)
			info := &relaycommon.RelayInfo{OriginModelName: tc.name, TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.kind, UpstreamModelName: tc.name, ChannelBaseUrl: "https://example.test", ApiKey: "fixture-key"}}
			a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(tc.kind)))
			a.Init(info)
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			wire, err := PrepareVideoRequestSnapshot(c, info, a)
			require.NoError(t, err)
			var view map[string]any
			require.NoError(t, common.Unmarshal(info.BillingRequestInput.Body, &view))
			require.Equal(t, tc.seconds, view["duration"])
			require.Equal(t, tc.resolution, view["resolution"])
			sent, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			again, err := io.ReadAll(sent)
			require.NoError(t, err)
			require.JSONEq(t, string(wire), string(again))
			cost, _, err := billingexpr.RunExprWithRequest(`tier("video",param("duration") * 100000 * param("sampleCount"))`, billingexpr.TokenParams{}, *info.BillingRequestInput)
			require.NoError(t, err)
			count := float64(1)
			if tc.kind == constant.ChannelTypeVertexAi {
				count = 2
			}
			require.Equal(t, tc.seconds*100000*count, cost)
			if tc.kind == constant.ChannelTypeKling {
				url, err := a.BuildRequestURL(info)
				require.NoError(t, err)
				require.Contains(t, url, "text2video")
				require.Equal(t, constant.TaskActionTextGenerate, info.Action)
			}
		})
	}
}

func TestVideoAdaptersRejectFalseSuccessAndUnknownStatus(t *testing.T) {
	for _, tc := range []struct {
		kind               int
		good, bad, unknown string
	}{
		{constant.ChannelTypeAli, `{"output":{"task_status":"SUCCEEDED","video_url":"https://cdn.test/v.mp4"}}`, `{"output":{"task_status":"SUCCEEDED"}}`, `{"output":{"task_status":"UNKNOWN"}}`},
		{constant.ChannelTypeDoubaoVideo, `{"status":"succeeded","content":{"video_url":"https://cdn.test/v.mp4"}}`, `{"status":"succeeded"}`, `{"status":"new"}`},
		{constant.ChannelTypeVidu, `{"state":"success","creations":[{"url":"https://cdn.test/v.mp4"}]}`, `{"state":"success"}`, `{"state":"new"}`},
		{constant.ChannelTypeKling, `{"code":0,"data":{"task_status":"succeed","task_result":{"videos":[{"url":"https://cdn.test/v.mp4"}]}}}`, `{"code":0,"data":{"task_status":"succeed"}}`, `{"code":5,"data":{"task_status":"succeed"}}`},
		{constant.ChannelTypeJimeng, `{"code":10000,"data":{"status":"done","video_url":"https://cdn.test/v.mp4"}}`, `{"code":10000,"data":{"status":"done"}}`, `{"code":123,"data":{"status":"done"}}`},
		{constant.ChannelTypeGemini, `{"name":"models/veo/operations/1","done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"uri":"https://cdn.test/v.mp4"}}]}}}`, `{"name":"x","done":true}`, `{}`},
		{constant.ChannelTypeVertexAi, `{"name":"x","done":true,"response":{"videos":[{"mimeType":"video/mp4","bytesBase64Encoded":"dmlkZW8="}]}}`, `{"name":"x","done":true}`, `{}`},
	} {
		t.Run(strconv.Itoa(tc.kind), func(t *testing.T) {
			a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(tc.kind)))
			result, err := a.ParseTaskResult([]byte(tc.good))
			require.NoError(t, err)
			require.Equal(t, model.TaskStatusSuccess, result.Status)
			_, err = a.ParseTaskResult([]byte(tc.bad))
			require.Error(t, err)
			_, err = a.ParseTaskResult([]byte(tc.unknown))
			require.Error(t, err)
		})
	}
}

func TestVideoSubmissionEmptyTaskIDsAreRejected(t *testing.T) {
	for _, tc := range []struct {
		kind int
		body string
	}{
		{constant.ChannelTypeMiniMax, `{"base_resp":{"status_code":0}}`},
		{constant.ChannelTypeVidu, `{"state":"created"}`},
		{constant.ChannelTypeKling, `{"code":0,"data":{}}`},
		{constant.ChannelTypeJimeng, `{"code":10000,"data":{}}`},
		{constant.ChannelTypeAli, `{"output":{}}`},
		{constant.ChannelTypeDoubaoVideo, `{}`},
		{constant.ChannelTypeSora, `{}`},
		{constant.ChannelTypeXai, `{}`},
	} {
		a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(tc.kind)))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		_, _, err := a.DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, info)
		require.NotNil(t, err, "channel %d", tc.kind)
	}
}

func TestVideoTasksPersistSelectedKeyAndJimengQueryModel(t *testing.T) {
	for _, kind := range []int{constant.ChannelTypeAli, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVidu, constant.ChannelTypeKling, constant.ChannelTypeMiniMax, constant.ChannelTypeGemini, constant.ChannelTypeVertexAi, constant.ChannelTypeSora, constant.ChannelTypeXai, constant.ChannelTypeJimeng} {
		info := &relaycommon.RelayInfo{UserId: 123, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: kind, ApiKey: "selected-key"}}
		task := model.InitTask(constant.TaskPlatform(strconv.Itoa(kind)), info)
		require.Equal(t, "selected-key", task.PrivateData.Key)
	}
	service.InitHttpClient()
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		io.WriteString(w, `{"code":10000,"data":{"status":"in_queue"}}`)
	}))
	defer server.Close()
	a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeJimeng)))
	a.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "sk-test"}})
	resp, err := a.FetchTask(server.URL, "sk-test", map[string]any{"task_id": "task1", "upstream_model": "jimeng_t2v_v30"}, "")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, "jimeng_t2v_v30", payload["req_key"])
}

func TestVideoDurationConflictAndGenericTemplateMissingFields(t *testing.T) {
	saved := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
	t.Cleanup(func() { billing_setting.ReplaceBundle(saved) })
	name := "grok-imagine-video"
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{name: "tiered_expr"}, BillingExpr: map[string]string{name: `param("duration")==nil ? tier("video_config_preview",0) : tier("video",param("duration")*100000)`}})
	for _, body := range []string{`{"model":"grok-imagine-video","prompt":"hi","duration":5,"seconds":"6"}`, `{"model":"grok-imagine-video","prompt":"hi"}`, `{"model":"grok-imagine-video","prompt":"hi","duration":5.5}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		defer common.CleanupBodyStorage(c)
		info := &relaycommon.RelayInfo{OriginModelName: name, TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeXai, UpstreamModelName: name}}
		a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeXai)))
		a.Init(info)
		if validationErr := a.ValidateRequestAndSetAction(c, info); validationErr != nil {
			require.True(t, validationErr.LocalError)
			continue
		}
		_, err := PrepareVideoRequestSnapshot(c, info, a)
		require.Error(t, err)
	}
}

func TestGenericVideoTemplatesSaveAndCalculate(t *testing.T) {
	for _, mode := range []string{"seconds", "fixed"} {
		body, err := os.ReadFile("testdata/video_" + mode + "_expression.txt")
		require.NoError(t, err)
		expr := strings.TrimSpace(string(body))
		require.NoError(t, billing_setting.SmokeTestExpr(expr))
		for _, tc := range []struct {
			body  string
			cost  float64
			valid bool
		}{
			{`{"duration":5,"resolution":"720P","sampleCount":2}`, 1000000, true},
			{`{"duration":5,"resolution":"1080P"}`, 0, false},
			{`{"duration":16,"resolution":"720P"}`, 0, false},
		} {
			cost, _, err := billingexpr.RunExprWithRequest(expr, billingexpr.TokenParams{}, billingexpr.RequestInput{Body: []byte(tc.body)})
			if tc.valid {
				require.NoError(t, err)
				want := tc.cost
				if mode == "fixed" {
					want = 4000000
				}
				require.Equal(t, want, cost)
			} else {
				require.Error(t, err)
			}
		}
	}
}

func TestVideoMetadataCannotKeepAnUnsentPriceParameter(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"viduq2","prompt":"hi","duration":5,"size":"720p","metadata":{"model":"other","duration":15,"resolution":"1080p"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)
	info := &relaycommon.RelayInfo{OriginModelName: "viduq2", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVidu, UpstreamModelName: "viduq2"}}
	a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVidu)))
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	_, err := PrepareVideoRequestSnapshot(c, info, a)
	require.NoError(t, err)
	cost, _, err := billingexpr.RunExprWithRequest(`tier("video",param("metadata.duration")*100000)`, billingexpr.TokenParams{}, *info.BillingRequestInput)
	require.NoError(t, err)
	require.Equal(t, 500000.0, cost)
	var view map[string]any
	require.NoError(t, common.Unmarshal(info.BillingRequestInput.Body, &view))
	metadata := view["metadata"].(map[string]any)
	require.Equal(t, "720p", metadata["resolution"])
	require.Equal(t, "viduq2", metadata["model"])
}

func TestSoraConversionHidesLegacyTaskID(t *testing.T) {
	a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)))
	converter := a.(interface {
		ConvertToOpenAIVideo(*model.Task) ([]byte, error)
	})
	body, err := converter.ConvertToOpenAIVideo(&model.Task{TaskID: "task_public", Data: []byte(`{"id":"provider-id","task_id":"provider-id","status":"completed"}`)})
	require.NoError(t, err)
	require.NotContains(t, string(body), "provider-id")
	require.Contains(t, string(body), "task_public")
}

func TestXAIFirstFrameSurvivesValidationAndBillingSnapshot(t *testing.T) {
	for _, field := range []string{`"image":{"url":"data:image/png;base64,aGVsbG8="}`, `"input_reference":"data:image/png;base64,aGVsbG8="`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"grok-imagine-video-1.5","prompt":"animate","duration":4,"resolution":"480p",`+field+`}`))
		c.Request.Header.Set("Content-Type", "application/json")
		info := &relaycommon.RelayInfo{OriginModelName: "grok-imagine-video-1.5", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeXai, UpstreamModelName: "grok-imagine-video-1.5", ChannelBaseUrl: "https://example.test", ApiKey: "fixture-key"}}
		a := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeXai)))
		a.Init(info)
		require.Nil(t, a.ValidateRequestAndSetAction(c, info))
		require.Equal(t, constant.TaskActionGenerate, info.Action)
		wire, err := PrepareVideoRequestSnapshot(c, info, a)
		require.NoError(t, err)
		var sent map[string]any
		require.NoError(t, common.Unmarshal(wire, &sent))
		require.Equal(t, map[string]any{"url": "data:image/png;base64,aGVsbG8="}, sent["image"])
		require.NotContains(t, sent, "input_reference")
		cost, _, err := billingexpr.RunExprWithRequest(`tier("video",param("duration") * 100000 * param("sampleCount"))`, billingexpr.TokenParams{}, *info.BillingRequestInput)
		require.NoError(t, err)
		require.Equal(t, float64(400000), cost)
		common.CleanupBodyStorage(c)
	}
}
