package dto

import (
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// Keep schema payloads opaque on the wire while allowing existing callers to
// construct DTOs with maps. A missing or null optional field keeps its old nil
// semantics; a non-null payload retains property order and integer precision.
func rawSchemaValue(raw json.RawMessage) any {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil
	}
	return raw
}

func IsSchemaObject(value any) bool {
	switch schema := value.(type) {
	case map[string]any:
		return true
	case json.RawMessage:
		return common.GetJsonType(schema) == "object"
	default:
		return false
	}
}

func (f *FunctionRequest) UnmarshalJSON(data []byte) error {
	type fields FunctionRequest
	var decoded struct {
		fields
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*f = FunctionRequest(decoded.fields)
	f.Parameters = rawSchemaValue(decoded.Parameters)
	return nil
}

func (t *Tool) UnmarshalJSON(data []byte) error {
	type fields Tool
	var decoded struct {
		fields
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if schema := rawSchemaValue(decoded.InputSchema); schema != nil && !IsSchemaObject(schema) {
		return fmt.Errorf("tool input_schema must be a JSON object")
	}
	*t = Tool(decoded.fields)
	t.InputSchema = rawSchemaValue(decoded.InputSchema)
	return nil
}

func (f *FormatJsonSchema) UnmarshalJSON(data []byte) error {
	type fields FormatJsonSchema
	var decoded struct {
		fields
		Schema json.RawMessage `json:"schema"`
	}
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*f = FormatJsonSchema(decoded.fields)
	f.Schema = rawSchemaValue(decoded.Schema)
	return nil
}

func (c *ClaudeRequest) UnmarshalJSON(data []byte) error {
	type fields ClaudeRequest
	var decoded struct {
		fields
		Tools json.RawMessage `json:"tools"`
	}
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ClaudeRequest(decoded.fields)
	c.Tools = rawSchemaValue(decoded.Tools)
	return nil
}

func (t *GeminiChatTool) UnmarshalJSON(data []byte) error {
	type fields GeminiChatTool
	var decoded struct {
		fields
		FunctionDeclarations json.RawMessage `json:"functionDeclarations"`
	}
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*t = GeminiChatTool(decoded.fields)
	t.FunctionDeclarations = rawSchemaValue(decoded.FunctionDeclarations)
	return nil
}
