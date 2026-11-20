package gemini_test

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestManagedGeminiExactReasoningWire(t *testing.T) {
	var req dto.GeneralOpenAIRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"gemini-4-hydrogen","messages":[{"role":"user","content":"fixture"}],"reasoning_effort":"xhigh","extra_body":{"google":{"thinking_config":{"thinking_level":"xhigh"}}}}`, &req))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGemini, UpstreamModelName: req.Model}}
	body, err := (&gemini.Adaptor{}).ConvertOpenAIRequest(c, info, &req)
	require.NoError(t, err)
	wire, err := common.Marshal(body)
	require.NoError(t, err)
	require.Equal(t, "xhigh", gjson.GetBytes(wire, "generationConfig.thinkingConfig.thinkingLevel").String())
	require.False(t, gjson.GetBytes(wire, "generationConfig.thinkingConfig.thinkingBudget").Exists())
}
