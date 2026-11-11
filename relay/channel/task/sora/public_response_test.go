package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubmitPublicVideoDoesNotExposeProvider(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		body := `{"id":"provider-task","model":"private-provider.example","status":"` + status + `","seconds":"4","size":"1280x720","remixed_from_video_id":"provider-remix","error":{"code":"https://private-provider.example","message":"secret-key"},"metadata":{"url":"https://private-provider.example/video"}}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		a := &TaskAdaptor{}
		id, data, err := a.DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}, OriginModelName: "public-model"})
		require.Nil(t, err)
		require.Equal(t, "provider-task", id)
		require.Equal(t, body, string(data))
		for _, secret := range []string{"private-provider.example", "secret-key", "provider-task", "provider-remix"} {
			require.NotContains(t, w.Body.String(), secret)
		}
		require.Contains(t, w.Body.String(), "task_public")
		require.Contains(t, w.Body.String(), "public-model")
		require.Contains(t, w.Body.String(), `"seconds":"4"`)
	}
}
