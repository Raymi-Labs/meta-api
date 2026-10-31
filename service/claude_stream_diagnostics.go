package service

import (
	"errors"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// Request settings are explicitly labelled requested, not the effective
// post-override provider settings. Unknown values and content never enter logs.
func CaptureClaudeRequestSettings(c *gin.Context, request *dto.ClaudeRequest) {
	if c == nil || request == nil {
		return
	}
	settings := map[string]interface{}{"max_tokens": request.MaxTokens}
	if request.Thinking != nil {
		switch request.Thinking.Type {
		case "enabled", "disabled", "adaptive":
			settings["thinking_type"] = request.Thinking.Type
		}
	}
	var output struct {
		Effort string `json:"effort"`
	}
	if len(request.OutputConfig) > 0 && common.Unmarshal(request.OutputConfig, &output) == nil {
		switch output.Effort {
		case "low", "medium", "high", "max", "xhigh":
			settings["effort"] = output.Effort
		}
	}
	c.Set("claude_requested_settings", settings)
}

func CaptureClaudeStreamFailure(c *gin.Context, status *relaycommon.StreamStatus, receivedStop bool) {
	if c == nil || status == nil {
		return
	}
	diagnostics := map[string]interface{}{
		"end_reason": string(status.EndReason), "message_stop_received": receivedStop,
		"completion_delivered": status.HasDeliveredCompletion(),
	}
	if status.EndError != nil {
		kind := "read_error"
		if errors.Is(status.EndError, io.ErrUnexpectedEOF) {
			kind = "unexpected_eof"
		} else if strings.Contains(strings.ToLower(status.EndError.Error()), "connection reset by peer") {
			kind = "connection_reset"
		}
		diagnostics["read_error_kind"] = kind
	}
	c.Set("claude_stream_diagnostics", diagnostics)
}

func AppendClaudeStreamDiagnostics(c *gin.Context, admin map[string]interface{}) {
	if c == nil || admin == nil {
		return
	}
	for _, key := range []string{"claude_requested_settings", "claude_stream_diagnostics"} {
		if value, ok := c.Get(key); ok {
			admin[key] = value
		}
	}
}
