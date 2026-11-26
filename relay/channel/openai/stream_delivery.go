package openai

import (
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// filterChatStreamUsage keeps billing usage separate from the client's usage
// preference. Never drop content, tool calls or finish_reason in a mixed chunk.
func filterChatStreamUsage(data string, includeUsage bool) (string, error) {
	if includeUsage {
		return data, nil
	}
	var event map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(data, &event); err != nil {
		return "", err
	}
	if _, ok := event["usage"]; !ok {
		return data, nil
	}
	var choices []json.RawMessage
	if err := common.Unmarshal(event["choices"], &choices); err == nil && len(choices) == 0 {
		return "", nil
	}
	delete(event, "usage")
	encoded, err := common.Marshal(event)
	return string(encoded), err
}

func deliverClaudeStreamEvent(c *gin.Context, info *relaycommon.RelayInfo, event dto.ChatCompletionsStreamResponse, terminal **dto.ChatCompletionsStreamResponse) error {
	if len(event.Choices) == 0 {
		return nil // usage is collected separately; do not replay terminal data
	}
	if *terminal != nil {
		for _, choice := range event.Choices {
			if choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" || len(choice.Delta.ToolCalls) > 0 {
				return fmt.Errorf("upstream sent content after finish_reason")
			}
		}
		return nil
	}
	if event.IsFinished() {
		*terminal = event.Copy()
		for i := range (*terminal).Choices {
			(*terminal).Choices[i].Delta = dto.ChatCompletionsStreamResponseChoiceDelta{}
		}
		for i := range event.Choices {
			event.Choices[i].FinishReason = nil
		}
	}
	wire, err := common.Marshal(event)
	if err != nil {
		return err
	}
	return HandleStreamFormat(c, info, string(wire), false, false)
}

// Any committed lifecycle frame or ping rules out channel failover. A stream
// error must stay an error even if the HTTP headers already say 200.
func FinishChatStreamError(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) *types.NewAPIError {
	if info.SendResponseCount == 0 && !c.Writer.Written() {
		return apiErr
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyResponsesStreamErrorWritten) && c.Request.Context().Err() == nil {
		switch info.RelayFormat {
		case types.RelayFormatClaude:
			_ = helper.ClaudeData(c, dto.ClaudeResponse{Type: "error", Error: apiErr.ToClaudeError()})
			common.SetContextKey(c, constant.ContextKeyResponsesStreamErrorWritten, true)
		case types.RelayFormatOpenAI:
			_ = helper.ObjectData(c, map[string]any{"error": apiErr.ToOpenAIError()})
			common.SetContextKey(c, constant.ContextKeyResponsesStreamErrorWritten, true)
		}
	}
	return types.NewError(apiErr, apiErr.GetErrorCode(), types.ErrOptionWithSkipRetry())
}
