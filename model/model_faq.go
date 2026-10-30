package model

import (
	"database/sql/driver"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

type ModelFAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// ModelFAQ preserves editorial order; nil SQL values and nil slices are empty.
type ModelFAQ []ModelFAQItem

func (f ModelFAQ) Clone() ModelFAQ { return slices.Clone(f) }

func (f *ModelFAQ) Normalize() error {
	if len(*f) > 20 {
		return fmt.Errorf("模型 FAQ 最多配置 20 条")
	}
	normalized := make(ModelFAQ, 0, len(*f))
	for _, item := range *f {
		item.Question = strings.TrimSpace(item.Question)
		item.Answer = strings.TrimSpace(item.Answer)
		if item.Question == "" || item.Answer == "" {
			return fmt.Errorf("FAQ 的问题和答案不能为空")
		}
		if utf8.RuneCountInString(item.Question) > 200 {
			return fmt.Errorf("FAQ 问题不能超过 200 个字符")
		}
		if utf8.RuneCountInString(item.Answer) > 2000 {
			return fmt.Errorf("FAQ 答案不能超过 2000 个字符")
		}
		normalized = append(normalized, item)
	}
	encoded, err := normalized.MarshalJSON()
	if err != nil {
		return err
	}
	// Remain safely within MySQL TEXT's byte limit, including JSON escaping.
	if len(encoded) > 60000 {
		return fmt.Errorf("FAQ 内容过长，请减少问题或答案长度")
	}
	*f = normalized
	return nil
}

func (f ModelFAQ) MarshalJSON() ([]byte, error) {
	if f == nil {
		return []byte("[]"), nil
	}
	return common.Marshal([]ModelFAQItem(f))
}

func (f ModelFAQ) Value() (driver.Value, error) {
	data, err := f.MarshalJSON()
	return string(data), err
}

func (f *ModelFAQ) Scan(value interface{}) error {
	var data []byte
	switch value := value.(type) {
	case nil:
		*f = ModelFAQ{}
		return nil
	case string:
		data = []byte(value)
	case []byte:
		data = value
	default:
		return fmt.Errorf("unsupported model FAQ database value %T", value)
	}
	if len(data) == 0 {
		*f = ModelFAQ{}
		return nil
	}
	return common.Unmarshal(data, f)
}
