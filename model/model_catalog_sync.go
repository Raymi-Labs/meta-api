package model

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Share the same cross-node ability lock as manual creation and source sync.
func createCatalogModel(ctx context.Context, fresh *Model) (bool, error) {
	created := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := lockModelMetadataCreationAbility(tx, fresh.ModelName)
		if err != nil {
			return err
		}
		if !locked {
			return nil
		}
		var count int64
		if err := tx.Unscoped().Model(&Model{}).Where("model_name = ?", fresh.ModelName).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		var currentRules []Model
		if err := tx.Unscoped().Where("name_rule <> ?", NameRuleExact).Find(&currentRules).Error; err != nil {
			return err
		}
		if capabilityCoveredByRule(fresh.ModelName, currentRules) {
			return nil
		}
		if err := tx.Create(fresh).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// EnsureModelCatalog reconciles the same enabled abilities used by pricing.
// Remote metadata is optional: unknown IDs remain editable with unknown attributes.
// Existing rules and tombstones are deliberate admin choices; never shadow them.
func EnsureModelCatalog(ctx context.Context) (int, error) {
	var names []string
	if err := DB.WithContext(ctx).Model(&Ability{}).Where("enabled = ?", true).
		Distinct("model").Order("model").Pluck("model", &names).Error; err != nil {
		return 0, err
	}
	if len(names) == 0 {
		return 0, nil
	}
	var rows []Model
	if err := DB.WithContext(ctx).Unscoped().Select("model_name", "name_rule").Find(&rows).Error; err != nil {
		return 0, err
	}
	existing := make(map[string]bool, len(rows))
	var rules []Model
	for _, row := range rows {
		existing[row.ModelName] = true
		if row.NameRule != NameRuleExact {
			rules = append(rules, row)
		}
	}
	created := 0
	for _, name := range names {
		// Metadata names use varchar(128) on all supported database engines.
		if name == "" || strings.TrimSpace(name) != name || utf8.RuneCountInString(name) > 128 || existing[name] || capabilityCoveredByRule(name, rules) {
			continue
		}
		now := common.GetTimestamp()
		fresh := Model{ModelName: name, Status: 1, SyncOfficial: 1, NameRule: NameRuleExact, CreatedTime: now, UpdatedTime: now}
		added, err := createCatalogModel(ctx, &fresh)
		if err != nil {
			return created, err
		}
		if added {
			created++
		}
	}
	return created, nil
}
