package model

import (
	"errors"
	"slices"
)

// A declaration describes an exact model, not a route, permission, or selected effort.
type ReasoningProfile struct {
	SupportedLevels []string `json:"supportedLevels"`
	DefaultLevel    string   `json:"defaultLevel,omitempty"`
}

func knownReasoningLevel(level string) bool {
	switch level {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	}
	return false
}

func (p *ReasoningProfile) Validate() error {
	if p == nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, level := range p.SupportedLevels {
		if !knownReasoningLevel(level) || seen[level] {
			return errors.New("推理档位未知或重复")
		}
		seen[level] = true
	}
	if !seen[p.DefaultLevel] {
		return errors.New("默认推理档位必须属于已选档位")
	}
	return nil
}

func VisibleReasoningProfiles(names []string) (map[string]*ReasoningProfile, error) {
	profiles, err := officialForVisible(names)
	if err != nil {
		return nil, err
	}
	for batch := range slices.Chunk(names, 400) {
		var rows []Model
		if err := DB.Select("model_name", "reasoning_profile", "capabilities", "model_capability_sync_state").Where("model_name IN ? AND name_rule = ? AND status = ?", batch, NameRuleExact, 1).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			// Some database collations are case-insensitive. Never publish a near match.
			if !slices.Contains(batch, row.ModelName) {
				continue
			}
			if support, known := row.Capabilities["reasoning"]; known && !support {
				owned, auto := readModelCapabilityState(row.ModelCapabilitySyncState).Capabilities["reasoning"]
				if !auto || owned != support {
					profiles[row.ModelName] = &ReasoningProfile{SupportedLevels: []string{}}
					continue
				}
			}
			if row.ReasoningProfile != nil && row.ReasoningProfile.Validate() == nil {
				profiles[row.ModelName] = row.ReasoningProfile
			}
		}
	}
	return profiles, nil
}
