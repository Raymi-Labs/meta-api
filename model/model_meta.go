package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	NameRuleExact = iota
	NameRulePrefix
	NameRuleContains
	NameRuleSuffix
)

type BoundChannel struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

type Model struct {
	FAQ              ModelFAQ                        `json:"faq" gorm:"type:text"`
	OfficialPricing  OfficialPricing                 `json:"official_pricing" gorm:"type:text"`
	Id               int                             `json:"id"`
	ModelName        string                          `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_name_delete_at,priority:1"`
	DisplayName      string                          `json:"display_name" gorm:"type:varchar(128)"`
	ReleaseDate      string                          `json:"release_date" gorm:"type:varchar(10);default:''"`
	MaxOutputTokens  int64                           `json:"max_output_tokens" gorm:"type:bigint;default:0"`
	Capabilities     ModelCapabilities               `json:"capabilities" gorm:"type:text"`
	AttributeSources map[string]ModelAttributeSource `json:"attribute_sources,omitempty" gorm:"-"`
	ContextLength    int64                           `json:"context_length" gorm:"type:bigint;default:0"`
	InputModalities  ModelModalities                 `json:"input_modalities" gorm:"type:text"`
	OutputModalities ModelModalities                 `json:"output_modalities" gorm:"type:text"`
	Description      string                          `json:"description,omitempty" gorm:"type:text"`
	Icon             string                          `json:"icon,omitempty" gorm:"type:varchar(128)"`
	Tags             string                          `json:"tags,omitempty" gorm:"type:varchar(255)"`
	DisplayVendorID  int                             `json:"display_vendor_id" gorm:"-"`
	VendorID         int                             `json:"vendor_id,omitempty" gorm:"index"`
	Endpoints        string                          `json:"endpoints,omitempty" gorm:"type:text"`
	Status           int                             `json:"status" gorm:"default:1"`
	SyncOfficial     int                             `json:"sync_official" gorm:"default:1"`
	CreatedTime      int64                           `json:"created_time" gorm:"bigint"`
	UpdatedTime      int64                           `json:"updated_time" gorm:"bigint"`
	DeletedAt        gorm.DeletedAt                  `json:"-" gorm:"index;uniqueIndex:uk_model_name_delete_at,priority:2"`

	ModelCapabilitySyncState string `json:"-" gorm:"type:text"`

	BoundChannels []BoundChannel `json:"bound_channels,omitempty" gorm:"-"`
	EnableGroups  []string       `json:"enable_groups,omitempty" gorm:"-"`
	QuotaTypes    []int          `json:"quota_types,omitempty" gorm:"-"`
	NameRule      int            `json:"name_rule" gorm:"default:0"`

	MatchedModels []string `json:"matched_models,omitempty" gorm:"-"`
	MatchedCount  int      `json:"matched_count,omitempty" gorm:"-"`
}

func (mi *Model) Insert() error {
	now := common.GetTimestamp()
	mi.CreatedTime = now
	mi.UpdatedTime = now

	// 保存原始值（因为 Create 后可能被 GORM 的 default 标签覆盖为 1）
	originalStatus := mi.Status
	originalSyncOfficial := mi.SyncOfficial

	return DB.Transaction(func(tx *gorm.DB) error {
		// Share the automatic capability sync's creation lock for enabled models.
		// This also protects explicit admin creation and the legacy catalog sync.
		locked, err := lockModelMetadataCreationAbility(tx, mi.ModelName)
		if err != nil {
			return err
		}
		if locked {
			var count int64
			if err := tx.Model(&Model{}).Where("model_name = ?", mi.ModelName).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("模型名称已存在")
			}
		}
		// 先创建记录（GORM 会对零值字段应用默认值）
		if err := tx.Create(mi).Error; err != nil {
			return err
		}
		// 使用保存的原始值进行更新，确保零值能正确保存
		return tx.Model(&Model{}).Where("id = ?", mi.Id).Updates(map[string]interface{}{
			"status": originalStatus, "sync_official": originalSyncOfficial,
		}).Error
	})
}

func IsModelNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&Model{}).Where("model_name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

func (mi *Model) Update() error { return mi.UpdateOmittingMetadata(nil) }

func (mi *Model) UpdateOmittingMetadata(omitted []string) error {
	return mi.UpdateWithMetadataOverrides(omitted, nil)
}

func (mi *Model) UpdateWithMetadataOverrides(omitted []string, overrides map[string]*bool) error {
	if len(overrides) > 0 {
		keys := ModelCapabilities{}
		for key := range overrides {
			keys[key] = false
		}
		if err := keys.Validate(); err != nil {
			return err
		}
		return DB.Transaction(func(tx *gorm.DB) error {
			var current Model
			query := tx.Select("id", "capabilities")
			if tx.Dialector.Name() != "sqlite" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := query.First(&current, mi.Id).Error; err != nil {
				return err
			}
			mi.Capabilities = current.Capabilities.Clone()
			if mi.Capabilities == nil {
				mi.Capabilities = ModelCapabilities{}
			}
			for key, value := range overrides {
				if value == nil {
					delete(mi.Capabilities, key)
				} else {
					mi.Capabilities[key] = *value
				}
			}
			return mi.updateMetadata(tx, omitted)
		})
	}
	return mi.updateMetadata(DB, omitted)
}

func (mi *Model) updateMetadata(db *gorm.DB, omitted []string) error {
	mi.UpdatedTime = common.GetTimestamp()
	// 使用 Select 强制更新所有字段，包括零值
	return db.Model(&Model{}).Where("id = ?", mi.Id).
		Select("faq", "official_pricing", "model_name", "display_name", "release_date", "context_length", "max_output_tokens", "capabilities", "input_modalities", "output_modalities", "description", "icon", "tags", "vendor_id", "endpoints", "status", "sync_official", "name_rule", "updated_time").
		Omit(omitted...).Updates(mi).Error
}

func (mi *Model) Delete() error {
	return DB.Delete(mi).Error
}

func GetBoundChannelsByModelsMap(modelNames []string) (map[string][]BoundChannel, error) {
	result := make(map[string][]BoundChannel)
	if len(modelNames) == 0 {
		return result, nil
	}
	type row struct {
		Model string
		Name  string
		Type  int
	}
	var rows []row
	err := DB.Table("channels").
		Select("abilities.model as model, channels.name as name, channels.type as type").
		Joins("JOIN abilities ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ?", modelNames, true).
		Distinct().
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.Model] = append(result[r.Model], BoundChannel{Name: r.Name, Type: r.Type})
	}
	return result, nil
}
