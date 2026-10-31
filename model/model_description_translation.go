package model

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	_ "embed"
	"fmt"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

// Reviewed translations can ship without a dependency on free-service availability.
// They are matched by exact canonical source, never by model name.
//
//go:embed data/model_description_translations.json
var bundledModelDescriptionsJSON []byte
var bundledModelDescriptions = func() map[string]ModelDescriptions {
	var entries []struct {
		Source       string            `json:"source"`
		Translations ModelDescriptions `json:"translations"`
	}
	result := map[string]ModelDescriptions{}
	if common.Unmarshal(bundledModelDescriptionsJSON, &entries) == nil {
		for _, entry := range entries {
			result[ModelDescriptionHash(entry.Source)] = entry.Translations
		}
	}
	return result
}()

// Keep in parity with web/src/helpers/localeNormalize.js; tested against that file.
var ModelDescriptionLocales = []string{"en", "zh-CN", "zh-TW", "ja", "ko", "de", "es", "fr", "pt", "ru", "tr", "pl", "nl", "it", "vi", "uk", "bg"}

func ValidModelDescriptionLocale(locale string) bool {
	for _, supported := range ModelDescriptionLocales {
		if locale == supported {
			return true
		}
	}
	return false
}

type ModelDescriptions map[string]string

func (m ModelDescriptions) Clone() ModelDescriptions { return maps.Clone(m) }
func (m ModelDescriptions) Value() (driver.Value, error) {
	b, err := common.Marshal(m)
	return string(b), err
}
func (m *ModelDescriptions) Scan(v interface{}) error {
	if v == nil {
		*m = nil
		return nil
	}
	var b []byte
	switch v := v.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return fmt.Errorf("invalid model descriptions storage")
	}
	if len(b) == 0 {
		*m = nil
		return nil
	}
	return common.Unmarshal(b, m)
}
func ValidateDescriptionOverrides(overrides map[string]*string) error {
	for locale, text := range overrides {
		if !ValidModelDescriptionLocale(locale) {
			return fmt.Errorf("invalid description locale: %s", locale)
		}
		if text != nil && (!utf8.ValidString(*text) || utf8.RuneCountInString(*text) > 16000) {
			return fmt.Errorf("model description exceeds 16000 characters")
		}
	}
	return nil
}
func CanonicalModelDescription(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
}
func ModelDescriptionHash(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(CanonicalModelDescription(text))))
}

// Derived translations are shared by exact content, never by model-name guesses.
// Source changes select a new hash, making outdated translations unreachable.
type ModelDescriptionTranslation struct {
	CacheKey      string `gorm:"primaryKey;size:96"`
	SourceHash    string `gorm:"size:64;index"`
	Locale        string `gorm:"size:16"`
	Text          string `gorm:"type:text"`
	Chunk         int
	Completed     bool
	NextAttemptAt int64
	Failures      int
}

// A durable global lease/cooldown prevents duplicate workers and restart bypasses.
type ModelDescriptionWorker struct {
	ID         int    `gorm:"primaryKey;autoIncrement:false"`
	Lease      string `gorm:"size:64"`
	LeaseUntil int64
	NextRunAt  int64
	Failures   int
}

func DescriptionCacheKey(source, locale string) string {
	return ModelDescriptionHash(source) + ":" + locale
}

func FillModelDescriptionTranslations(ctx context.Context, models []*Model) error {
	hashes := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, m := range models {
		if m == nil {
			continue
		}
		m.DescriptionAutomatic = bundledModelDescriptions[ModelDescriptionHash(m.Description)].Clone()
		if m.DescriptionAutomatic == nil {
			m.DescriptionAutomatic = ModelDescriptions{}
		}
		m.DescriptionTranslations = m.DescriptionOverrides.Clone()
		if m.DescriptionTranslations == nil {
			m.DescriptionTranslations = ModelDescriptions{}
		}
		if CanonicalModelDescription(m.Description) == "" {
			continue
		}
		for locale, text := range m.DescriptionAutomatic {
			if _, manual := m.DescriptionTranslations[locale]; !manual {
				m.DescriptionTranslations[locale] = text
			}
		}
		hash := ModelDescriptionHash(m.Description)
		if !seen[hash] {
			hashes = append(hashes, hash)
			seen[hash] = true
		}
	}
	cache := map[string]ModelDescriptions{}
	for start := 0; start < len(hashes); start += 100 {
		end := min(start+100, len(hashes))
		var rows []ModelDescriptionTranslation
		if err := DB.WithContext(ctx).Where("source_hash IN ? AND completed = ?", hashes[start:end], true).Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if !ValidModelDescriptionLocale(r.Locale) {
				continue
			}
			if cache[r.SourceHash] == nil {
				cache[r.SourceHash] = ModelDescriptions{}
			}
			cache[r.SourceHash][r.Locale] = r.Text
		}
	}
	for _, m := range models {
		if m == nil {
			continue
		}
		for locale, text := range cache[ModelDescriptionHash(m.Description)] {
			if _, reviewed := m.DescriptionAutomatic[locale]; reviewed {
				continue
			}
			m.DescriptionAutomatic[locale] = text
			if _, manual := m.DescriptionTranslations[locale]; !manual {
				m.DescriptionTranslations[locale] = text
			}
		}
	}
	return nil
}

func ClaimModelDescriptionWorker(ctx context.Context, now int64, lease string) (ModelDescriptionWorker, bool, error) {
	state := ModelDescriptionWorker{ID: 1}
	db := DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return state, false, err
	}
	result := db.Model(&ModelDescriptionWorker{}).Where("id = ? AND lease_until <= ? AND next_run_at <= ?", 1, now, now).Updates(map[string]interface{}{"lease": lease, "lease_until": now + 120})
	if result.Error != nil || result.RowsAffected == 0 {
		return state, false, result.Error
	}
	err := db.First(&state, 1).Error
	return state, err == nil, err
}
func FinishModelDescriptionWorker(ctx context.Context, lease string, next int64, failures int) error {
	return DB.WithContext(ctx).Model(&ModelDescriptionWorker{}).Where("id = ? AND lease = ?", 1, lease).Updates(map[string]interface{}{"lease_until": 0, "next_run_at": next, "failures": failures}).Error
}
