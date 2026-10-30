package model

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ModelCapabilityMetadata contains display metadata, never routing or billing settings.
type ModelCapabilityMetadata struct {
	ReleaseDate      string
	ContextLength    int64
	MaxOutputTokens  int64
	Capabilities     ModelCapabilities
	FieldSources     map[string]ModelAttributeSource
	InputModalities  ModelModalities
	OutputModalities ModelModalities
	Provider         string
	OfficialPricing  OfficialPricing
}

// Matched counts eligible exact matches, including missing enabled models.
// Updated counts changed existing records (including released ownership), not Created.
// Skipped counts matched records disabled for sync, non-exact rules, unchanged or
// invalid metadata, and optimistic-update conflicts. Unrelated source entries do not count.
type ModelCapabilitySyncResult struct {
	SourceFailures int `json:"source_failures,omitempty"`
	PricingUpdated int `json:"pricing_updated"`
	Matched        int `json:"matched"`
	Updated        int `json:"updated"`
	Created        int `json:"created"`
	Skipped        int `json:"skipped"`
}

type modelCapabilityState struct {
	OfficialPricing  *OfficialPricing                `json:"official_pricing,omitempty"`
	PricingSyncedAt  int64                           `json:"pricing_synced_at,omitempty"`
	Version          int                             `json:"version"`
	Source           string                          `json:"source"`
	Provider         string                          `json:"provider,omitempty"`
	SyncedAt         int64                           `json:"synced_at"`
	ReleaseDate      *string                         `json:"release_date,omitempty"`
	MaxOutputTokens  *int64                          `json:"max_output_tokens,omitempty"`
	Capabilities     ModelCapabilities               `json:"capabilities,omitempty"`
	FieldSources     map[string]ModelAttributeSource `json:"field_sources,omitempty"`
	ContextLength    *int64                          `json:"context_length,omitempty"`
	InputModalities  ModelModalities                 `json:"input_modalities,omitempty"`
	OutputModalities ModelModalities                 `json:"output_modalities,omitempty"`
}

// Preserve SQL NULL and legacy serialization exactly for optimistic comparisons.
type modelCapabilitySnapshot struct {
	Model
	PricingSnapshot      sql.NullString `gorm:"column:capability_pricing_snapshot"`
	ReleaseSnapshot      sql.NullString `gorm:"column:capability_release_snapshot"`
	MaxOutputSnapshot    sql.NullInt64  `gorm:"column:capability_max_output_snapshot"`
	CapabilitiesSnapshot sql.NullString `gorm:"column:capability_flags_snapshot"`
	ContextSnapshot      sql.NullInt64  `gorm:"column:capability_context_snapshot"`
	InputSnapshot        sql.NullString `gorm:"column:capability_input_snapshot"`
	OutputSnapshot       sql.NullString `gorm:"column:capability_output_snapshot"`
	StateSnapshot        sql.NullString `gorm:"column:capability_state_snapshot"`
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
	if state.ReleaseDate != nil && !ValidModelReleaseDate(*state.ReleaseDate) {
		return modelCapabilityState{}
	}
	if state.ContextLength != nil && !validCapabilityContext(*state.ContextLength) {
		return modelCapabilityState{}
	}
	if state.MaxOutputTokens != nil && !validCapabilityContext(*state.MaxOutputTokens) {
		return modelCapabilityState{}
	}
	if state.Capabilities.Validate() != nil {
		return modelCapabilityState{}
	}
	for _, values := range []ModelModalities{state.InputModalities, state.OutputModalities} {
		if len(values) > 0 && !slices.Equal(values, validCapabilityModalities(values)) {
			return modelCapabilityState{}
		}
	}
	if state.OfficialPricing != nil {
		copy := *state.OfficialPricing
		if copy.Normalize(NameRuleExact) != nil || len(copy.Tiers) == 0 || state.FieldSources["official_pricing"].Kind != "official" {
			state.OfficialPricing = nil
			state.PricingSyncedAt = 0
			delete(state.FieldSources, "official_pricing")
		}
	}
	return state
}

func modelCapabilityChanges(local Model, entry ModelCapabilityMetadata, source string) (map[string]interface{}, error) {
	state := readModelCapabilityState(local.ModelCapabilitySyncState)
	changes := map[string]interface{}{}
	released, applied := false, false
	previousSources := maps.Clone(state.FieldSources)
	if state.FieldSources == nil {
		state.FieldSources = map[string]ModelAttributeSource{}
	}
	incomingSource := func(key string) ModelAttributeSource {
		if v, ok := entry.FieldSources[key]; ok {
			return v
		}
		return ModelAttributeSource{Kind: "community", URL: source}
	}
	allowed := func(key string) bool {
		return state.FieldSources[key].Kind != "official" || incomingSource(key).Kind == "official"
	}
	release := func(key string) { delete(state.FieldSources, key); released = true }
	appliedField := func(key string) { state.FieldSources[key] = incomingSource(key); applied = true }
	if state.OfficialPricing != nil && !local.OfficialPricing.Equal(*state.OfficialPricing) {
		state.OfficialPricing = nil
		state.PricingSyncedAt = 0
		release("official_pricing")
	}
	if state.ReleaseDate != nil && local.ReleaseDate != *state.ReleaseDate {
		state.ReleaseDate = nil
		release("release_date")
	}
	if state.ContextLength != nil && local.ContextLength != *state.ContextLength {
		state.ContextLength = nil
		release("context_length")
	}
	if state.MaxOutputTokens != nil && local.MaxOutputTokens != *state.MaxOutputTokens {
		state.MaxOutputTokens = nil
		release("max_output_tokens")
	}
	if len(state.InputModalities) > 0 && !slices.Equal(local.InputModalities, state.InputModalities) {
		state.InputModalities = nil
		release("input_modalities")
	}
	if len(state.OutputModalities) > 0 && !slices.Equal(local.OutputModalities, state.OutputModalities) {
		state.OutputModalities = nil
		release("output_modalities")
	}
	for key, value := range state.Capabilities {
		if current, ok := local.Capabilities[key]; !ok || current != value {
			delete(state.Capabilities, key)
			release("capabilities." + key)
		}
	}
	if ValidModelReleaseDate(entry.ReleaseDate) && (local.ReleaseDate == "" || state.ReleaseDate != nil) && allowed("release_date") {
		if local.ReleaseDate != entry.ReleaseDate {
			changes["release_date"] = entry.ReleaseDate
		}
		value := entry.ReleaseDate
		state.ReleaseDate = &value
		appliedField("release_date")
	}
	for _, field := range []struct {
		key               string
		current, incoming int64
		owned             **int64
	}{
		{"context_length", local.ContextLength, entry.ContextLength, &state.ContextLength},
		{"max_output_tokens", local.MaxOutputTokens, entry.MaxOutputTokens, &state.MaxOutputTokens},
	} {
		if validCapabilityContext(field.incoming) && (field.current == 0 || *field.owned != nil) && allowed(field.key) {
			if field.current != field.incoming {
				changes[field.key] = field.incoming
			}
			value := field.incoming
			*field.owned = &value
			appliedField(field.key)
		}
	}
	for _, field := range []struct {
		key               string
		current, incoming ModelModalities
		owned             *ModelModalities
	}{
		{"input_modalities", local.InputModalities, entry.InputModalities, &state.InputModalities},
		{"output_modalities", local.OutputModalities, entry.OutputModalities, &state.OutputModalities},
	} {
		if value := validCapabilityModalities(field.incoming); len(value) > 0 && (len(field.current) == 0 || len(*field.owned) > 0) && allowed(field.key) {
			if !slices.Equal(field.current, value) {
				changes[field.key] = value
			}
			*field.owned = value
			appliedField(field.key)
		}
	}
	if entry.Capabilities.Validate() == nil {
		values := local.Capabilities.Clone()
		if values == nil {
			values = ModelCapabilities{}
		}
		if state.Capabilities == nil {
			state.Capabilities = ModelCapabilities{}
		}
		for key, value := range entry.Capabilities {
			_, configured := local.Capabilities[key]
			_, owned := state.Capabilities[key]
			if (!configured || owned) && allowed("capabilities."+key) {
				values[key] = value
				state.Capabilities[key] = value
				appliedField("capabilities." + key)
			}
		}
		if !maps.Equal(values, local.Capabilities) {
			changes["capabilities"] = values
		}
	}
	// Only verified official sources may populate prices. Manual edits take
	// ownership of the entire price sheet, including its notes and currency.
	price := entry.OfficialPricing
	priceSource := incomingSource("official_pricing")
	if len(price.Tiers) > 0 && priceSource.Kind == "official" && price.SourceURL == priceSource.URL && price.SourceURL != "" && price.Normalize(NameRuleExact) == nil && (len(local.OfficialPricing.Tiers) == 0 || state.OfficialPricing != nil) {
		if !local.OfficialPricing.Equal(price) {
			changes["official_pricing"] = price
			state.PricingSyncedAt = common.GetTimestamp()
		}
		state.OfficialPricing = &price
		appliedField("official_pricing")
	}
	provenanceChanged := applied && (state.Source != source || state.Provider != entry.Provider || !maps.Equal(previousSources, state.FieldSources))
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

func applyModelCapabilitySnapshot(ctx context.Context, snapshot modelCapabilitySnapshot, entry ModelCapabilityMetadata, source string) (bool, bool, error) {
	changes, err := modelCapabilityChanges(snapshot.Model, entry, source)
	if err != nil || len(changes) == 0 {
		return false, false, err
	}
	// Compare every capability field plus its ownership snapshot in the same UPDATE.
	// A concurrent administrator save wins; the next run reads a fresh snapshot.
	query := DB.WithContext(ctx).Model(&Model{}).Where(map[string]interface{}{
		"id": snapshot.Id, "model_name": snapshot.ModelName,
		"sync_official": snapshot.SyncOfficial, "name_rule": NameRuleExact,
		"release_date":                snapshot.ReleaseSnapshot,
		"context_length":              snapshot.ContextSnapshot,
		"max_output_tokens":           snapshot.MaxOutputSnapshot,
		"capabilities":                snapshot.CapabilitiesSnapshot,
		"official_pricing":            snapshot.PricingSnapshot,
		"input_modalities":            snapshot.InputSnapshot,
		"output_modalities":           snapshot.OutputSnapshot,
		"model_capability_sync_state": snapshot.StateSnapshot,
	})
	// MySQL's default text collation is case-insensitive. A case-only rename or
	// a changed ownership snapshot must still invalidate this exact snapshot.
	if DB.Dialector.Name() == "mysql" {
		query = query.Where("BINARY model_name = BINARY ?", snapshot.ModelName)
		for column, value := range map[string]sql.NullString{
			"release_date":     snapshot.ReleaseSnapshot,
			"official_pricing": snapshot.PricingSnapshot,
			"capabilities":     snapshot.CapabilitiesSnapshot,
			"input_modalities": snapshot.InputSnapshot, "output_modalities": snapshot.OutputSnapshot,
			"model_capability_sync_state": snapshot.StateSnapshot,
		} {
			if value.Valid {
				query = query.Where("BINARY "+column+" = BINARY ?", value.String)
			}
		}
	}
	result := query.Updates(changes)
	_, priceChanged := changes["official_pricing"]
	return result.RowsAffected > 0, result.RowsAffected > 0 && priceChanged, result.Error
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
// billing settings and abilities are never overwritten. Callers refresh pricing after changes.
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
	err := DB.WithContext(ctx).Unscoped().Model(&Model{}).Select("*, official_pricing AS capability_pricing_snapshot, release_date AS capability_release_snapshot, context_length AS capability_context_snapshot, max_output_tokens AS capability_max_output_snapshot, capabilities AS capability_flags_snapshot, input_modalities AS capability_input_snapshot, output_modalities AS capability_output_snapshot, model_capability_sync_state AS capability_state_snapshot").Find(&rows).Error
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
		changed, pricingChanged, err := applyModelCapabilitySnapshot(ctx, row, entry, source)
		if err != nil {
			return result, err
		}
		if changed {
			if pricingChanged {
				result.PricingUpdated++
			}
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
		if value, ok := changes["official_pricing"].(OfficialPricing); ok {
			fresh.OfficialPricing = value
		}
		if value, ok := changes["release_date"].(string); ok {
			fresh.ReleaseDate = value
		}
		if value, ok := changes["context_length"].(int64); ok {
			fresh.ContextLength = value
		}
		if value, ok := changes["max_output_tokens"].(int64); ok {
			fresh.MaxOutputTokens = value
		}
		if value, ok := changes["capabilities"].(ModelCapabilities); ok {
			fresh.Capabilities = value
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
			if len(fresh.OfficialPricing.Tiers) > 0 {
				result.PricingUpdated++
			}
			result.Created++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}
