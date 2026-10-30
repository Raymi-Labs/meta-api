package model

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"slices"

	"github.com/QuantumNous/new-api/common"
)

// Absent means unknown; false is an explicit unsupported value.
type ModelCapabilities map[string]bool

var ModelCapabilityKeys = []string{
	"reasoning", "streaming", "function_calling", "web_search", "url_context",
	"code_interpreter", "computer_use", "file_search", "memory", "structured_output",
	"structured_decision", "citations", "prompt_caching", "background", "server_side_context",
}

func (m ModelCapabilities) Clone() ModelCapabilities { return maps.Clone(m) }
func (m ModelCapabilities) Validate() error {
	for key := range m {
		if !slices.Contains(ModelCapabilityKeys, key) {
			return fmt.Errorf("模型能力包含不支持的类型")
		}
	}
	return nil
}
func (m ModelCapabilities) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return common.Marshal(map[string]bool(m))
}
func (m *ModelCapabilities) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	decoded := ModelCapabilities{}
	for key, value := range raw {
		var v interface{}
		if err := common.Unmarshal(value, &v); err != nil {
			return err
		}
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("模型能力必须为布尔值，未知能力请移除字段")
		}
		decoded[key] = b
	}
	if err := decoded.Validate(); err != nil {
		return err
	}
	*m = decoded
	return nil
}
func (m ModelCapabilities) Value() (driver.Value, error) {
	b, err := m.MarshalJSON()
	return string(b), err
}
func (m *ModelCapabilities) Scan(value interface{}) error {
	if value == nil {
		*m = ModelCapabilities{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("unsupported model capabilities database value %T", value)
	}
	if len(data) == 0 {
		*m = ModelCapabilities{}
		return nil
	}
	return common.Unmarshal(data, m)
}

type ModelAttributeSource struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// Sources are derived from private snapshots, never trusted from an admin payload.
// Only fields still equal to their last automatic value retain that attribution.
func (m Model) DisplayAttributeSources() map[string]ModelAttributeSource {
	s := readModelCapabilityState(m.ModelCapabilitySyncState)
	out := map[string]ModelAttributeSource{}
	add := func(key string, same bool) {
		if !same {
			return
		}
		source, ok := s.FieldSources[key]
		if !ok {
			source = ModelAttributeSource{Kind: "community", URL: s.Source}
		}
		if parsed, err := url.Parse(source.URL); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" {
			parsed.User = nil
			parsed.RawQuery = ""
			parsed.Fragment = ""
			source.URL = parsed.String()
			out[key] = source
		}
	}
	add("context_length", s.ContextLength != nil && m.ContextLength == *s.ContextLength)
	add("max_output_tokens", s.MaxOutputTokens != nil && m.MaxOutputTokens == *s.MaxOutputTokens)
	add("input_modalities", len(s.InputModalities) > 0 && slices.Equal(s.InputModalities, m.InputModalities))
	add("output_modalities", len(s.OutputModalities) > 0 && slices.Equal(s.OutputModalities, m.OutputModalities))
	add("release_date", s.ReleaseDate != nil && m.ReleaseDate == *s.ReleaseDate)
	for key, value := range s.Capabilities {
		local, ok := m.Capabilities[key]
		add("capabilities."+key, ok && local == value)
	}
	return out
}

// Exact names only, sharing the existing sync eligibility and enabled-model scope.
func ModelAttributeSyncCandidates(ctx context.Context) (map[string]bool, error) {
	names := map[string]bool{}
	if DB == nil {
		return names, nil
	}
	var configured []string
	if err := DB.WithContext(ctx).Model(&Model{}).Where("name_rule = ? AND sync_official = ?", NameRuleExact, 1).Pluck("model_name", &configured).Error; err != nil {
		return nil, err
	}
	var enabled []string
	if err := DB.WithContext(ctx).Model(&Ability{}).Where("enabled = ?", true).Distinct("model").Pluck("model", &enabled).Error; err != nil {
		return nil, err
	}
	for _, name := range append(configured, enabled...) {
		names[name] = true
	}
	return names, nil
}
