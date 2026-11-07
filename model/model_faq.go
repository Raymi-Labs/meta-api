package model

import (
	"context"
	"database/sql/driver"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

type ModelFAQItem struct {
	Question             string            `json:"question"`
	Answer               string            `json:"answer"`
	QuestionOverrides    ModelDescriptions `json:"question_overrides,omitempty"`
	AnswerOverrides      ModelDescriptions `json:"answer_overrides,omitempty"`
	QuestionAutomatic    ModelDescriptions `json:"question_automatic,omitempty"`
	AnswerAutomatic      ModelDescriptions `json:"answer_automatic,omitempty"`
	QuestionTranslations ModelDescriptions `json:"question_translations,omitempty"`
	AnswerTranslations   ModelDescriptions `json:"answer_translations,omitempty"`
}

// ModelFAQ preserves editorial order; nil SQL values and nil slices are empty.
type ModelFAQ []ModelFAQItem

func (f ModelFAQ) Clone() ModelFAQ {
	next := slices.Clone(f)
	for i := range next {
		next[i].QuestionOverrides = f[i].QuestionOverrides.Clone()
		next[i].AnswerOverrides = f[i].AnswerOverrides.Clone()
		next[i].QuestionAutomatic = f[i].QuestionAutomatic.Clone()
		next[i].AnswerAutomatic = f[i].AnswerAutomatic.Clone()
		next[i].QuestionTranslations = f[i].QuestionTranslations.Clone()
		next[i].AnswerTranslations = f[i].AnswerTranslations.Clone()
	}
	return next
}

func normalizeFAQOverrides(values ModelDescriptions, limit int) (ModelDescriptions, error) {
	var result ModelDescriptions
	for locale, text := range values {
		if !ValidModelDescriptionLocale(locale) || !utf8.ValidString(text) || utf8.RuneCountInString(text) > limit {
			return nil, fmt.Errorf("FAQ 翻译语言或长度无效: %s", locale)
		}
		if text = strings.TrimSpace(text); text != "" {
			if result == nil {
				result = ModelDescriptions{}
			}
			result[locale] = text
		}
	}
	return result, nil
}

func (f ModelFAQ) stored() ModelFAQ {
	next := f.Clone()
	for i := range next {
		next[i].QuestionAutomatic, next[i].AnswerAutomatic = nil, nil
		next[i].QuestionTranslations, next[i].AnswerTranslations = nil, nil
	}
	return next
}

func (f *ModelFAQ) Normalize() error {
	if len(*f) > 20 {
		return fmt.Errorf("模型 FAQ 最多配置 20 条")
	}
	normalized := make(ModelFAQ, 0, len(*f))
	for _, item := range f.stored() {
		var err error
		if item.QuestionOverrides, err = normalizeFAQOverrides(item.QuestionOverrides, 800); err != nil {
			return err
		}
		if item.AnswerOverrides, err = normalizeFAQOverrides(item.AnswerOverrides, 8000); err != nil {
			return err
		}
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
	data, err := f.stored().MarshalJSON()
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

// Automatic content lives in the shared translation cache, never in FAQ TEXT.
func FillModelFAQTranslations(ctx context.Context, models []*Model) error {
	hashes := []string{}
	seen := map[string]bool{}
	for _, m := range models {
		if m == nil {
			continue
		}
		m.FAQ = m.FAQ.stored()
		for i := range m.FAQ {
			m.FAQ[i].QuestionTranslations = m.FAQ[i].QuestionOverrides.Clone()
			m.FAQ[i].AnswerTranslations = m.FAQ[i].AnswerOverrides.Clone()
		}
		for _, item := range m.FAQ {
			for _, source := range []string{item.Question, item.Answer} {
				hash := ModelDescriptionHash(source)
				if !seen[hash] {
					hashes = append(hashes, hash)
					seen[hash] = true
				}
			}
		}
	}
	cache := map[string]ModelDescriptions{}
	for start := 0; start < len(hashes); start += 100 {
		var rows []ModelDescriptionTranslation
		if err := DB.WithContext(ctx).Where("source_hash IN ? AND completed = ?", hashes[start:min(start+100, len(hashes))], true).Order("cache_key ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if !ValidModelDescriptionLocale(row.Locale) || strings.TrimSpace(row.Text) == "" {
				continue
			}
			if cache[row.SourceHash] == nil {
				cache[row.SourceHash] = ModelDescriptions{}
			}
			if _, exists := cache[row.SourceHash][row.Locale]; !exists || strings.HasPrefix(row.CacheKey, SystemTranslationModel+":") {
				cache[row.SourceHash][row.Locale] = row.Text
			}
		}
	}
	merge := func(auto, manual ModelDescriptions) ModelDescriptions {
		result := auto.Clone()
		if result == nil {
			result = ModelDescriptions{}
		}
		for locale, text := range manual {
			if strings.TrimSpace(text) != "" {
				result[locale] = text
			}
		}
		return result
	}
	for _, m := range models {
		if m == nil {
			continue
		}
		for i := range m.FAQ {
			item := &m.FAQ[i]
			item.QuestionAutomatic = cache[ModelDescriptionHash(item.Question)].Clone()
			item.AnswerAutomatic = cache[ModelDescriptionHash(item.Answer)].Clone()
			item.QuestionTranslations = merge(item.QuestionAutomatic, item.QuestionOverrides)
			item.AnswerTranslations = merge(item.AnswerAutomatic, item.AnswerOverrides)
		}
	}
	return nil
}
