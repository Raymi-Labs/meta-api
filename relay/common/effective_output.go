package common

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func (info *RelayInfo) ObserveGroupHealthOutput() {
	if info == nil {
		return
	}
	now := time.Now()
	if info.GroupHealthFirstOutputTime.IsZero() {
		info.GroupHealthFirstOutputTime = now
	}
	info.GroupHealthLastOutputTime = now
}

// ObserveOpenAIStreamOutput deliberately ignores role-only, usage and empty
// chunks. Reasoning and tool-call content are useful output, unlike a heartbeat
// or a tool index/ID announcement. Completion-style choices use text directly.
func (info *RelayInfo) ObserveOpenAIStreamOutput(data string) {
	if info == nil {
		return
	}
	var event struct {
		Choices []struct {
			Text  string                                       `json:"text"`
			Delta dto.ChatCompletionsStreamResponseChoiceDelta `json:"delta"`
		} `json:"choices"`
	}
	if common.UnmarshalJsonStr(data, &event) != nil {
		return
	}
	for _, choice := range event.Choices {
		delta := choice.Delta
		effective := choice.Text != "" || delta.GetContentString() != "" || delta.GetReasoningContent() != ""
		if delta.FunctionCall != nil {
			effective = effective || delta.FunctionCall.Name != "" || delta.FunctionCall.Arguments != ""
		}
		for _, call := range delta.ToolCalls {
			effective = effective || call.Function.Name != "" || call.Function.Arguments != ""
		}
		if effective {
			info.ObserveGroupHealthOutput()
			return
		}
	}
}
