package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// These tests run serially because they capture the shared logger sink.
func captureResponsesToolWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	original := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = original
		common.LogWriterMu.Unlock()
	})
	return &logs
}

func TestResponsesToolDeclarationsPreserveResponseAndBillableUsage(t *testing.T) {
	const declarations = `{"type":"namespace","name":"workspace","tools":[{"type":"function","name":"read","parameters":{"type":"object"}},{"type":"custom","name":"patch","format":{"type":"text"}}]},{"type":"function","name":"standalone"},{"type":"custom","name":"freeform"},`
	const bodyFormat = `{"id":"resp_tools","object":"response","status":"completed","model":"gpt-5.1","tools":[%s{"type":"web_search"},{"type":"file_search"},{"type":"image_generation"}],"usage":{"input_tokens":20,"output_tokens":7,"total_tokens":27,"input_tokens_details":{"cached_tokens":11}},"output":[{"type":"function_call","id":"fc_read","call_id":"call_read","name":"workspace.read","arguments":"{\"path\":\"example.txt\"}"},{"type":"web_search_call","id":"ws_1","status":"completed"},{"type":"web_search_call","id":"ws_2","status":"completed"},{"type":"image_generation_call","id":"img_1","status":"completed","quality":"medium","size":"1024x1024","result":"aW1hZ2U="}]}`

	for _, mode := range []string{"json", "sse", "aggregate"} {
		t.Run(mode, func(t *testing.T) {
			logs := captureResponsesToolWarnings(t)
			var baselineUsage *dto.Usage
			var baselineQuota service.ToolCallResult
			var baselineOutput any
			for _, withDeclarations := range []bool{false, true} {
				c, recorder := newResponsesStreamTestContext()
				info := &relaycommon.RelayInfo{
					RelayFormat: types.RelayFormatOpenAIResponses,
					ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
					ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
						dto.BuildInToolWebSearchPreview: {ToolName: dto.BuildInToolWebSearchPreview},
						dto.BuildInToolFileSearch:       {ToolName: dto.BuildInToolFileSearch},
						dto.BuildInToolImageGeneration:  {ToolName: dto.BuildInToolImageGeneration},
						// A declared function may already be present in the request map.
						"function": {ToolName: "function"},
					}},
				}
				tools := ""
				if withDeclarations {
					tools = declarations
				}
				body := fmt.Sprintf(bodyFormat, tools)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
				handler := OaiResponsesHandler
				if mode != "json" {
					resp = newResponsesSSEResponse(`data: {"type":"response.completed","response":` + body + `}`)
					handler = OaiResponsesStreamHandler
					if mode == "aggregate" {
						handler = OaiResponsesAggregateHandler
					}
				}
				usage, apiErr := handler(c, info, resp)
				require.Nil(t, apiErr)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Equal(t, 20, usage.PromptTokens)
				require.Equal(t, 7, usage.CompletionTokens)
				require.Equal(t, 27, usage.TotalTokens)
				require.Equal(t, 11, usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, 2, usage.WebSearchRequests)
				registered := info.ResponsesUsageInfo.BuiltInTools
				require.Equal(t, 2, registered[dto.BuildInToolWebSearchPreview].CallCount)
				require.Equal(t, 1, registered[dto.BuildInToolFileSearch].CallCount)
				require.Equal(t, 1, registered[dto.BuildInToolImageGeneration].CallCount)
				require.Zero(t, registered["function"].CallCount)
				require.True(t, c.GetBool("image_generation_call"))
				require.Equal(t, 1, c.GetInt("image_generation_call_count"))
				quota := service.ComputeToolCallQuota(service.ToolCallUsage{
					ModelName: "gpt-5.1", WebSearchCalls: registered[dto.BuildInToolWebSearchPreview].CallCount,
					WebSearchToolName: dto.BuildInToolWebSearchPreview, FileSearchCalls: registered[dto.BuildInToolFileSearch].CallCount,
					ImageGenerationCalls:   c.GetInt("image_generation_call_count"),
					ImageGenerationQuality: c.GetString("image_generation_call_quality"), ImageGenerationSize: c.GetString("image_generation_call_size"),
				}, 1)
				require.Positive(t, quota.TotalQuota)
				if withDeclarations {
					require.Equal(t, baselineUsage, usage)
					require.Equal(t, baselineQuota, quota)
				} else {
					baselineUsage, baselineQuota = usage, quota
				}

				var expected, actual map[string]any
				require.NoError(t, common.UnmarshalJsonStr(body, &expected))
				if mode == "sse" {
					for _, line := range strings.Split(recorder.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: ") || !strings.Contains(line, `"type":"response.completed"`) {
							continue
						}
						var event map[string]any
						require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
						actual = event["response"].(map[string]any)
					}
				} else {
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &actual))
				}
				require.NotNil(t, actual)
				require.Equal(t, expected["tools"], actual["tools"])
				if mode != "aggregate" {
					require.Equal(t, expected["output"], actual["output"])
				}
				call := actual["output"].([]any)[0].(map[string]any)
				for key, value := range expected["output"].([]any)[0].(map[string]any) {
					require.Equal(t, value, call[key], "function call field %s", key)
				}
				if withDeclarations {
					require.Equal(t, baselineOutput, actual["output"])
				} else {
					baselineOutput = actual["output"]
				}
			}
			require.Empty(t, logs.String(), "valid client tool declarations must not emit warnings or errors")
		})
	}
}

func TestResponsesMissingToolMetadataWarningsAreBoundedAndSanitized(t *testing.T) {
	logs := captureResponsesToolWarnings(t)
	c, _ := newResponsesStreamTestContext()
	info := &relaycommon.RelayInfo{ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
		dto.BuildInToolFileSearch: nil,
	}}}
	response := &dto.OpenAIResponsesResponse{}
	for i := 0; i < 100; i++ {
		for _, toolType := range []string{"namespace", "function", "custom", "web_search", "web_search_preview", "file_search", "image_generation", fmt.Sprintf("private-upstream-value-%d\n[ERR] injected", i)} {
			response.Tools = append(response.Tools, map[string]any{"type": toolType})
		}
	}
	require.NotNil(t, buildResponsesUsage(c, info, response))
	require.Equal(t, 4, strings.Count(logs.String(), "[WARN]"))
	for _, category := range []string{"web_search_preview", "file_search", "image_generation", "unknown"} {
		require.Equal(t, 1, strings.Count(logs.String(), "tool_type="+category))
	}
	require.NotContains(t, logs.String(), "[ERR]")
	require.NotContains(t, logs.String(), "private-upstream-value")
	require.NotContains(t, logs.String(), "namespace")
}
