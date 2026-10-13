package service

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const requestResponsesStreamFailureKey = "request_responses_stream_failure_diagnostics"
const maxFailureOutputTypes = 16

// CaptureResponsesEmptyOutputDiagnostics keeps a bounded, metadata-only snapshot.
// Upstream status/type strings are untrusted: unknown values must not enter logs.
func CaptureResponsesEmptyOutputDiagnostics(c *gin.Context, summary *relaycommon.ResponsesCompletedSummary, decodeErrors int) {
	if c == nil {
		return
	}
	diagnostics := map[string]interface{}{
		"terminal_event":             "response.completed",
		"completed_response_present": summary != nil,
		"json_decode_errors":         decodeErrors,
		"response_written":           c.Writer.Written(),
	}
	if summary != nil {
		status := "unknown"
		switch summary.Status {
		case "completed", "failed", "incomplete", "in_progress", "queued", "cancelled":
			status = summary.Status
		}
		outputTypes := make([]string, 0, min(len(summary.OutputTypes), maxFailureOutputTypes))
		seen := make(map[string]bool)
		truncated := false
		for _, outputType := range summary.OutputTypes {
			label := failureOutputTypeLabel(outputType)
			if seen[label] {
				continue
			}
			if len(outputTypes) == maxFailureOutputTypes {
				truncated = true
				break
			}
			seen[label] = true
			outputTypes = append(outputTypes, label)
		}
		diagnostics["responses_completed_summary"] = map[string]interface{}{
			"status":                      status,
			"output_count":                summary.OutputCount,
			"output_types":                outputTypes,
			"output_types_truncated":      truncated,
			"message_count":               summary.MessageCount,
			"message_text_chars":          summary.MessageTextChars,
			"function_call_count":         summary.FunctionCallCount,
			"web_search_call_count":       summary.WebSearchCallCount,
			"image_generation_call_count": summary.ImageGenerationCallCount,
			"actionable_tool_call_count":  summary.ActionableToolCallCount,
		}
	}
	c.Set(requestResponsesStreamFailureKey, diagnostics)
}

func failureOutputTypeLabel(outputType string) string {
	switch outputType {
	case "message", "reasoning", "function_call", "web_search_call", "image_generation_call",
		"computer_call", "custom_tool_call", "file_search_call", "code_interpreter_call",
		"local_shell_call", "shell_call", "shell_call_output", "apply_patch_call",
		"apply_patch_call_output", "mcp_call", "mcp_list_tools", "mcp_approval_request",
		"tool_search_call", "tool_search_output":
		return outputType
	default:
		return "unknown"
	}
}
