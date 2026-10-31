package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const usageStart = `{"type":"message_start","message":{"id":"msg_fixture","model":"claude-fixture","usage":{"input_tokens":100,"output_tokens":1,"cache_read_input_tokens":30,"cache_creation_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":15}}}}`
const usageText = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"A short generated answer for the regression test."}}`
const usageStop = `{"type":"message_stop"}`

func runUsageStream(t *testing.T, format types.RelayFormat, events ...string) (*dto.Usage, *types.NewAPIError, *relaycommon.RelayInfo, *gin.Context, string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{RelayFormat: format, IsStream: true, DisablePing: true, ShouldIncludeUsage: true,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
	info.SetEstimatePromptTokens(80)
	body := "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
	usage, err := ClaudeStreamHandler(c, resp, info)
	return usage, err, info, c, w.Body.String()
}

func TestClaudeUsageTerminalFrames(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		for _, tc := range []struct {
			name   string
			events []string
		}{
			{"standard cumulative delta", []string{usageStart, usageText,
				`{"type":"message_delta","usage":{"output_tokens":12}}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}`, usageStop}},
			{"usage on message stop", []string{usageStart, usageText, `{"type":"message_stop","usage":{"output_tokens":20}}`}},
			{"all usage on message stop", []string{`{"type":"message_start","message":{"id":"msg_fixture","model":"claude-fixture"}}`, usageText,
				`{"type":"message_stop","usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":30,"cache_creation_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":15}}}`}},
		} {
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				u, err, info, ctx, body := runUsageStream(t, format, tc.events...)
				require.Nil(t, err)
				require.True(t, info.StreamStatus.IsSuccessful())
				require.Equal(t, 100, u.PromptTokens)
				require.Equal(t, 20, u.CompletionTokens)
				require.Equal(t, 120, u.TotalTokens)
				require.Equal(t, 30, u.PromptTokensDetails.CachedTokens)
				require.Equal(t, 20, u.PromptTokensDetails.CachedCreationTokens)
				require.Equal(t, 5, u.ClaudeCacheCreation5mTokens)
				require.Equal(t, 15, u.ClaudeCacheCreation1hTokens)
				require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
				if format == types.RelayFormatOpenAI {
					require.Contains(t, body, `"completion_tokens":20`)
					require.Equal(t, 1, strings.Count(body, "[DONE]"))
				} else {
					require.Contains(t, body, tc.events[len(tc.events)-1], "native terminal frame must remain intact")
				}
			})
		}
	}
}

func TestClaudeUsageMissingFinalUsagePreservesCache(t *testing.T) {
	u, err, info, c, _ := runUsageStream(t, types.RelayFormatClaude, usageStart, usageText, usageStop)
	require.Nil(t, err)
	require.True(t, info.StreamStatus.IsSuccessful())
	require.Equal(t, 100, u.PromptTokens)
	require.Greater(t, u.CompletionTokens, 1)
	require.Equal(t, 30, u.PromptTokensDetails.CachedTokens)
	require.Equal(t, 20, u.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 5, u.ClaudeCacheCreation5mTokens)
	require.Equal(t, 15, u.ClaudeCacheCreation1hTokens)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
}

func TestClaudeUsageExplicitZeroIsNotMissing(t *testing.T) {
	u, err, info, ctx, _ := runUsageStream(t, types.RelayFormatOpenAI, usageStart,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":0,"output_tokens":0}}`, usageStop)
	require.Nil(t, err)
	require.True(t, info.StreamStatus.IsSuccessful())
	require.Zero(t, u.PromptTokens)
	require.Zero(t, u.CompletionTokens)
	require.Equal(t, 30, u.PromptTokensDetails.CachedTokens)
	require.Equal(t, 20, u.PromptTokensDetails.CachedCreationTokens)
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
}

func TestClaudeUsageErrorAndTruncatedStreamAreNotSuccess(t *testing.T) {
	// Sanitized shape of the incident: HTTP 200 starts SSE, then an explicit
	// invalid_request_error rejects the request. No successful usage is invented.
	u, err, info, _, body := runUsageStream(t, types.RelayFormatOpenAI, usageStart,
		`{"type":"error","error":{"type":"invalid_request_error","message":"Output blocked by content filtering policy: Request blocked by Anthropic's Usage Policy."}}`,
		`{"type":"message_stop","usage":{"output_tokens":20}}`)
	require.Nil(t, u)
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.False(t, info.StreamStatus.IsSuccessful())
	require.NotContains(t, body, "[DONE]")
	for _, events := range [][]string{
		{usageStart, usageText},
		{usageStart, usageText, `{"type":"message_delta","usage":{"output_tokens":20}}`},
	} {
		_, err, info, _, _ := runUsageStream(t, types.RelayFormatClaude, events...)
		require.NotNil(t, err)
		require.False(t, info.StreamStatus.IsSuccessful())
	}
	u, err, info, _, _ = runUsageStream(t, types.RelayFormatClaude)
	require.NotNil(t, err)
	require.False(t, info.StreamStatus.IsSuccessful())
	require.Nil(t, u, "an interrupted stream must not provide billable usage")
}

func TestClaudeNonStreamMissingUsage(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		info := &relaycommon.RelayInfo{RelayFormat: format, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
		info.SetEstimatePromptTokens(80)
		body := `{"type":"message","id":"msg_fixture","model":"claude-fixture","role":"assistant","content":[{"type":"text","text":"An answer with missing usage."}],"stop_reason":"end_turn"}`
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
		u, err := ClaudeHandler(ctx, resp, info)
		require.Nil(t, err)
		require.Equal(t, 80, u.PromptTokens)
		require.Equal(t, service.EstimateTokenByModel("claude-fixture", "An answer with missing usage."), u.CompletionTokens)
		require.True(t, u.InputTokensEstimated)
		if format == types.RelayFormatClaude {
			require.Equal(t, body, w.Body.String())
		} else {
			require.EqualValues(t, 80, gjson.Get(w.Body.String(), "usage.prompt_tokens").Int())
		}
	}
}
