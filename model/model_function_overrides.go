package model

import (
	"database/sql/driver"
	"fmt"
	"maps"
	"slices"

	"github.com/QuantumNous/new-api/common"
)

// Missing keys follow automatic metadata; false explicitly excludes a task.
// Sync workers never write this administrator-owned field.
type ModelFunctionOverrides map[string]bool

var ModelFunctionKeys = []string{"text_chat", "image_understanding", "image_generation", "image_editing", "speech_generation", "speech_recognition", "realtime_audio", "video_generation", "embedding", "rerank"}

func (m ModelFunctionOverrides) Clone() ModelFunctionOverrides { return maps.Clone(m) }
func (m ModelFunctionOverrides) Validate() error {
	for key := range m {
		if !slices.Contains(ModelFunctionKeys, key) {
			return fmt.Errorf("invalid model function: %s", key)
		}
	}
	return nil
}
func (m ModelFunctionOverrides) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return common.Marshal(map[string]bool(m))
}
func (m *ModelFunctionOverrides) UnmarshalJSON(data []byte) error {
	var raw map[string]*bool
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	decoded := ModelFunctionOverrides{}
	for key, value := range raw {
		if value == nil {
			return fmt.Errorf("model function override must be boolean")
		}
		decoded[key] = *value
	}
	if err := decoded.Validate(); err != nil {
		return err
	}
	*m = decoded
	return nil
}
func (m ModelFunctionOverrides) Value() (driver.Value, error) {
	b, e := m.MarshalJSON()
	return string(b), e
}
func (m *ModelFunctionOverrides) Scan(value interface{}) error {
	if value == nil {
		*m = ModelFunctionOverrides{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("invalid model function database value %T", value)
	}
	if len(data) == 0 {
		*m = ModelFunctionOverrides{}
		return nil
	}
	return common.Unmarshal(data, m)
}
