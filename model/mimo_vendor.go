package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	defaultMiMoVendorName        = "小米 MiMo"
	legacyMiMoVendorName         = "MiMo"
	legacyXiaomiMiMoVendorName   = "Xiaomi MiMo"
	defaultMiMoVendorDescription = "小米 MiMo 系列模型"
)

type mimoVendorNormalizationResult struct {
	VendorID           int
	CreatedVendor      bool
	RenamedVendor      bool
	UpdatedIcon        bool
	UpdatedDescription bool
	ReassignedModels   int64
	RemovedAliases     int
}

func (r mimoVendorNormalizationResult) changed() bool {
	return r.CreatedVendor || r.RenamedVendor || r.UpdatedIcon || r.UpdatedDescription ||
		r.ReassignedModels > 0 || r.RemovedAliases > 0
}

func isMiMoVendorAlias(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case strings.ToLower(defaultMiMoVendorName),
		strings.ToLower(legacyMiMoVendorName),
		strings.ToLower(legacyXiaomiMiMoVendorName):
		return true
	default:
		return false
	}
}

// CanonicalVendorName normalizes known display vendor aliases while leaving
// unrelated administrator-defined vendor names untouched.
func CanonicalVendorName(name string) string {
	trimmed := strings.TrimSpace(name)
	if isMiMoVendorAlias(trimmed) {
		return defaultMiMoVendorName
	}
	if isInceptionVendorAlias(trimmed) {
		return defaultInceptionVendorName
	}
	return trimmed
}

func vendorNameCandidates(name string) []string {
	canonicalName := CanonicalVendorName(name)
	if canonicalName == defaultInceptionVendorName {
		return []string{defaultInceptionVendorName, "Inception Labs", "InceptionLabs"}
	}
	if canonicalName != defaultMiMoVendorName {
		return []string{canonicalName}
	}
	return []string{
		defaultMiMoVendorName,
		legacyMiMoVendorName,
		legacyXiaomiMiMoVendorName,
	}
}

func findVendorByCanonicalName(db *gorm.DB, name string) (*Vendor, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	canonicalName := CanonicalVendorName(name)
	if canonicalName == "" {
		return nil, gorm.ErrRecordNotFound
	}

	query := db
	if canonicalName == defaultMiMoVendorName || canonicalName == defaultInceptionVendorName {
		candidates := vendorNameCandidates(canonicalName)
		for i := range candidates {
			candidates[i] = strings.ToLower(candidates[i])
		}
		query = query.Where("LOWER(TRIM(name)) IN ?", candidates)
	} else {
		query = query.Where("name = ?", canonicalName)
	}

	var vendors []Vendor
	if err := query.Order("id ASC").Find(&vendors).Error; err != nil {
		return nil, err
	}
	for i := range vendors {
		if vendors[i].Name == canonicalName {
			return &vendors[i], nil
		}
	}
	if len(vendors) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &vendors[0], nil
}

// FindVendorByCanonicalName finds the canonical record first and falls back to
// legacy aliases. The fallback prevents a sync request from creating a second
// vendor when a known alias already exists in the database.
func FindVendorByCanonicalName(name string) (*Vendor, error) {
	return findVendorByCanonicalName(DB, name)
}

// ResolveVendorNameForModel keeps model-name inference in one place so
// upstream synchronization and the pricing cache use the same boundary rules.
func ResolveVendorNameForModel(modelName string, fallbackVendorName string) string {
	if getDefaultVendorName(modelName) == defaultMiMoVendorName {
		return defaultMiMoVendorName
	}
	if isInceptionModel(modelName) {
		return defaultInceptionVendorName
	}
	return CanonicalVendorName(fallbackVendorName)
}

// GetDefaultVendorIcon exposes the canonical icon lookup to synchronization
// code without coupling controllers to the underlying icon map.
func GetDefaultVendorIcon(vendorName string) string {
	return getDefaultVendorIcon(CanonicalVendorName(vendorName))
}

func defaultMiMoVendorUpdates(vendor *Vendor, now int64) map[string]interface{} {
	updates := make(map[string]interface{})
	if vendor.Name != defaultMiMoVendorName {
		updates["name"] = defaultMiMoVendorName
	}
	if strings.TrimSpace(vendor.Icon) == "" {
		updates["icon"] = getDefaultVendorIcon(defaultMiMoVendorName)
	}
	if strings.TrimSpace(vendor.Description) == "" {
		updates["description"] = defaultMiMoVendorDescription
	}
	if len(updates) > 0 {
		updates["updated_time"] = now
	}
	return updates
}

func normalizeMiMoVendorMetadata(db *gorm.DB) (mimoVendorNormalizationResult, error) {
	result := mimoVendorNormalizationResult{}
	if db == nil {
		return result, fmt.Errorf("database is nil")
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		var vendors []Vendor
		if err := tx.Order("id ASC").Find(&vendors).Error; err != nil {
			return err
		}

		var canonical *Vendor
		aliases := make([]*Vendor, 0)
		for i := range vendors {
			vendor := &vendors[i]
			if !isMiMoVendorAlias(vendor.Name) {
				continue
			}
			if vendor.Name == defaultMiMoVendorName && canonical == nil {
				canonical = vendor
				continue
			}
			aliases = append(aliases, vendor)
		}

		aliasIDs := make([]int, 0, len(aliases))
		for _, alias := range aliases {
			aliasIDs = append(aliasIDs, alias.Id)
		}

		var relatedModels []Model
		query := tx.Where("(vendor_id = ? OR vendor_id IS NULL)", 0)
		if len(aliasIDs) > 0 {
			query = query.Or("vendor_id IN ?", aliasIDs)
		}
		if err := query.Find(&relatedModels).Error; err != nil {
			return err
		}

		nonMiMoByAlias := make(map[int]bool, len(aliasIDs))
		hasMiMoCandidate := false
		for i := range relatedModels {
			item := &relatedModels[i]
			if getDefaultVendorName(item.ModelName) == defaultMiMoVendorName {
				hasMiMoCandidate = true
				continue
			}
			if item.VendorID != 0 {
				nonMiMoByAlias[item.VendorID] = true
			}
		}

		target := canonical
		if target == nil {
			for _, alias := range aliases {
				if !nonMiMoByAlias[alias.Id] {
					target = alias
					break
				}
			}
		}

		if target == nil && !hasMiMoCandidate {
			return nil
		}

		now := common.GetTimestamp()
		if target == nil {
			target = &Vendor{
				Name:        defaultMiMoVendorName,
				Description: defaultMiMoVendorDescription,
				Icon:        getDefaultVendorIcon(defaultMiMoVendorName),
				Status:      1,
				CreatedTime: now,
				UpdatedTime: now,
			}
			if err := tx.Create(target).Error; err != nil {
				return err
			}
			result.CreatedVendor = true
		} else {
			updates := defaultMiMoVendorUpdates(target, now)
			if len(updates) > 0 {
				if _, ok := updates["name"]; ok {
					result.RenamedVendor = true
				}
				if _, ok := updates["icon"]; ok {
					result.UpdatedIcon = true
				}
				if _, ok := updates["description"]; ok {
					result.UpdatedDescription = true
				}
				if err := tx.Model(&Vendor{}).Where("id = ?", target.Id).Updates(updates).Error; err != nil {
					return err
				}
				target.Name = defaultMiMoVendorName
				if icon, ok := updates["icon"].(string); ok {
					target.Icon = icon
				}
			}
		}

		result.VendorID = target.Id
		modelIDsToMove := make([]int, 0)
		for i := range relatedModels {
			item := &relatedModels[i]
			if item.VendorID == target.Id || getDefaultVendorName(item.ModelName) != defaultMiMoVendorName {
				continue
			}
			modelIDsToMove = append(modelIDsToMove, item.Id)
		}
		if len(modelIDsToMove) > 0 {
			update := tx.Model(&Model{}).
				Where("id IN ?", modelIDsToMove).
				Updates(map[string]interface{}{
					"vendor_id":    target.Id,
					"updated_time": now,
				})
			if update.Error != nil {
				return update.Error
			}
			result.ReassignedModels = update.RowsAffected
		}

		for _, alias := range aliases {
			if alias.Id == target.Id {
				continue
			}
			var remaining int64
			if err := tx.Model(&Model{}).Where("vendor_id = ?", alias.Id).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining != 0 {
				continue
			}
			if err := tx.Delete(&Vendor{}, alias.Id).Error; err != nil {
				return err
			}
			result.RemovedAliases++
		}

		return nil
	})

	return result, err
}

// NormalizeMiMoVendorMetadata persistently normalizes legacy MiMo metadata.
// It is safe to call repeatedly and never replaces a non-zero, non-legacy
// vendor assignment chosen by an administrator.
func NormalizeMiMoVendorMetadata() error {
	result, err := normalizeMiMoVendorMetadata(DB)
	if err != nil {
		return err
	}
	if result.changed() {
		common.SysLog(fmt.Sprintf(
			"normalized Xiaomi MiMo vendor metadata: vendor_id=%d, created=%t, renamed=%t, icon_updated=%t, description_updated=%t, reassigned_models=%d, removed_aliases=%d",
			result.VendorID,
			result.CreatedVendor,
			result.RenamedVendor,
			result.UpdatedIcon,
			result.UpdatedDescription,
			result.ReassignedModels,
			result.RemovedAliases,
		))
	}
	return nil
}
