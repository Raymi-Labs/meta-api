package gemini_test

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCronCreateNullableToolWire(t *testing.T) {
	for _, tc := range []struct {
		name    string
		channel int
		convert func(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error)
	}{
		{"gemini", constant.ChannelTypeGemini, (&gemini.Adaptor{}).ConvertOpenAIRequest},
		{"vertex", constant.ChannelTypeVertexAi, (&vertex.Adaptor{RequestMode: vertex.RequestModeGemini}).ConvertOpenAIRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req dto.GeneralOpenAIRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"gemini-4-neon","stream":true,"messages":[{"role":"user","content":"schedule"}],"tools":[{"type":"function","function":{"name":"CronCreate","parameters":{"type":"object","properties":{"task":{"type":"string"},"delayMinutes":{"description":"delay","anyOf":[{"type":"integer","exclusiveMinimum":0,"maximum":525600},{"type":"null"}]},"maxRuns":{"oneOf":[{"type":"null"},{"type":"integer","maximum":9007199254740993}]}},"required":["task"]}}}]}`, &req))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channel, UpstreamModelName: req.Model}}
			body, err := tc.convert(c, info, &req)
			require.NoError(t, err)
			wire, err := common.Marshal(body)
			require.NoError(t, err)
			fn := gjson.GetBytes(wire, "tools.0.functionDeclarations.0")
			require.Equal(t, "CronCreate", fn.Get("name").String())
			require.Equal(t, `["task"]`, fn.Get("parameters.required").Raw)
			var keys []string
			fn.Get("parameters.properties").ForEach(func(key, _ gjson.Result) bool {
				keys = append(keys, key.String())
				return true
			})
			require.Equal(t, []string{"task", "delayMinutes", "maxRuns"}, keys)
			require.JSONEq(t, `{"type":"INTEGER","nullable":true,"description":"delay","maximum":525600}`, fn.Get("parameters.properties.delayMinutes").Raw)
			require.Equal(t, "9007199254740993", fn.Get("parameters.properties.maxRuns.maximum").Raw)
		})
	}
}
