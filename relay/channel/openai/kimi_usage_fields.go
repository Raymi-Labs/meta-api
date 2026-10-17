package openai

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
)

type rawUsageObject map[string]json.RawMessage

type kimiUsageField struct {
	details, aliasDetails, count, total, aliasTotal string
}

type kimiUsageCandidate struct {
	value json.RawMessage
	path  string
}

var kimiUsageFields = [...]kimiUsageField{
	{"prompt_tokens_details", "input_tokens_details", "cached_tokens", "prompt_tokens", "input_tokens"},
	{"completion_tokens_details", "output_tokens_details", "reasoning_tokens", "completion_tokens", "output_tokens"},
}

func usesKimiChatUsage(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil || info.RelayMode != relayconstant.RelayModeChatCompletions ||
		(info.ChannelType != constant.ChannelTypeMoonshot && info.ChannelType != constant.ChannelTypeOpenAI) {
		return false
	}
	name := info.UpstreamModelName
	if name == "" {
		name = info.OriginModelName
	}
	return name == "kimi-k3" || name == "k3"
}

func validateKimiUsageTotals(info *relaycommon.RelayInfo, usage *dto.Usage) error {
	if !usesKimiChatUsage(info) || usage == nil {
		return nil
	}
	if usage.PromptTokens < 0 || usage.CompletionTokens < 0 || usage.TotalTokens < 0 ||
		usage.InputTokens < 0 || usage.OutputTokens < 0 {
		return fmt.Errorf("upstream returned negative token totals")
	}
	return nil
}

func usageObject(raw json.RawMessage) rawUsageObject {
	var object rawUsageObject
	if common.Unmarshal(raw, &object) != nil {
		return nil
	}
	return object
}

func nonnegativeUsageCount(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, false
	}
	var count int
	err := common.Unmarshal(raw, &count)
	return count, err == nil && count >= 0
}

func singleChoiceUsage(info *relaycommon.RelayInfo, root rawUsageObject) rawUsageObject {
	if request, ok := info.Request.(*dto.GeneralOpenAIRequest); ok && request.N > 1 {
		return nil // one chunk can contain just one of several requested choices
	}
	var choices []rawUsageObject
	if common.Unmarshal(root["choices"], &choices) == nil && len(choices) == 1 {
		if raw, present := choices[0]["index"]; present {
			index, valid := nonnegativeUsageCount(raw)
			if !valid || index != 0 {
				return nil
			}
		}
		return usageObject(choices[0]["usage"])
	}
	return nil
}

func kimiUsageTotal(usage rawUsageObject, field kimiUsageField) (int, bool) {
	// A reusable snapshot needs an unambiguous total, not whichever alias
	// one parser happens to prefer. Both supplied totals must be valid and
	// agree, including explicit zero; null/invalid/conflicting totals cannot
	// justify carrying a count into another event.
	var total int
	found := false
	for _, name := range []string{field.total, field.aliasTotal} {
		raw, present := usage[name]
		if !present {
			continue
		}
		value, valid := nonnegativeUsageCount(raw)
		if !valid || (found && value != total) {
			return 0, false
		}
		total, found = value, true
	}
	return total, found
}

// A present but invalid/null authoritative field blocks lower-priority aliases.
// Field paths are constants, never derived from response text.
func selectKimiUsageCount(usage, choice rawUsageObject, field kimiUsageField) (json.RawMessage, string, bool) {
	details := usageObject(usage[field.details])
	if raw, exists := usage[field.details]; exists && details == nil &&
		!bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, "usage." + field.details, true
	}
	if value, exists := details[field.count]; exists {
		return value, "usage." + field.details + "." + field.count, true
	}
	candidates := []kimiUsageCandidate{
		{usageObject(usage[field.aliasDetails])[field.count], "usage." + field.aliasDetails + "." + field.count},
		{usage[field.count], "usage." + field.count},
	}
	if field.count == "cached_tokens" {
		candidates = append(candidates, kimiUsageCandidate{usage["prompt_cache_hit_tokens"], "usage.prompt_cache_hit_tokens"})
	}
	candidates = append(candidates,
		kimiUsageCandidate{usageObject(choice[field.details])[field.count], "choices[0].usage." + field.details + "." + field.count},
		kimiUsageCandidate{usageObject(choice[field.aliasDetails])[field.count], "choices[0].usage." + field.aliasDetails + "." + field.count},
		kimiUsageCandidate{choice[field.count], "choices[0].usage." + field.count},
	)
	if field.count == "cached_tokens" {
		candidates = append(candidates, kimiUsageCandidate{usage["cache_read_tokens"], "usage.cache_read_tokens"})
	}
	for _, candidate := range candidates {
		if len(candidate.value) != 0 {
			return candidate.value, candidate.path, true
		}
	}
	// A null details object with no fresh count is explicit unknown evidence.
	// A valid alias in this same event may fill it, but a prior chunk may not.
	for _, name := range []string{field.details, field.aliasDetails} {
		if raw, exists := usage[name]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return raw, "usage." + name, true
		}
	}
	return nil, "", false
}

func kimiCountEvidence(usage, choice rawUsageObject, field kimiUsageField) relaycommon.UsageCountEvidence {
	raw, source, present := selectKimiUsageCount(usage, choice, field)
	evidence := relaycommon.UsageCountEvidence{Status: "missing"}
	if !present {
		return evidence
	}
	evidence.Source = source
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		evidence.Status = "unknown"
		return evidence
	}
	count, valid := nonnegativeUsageCount(raw)
	if !valid {
		evidence.Status = "invalid"
		return evidence
	}
	// A conflicting total disables cross-event restoration, but a fresh
	// count still must not exceed *either* explicitly reported total.
	for _, name := range []string{field.total, field.aliasTotal} {
		if total, known := nonnegativeUsageCount(usage[name]); known && count > total {
			evidence.Status = "invalid"
			return evidence
		}
	}
	evidence.Status = "reported"
	evidence.Value = &count
	return evidence
}

// normalizeKimiChatUsage only mirrors counts actually reported by a K3
// supplier. Explicit canonical zero wins. Unknown is not turned into zero.
// Raw maps preserve provider metadata without changing shared Usage schemas.
func normalizeKimiChatUsage(info *relaycommon.RelayInfo, body []byte) []byte {
	if !usesKimiChatUsage(info) {
		return body
	}
	root := usageObject(body)
	usage := usageObject(root["usage"])
	if usage == nil {
		return body
	}
	choiceUsage := singleChoiceUsage(info, root)
	changed := false
	for _, field := range kimiUsageFields {
		if evidence := kimiCountEvidence(usage, choiceUsage, field); evidence.Status != "reported" {
			continue
		}
		details := usageObject(usage[field.details])
		value, _, _ := selectKimiUsageCount(usage, choiceUsage, field)
		if _, exists := details[field.count]; !exists {
			if details == nil {
				details = rawUsageObject{}
			}
			details[field.count] = value
			raw, err := common.Marshal(details)
			if err != nil {
				return body
			}
			usage[field.details] = raw
			changed = true
		}
		if _, exists := usage[field.count]; !exists {
			usage[field.count] = value // compatibility alias, never added to totals
			changed = true
		}
	}
	if !changed {
		return body
	}
	raw, err := common.Marshal(usage)
	if err != nil {
		return body
	}
	root["usage"] = raw
	raw, err = common.Marshal(root)
	if err != nil {
		return body
	}
	return raw
}

// Reformatting choices must not fabricate absent usage details or lose vendor
// aliases. Only fallback token totals may come from the computed representation.
func preserveKimiChatUsage(info *relaycommon.RelayInfo, original, formatted []byte, computedTotals bool) []byte {
	if !usesKimiChatUsage(info) {
		return formatted
	}
	source, dest := usageObject(original), usageObject(formatted)
	if dest == nil {
		return formatted
	}
	raw, present := source["usage"]
	if computedTotals {
		u := usageObject(raw)
		if u == nil {
			u = rawUsageObject{}
		}
		computed := usageObject(dest["usage"])
		for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
			if v, ok := computed[key]; ok {
				u[key] = v
			}
		}
		var err error
		raw, err = common.Marshal(u)
		if err != nil {
			return formatted
		}
		present = true
	}
	if present {
		dest["usage"] = raw
	} else {
		delete(dest, "usage")
	}
	out, err := common.Marshal(dest)
	if err != nil {
		return formatted
	}
	return out
}
