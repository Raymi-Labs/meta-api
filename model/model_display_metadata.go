package model

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// ModelModalities is a JSON string array on the API and portable TEXT in SQL.
// An empty array means the model's modalities have not been configured.
type ModelModalities []string

func (m ModelModalities) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("[]"), nil
	}
	return common.Marshal([]string(m))
}

func (m ModelModalities) Value() (driver.Value, error) {
	data, err := m.MarshalJSON()
	if err != nil {
		return nil, err
	}
	// A string avoids PostgreSQL treating JSON bytes as a bytea parameter.
	return string(data), nil
}

func (m *ModelModalities) Scan(value interface{}) error {
	var data []byte
	switch value := value.(type) {
	case nil:
		*m = ModelModalities{}
		return nil
	case string:
		data = []byte(value)
	case []byte:
		data = value
	default:
		return fmt.Errorf("unsupported model modalities database value %T", value)
	}
	if len(data) == 0 {
		*m = ModelModalities{}
		return nil
	}
	var decoded []string
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = ModelModalities(decoded)
	return nil
}

func normalizeModelModalities(values ModelModalities) (ModelModalities, error) {
	order := []string{"text", "image", "audio", "video", "file"}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		switch value {
		case "text", "image", "audio", "video", "file":
			seen[value] = true
		default:
			return nil, fmt.Errorf("模型模态包含不支持的类型")
		}
	}
	result := make(ModelModalities, 0, len(seen))
	for _, value := range order {
		if seen[value] {
			result = append(result, value)
		}
	}
	return result, nil
}

// NormalizeDisplayMetadata validates presentation only; it does not change
// model identifiers, routing capabilities, permissions, or billing.
func (m *Model) NormalizeDisplayMetadata() error {
	m.DisplayName = strings.TrimSpace(m.DisplayName)
	if utf8.RuneCountInString(m.DisplayName) > 128 {
		return fmt.Errorf("显示名称不能超过 128 个字符")
	}
	// Keep numeric values exact in the browser as well as the SQL bigint column.
	if m.ContextLength < 0 || m.ContextLength > 9007199254740991 {
		return fmt.Errorf("上下文长度必须是非负安全整数")
	}
	var err error
	if m.InputModalities, err = normalizeModelModalities(m.InputModalities); err != nil {
		return err
	}
	if m.OutputModalities, err = normalizeModelModalities(m.OutputModalities); err != nil {
		return err
	}
	return nil
}
