package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const defaultAlibabaVendorName = "阿里巴巴"

func isAlibabaVendorAlias(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "阿里巴巴", "alibaba", "alibaba (china)":
		return true
	}
	return false
}

func isAlibabaPlanVendor(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "alibaba token plan", "alibaba token plan (china)":
		return true
	}
	return false
}

// A provider/package is not necessarily the author. Match known families with
// boundaries and reject mixed author names, rather than matching substrings.
func isAlibabaModel(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if namespace, rest, found := strings.Cut(name, "/"); found {
		switch namespace {
		case "qwen", "alibaba", "aliyun", "wan-ai":
			name = rest
		default:
			return false
		}
	}
	if strings.ContainsAny(name, "/:") {
		return false
	}
	for vendor, segments := range channelVendorSegments {
		if vendor != "qwen" && hasModelNameSegment(name, segments...) {
			return false
		}
	}
	if hasModelNameSegment(name, "llama", "meta") {
		return false
	}
	for _, prefix := range []string{"qwen", "qwq", "qvq", "wan", "happyhorse", "happyoyster", "z-image"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		if rest == "" || rest[0] == '-' || (rest[0] >= '0' && rest[0] <= '9') {
			return true
		}
	}
	return false
}

// AlibabaMetadataVendor resolves author metadata without discarding the
// original provider stored separately in the capability sync snapshot.
func AlibabaMetadataVendor(name string) string {
	if isAlibabaModel(name) {
		return defaultAlibabaVendorName
	}
	return map[string]string{
		"mimo": defaultMiMoVendorName, "deepseek": "DeepSeek", "openai": "OpenAI",
		"anthropic": "Anthropic", "google": "Google", "moonshot": "Moonshot",
		"zhipu": "智谱", "xai": "xAI", "minimax": "MiniMax", "mistral": "Mistral",
		"inception": defaultInceptionVendorName,
	}[inferModelDisplayVendor(name)]
}

type alibabaVendorNormalizationResult struct {
	VendorID         int
	ReassignedModels int64
	RemovedAliases   int
	UpdatedVendor    bool
}

// Preserve unknown future fields and provenance. A missing snapshot vendor_id
// means manual/legacy ownership and must not become automatically owned.
func reassignAlibabaVendorSnapshot(state string, oldID, newID int) string {
	var fields map[string]json.RawMessage
	var owned int
	if common.UnmarshalJsonStr(state, &fields) != nil || common.Unmarshal(fields["vendor_id"], &owned) != nil || owned != oldID {
		return state
	}
	fields["vendor_id"], _ = common.Marshal(newID)
	updated, err := common.Marshal(fields)
	if err != nil {
		return state
	}
	return string(updated)
}

func normalizeAlibabaVendorMetadata(db *gorm.DB) (alibabaVendorNormalizationResult, error) {
	result := alibabaVendorNormalizationResult{}
	if db == nil {
		return result, fmt.Errorf("database is nil")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var vendors []Vendor
		if err := tx.Order("id ASC").Find(&vendors).Error; err != nil {
			return err
		}
		var target *Vendor
		var aliases []*Vendor
		var ids []int
		for i := range vendors {
			v := &vendors[i]
			if v.Name == defaultAlibabaVendorName {
				target = v
			} else if isAlibabaVendorAlias(v.Name) || isAlibabaPlanVendor(v.Name) {
				aliases = append(aliases, v)
				ids = append(ids, v.Id)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		var rows []Model
		if err := tx.Unscoped().Where("vendor_id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		protected := map[int]bool{}
		hasCandidate := false
		for _, row := range rows {
			if row.DeletedAt.Valid || row.NameRule != NameRuleExact || !isAlibabaModel(row.ModelName) {
				protected[row.VendorID] = true
			} else {
				hasCandidate = true
			}
		}
		if target == nil {
			for _, alias := range aliases {
				if !protected[alias.Id] && isAlibabaVendorAlias(alias.Name) {
					target = alias
					break
				}
			}
		}
		if target == nil && !hasCandidate {
			return nil
		}
		now := common.GetTimestamp()
		if target == nil {
			target = &Vendor{Name: defaultAlibabaVendorName, Icon: getDefaultVendorIcon(defaultAlibabaVendorName), Status: 1, CreatedTime: now, UpdatedTime: now}
			if err := tx.Create(target).Error; err != nil {
				return err
			}
			result.UpdatedVendor = true
		} else if target.Name != defaultAlibabaVendorName {
			updates := map[string]interface{}{"name": defaultAlibabaVendorName, "updated_time": now}
			if strings.TrimSpace(target.Icon) == "" {
				updates["icon"] = getDefaultVendorIcon(defaultAlibabaVendorName)
			}
			if err := tx.Model(target).Updates(updates).Error; err != nil {
				return err
			}
			result.UpdatedVendor = true
		}
		result.VendorID = target.Id
		for _, row := range rows {
			if row.VendorID == target.Id || row.DeletedAt.Valid || row.NameRule != NameRuleExact || !isAlibabaModel(row.ModelName) {
				continue
			}
			updates := map[string]interface{}{"vendor_id": target.Id, "updated_time": now}
			if state := reassignAlibabaVendorSnapshot(row.ModelCapabilitySyncState, row.VendorID, target.Id); state != row.ModelCapabilitySyncState {
				updates["model_capability_sync_state"] = state
			}
			updated := tx.Model(&Model{}).Where("id = ? AND vendor_id = ?", row.Id, row.VendorID).Updates(updates)
			if updated.Error != nil {
				return updated.Error
			}
			result.ReassignedModels += updated.RowsAffected
		}
		for _, alias := range aliases {
			if alias.Id == target.Id {
				continue
			}
			var references int64
			if err := tx.Unscoped().Model(&Model{}).Where("vendor_id = ?", alias.Id).Count(&references).Error; err != nil {
				return err
			}
			if references > 0 {
				continue
			}
			if err := tx.Delete(alias).Error; err != nil {
				return err
			}
			result.RemovedAliases++
		}
		return nil
	})
	return result, err
}

func NormalizeAlibabaVendorMetadata() error {
	result, err := normalizeAlibabaVendorMetadata(DB)
	if err == nil && (result.UpdatedVendor || result.ReassignedModels > 0 || result.RemovedAliases > 0) {
		common.SysLog(fmt.Sprintf("normalized Alibaba vendor metadata: vendor_id=%d, reassigned_models=%d, removed_aliases=%d", result.VendorID, result.ReassignedModels, result.RemovedAliases))
	}
	return err
}
