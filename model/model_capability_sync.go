package model

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ModelCapabilityMetadata contains display metadata, never routing or billing settings.
type ModelCapabilityMetadata struct {
	ContextLength    int64
	InputModalities  ModelModalities
	OutputModalities ModelModalities
	Provider         string
}

// Matched counts eligible exact matches, including missing enabled models.
// Updated counts changed existing records (including released ownership), not Created.
// Skipped counts matched records disabled for sync, non-exact rules, unchanged or
// invalid metadata, and optimistic-update conflicts. Unrelated source entries do not count.
type ModelCapabilitySyncResult struct {
	Matched int `json:"matched"`
	Updated int `json:"updated"`
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

type modelCapabilityState struct {
	Version          int             `json:"version"`
	Source           string          `json:"source"`
	Provider         string          `json:"provider,omitempty"`
	SyncedAt         int64           `json:"synced_at"`
	ContextLength    *int64          `json:"context_length,omitempty"`
	InputModalities  ModelModalities `json:"input_modalities,omitempty"`
	OutputModalities ModelModalities `json:"output_modalities,omitempty"`
}

// Preserve SQL NULL and legacy serialization exactly for optimistic comparisons.
type modelCapabilitySnapshot struct {
	Model
	ContextSnapshot sql.NullInt64  `gorm:"column:capability_context_snapshot"`
	InputSnapshot   sql.NullString `gorm:"column:capability_input_snapshot"`
	OutputSnapshot  sql.NullString `gorm:"column:capability_output_snapshot"`
	StateSnapshot   sql.NullString `gorm:"column:capability_state_snapshot"`
}

func validCapabilityContext(value int64) bool { return value > 0 && value <= 9007199254740991 }

func validCapabilityModalities(values ModelModalities) ModelModalities {
	if len(values) == 0 {
		return nil
	}
	normalized, err := normalizeModelModalities(values)
	if err != nil {
		return nil
	}
	return normalized
}

func readModelCapabilityState(raw string) modelCapabilityState {
	var state modelCapabilityState
	if common.UnmarshalJsonStr(raw, &state) != nil || state.Version != 1 || strings.TrimSpace(state.Source) == "" {
		return modelCapabilityState{}
	}
	if state.ContextLength != nil && !validCapabilityContext(*state.ContextLength) {
		return modelCapabilityState{}
	}
	for _, values := range []ModelModalities{state.InputModalities, state.OutputModalities} {
		if len(values) > 0 && !slices.Equal(values, validCapabilityModalities(values)) {
			return modelCapabilityState{}
		}
	}
	return state
}

func modelCapabilityChanges(local Model, entry ModelCapabilityMetadata, source string) (map[string]interface{}, error) {
	state := readModelCapabilityState(local.ModelCapabilitySyncState)
	changes := map[string]interface{}{}
	released, applied := false, false
	if state.ContextLength != nil && local.ContextLength != *state.ContextLength {
		state.ContextLength, released = nil, true
	}
	if len(state.InputModalities) > 0 && !slices.Equal(local.InputModalities, state.InputModalities) {
		state.InputModalities, released = nil, true
	}
	if len(state.OutputModalities) > 0 && !slices.Equal(local.OutputModalities, state.OutputModalities) {
		state.OutputModalities, released = nil, true
	}
	if validCapabilityContext(entry.ContextLength) && (local.ContextLength == 0 || state.ContextLength != nil) {
		if local.ContextLength != entry.ContextLength {
			changes["context_length"] = entry.ContextLength
		}
		value := entry.ContextLength
		state.ContextLength, applied = &value, true
	}
	if values := validCapabilityModalities(entry.InputModalities); len(values) > 0 && (len(local.InputModalities) == 0 || len(state.InputModalities) > 0) {
		if !slices.Equal(local.InputModalities, values) {
			changes["input_modalities"] = values
		}
		state.InputModalities, applied = values, true
	}
	if values := validCapabilityModalities(entry.OutputModalities); len(values) > 0 && (len(local.OutputModalities) == 0 || len(state.OutputModalities) > 0) {
		if !slices.Equal(local.OutputModalities, values) {
			changes["output_modalities"] = values
		}
		state.OutputModalities, applied = values, true
	}
	provenanceChanged := applied && (state.Source != source || state.Provider != entry.Provider)
	if len(changes) == 0 && !released && !provenanceChanged {
		return nil, nil
	}
	if applied {
		state.Source, state.Provider = source, entry.Provider
	}
	state.Version, state.SyncedAt = 1, common.GetTimestamp()
	encoded, err := common.Marshal(state)
	if err != nil {
		return nil, err
	}
	changes["model_capability_sync_state"] = string(encoded)
	changes["updated_time"] = state.SyncedAt
	return changes, nil
}

func applyModelCapabilitySnapshot(ctx context.Context, snapshot modelCapabilitySnapshot, entry ModelCapabilityMetadata, source string) (bool, error) {
	changes, err := modelCapabilityChanges(snapshot.Model, entry, source)
	if err != nil || len(changes) == 0 {
		return false, err
	}
	// Compare every capability field plus its ownership snapshot in the same UPDATE.
	// A concurrent administrator save wins; the next run reads a fresh snapshot.
	query := DB.WithContext(ctx).Model(&Model{}).Where(map[string]interface{}{
		"id": snapshot.Id, "model_name": snapshot.ModelName,
		"sync_official": snapshot.SyncOfficial, "name_rule": NameRuleExact,
		"context_length":              snapshot.ContextSnapshot,
		"input_modalities":            snapshot.InputSnapshot,
		"output_modalities":           snapshot.OutputSnapshot,
		"model_capability_sync_state": snapshot.StateSnapshot,
	})
	// MySQL's default text collation is case-insensitive. A case-only rename or
	// a changed ownership snapshot must still invalidate this exact snapshot.
	if DB.Dialector.Name() == "mysql" {
		query = query.Where("BINARY model_name = BINARY ?", snapshot.ModelName)
		for column, value := range map[string]sql.NullString{
			"input_modalities": snapshot.InputSnapshot, "output_modalities": snapshot.OutputSnapshot,
			"model_capability_sync_state": snapshot.StateSnapshot,
		} {
			if value.Valid {
				query = query.Where("BINARY "+column+" = BINARY ?", value.String)
			}
		}
	}
	result := query.Updates(changes)
	return result.RowsAffected > 0, result.Error
}

func capabilityCoveredByRule(name string, rules []Model) bool {
	for _, rule := range rules {
		switch rule.NameRule {
		case NameRulePrefix:
			if strings.HasPrefix(name, rule.ModelName) {
				return true
			}
		case NameRuleSuffix:
			if strings.HasSuffix(name, rule.ModelName) {
				return true
			}
		case NameRuleContains:
			if strings.Contains(name, rule.ModelName) {
				return true
			}
		}
	}
	return false
}

// lockModelMetadataCreationAbility is shared by automatic sync and Model.Insert.
// Use an existing, deterministically ordered ability row as the cross-node lock.
// SQLite has no FOR UPDATE; its write conflict prevents competing transactions
// from both creating a row. The caller must keep the lock through model creation.
func lockModelMetadataCreationAbility(tx *gorm.DB, name string) (bool, error) {
	query := tx.Where("model = ? AND enabled = ?", name, true).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "channel_id"}}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "group"}})
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var abilities []Ability
	if err := query.Limit(1).Find(&abilities).Error; err != nil {
		return false, err
	}
	return len(abilities) > 0, nil
}

// ApplyModelCapabilityMetadata fills empty display fields and refreshes only fields
// still equal to their last automatic value. Manual values, IDs, descriptions,
// prices and abilities are never overwritten. Callers refresh pricing after changes.
func ApplyModelCapabilityMetadata(ctx context.Context, entries map[string]ModelCapabilityMetadata, source string) (ModelCapabilitySyncResult, error) {
	var result ModelCapabilitySyncResult
	source = strings.TrimSpace(source)
	if source == "" {
		return result, errors.New("model capability metadata source is required")
	}
	if DB == nil {
		return result, errors.New("database unavailable")
	}
	if len(entries) == 0 {
		return result, nil
	}
	var rows []modelCapabilitySnapshot
	err := DB.WithContext(ctx).Unscoped().Model(&Model{}).Select("*, context_length AS capability_context_snapshot, input_modalities AS capability_input_snapshot, output_modalities AS capability_output_snapshot, model_capability_sync_state AS capability_state_snapshot").Find(&rows).Error
	if err != nil {
		return result, err
	}
	existing := make(map[string]bool, len(rows))
	rules := make([]Model, 0)
	for _, row := range rows {
		existing[row.ModelName] = true
		if row.NameRule != NameRuleExact {
			rules = append(rules, row.Model)
		}
		entry, found := entries[row.ModelName] // Go equality avoids database collation aliases.
		if !found {
			continue
		}
		if row.DeletedAt.Valid || row.SyncOfficial == 0 || row.NameRule != NameRuleExact {
			result.Skipped++
			continue
		}
		result.Matched++
		changed, err := applyModelCapabilitySnapshot(ctx, row, entry, source)
		if err != nil {
			return result, err
		}
		if changed {
			result.Updated++
		} else {
			result.Skipped++
		}
	}
	var enabled []string
	if err := DB.WithContext(ctx).Model(&Ability{}).Where("enabled = ?", true).Distinct("model").Pluck("model", &enabled).Error; err != nil {
		return result, err
	}
	for _, name := range enabled {
		entry, found := entries[name]
		if !found || existing[name] {
			continue
		}
		// Exact metadata outranks matching rules in pricing. Do not replace an
		// administrator's existing (or deliberately deleted) rule implicitly.
		if capabilityCoveredByRule(name, rules) {
			result.Skipped++
			continue
		}
		result.Matched++
		fresh := Model{ModelName: name, Status: 1, SyncOfficial: 1, NameRule: NameRuleExact}
		changes, err := modelCapabilityChanges(fresh, entry, source)
		if err != nil {
			return result, err
		}
		if len(changes) == 0 {
			result.Skipped++
			continue
		}
		if value, ok := changes["context_length"].(int64); ok {
			fresh.ContextLength = value
		}
		if value, ok := changes["input_modalities"].(ModelModalities); ok {
			fresh.InputModalities = value
		}
		if value, ok := changes["output_modalities"].(ModelModalities); ok {
			fresh.OutputModalities = value
		}
		fresh.ModelCapabilitySyncState = changes["model_capability_sync_state"].(string)
		fresh.CreatedTime, fresh.UpdatedTime = common.GetTimestamp(), common.GetTimestamp()
		created := false
		// Only a single missing model is held in a transaction. Recheck both the
		// model and its enabled ability so a stale catalog cannot enable routing.
		err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			locked, err := lockModelMetadataCreationAbility(tx, name)
			if err != nil {
				return err
			}
			if !locked {
				return nil
			}
			var count int64
			if err := tx.Unscoped().Model(&Model{}).Where("model_name = ?", name).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			var currentRules []Model
			if err := tx.Unscoped().Where("name_rule <> ?", NameRuleExact).Find(&currentRules).Error; err != nil {
				return err
			}
			if capabilityCoveredByRule(name, currentRules) {
				return nil
			}
			if err := tx.Create(&fresh).Error; err != nil {
				return err
			}
			created = true
			return nil
		})
		if err != nil {
			return result, err
		}
		if created {
			result.Created++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}
