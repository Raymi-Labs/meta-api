package hailuo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const validH3 = `{"model":"MiniMax-H3","prompt":"a young adult walking by the sea","duration":5,"resolution":"2K","ratio":"16:9"}`

func TestH3NormalizeBillableParameters(t *testing.T) {
	for _, tc := range []struct {
		name, body, model string
		valid             bool
	}{
		{"text", validH3, "MiniMax-H3", true},
		{"aliases", `{"prompt":"test","seconds":"5","size":"768p","aspect_ratio":"16:9"}`, "MiniMax-H3", true},
		{"content", `{"content":[{"type":"text","text":"test"}],"duration":4,"resolution":"768P","ratio":"1:1"}`, "MiniMax-H3-Highspeed", true},
		{"metadata", `{"prompt":"test","metadata":{"duration":5,"resolution":"480P","ratio":"9:16"}}`, "MiniMax-H3-Max", true},
		{"missing-duration", `{"prompt":"test","resolution":"2K","ratio":"16:9"}`, "MiniMax-H3", false},
		{"fraction", `{"prompt":"test","duration":5.5,"resolution":"2K","ratio":"16:9"}`, "MiniMax-H3", false},
		{"too-long", `{"prompt":"test","duration":16,"resolution":"2K","ratio":"16:9"}`, "MiniMax-H3", false},
		{"max-short", `{"prompt":"test","duration":4,"resolution":"768P","ratio":"16:9"}`, "MiniMax-H3-Max", false},
		{"unsupported-resolution", validH3, "MiniMax-H3-Lite", false},
		{"unknown-model", validH3, "MiniMax-H3-Future", false},
		{"metadata-conflict", `{"prompt":"test","duration":5,"resolution":"2K","ratio":"16:9","metadata":{"duration":15}}`, "MiniMax-H3", false},
		{"alias-conflict", `{"prompt":"test","duration":5,"seconds":"15","resolution":"2K","ratio":"16:9"}`, "MiniMax-H3", false},
		{"model-override", `{"prompt":"test","duration":5,"resolution":"2K","ratio":"16:9","metadata":{"model":"MiniMax-H3-Max"}}`, "MiniMax-H3", false},
		{"reference", `{"prompt":"test","image":"https://example.test/a.png","duration":5,"resolution":"2K","ratio":"16:9"}`, "MiniMax-H3", false},
		{"content-reference", `{"content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}],"duration":5,"resolution":"2K","ratio":"16:9"}`, "MiniMax-H3", false},
		{"adaptive", `{"prompt":"test","duration":5,"resolution":"2K","ratio":"adaptive"}`, "MiniMax-H3", false},
		{"ignored-option", `{"prompt":"test","duration":5,"resolution":"2K","ratio":"16:9","watermark":false}`, "MiniMax-H3", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]any
			require.NoError(t, common.Unmarshal([]byte(tc.body), &raw))
			before, _ := common.Marshal(raw)
			payload, err := normalizeH3(raw, tc.model)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, tc.model, payload.Model)
				require.NotEmpty(t, payload.Content[0].Text)
			} else {
				require.Error(t, err)
			}
			after, _ := common.Marshal(raw)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

func TestH3HTTPSubmitAndQuery(t *testing.T) {
	service.InitHttpClient()
	saved := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
	t.Cleanup(func() { billing_setting.ReplaceBundle(saved) })
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{"alias-h3": "tiered_expr"}, BillingExpr: map[string]string{"alias-h3": `tier("video",param("duration") * 100000)`}})
	paths := []string{}
	queryBody := `{"task":{"id":"upstream-123","status":"running"}}`
	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		require.Equal(t, "Bearer saved-key", r.Header.Get("Authorization"))
		if r.Method == http.MethodPost {
			sent, _ = io.ReadAll(r.Body)
			io.WriteString(w, `{"task_id":"upstream-123"}`)
		} else {
			io.WriteString(w, queryBody)
		}
	}))
	defer server.Close()
	info := &relaycommon.RelayInfo{OriginModelName: "alias-h3", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "MiniMax-H3", ChannelBaseUrl: server.URL, ApiKey: "saved-key"}}
	info.PublicTaskID = "task_public"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"alias-h3","prompt":"test","seconds":"5","size":"2k","aspect_ratio":"16:9"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	require.NoError(t, a.PrepareTaskRequest(c, info))
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	resp, err := a.DoRequest(c, info, body)
	require.NoError(t, err)
	id, _, taskErr := a.DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	require.Equal(t, "upstream-123", id)
	require.JSONEq(t, string(info.BillingRequestInput.Body), string(sent))
	require.NotContains(t, string(sent), "seconds")
	require.Contains(t, string(sent), `"resolution":"2K"`)
	for _, tc := range []struct{ state, want string }{{"queued", model.TaskStatusQueued}, {"running", model.TaskStatusInProgress}, {"succeeded", model.TaskStatusSuccess}, {"failed", model.TaskStatusFailure}, {"cancelled", model.TaskStatusFailure}} {
		queryBody = `{"task":{"id":"upstream-123","status":"` + tc.state + `","modality":"video","content":{"url":"https://cdn.example.test/video.mp4"},"usage":{"total_tokens":999999}}}`
		resp, err = a.FetchTask(server.URL, "saved-key", map[string]any{"task_id": id, "upstream_model": "MiniMax-H3"}, "")
		require.NoError(t, err)
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, err)
		result, err := a.ParseTaskResult(data)
		require.NoError(t, err)
		require.Equal(t, tc.want, result.Status)
		require.Zero(t, result.TotalTokens)
	}
	require.Equal(t, "/v2/video_generation", paths[0])
	require.Equal(t, "/v2/query/video_generation/upstream-123", paths[1])
	for _, bad := range []string{`{}`, `{"error":{"message":"denied"}}`, `{"task":{"id":"wrong","status":"succeeded","content":{"url":"https://cdn.test/a"}}}`, `{"task":{"id":"upstream-123","status":"succeeded"}}`, `{"task":{"id":"upstream-123","status":"new-state"}}`, `{"task":{"id":"upstream-123","status":"succeeded","content":{"url":"file:///secret"}}}`} {
		_, err := a.ParseTaskResult([]byte(bad))
		require.Error(t, err)
	}
	_, err = a.FetchTask(server.URL, "saved-key", map[string]any{"task_id": "../secret", "upstream_model": "MiniMax-H3"}, "")
	require.Error(t, err)
	// The same adaptor can later poll an old persisted V1 task without leakage.
	a.Init(info)
	resp, err = a.FetchTask(server.URL, "saved-key", map[string]any{"task_id": "legacy-id", "upstream_model": "MiniMax-Hailuo-2.3"}, "")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, "/v1/query/video_generation?task_id=legacy-id", paths[len(paths)-1])
}

func TestH3MissingPriceAndLegacyPayload(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(validH3))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{OriginModelName: "missing-h3-price-test", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "MiniMax-H3"}}
	a := &TaskAdaptor{}
	a.Init(info)
	require.ErrorContains(t, a.PrepareTaskRequest(c, info), "explicit task price")
	_, err := a.BuildRequestBody(c, info)
	require.Error(t, err)
	info.UpstreamModelName = "MiniMax-Hailuo-2.3"
	legacy, err := a.convertToRequestPayload(&relaycommon.TaskSubmitReq{Prompt: "legacy", Duration: 6, Size: "1080P"}, info)
	require.NoError(t, err)
	require.Equal(t, 6, *legacy.Duration)
	require.Equal(t, "1080P", legacy.Resolution)
}

func TestH3GeneratedSecondsTemplateCanSaveAndChargesCorrectly(t *testing.T) {
	data, err := os.ReadFile("testdata/h3_seconds_expression.txt")
	require.NoError(t, err)
	expr := strings.TrimSpace(string(data))
	require.NoError(t, billing_setting.SmokeTestExpr(expr))
	for _, tc := range []struct {
		body  string
		cost  float64
		valid bool
	}{
		{`{"duration":5,"resolution":"768P"}`, 500000, true},
		{`{"duration":10,"resolution":"2K"}`, 2000000, true},
		{`{"duration":5,"resolution":"1080P"}`, 0, false},
		{`{"duration":16,"resolution":"2K"}`, 0, false},
	} {
		cost, _, err := billingexpr.RunExprWithRequest(expr, billingexpr.TokenParams{}, billingexpr.RequestInput{Body: []byte(tc.body)})
		if tc.valid {
			require.NoError(t, err)
			require.Equal(t, tc.cost, cost)
		} else {
			require.Error(t, err)
		}
	}
}
