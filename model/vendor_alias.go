package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Only confirmed identity aliases: providers, plans and model families are not
// inferred from similar names. MiMo/Alibaba retain their model-aware migrations.
var identityVendorAliases = map[string][]string{
	"Moonshot AI": {"Moonshot AI", "Moonshot", "MoonshotAI"},
	"智谱":          {"智谱", "Z.AI", "ChatGLM", "Zhipu", "Zhipu AI"},
	"Inception":   {"Inception", "Inception Labs", "InceptionLabs"},
}

func canonicalIdentityVendor(name string) string {
	for canonical, aliases := range identityVendorAliases {
		for _, alias := range aliases {
			if strings.EqualFold(strings.TrimSpace(name), alias) {
				return canonical
			}
		}
	}
	return ""
}

type vendorAliasNormalizationResult struct {
	RenamedVendors   int
	ReassignedModels int64
	RemovedAliases   int
}

func normalizeIdentityVendorMetadata(db *gorm.DB) (vendorAliasNormalizationResult, error) {
	result := vendorAliasNormalizationResult{}
	if db == nil {
		return result, fmt.Errorf("database is nil")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var vendors []Vendor
		if err := tx.Order("id ASC").Find(&vendors).Error; err != nil {
			return err
		}
		// Fixed order keeps diagnostics and lock acquisition deterministic.
		for _, name := range []string{"Moonshot AI", "智谱", "Inception"} {
			var aliases []*Vendor
			var target *Vendor
			for i := range vendors {
				v := &vendors[i]
				if canonicalIdentityVendor(v.Name) != name {
					continue
				}
				aliases = append(aliases, v)
				if target == nil || (v.Name == name && target.Name != name) {
					target = v
				}
			}
			if target == nil {
				continue
			}
			now := common.GetTimestamp()
			updates := map[string]interface{}{}
			if target.Name != name {
				updates["name"] = name
			}
			if strings.TrimSpace(target.Icon) == "" {
				updates["icon"] = getDefaultVendorIcon(name)
			}
			if len(updates) > 0 {
				updates["updated_time"] = now
				if err := tx.Model(target).Updates(updates).Error; err != nil {
					return err
				}
				result.RenamedVendors++
			}
			for _, alias := range aliases {
				if alias.Id == target.Id {
					continue
				}
				var rows []Model
				if err := tx.Unscoped().Where("vendor_id = ?", alias.Id).Find(&rows).Error; err != nil {
					return err
				}
				for _, row := range rows {
					changes := map[string]interface{}{"vendor_id": target.Id, "updated_time": now}
					// Existing helper changes only a matching owned ID, preserving provenance,
					// unknown fields and manual/malformed snapshots verbatim.
					if state := reassignAlibabaVendorSnapshot(row.ModelCapabilitySyncState, alias.Id, target.Id); state != row.ModelCapabilitySyncState {
						changes["model_capability_sync_state"] = state
					}
					changed := tx.Unscoped().Model(&Model{}).Where("id = ? AND vendor_id = ?", row.Id, alias.Id).Updates(changes)
					if changed.Error != nil {
						return changed.Error
					}
					result.ReassignedModels += changed.RowsAffected
				}
				var refs int64
				if err := tx.Unscoped().Model(&Model{}).Where("vendor_id = ?", alias.Id).Count(&refs).Error; err != nil {
					return err
				}
				if refs != 0 {
					return fmt.Errorf("vendor alias still referenced: %d", alias.Id)
				}
				if err := tx.Delete(alias).Error; err != nil {
					return err
				}
				result.RemovedAliases++
			}
		}
		return nil
	})
	if err != nil {
		return vendorAliasNormalizationResult{}, err
	}
	return result, nil
}

func NormalizeIdentityVendorMetadata() error {
	result, err := normalizeIdentityVendorMetadata(DB)
	if err == nil && result != (vendorAliasNormalizationResult{}) {
		common.SysLog(fmt.Sprintf("normalized vendor identities: updated=%d models=%d aliases=%d", result.RenamedVendors, result.ReassignedModels, result.RemovedAliases))
	}
	return err
}
