package openaicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type legacyFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// NormalizeChatToolProtocol converts the deprecated Chat Completions function
// protocol into the modern tool protocol before the request enters the
// Responses session bridge. Modern requests are left unchanged.
func NormalizeChatToolProtocol(req *dto.GeneralOpenAIRequest) (dto.ChatToolProtocol, error) {
	if req == nil {
		return dto.ChatToolProtocolNone, fmt.Errorf("request is nil")
	}

	functions, functionsPresent, err := parseLegacyFunctions(req.Functions)
	if err != nil {
		return dto.ChatToolProtocolNone, err
	}
	functionChoice, functionChoicePresent, forcedFunction, err := parseLegacyFunctionChoice(req.FunctionCall)
	if err != nil {
		return dto.ChatToolProtocolNone, err
	}

	modernPresent := len(req.Tools) > 0 || req.ToolChoice != nil
	legacyTopLevelPresent := functionsPresent || functionChoicePresent
	if modernPresent && legacyTopLevelPresent {
		return dto.ChatToolProtocolNone, fmt.Errorf("ambiguous tool protocol: do not send tools/tool_choice together with functions/function_call")
	}

	legacyHistoryPresent, err := normalizeLegacyFunctionHistory(req.Messages)
	if err != nil {
		return dto.ChatToolProtocolNone, err
	}

	protocol := dto.ChatToolProtocolNone
	if modernPresent {
		protocol = dto.ChatToolProtocolModern
	} else if legacyTopLevelPresent || legacyHistoryPresent {
		protocol = dto.ChatToolProtocolLegacy
	}

	if legacyTopLevelPresent {
		if forcedFunction != "" && !containsLegacyFunction(functions, forcedFunction) {
			return dto.ChatToolProtocolNone, fmt.Errorf("function_call references an undefined function")
		}
		if req.ParallelTooCalls != nil && *req.ParallelTooCalls {
			return dto.ChatToolProtocolNone, fmt.Errorf("legacy functions protocol does not support parallel_tool_calls=true")
		}

		req.Tools = make([]dto.ToolCallRequest, 0, len(functions))
		for _, function := range functions {
			req.Tools = append(req.Tools, dto.ToolCallRequest{
				Type:     "function",
				Function: function,
			})
		}
		req.ToolChoice = functionChoice
		parallel := false
		req.ParallelTooCalls = &parallel
	}

	// Remove deprecated fields after normalization so session hashes and the
	// upstream payload have one canonical representation.
	req.Functions = nil
	req.FunctionCall = nil
	return protocol, nil
}

func parseLegacyFunctions(raw json.RawMessage) ([]dto.FunctionRequest, bool, error) {
	if !rawJSONPresent(raw) {
		return nil, false, nil
	}

	var functions []dto.FunctionRequest
	if err := common.Unmarshal(raw, &functions); err != nil {
		return nil, false, fmt.Errorf("invalid functions: %w", err)
	}
	if len(functions) == 0 {
		return nil, false, nil
	}

	seen := make(map[string]struct{}, len(functions))
	for i := range functions {
		functions[i].Name = strings.TrimSpace(functions[i].Name)
		if functions[i].Name == "" {
			return nil, false, fmt.Errorf("functions[%d].name is required", i)
		}
		if _, exists := seen[functions[i].Name]; exists {
			return nil, false, fmt.Errorf("duplicate function name at functions[%d]", i)
		}
		seen[functions[i].Name] = struct{}{}
		if functions[i].Parameters != nil {
			if !dto.IsSchemaObject(functions[i].Parameters) {
				return nil, false, fmt.Errorf("functions[%d].parameters must be a JSON object", i)
			}
		}
	}
	return functions, true, nil
}

func parseLegacyFunctionChoice(raw json.RawMessage) (any, bool, string, error) {
	if !rawJSONPresent(raw) {
		return nil, false, "", nil
	}

	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, false, "", fmt.Errorf("invalid function_call: %w", err)
	}
	switch choice := value.(type) {
	case string:
		choice = strings.TrimSpace(choice)
		if choice != "auto" && choice != "none" {
			return nil, false, "", fmt.Errorf("invalid function_call value %q", choice)
		}
		return choice, true, "", nil
	case map[string]any:
		name := strings.TrimSpace(common.Interface2String(choice["name"]))
		if name == "" {
			return nil, false, "", fmt.Errorf("function_call.name is required")
		}
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": name,
			},
		}, true, name, nil
	default:
		return nil, false, "", fmt.Errorf("function_call must be a string or object")
	}
}

func normalizeLegacyFunctionHistory(messages []dto.Message) (bool, error) {
	legacyPresent := false
	usedCallIDs := make(map[string]struct{})
	for i := range messages {
		for _, toolCall := range messages[i].ParseToolCalls() {
			if toolCall.ID != "" {
				usedCallIDs[toolCall.ID] = struct{}{}
			}
		}
		if messages[i].ToolCallId != "" {
			usedCallIDs[messages[i].ToolCallId] = struct{}{}
		}
	}

	type pendingLegacyCall struct {
		id   string
		name string
	}
	var pending *pendingLegacyCall
	nextCallID := 1

	for i := range messages {
		message := &messages[i]
		role := strings.TrimSpace(message.Role)
		if pending != nil && role != "function" {
			return false, fmt.Errorf("legacy function call must be followed by a role=function result")
		}

		if rawJSONPresent(message.FunctionCall) {
			legacyPresent = true
			if role != "assistant" {
				return false, fmt.Errorf("messages[%d].function_call is only valid for role=assistant", i)
			}
			if len(message.ParseToolCalls()) > 0 {
				return false, fmt.Errorf("messages[%d] mixes function_call and tool_calls", i)
			}

			var functionCall legacyFunctionCall
			if err := common.Unmarshal(message.FunctionCall, &functionCall); err != nil {
				return false, fmt.Errorf("invalid messages[%d].function_call: %w", i, err)
			}
			functionCall.Name = strings.TrimSpace(functionCall.Name)
			if functionCall.Name == "" {
				return false, fmt.Errorf("messages[%d].function_call.name is required", i)
			}

			callID := ""
			for callID == "" {
				candidate := fmt.Sprintf("legacy_call_%d", nextCallID)
				nextCallID++
				if _, exists := usedCallIDs[candidate]; !exists {
					callID = candidate
					usedCallIDs[callID] = struct{}{}
				}
			}
			toolCalls, err := common.Marshal([]dto.ToolCallRequest{{
				ID:   callID,
				Type: "function",
				Function: dto.FunctionRequest{
					Name:      functionCall.Name,
					Arguments: functionCall.Arguments,
				},
			}})
			if err != nil {
				return false, fmt.Errorf("marshal messages[%d].tool_calls: %w", i, err)
			}
			message.ToolCalls = toolCalls
			message.FunctionCall = nil
			pending = &pendingLegacyCall{id: callID, name: functionCall.Name}
			continue
		}

		if role != "function" {
			continue
		}
		legacyPresent = true
		if pending == nil && strings.TrimSpace(message.ToolCallId) == "" {
			return false, fmt.Errorf("messages[%d] role=function has no preceding function_call", i)
		}
		if pending != nil {
			if message.Name != nil && strings.TrimSpace(*message.Name) != "" && strings.TrimSpace(*message.Name) != pending.name {
				return false, fmt.Errorf("messages[%d] function result name does not match the pending call", i)
			}
			message.ToolCallId = pending.id
			pending = nil
		}
		message.Role = "tool"
	}

	if pending != nil {
		return false, fmt.Errorf("legacy function call is missing its role=function result")
	}
	return legacyPresent, nil
}

func containsLegacyFunction(functions []dto.FunctionRequest, name string) bool {
	for _, function := range functions {
		if function.Name == name {
			return true
		}
	}
	return false
}

func rawJSONPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}
