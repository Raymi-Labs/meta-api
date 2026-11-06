package model

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reviewed exact-source replacements. Raw source/provenance is not proof of
// the actual request provider. Never perform global brand-name removal.
//
//go:embed data/public_model_descriptions.json
var reviewedPublicDescriptionsJSON []byte

type publicDescriptionReview struct {
	Model           string            `json:"model"`
	Source          string            `json:"source"`
	OldSource       string            `json:"old_source"`
	OldTranslations ModelDescriptions `json:"old_translations"`
	Translations    ModelDescriptions `json:"translations"`
}

var publicDescriptionReviews = func() map[string]publicDescriptionReview {
	var rows []publicDescriptionReview
	if err := common.Unmarshal(reviewedPublicDescriptionsJSON, &rows); err != nil {
		panic(err)
	}
	result := map[string]publicDescriptionReview{}
	for _, row := range rows {
		result[row.Model] = row
	}
	return result
}()
var providerDescriptionTemplate = regexp.MustCompile(`^([^\r\n]+) is an AI model provided by ([A-Za-z0-9_-]+(?:[. ][A-Za-z0-9_-]+)*)\.$`)
var vendorAccessTemplate = regexp.MustCompile(`^通过 OpenRouter 接入的 ([^\r\n]+) 模型$`)

// Ember's catalog reference is plain model text, not an external navigation link.
// Check the actual hostname so lookalike domains and unrelated model docs survive.
var emberReferenceLink = regexp.MustCompile(`\[([^\]\r\n]+)\]\((https?://[^)\s]+)\)(\.\)\.?)?`)

func cleanEmberReferenceLinks(name, text string) string {
	if name != "ember-1" && name != "fireworks/ember-1" {
		return text
	}
	return emberReferenceLink.ReplaceAllStringFunc(text, func(link string) string {
		parts := emberReferenceLink.FindStringSubmatch(link)
		address, err := url.Parse(parts[2])
		if err != nil || !strings.EqualFold(address.Hostname(), "openrouter.ai") {
			return link
		}
		suffix := ""
		// Some legacy translations duplicated the closing parenthesis after a period.
		if parts[3] != "" {
			suffix = "."
		}
		return parts[1] + suffix
	})
}

// NormalizePublicModelDescription strips only a full catalog placeholder, not
// normal prose that mentions an author, provider, license or technical limit.
func NormalizePublicModelDescription(name, text string) string {
	text = cleanEmberReferenceLinks(name, text)
	trimmed := strings.TrimSpace(text)
	if matches := providerDescriptionTemplate.FindStringSubmatch(trimmed); matches != nil {
		if review, ok := publicDescriptionReviews[name]; ok {
			return review.Source
		}
		return matches[1] + " is an AI model."
	}
	if review, ok := publicDescriptionReviews[name]; ok {
		for locale, old := range review.OldTranslations {
			if trimmed == strings.TrimSpace(old) {
				return review.Translations[locale]
			}
		}
	}
	return text
}
func normalizePublicDescriptionLocale(name, locale, text string) string {
	text = cleanEmberReferenceLinks(name, text)
	if review, ok := publicDescriptionReviews[name]; ok {
		trimmed := strings.TrimSpace(text)
		if providerDescriptionTemplate.MatchString(trimmed) || trimmed == strings.TrimSpace(review.OldTranslations[locale]) && trimmed != "" {
			return review.Translations[locale]
		}
	}
	return text
}
func reviewedPublicTranslations(name, source string) ModelDescriptions {
	review, ok := publicDescriptionReviews[name]
	if ok && ModelDescriptionHash(source) == ModelDescriptionHash(review.Source) {
		return review.Translations.Clone()
	}
	return nil
}
func NormalizePublicVendorDescription(name, text string) string {
	if vendorAccessTemplate.MatchString(strings.TrimSpace(text)) {
		return name + " 的模型系列。可用型号、接口与价格见模型列表。"
	}
	return text
}
func normalizePublicModelMetadata(m *Model) {
	m.Description = NormalizePublicModelDescription(m.ModelName, m.Description)
	if m.DescriptionOverrides != nil {
		m.DescriptionOverrides = m.DescriptionOverrides.Clone()
		for locale, text := range m.DescriptionOverrides {
			m.DescriptionOverrides[locale] = normalizePublicDescriptionLocale(m.ModelName, locale, text)
		}
	}
}

// Keep owned descriptions consistent without inventing automatic ownership.
// Unknown keys and private provider/source provenance remain intact.
func cleanDescriptionSnapshot(raw, old string) string {
	var fields map[string]json.RawMessage
	var owned string
	if common.UnmarshalJsonStr(raw, &fields) != nil || common.Unmarshal(fields["description"], &owned) != nil || owned != old {
		return raw
	}
	delete(fields, "description") // reviewed copy is no longer owned by the imported description
	var sources map[string]json.RawMessage
	if common.Unmarshal(fields["field_sources"], &sources) == nil {
		delete(sources, "description")
		fields["field_sources"], _ = common.Marshal(sources)
	}
	encoded, err := common.Marshal(fields)
	if err != nil {
		return raw
	}
	return string(encoded)
}

type publicDescriptionCleanupResult struct{ Models, Vendors int64 }

func normalizePublicDescriptions(db *gorm.DB) (publicDescriptionCleanupResult, error) {
	result := publicDescriptionCleanupResult{}
	err := db.Transaction(func(tx *gorm.DB) error {
		locked := tx
		if tx.Dialector.Name() != "sqlite" {
			locked = tx.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		// Bounded batches; inspect all active metadata rather than hard-coding DB IDs.
		var models []Model
		if err := locked.FindInBatches(&models, 100, func(batch *gorm.DB, _ int) error {
			for _, m := range models {
				old := m.Description
				next := m
				normalizePublicModelMetadata(&next)
				updates := map[string]interface{}{}
				if old != next.Description {
					updates["description"] = next.Description
					if state := cleanDescriptionSnapshot(m.ModelCapabilitySyncState, old); state != m.ModelCapabilitySyncState {
						updates["model_capability_sync_state"] = state
					}
				}
				for locale, text := range m.DescriptionOverrides {
					if next.DescriptionOverrides[locale] != text {
						updates["description_overrides"] = next.DescriptionOverrides
						break
					}
				}
				if len(updates) == 0 {
					continue
				}
				updates["updated_time"] = common.GetTimestamp()
				r := tx.Model(&Model{}).Where("id = ?", m.Id).Updates(updates)
				if r.Error != nil {
					return r.Error
				}
				result.Models += r.RowsAffected
			}
			return nil
		}).Error; err != nil {
			return err
		}
		var vendors []Vendor
		return locked.FindInBatches(&vendors, 100, func(batch *gorm.DB, _ int) error {
			for _, v := range vendors {
				next := NormalizePublicVendorDescription(v.Name, v.Description)
				if next == v.Description {
					continue
				}
				r := tx.Model(&Vendor{}).Where("id = ?", v.Id).Updates(map[string]interface{}{"description": next, "updated_time": common.GetTimestamp()})
				if r.Error != nil {
					return r.Error
				}
				result.Vendors += r.RowsAffected
			}
			return nil
		}).Error
	})
	if err != nil {
		return publicDescriptionCleanupResult{}, err
	}
	return result, nil
}
func NormalizePublicDescriptions() error {
	result, err := normalizePublicDescriptions(DB)
	if err == nil && result != (publicDescriptionCleanupResult{}) {
		common.SysLog(fmt.Sprintf("normalized public model descriptions: models=%d vendors=%d", result.Models, result.Vendors))
	}
	return err
}
