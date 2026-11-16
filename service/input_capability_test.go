package service

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInputCapabilityStructuredBlocksAndAdapters(t *testing.T) {
	old := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = old })
	for _, tc := range []struct {
		name, path, body, model, mapping, settings, code string
		channel                                          int
	}{
		{"text mentioning video", "/v1/chat/completions", `{"messages":[{"content":"video_url video/mp4"}]}`, "claude-x", "", "", "", constant.ChannelTypeAnthropic},
		{"image", "/v1/chat/completions", `{"messages":[{"content":[{"type":"image_url","image_url":{"url":"https://example.test/video.mp4"}}]}]}`, "claude-x", "", "", "", constant.ChannelTypeAnthropic},
		{"video Claude", "/v1/chat/completions", `{"messages":[{"content":[{"type":"text","text":"hello"},{"type":"video_url","video_url":{"url":"https://example.test/a"}}]}]}`, "alias", "", "", "unsupported_video_input", constant.ChannelTypeAnthropic},
		{"video compatible unknown", "/v1/chat/completions", `{"messages":[{"content":[{"type":"video_url"}]}]}`, "alias", "", "", "", constant.ChannelTypeOpenAI},
		{"mapped Vertex Claude", "/pg/chat/completions", `{"messages":[{"content":[{"type":"video_url"}]}]}`, "alias", `{"alias":"claude-x"}`, "", "unsupported_video_input", constant.ChannelTypeVertexAi},
		{"mapped Vertex Gemini", "/v1/chat/completions", `{"messages":[{"content":[{"type":"video_url"}]}]}`, "claude-alias", `{"claude-alias":"gemini-x"}`, "", "", constant.ChannelTypeVertexAi},
		{"AWS Claude", "/v1/chat/completions", `{"messages":[{"content":[{"type":"video_url"}]}]}`, "claude-x", "", "", "unsupported_video_input", constant.ChannelTypeAws},
		{"AWS Nova unknown", "/v1/chat/completions", `{"messages":[{"content":[{"type":"video_url"}]}]}`, "nova-pro", "", "", "", constant.ChannelTypeAws},
		{"Responses Claude", "/v1/responses", `{}`, "alias", "", "", "unsupported_endpoint", constant.ChannelTypeAnthropic},
		{"Responses AWS", "/v1/responses", `{}`, "alias", "", "", "unsupported_endpoint", constant.ChannelTypeAws},
		{"Responses compact Vertex", "/v1/responses/compact", `{}`, "alias", "", "", "unsupported_endpoint", constant.ChannelTypeVertexAi},
		{"native Claude", "/v1/messages", `{}`, "alias", "", "", "", constant.ChannelTypeAnthropic},
		{"pass through", "/v1/responses", `{}`, "alias", "", `{"pass_through_body_enabled":true}`, "", constant.ChannelTypeAnthropic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			require.NoError(t, PrepareInputCapability(c))
			ch := &model.Channel{Type: tc.channel, ModelMapping: &tc.mapping, Setting: &tc.settings}
			err := ChannelInputCapabilityError(c, ch, tc.model)
			if tc.code == "" {
				require.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			require.Equal(t, tc.code, string(err.GetErrorCode()))
			require.Equal(t, 400, err.StatusCode)
			require.True(t, types.IsSkipRetryError(err))
			model_setting.GetGlobalSettings().PassThroughRequestEnabled = true
			require.Nil(t, ChannelInputCapabilityError(c, ch, tc.model))
			model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
		})
	}
}

func TestInputFailureDiagnosticsAreNarrow(t *testing.T) {
	for _, tc := range []struct {
		message, category string
		status            int
	}{
		{"Unsupported MIME type video/mp4 in input_file.file_data", "unsupported_video", 400},
		{"Invalid file_data", "unsupported_file", 400},
		{"Check the request parameter input[0].content[0].file_data. (request id: example)", "unsupported_file", 400},
		{"Check the request parameter input[0].content[0].text.", "", 400},
		{"video_url cannot be converted to Claude Messages", "unsupported_video", 400},
		{"Unsupported MIME type application/pdf", "unsupported_file", 422},
		{"video/mp4 timeout", "", 504},
		{"file_data server unavailable", "", 500},
		{"Invalid video codec", "", 400},
		{"Unsupported MIME type video/mp4", "", 500},
	} {
		err := types.NewErrorWithStatusCode(errors.New(tc.message), types.ErrorCodeInvalidRequest, tc.status)
		category, _, hint := describeInputFailure(err)
		require.Equal(t, tc.category, category, tc.message)
		if category != "" {
			require.NotEmpty(t, hint)
		}
	}
}
