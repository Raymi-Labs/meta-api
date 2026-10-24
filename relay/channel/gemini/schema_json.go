package gemini

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/tidwall/gjson"
)

// Visit object fields in source order. Only explicitly transformed values are
// re-encoded; numeric limits and unrelated nested values retain their bytes.
func transformSchemaObject(value gjson.Result, transform func(string, gjson.Result) (json.RawMessage, bool)) json.RawMessage {
	out := []byte{'{'}
	value.ForEach(func(key, value gjson.Result) bool {
		raw, keep := transform(key.String(), value)
		if !keep {
			return true
		}
		if len(out) > 1 {
			out = append(out, ',')
		}
		name, _ := common.Marshal(key.String())
		out = append(out, name...)
		out = append(out, ':')
		out = append(out, raw...)
		return true
	})
	return append(out, '}')
}

func transformSchemaArray(value gjson.Result, transform func(json.RawMessage) json.RawMessage) json.RawMessage {
	out := []byte{'['}
	value.ForEach(func(_, item gjson.Result) bool {
		if len(out) > 1 {
			out = append(out, ',')
		}
		out = append(out, transform(json.RawMessage(item.Raw))...)
		return true
	})
	return append(out, ']')
}

func cleanRawFunctionParameters(raw json.RawMessage) any {
	root := gjson.ParseBytes(raw)
	properties := root.Get("properties")
	if properties.IsObject() && len(properties.Map()) == 0 {
		return nil
	}
	return cleanRawFunctionSchema(raw, 0)
}

func cleanRawFunctionSchema(raw json.RawMessage, depth int) json.RawMessage {
	value := gjson.ParseBytes(raw)
	if value.IsArray() {
		if depth >= geminiFunctionSchemaMaxDepth {
			return json.RawMessage(`[]`)
		}
		return transformSchemaArray(value, func(item json.RawMessage) json.RawMessage {
			return cleanRawFunctionSchema(item, depth+1)
		})
	}
	if !value.IsObject() {
		return raw
	}
	typeFields := make(map[string]any)
	if field := value.Get("type"); field.Exists() {
		var decoded any
		if common.Unmarshal([]byte(field.Raw), &decoded) == nil {
			typeFields["type"] = decoded
		}
	}
	if field := value.Get("nullable"); field.Exists() {
		var decoded any
		if common.Unmarshal([]byte(field.Raw), &decoded) == nil {
			typeFields["nullable"] = decoded
		}
	}
	normalizeGeminiSchemaTypeAndNullable(typeFields)
	out := transformSchemaObject(value, func(key string, field gjson.Result) (json.RawMessage, bool) {
		if _, allowed := geminiOpenAPISchemaAllowedFields[key]; !allowed {
			return nil, false
		}
		if depth >= geminiFunctionSchemaMaxDepth && (key == "properties" || key == "items" || key == "anyOf") {
			return nil, false
		}
		switch key {
		case "type", "nullable":
			if normalized, exists := typeFields[key]; exists {
				encoded, _ := common.Marshal(normalized)
				return encoded, true
			}
			return nil, false
		case "properties":
			if field.IsObject() {
				return transformSchemaObject(field, func(_ string, property gjson.Result) (json.RawMessage, bool) {
					return cleanRawFunctionSchema(json.RawMessage(property.Raw), depth+1), true
				}), true
			}
		case "items":
			if field.IsObject() {
				return cleanRawFunctionSchema(json.RawMessage(field.Raw), depth+1), true
			}
			if items := field.Array(); field.IsArray() && len(items) > 0 {
				return cleanRawFunctionSchema(json.RawMessage(items[0].Raw), depth+1), true
			}
		case "anyOf":
			if field.IsArray() {
				return transformSchemaArray(field, func(item json.RawMessage) json.RawMessage {
					return cleanRawFunctionSchema(item, depth+1)
				}), true
			}
		}
		return json.RawMessage(field.Raw), true
	})
	if nullable, ok := typeFields["nullable"].(bool); ok && nullable && !value.Get("nullable").Exists() {
		out = out[:len(out)-1]
		if len(out) > 1 {
			out = append(out, ',')
		}
		out = append(out, []byte(`"nullable":true}`)...)
	}
	return out
}

func cleanRawResponseSchema(raw json.RawMessage, depth int) json.RawMessage {
	if depth >= 5 {
		return raw
	}
	value := gjson.ParseBytes(raw)
	if !value.IsObject() {
		return raw
	}
	schemaType := value.Get("type").String()
	return transformSchemaObject(value, func(key string, field gjson.Result) (json.RawMessage, bool) {
		if key == "title" || key == "$schema" || schemaType == "object" && key == "additionalProperties" {
			return nil, false
		}
		if schemaType == "object" {
			if key == "properties" && field.IsObject() {
				return transformSchemaObject(field, func(_ string, property gjson.Result) (json.RawMessage, bool) {
					return cleanRawResponseSchema(json.RawMessage(property.Raw), depth+1), true
				}), true
			}
			if (key == "allOf" || key == "anyOf" || key == "oneOf") && field.IsArray() {
				return transformSchemaArray(field, func(item json.RawMessage) json.RawMessage {
					return cleanRawResponseSchema(item, depth+1)
				}), true
			}
		} else if schemaType == "array" && key == "items" && field.IsObject() {
			return cleanRawResponseSchema(json.RawMessage(field.Raw), depth+1), true
		}
		return json.RawMessage(field.Raw), true
	})
}
