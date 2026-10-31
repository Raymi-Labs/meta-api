package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// SanitizeClaudeToolInput repairs tool arguments that are valid JSON but
// violate conditional rules enforced by Claude Code at runtime.
func SanitizeClaudeToolInput(toolName string, input any) any {
	sanitized, _ := sanitizeClaudeToolInput(toolName, input)
	return sanitized
}

func sanitizeClaudeToolInput(toolName string, input any) (any, bool) {
	if !strings.EqualFold(strings.TrimSpace(toolName), "SendMessage") {
		return input, false
	}

	object, ok := input.(map[string]any)
	if !ok {
		return input, false
	}

	changed := removeShutdownApprovalReason(object)
	if message, ok := object["message"].(map[string]any); ok {
		changed = removeShutdownApprovalReason(message) || changed
	}
	return object, changed
}

// SanitizeClaudeToolArguments is the JSON-string counterpart used by the
// Responses streaming bridge.
func SanitizeClaudeToolArguments(toolName, raw string) string {
	if !strings.EqualFold(strings.TrimSpace(toolName), "SendMessage") || raw == "" {
		return raw
	}

	var input any
	if err := common.DecodeJsonUseNumberStrict(strings.NewReader(raw), &input); err != nil {
		return raw
	}
	sanitized, changed := sanitizeClaudeToolInput(toolName, input)
	if !changed {
		return raw
	}
	encoded, err := common.Marshal(sanitized)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func removeShutdownApprovalReason(object map[string]any) bool {
	if object["type"] != "shutdown_response" || object["approve"] != true {
		return false
	}
	if _, ok := object["reason"]; !ok {
		return false
	}
	delete(object, "reason")
	return true
}
