package openai

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiRequestPreservesMixedContentAndTools(t *testing.T) {
	call := `{"functionCall":{"name":"lookup","args":{"city":"杭州"}}}`
	for _, tc := range []struct {
		name, parts, content string
		calls                int
	}{
		{"text_and_two_calls", `[{"text":"checking"},` + call + `,{"functionCall":{"name":"finish","args":{"ok":true}}}]`, `"checking"`, 2},
		{"media_and_call", `[{"text":"before"},` + call + `,{"inlineData":{"mimeType":"image/png","data":"AAAA"}},{"fileData":{"mimeType":"image/jpeg","fileUri":"https://example.test/image.jpg"}},{"text":"after"}]`, `[{"type":"text","text":"before"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA","detail":"auto","MimeType":"image/png"}},{"type":"image_url","image_url":{"url":"https://example.test/image.jpg","detail":"auto","MimeType":"image/jpeg"}},{"type":"text","text":"after"}]`, 1},
		{"tool_only", `[` + call + `]`, `null`, 1},
		{"text_only", `[{"text":"hello"}]`, `"hello"`, 0},
		{"empty", `[]`, ``, 0},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				var input dto.GeminiChatRequest
				require.NoError(t, common.Unmarshal([]byte(`{"contents":[{"role":"model","parts":`+tc.parts+`}],"generationConfig":{"maxOutputTokens":37}}`), &input))
				before, err := common.Marshal(input)
				require.NoError(t, err)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{IsStream: stream, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "mapped-model", SupportsChatStreamOptions: true}}
				converted, err := (&Adaptor{}).ConvertGeminiRequest(c, info, &input)
				require.NoError(t, err)
				wire, err := common.Marshal(converted)
				require.NoError(t, err)
				var output struct {
					Model     string `json:"model"`
					MaxTokens int    `json:"max_tokens"`
					Stream    bool   `json:"stream"`
					Messages  []struct {
						Role    string                `json:"role"`
						Content json.RawMessage       `json:"content"`
						Calls   []dto.ToolCallRequest `json:"tool_calls"`
					} `json:"messages"`
				}
				require.NoError(t, common.Unmarshal(wire, &output))
				require.Equal(t, "mapped-model", output.Model)
				require.Equal(t, 37, output.MaxTokens)
				require.Equal(t, stream, output.Stream)
				if tc.name == "empty" {
					require.Empty(t, output.Messages)
				} else {
					require.Len(t, output.Messages, 1)
					message := output.Messages[0]
					require.Equal(t, "assistant", message.Role)
					if tc.content == "null" {
						require.True(t, len(message.Content) == 0 || string(message.Content) == "null")
					} else {
						require.JSONEq(t, tc.content, string(message.Content))
					}
					require.Len(t, message.Calls, tc.calls)
					if tc.calls > 0 {
						require.Equal(t, "call_1", message.Calls[0].ID)
						require.Equal(t, "lookup", message.Calls[0].Function.Name)
						require.JSONEq(t, `{"city":"杭州"}`, message.Calls[0].Function.Arguments)
					}
					if tc.calls > 1 {
						require.Equal(t, "call_2", message.Calls[1].ID)
						require.Equal(t, "finish", message.Calls[1].Function.Name)
					}
				}
				after, err := common.Marshal(input)
				require.NoError(t, err)
				require.Equal(t, string(before), string(after))
			})
		}
	}
}
