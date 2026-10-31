package gemini_test

import (
	"fmt"
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

func TestChatImageConfigSurvivesGeminiAndVertexConversion(t *testing.T) {
	for _, tc := range []struct {
		name, fields, want string
		invalid            bool
	}{
		{"top_level", `,"imageConfig":{"aspectRatio":"16:9","imageSize":"2K","futureLimit":9007199254740993}`, `{"aspectRatio":"16:9","imageSize":"2K","futureLimit":9007199254740993}`, false},
		{"missing", ``, ``, false},
		{"null", `,"imageConfig":null`, `null`, false},
		{"empty_object", `,"imageConfig":{}`, `{}`, false},
		{"legacy", `,"extra_body":{"google":{"image_config":{"aspect_ratio":"4:3","image_size":"1K"}}}`, `{"aspectRatio":"4:3","imageSize":"1K"}`, false},
		{"legacy_overrides_whole_config", `,"imageConfig":{"aspectRatio":"16:9","imageSize":"2K","futureLimit":9007199254740993},"extra_body":{"google":{"image_config":{"aspect_ratio":"1:1"}}}`, `{"aspectRatio":"1:1"}`, false},
		{"empty_legacy_keeps_top", `,"imageConfig":{"imageSize":"2K"},"extra_body":{"google":{"image_config":{}}}`, `{"imageSize":"2K"}`, false},
		{"legacy_wrong_name", `,"imageConfig":{"imageSize":"2K"},"extra_body":{"google":{"imageConfig":{}}}`, ``, true},
		{"legacy_wrong_field", `,"extra_body":{"google":{"image_config":{"aspectRatio":"1:1"}}}`, ``, true},
	} {
		for _, channelType := range []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi} {
			t.Run(fmt.Sprintf("%s/channel=%d", tc.name, channelType), func(t *testing.T) {
				var req dto.GeneralOpenAIRequest
				require.NoError(t, common.Unmarshal([]byte(`{"model":"gemini-2.5-flash-image","messages":[{"role":"user","content":"draw"}],"max_tokens":37`+tc.fields+`}`), &req))
				before, err := common.Marshal(req)
				require.NoError(t, err)
				copied, err := common.DeepCopy(&req)
				require.NoError(t, err)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channelType, UpstreamModelName: req.Model}}
				var result any
				if channelType == constant.ChannelTypeGemini {
					result, err = (&gemini.Adaptor{}).ConvertOpenAIRequest(c, info, copied)
				} else {
					result, err = (&vertex.Adaptor{RequestMode: vertex.RequestModeGemini}).ConvertOpenAIRequest(c, info, copied)
				}
				if tc.invalid {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				wire, err := common.Marshal(result)
				require.NoError(t, err)
				actual := gjson.GetBytes(wire, "generationConfig.imageConfig")
				if tc.want == "" {
					require.False(t, actual.Exists())
				} else {
					require.JSONEq(t, tc.want, actual.Raw)
					if tc.name == "top_level" {
						require.Equal(t, "9007199254740993", actual.Get("futureLimit").Raw)
					}
				}
				require.Equal(t, int64(37), gjson.GetBytes(wire, "generationConfig.maxOutputTokens").Int())
				require.Equal(t, "draw", gjson.GetBytes(wire, "contents.0.parts.0.text").String())
				after, err := common.Marshal(req)
				require.NoError(t, err)
				require.Equal(t, string(before), string(after))
			})
		}
	}
}
