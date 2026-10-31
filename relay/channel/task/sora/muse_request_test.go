package sora

import (
	"encoding/base64"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMuseLongMultiReferenceJSON(t *testing.T) {
	for _, seconds := range []string{`180`, `"180"`, `31`, `"60"`} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"muse-video","prompt":"four references","seconds":`+seconds+`,"images":["https://example.test/1.png","https://example.test/2.png","https://example.test/3.png","data:image/webp;base64,aGVsbG8="]}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "muse-video"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		a := &TaskAdaptor{}
		require.Nil(t, a.ValidateRequestAndSetAction(ctx, info))
		require.Equal(t, constant.TaskActionGenerate, info.Action)
		reader, err := a.BuildRequestBody(ctx, info)
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		var payload map[string]interface{}
		require.NoError(t, common.Unmarshal(data, &payload))
		require.Equal(t, strings.Trim(seconds, `"`), payload["seconds"])
		require.Equal(t, "1280x720", payload["size"])
		images := payload["images"].([]interface{})
		require.Len(t, images, 4)
		require.Equal(t, "https://example.test/1.png", images[0])
		require.NotContains(t, payload, "input_reference")
		common.CleanupBodyStorage(ctx)
	}
}
func TestMuseRejectsUnsupportedOrConflictingInputs(t *testing.T) {
	for _, fields := range []string{`"seconds":181`, `"seconds":3`, `"seconds":8.5`, `"seconds":"8.5"`, `"seconds":60,"duration":30`, `"resolution":"1080p"`, `"size":"1024x1024"`, `"images":[]`, `"images":["a","b","c","d","e"]`, `"images":["https://example.test/a"],"input_reference":"https://example.test/b"`, `"images":[null]`, `"input_reference":"file-123"`, `"input_reference":"data:image/png;base64,%%%"`, `"image":"https://example.test/a"`} {
		t.Run(fields, func(t *testing.T) {
			var payload map[string]interface{}
			require.NoError(t, common.Unmarshal([]byte(`{"prompt":"test",`+fields+`}`), &payload))
			require.Error(t, normalizeMuseRequest(payload))
		})
	}
	var p map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(`{"prompt":"`+strings.Repeat("字", 4001)+`"}`), &p))
	require.Error(t, normalizeMuseRequest(p))
}

func TestMuseInlineImageByteBudgets(t *testing.T) {
	encode := func(n int) string {
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, n))
	}
	require.Error(t, normalizeMuseRequest(map[string]interface{}{"prompt": "test", "input_reference": encode(10_000_001)}))
	reference := encode(8_000_001)
	require.Error(t, normalizeMuseRequest(map[string]interface{}{"prompt": "test", "images": []interface{}{reference, reference, reference}}))
	require.NoError(t, normalizeMuseRequest(map[string]interface{}{"prompt": "test", "input_reference": encode(10_000_000)}))
}
