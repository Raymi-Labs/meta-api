package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeReviewResultsPreserveVerdictsAndToolIDs(t *testing.T) {
	for _, outcome := range []string{"not_flagged", "flagged"} {
		t.Run(outcome, func(t *testing.T) {
			result := `[{"type":"dangerous_tool_use","status":{"type":"available","tool_uses":{"toolu_review":{"type":"evaluated","outcome":"` + outcome + `"}}}}]`
			t.Run("json", func(t *testing.T) {
				w := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(w)
				info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{}}
				state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
				body := `{"id":"msg_review","type":"message","model":"claude-test","role":"assistant","content":[{"type":"tool_use","id":"toolu_review","name":"Bash","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":2,"output_tokens":9},"safeguard_results":` + result + `}`
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
				require.Nil(t, HandleClaudeResponseData(ctx, info, state, resp, []byte(body)))
				require.JSONEq(t, body, w.Body.String())
			})
			t.Run("stream", func(t *testing.T) {
				w := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(w)
				info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{}}
				state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
				events := []string{
					`{"type":"message_start","message":{"id":"msg_review","model":"claude-test","usage":{"input_tokens":2}}}`,
					`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_review","name":"Bash","input":{}}}`,
					`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9},"safeguard_results":` + result + `}`,
					`{"type":"message_stop"}`,
				}
				for _, event := range events {
					require.Nil(t, HandleStreamResponseData(ctx, info, state, event))
				}
				HandleStreamFinalResponse(ctx, info, state)
				wire := w.Body.String()
				require.Contains(t, wire, `"safeguard_results":`+result)
				require.Contains(t, wire, `"id":"toolu_review"`)
				require.Less(t, strings.Index(wire, "safeguard_results"), strings.LastIndex(wire, "message_stop"))
			})
		})
	}
}
