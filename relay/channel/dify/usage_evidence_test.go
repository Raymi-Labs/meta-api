package dify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDifyLocalDebugTokensDoNotBecomeMeasuredUsage(t *testing.T) {
	previous := constant.DifyDebug
	constant.DifyDebug = true
	t.Cleanup(func() { constant.DifyDebug = previous })
	for _, withDebug := range []bool{false, true} {
		body := ""
		if withDebug {
			body += `data: {"event":"node_finished","data":{"node_type":"llm","status":"succeeded"}}` + "\n\n"
		}
		body += `data: {"event":"message","answer":"hello"}` + "\n\n" + `data: {"event":"message_end","metadata":{"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}}` + "\n\n"
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "dify-test"}}
		usage, err := difyStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
		require.Nil(t, err)
		require.Equal(t, withDebug, usage.InputTokensEstimated)
		service.ObserveGroupHealthCacheUsage(info, usage)
		if withDebug {
			require.Equal(t, 11, usage.CompletionTokens, "billing count stays unchanged")
			require.Nil(t, info.GroupHealthTokenUsage)
		} else {
			require.EqualValues(t, 10, info.GroupHealthTokenUsage.OutputTokens)
		}
	}
}
