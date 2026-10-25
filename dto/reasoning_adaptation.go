package dto

import (
	"fmt"
	"strings"
)

// Omitted settings keep the historic global mapping. Auto is opt-in for existing
// channels; the new-channel editor selects it by default.
type ReasoningAdaptation struct {
	Mode  string                    `json:"mode,omitempty"`
	Rules []ReasoningAdaptationRule `json:"rules,omitempty"`
}

type ReasoningAdaptationRule struct {
	Model    string            `json:"model"`
	Protocol string            `json:"protocol"`
	Mapping  map[string]string `json:"mapping"`
}

const ReasoningTemplateZhipuGLM53 = "zhipu-glm53-v1"

func IsReasoningEffort(value string) bool {
	switch value {
	case "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func (s ReasoningAdaptation) Validate() error {
	switch s.Mode {
	case "", "inherit", "auto", ReasoningTemplateZhipuGLM53:
	default:
		return fmt.Errorf("reasoning_adaptation: invalid mode")
	}
	if len(s.Rules) > 32 {
		return fmt.Errorf("reasoning_adaptation: at most 32 rules")
	}
	seen := make(map[string]bool)
	for _, rule := range s.Rules {
		if rule.Model == "" || len(rule.Model) > 255 || rule.Model != strings.TrimSpace(rule.Model) || strings.ContainsAny(rule.Model, "*?\r\n\t") {
			return fmt.Errorf("reasoning_adaptation: model must be an exact model name")
		}
		if rule.Protocol != "chat" {
			return fmt.Errorf("reasoning_adaptation: only chat protocol is supported")
		}
		key := rule.Protocol + ":" + rule.Model
		if seen[key] {
			return fmt.Errorf("reasoning_adaptation: duplicate model and protocol")
		}
		seen[key] = true
		if len(rule.Mapping) == 0 || len(rule.Mapping) > 6 {
			return fmt.Errorf("reasoning_adaptation: mapping must contain 1 to 6 effort levels")
		}
		for from, to := range rule.Mapping {
			if !IsReasoningEffort(from) || !IsReasoningEffort(to) {
				return fmt.Errorf("reasoning_adaptation: invalid effort mapping")
			}
		}
	}
	return nil
}
