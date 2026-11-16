package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestSyncRelayReasoningEffortFromResponsesPayload(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{}
	SyncRelayReasoningEffortFromResponsesPayload(info, []byte(`{
		"model":"gpt-5",
		"reasoning":{"effort":"high","summary":"auto"}
	}`))

	require.Equal(t, "high", info.ReasoningEffort)
}

func TestSyncRelayReasoningEffortFromResponsesPayload_ClearsMissingReasoning(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{ReasoningEffort: "medium"}
	SyncRelayReasoningEffortFromResponsesPayload(info, []byte(`{"model":"gpt-5"}`))

	require.Empty(t, info.ReasoningEffort)
}

func TestExtractReasoningSummaryTextFromResponses_CompatWrapper(t *testing.T) {
	t.Parallel()

	resp := &dto.OpenAIResponsesResponse{
		Output: []dto.ResponsesOutput{
			{
				Type: "reasoning",
				Summary: []dto.ResponsesReasoningSummaryPart{
					{Type: "summary_text", Text: "First summary."},
					{Type: "summary_text", Text: "Second summary."},
				},
			},
		},
	}

	require.Equal(t, "First summary.\n\nSecond summary.", ExtractReasoningSummaryTextFromResponses(resp))
}
