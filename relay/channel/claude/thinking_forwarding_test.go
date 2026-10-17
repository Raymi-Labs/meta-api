package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNativeClaudeThinkingForwarding(t *testing.T) {
	const body = `{"model":"claude-test","max_tokens":64,"thinking":{"type":"adaptive","block_binding":{"prefix_mismatch_behavior":"drop_block"},"future":[1,null]},"messages":[{"role":"user","content":"hello"}]}`
	var request dto.ClaudeRequest
	require.NoError(t, common.UnmarshalJsonStr(body, &request))
	copy, err := common.DeepCopy(&request)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertClaudeRequest(nil, &relaycommon.RelayInfo{}, copy)
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, body, string(encoded))
}

func TestClaudeAllBetaLinesAndConfiguredHeaders(t *testing.T) {
	settings := model_setting.GetClaudeSettings()
	original := settings.HeadersSettings
	t.Cleanup(func() { settings.HeadersSettings = original })
	settings.HeadersSettings = map[string]map[string][]string{"claude-test": {"anthropic-beta": {"configured-beta", "second-beta"}}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Add("anthropic-beta", "first-beta, second-beta")
	c.Request.Header.Add("anthropic-beta", "third-beta")
	headers := http.Header{}
	CommonClaudeHeadersOperation(c, &headers, &relaycommon.RelayInfo{OriginModelName: "claude-test"})
	require.Equal(t, "first-beta,second-beta,third-beta,configured-beta", headers.Get("anthropic-beta"))
	require.Len(t, c.Request.Header.Values("anthropic-beta"), 2)
}
