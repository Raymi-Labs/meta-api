package xai_video

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReferenceImageValidationAndForwarding(t *testing.T) {
	for _, fields := range []string{
		`"image":{"url":"https://example.com/frame.png"}`,
		`"image":"https://example.com/frame.png"`,
		`"input_reference":"https://example.com/frame.png"`,
		`"image":{"url":"https://example.com/frame.png"},"input_reference":"https://example.com/frame.png"`,
	} {
		for _, modelName := range []string{"grok-imagine-video", "grok-imagine-video-1.5"} {
			t.Run(modelName+fields, func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				raw := `{"model":"` + modelName + `","prompt":"animate","seconds":"4",` + fields + `}`
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(raw))
				ctx.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
				info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
				a := &TaskAdaptor{}
				require.Nil(t, a.ValidateRequestAndSetAction(ctx, info))
				require.Equal(t, constant.TaskActionGenerate, info.Action)
				req, err := relaycommon.GetTaskRequest(ctx)
				require.NoError(t, err)
				require.Equal(t, []string{"https://example.com/frame.png"}, req.Images)
				body, err := a.BuildRequestBody(ctx, info)
				require.NoError(t, err)
				data, err := io.ReadAll(body)
				require.NoError(t, err)
				var out map[string]any
				require.NoError(t, common.Unmarshal(data, &out))
				require.Equal(t, map[string]any{"url": "https://example.com/frame.png"}, out["image"])
				require.NotContains(t, out, "input_reference")
				require.EqualValues(t, 4, out["duration"])
			})
		}
	}
}

func TestReferenceImageRejectsAmbiguousInputs(t *testing.T) {
	for _, fields := range []string{
		`"image":"data:image/png;base64,%%%"`, `"image":"data:text/plain;base64,aGVsbG8="`, `"image":"https://secret@example.com/a.png"`,
		`"image":null`, `"image":{}`, `"image":42`, `"image":{"url":false}`,
		`"image":{"url":"https://example.com/a.png","extra":1}`,
		`"image":""`, `"image":"file:///tmp/private.png"`,
		`"image":"https://example.com/a.png","input_reference":"https://example.com/b.png"`,
		`"images":["https://example.com/a.png"]`, `"input_reference":{}`, `"image":"data:image/png;base64,"`,
	} {
		t.Run(fields, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"animate",`+fields+`}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			a := &TaskAdaptor{}
			err := a.ValidateRequestAndSetAction(ctx, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
			require.NotNil(t, err)
			require.Equal(t, http.StatusBadRequest, err.StatusCode)
			require.True(t, err.LocalError)
			_, buildErr := a.BuildRequestBody(ctx, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
			require.Error(t, buildErr)
		})
	}
}

func TestReferenceImageTextAndOtherProvidersStayUnchanged(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"animate","model":"grok-imagine-video"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(ctx)
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info))
	require.Equal(t, constant.TaskActionTextGenerate, info.Action)
	var shared relaycommon.TaskSubmitReq
	require.Error(t, common.Unmarshal([]byte(`{"image":{"url":"https://example.com/a.png"}}`), &shared))
	require.NoError(t, common.Unmarshal([]byte(`{"image":"https://example.com/a.png"}`), &shared))
	payload := map[string]any{"image": map[string]any{"url": "data:image/png;base64,aGVsbG8="}}
	ref, err := normalizeReferenceImage(payload)
	require.NoError(t, err)
	require.Equal(t, "data:image/png;base64,aGVsbG8=", ref)
}
