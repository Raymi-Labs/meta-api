package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHeaderTimeoutFailoverGuards(t *testing.T) {
	old := common.RetryTimes
	common.RetryTimes = 3
	t.Cleanup(func() { common.RetryTimes = old })
	for _, name := range []string{"plain", "function_tool", "ordinary_504", "second_attempt", "budget_exhausted", "output", "canceled", "fixed", "responses", "container", "mcp", "server_tool", "extra_body", "override", "passthrough", "disabled"} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set("use_channel", []string{"1"})
			req := &dto.GeneralOpenAIRequest{}
			info := &relaycommon.RelayInfo{Request: req, RelayFormat: types.RelayFormatOpenAI}
			err := types.NewErrorWithStatusCode(errors.New("timeout awaiting response headers"), types.ErrorCodeDoRequestFailed, 504, types.ErrOptionWithSkipRetry())
			err.ResponseHeaderTimeout = true
			switch name {
			case "function_tool":
				req.Tools = []dto.ToolCallRequest{{Type: "function"}}
			case "ordinary_504":
				err.ResponseHeaderTimeout = false
			case "second_attempt":
				c.Set("use_channel", []string{"1", "2"})
			case "budget_exhausted":
				c.Set("use_channel", []string{"1", "2", "3", "4"})
			case "output":
				c.Writer.Write([]byte("data: body\n\n"))
			case "canceled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "fixed":
				c.Set("specific_channel_id", 1)
			case "responses":
				info.Request = &dto.OpenAIResponsesRequest{PreviousResponseID: "resp_old"}
			case "container", "mcp", "server_tool":
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				info.RelayFormat = types.RelayFormatClaude
				claude := &dto.ClaudeRequest{}
				info.Request = claude
				if name == "container" {
					claude.Container = []byte(`"upstream-container"`)
				}
				if name == "mcp" {
					claude.McpServers = []byte(`[{"url":"https://example.invalid"}]`)
				}
				if name == "server_tool" {
					claude.Tools = []map[string]string{{"type": "web_search_20250305"}}
				}
			case "extra_body":
				req.ExtraBody = []byte(`{"conversation":"bound"}`)
			case "override":
				info.ChannelMeta = &relaycommon.ChannelMeta{ParamOverride: map[string]interface{}{"container": "bound"}}
			case "passthrough":
				info.ChannelMeta = &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true}}
			case "disabled":
				common.RetryTimes = 0
				defer func() { common.RetryTimes = 3 }()
			}
			require.Equal(t, name == "plain" || name == "function_tool" || name == "second_attempt", canFailoverResponseHeaderTimeout(c, info, err))
			claudeErr := types.WithOpenAIError(types.OpenAIError{Code: "claude_conversion_unsupported", Message: "The request cannot be represented by the selected Anthropic protocol"}, http.StatusBadRequest)
			claudeErr.UpstreamStatusCode = http.StatusBadRequest
			require.Equal(t, name == "plain" || name == "function_tool" || name == "second_attempt" || name == "ordinary_504", canFailoverClaudeCapability(c, info, claudeErr), "Claude rejection shares the replay safety boundary")
		})
	}
}
