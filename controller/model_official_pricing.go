package controller

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

var openAILongContextPrice = regexp.MustCompile(`^(?:API )?[Pp]rompts with more than ([0-9]+(?:\.[0-9]+)?[KM]?) input tokens are priced at ([0-9]+(?:\.[0-9]+)?)x input and cache rates and ([0-9]+(?:\.[0-9]+)?)x output for the full request\.$`)
var openAICacheWriteRetention = regexp.MustCompile(`^Cache-write prices shown use ([1-9][0-9]*)-(minute|hour) retention\. Standard cache writes cost (\$[0-9]+(?:\.[0-9]+)?) per million tokens for ([1-9][0-9]*)-(minute|hour) retention and (\$[0-9]+(?:\.[0-9]+)?) for ([1-9][0-9]*)-(minute|hour) retention, before long-context adjustments\.$`)

// Keep retention periods explicit in the existing tier labels. The card's
// cache-write amount must agree with the declared default; an inconsistent or
// unfamiliar note cannot silently turn into an unconditional standard rate.
func openAICacheWriteTiers(tier model.OfficialPricingTier, rule []string) ([]model.OfficialPricingTier, error) {
	if rule == nil {
		return []model.OfficialPricingTier{tier}, nil
	}
	if tier.CacheWrite == nil {
		return nil, errors.New("missing official default cache-write price")
	}
	label := func(number, unit string) string { return number + map[string]string{"minute": "m", "hour": "h"}[unit] }
	defaultRetention, err := time.ParseDuration(label(rule[1], rule[2]))
	if err != nil {
		return nil, errors.New("invalid official default cache retention")
	}
	seen := map[time.Duration]bool{}
	matchedDefault := false
	var tiers []model.OfficialPricingTier
	for _, offset := range []int{3, 6} {
		retentionLabel := label(rule[offset+1], rule[offset+2])
		retention, err := time.ParseDuration(retentionLabel)
		if err != nil || retention <= 0 || seen[retention] {
			return nil, errors.New("invalid or duplicate official cache retention")
		}
		seen[retention] = true
		price, err := strconv.ParseFloat(strings.TrimPrefix(rule[offset], "$"), 64)
		if err != nil {
			return nil, errors.New("invalid official cache-write price")
		}
		if retention == defaultRetention {
			if price != *tier.CacheWrite {
				return nil, errors.New("official cache-write card and retention prices disagree")
			}
			matchedDefault = true
		}
		variant := tier
		variant.Label += " · cache " + retentionLabel
		variant.CacheWrite = &price
		if retention == defaultRetention {
			tiers = append([]model.OfficialPricingTier{variant}, tiers...)
		} else {
			tiers = append(tiers, variant)
		}
	}
	if !matchedDefault {
		return nil, errors.New("official default cache retention has no price")
	}
	return tiers, nil
}

// Consume only the exact model's standard text-token card. Comparison cards,
// tool fees and other modalities must never become that model's token prices.
// A partial/ambiguous price sheet is an error, so the caller retains old prices.
func parseOpenAIPriceLines(lines []string, address string) (model.OfficialPricing, error) {
	empty := model.OfficialPricing{}
	start := slices.Index(lines, "Pricing")
	if start < 0 {
		return empty, nil
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if slices.Contains([]string{"Modalities", "Endpoints", "Features", "Snapshots", "Rate limits"}, lines[i]) {
			end = i
			break
		}
	}
	section := lines[start+1 : end]
	card := slices.Index(section, "Text tokens")
	if card < 0 {
		return empty, nil
	} // Not a text-token model; leave manual configuration alone.
	section = section[card+1:]
	if len(section) == 0 || section[0] != "Per 1M tokens" {
		return empty, errors.New("unsupported official text price unit")
	}
	tier := model.OfficialPricingTier{Label: "Standard", Unit: "million_tokens"}
	fields := map[string]**float64{"Input": &tier.Input, "Output": &tier.Output, "Cached input": &tier.CacheRead, "Cache writes": &tier.CacheWrite}
	seen := map[string]bool{}
	var notes []string
	var longContext []string
	var cacheRetention []string
	for i := 1; i < len(section); i++ {
		line := section[i]
		if line == "Quick comparison" || line == "Image tokens" || line == "Audio tokens" || line == "Video tokens" {
			break
		}
		if target, ok := fields[line]; ok {
			if seen[line] || i+1 >= len(section) {
				return empty, errors.New("ambiguous official price field")
			}
			seen[line] = true
			i++
			if section[i] == "-" || section[i] == "—" || section[i] == "N/A" {
				continue
			}
			if !officialCachePrice.MatchString(section[i]) {
				return empty, errors.New("invalid official token price")
			}
			value, err := strconv.ParseFloat(strings.TrimPrefix(section[i], "$"), 64)
			if err != nil {
				return empty, err
			}
			*target = &value
			continue
		}
		// This is a navigation link on standard-price cards, not a selected Batch rate.
		if line == "∙" || line == "Batch API price" {
			continue
		}
		if match := openAICacheWriteRetention.FindStringSubmatch(line); match != nil {
			if cacheRetention != nil {
				return empty, errors.New("duplicate official cache retention rule")
			}
			cacheRetention = match
			notes = append(notes, line)
			continue
		}
		if strings.Contains(line, "$") || line == "Text tokens" || line == "Per 1M tokens" {
			return empty, errors.New("unrecognized official price layout")
		}
		if match := openAILongContextPrice.FindStringSubmatch(line); match != nil {
			if longContext != nil {
				return empty, errors.New("duplicate long-context price rule")
			}
			longContext = match
		} else if strings.Contains(strings.ToLower(line), "more than") && strings.Contains(strings.ToLower(line), "tokens") && strings.Contains(strings.ToLower(line), "pric") {
			return empty, errors.New("unsupported long-context price rule")
		}
		notes = append(notes, line)
	}
	if tier.Input == nil || tier.Output == nil {
		return empty, errors.New("incomplete official token prices")
	}
	tiers, err := openAICacheWriteTiers(tier, cacheRetention)
	if err != nil {
		return empty, err
	}
	pricing := model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: strings.Join(notes, " "), Tiers: tiers}
	if longContext != nil {
		inputMultiplier, _ := strconv.ParseFloat(longContext[2], 64)
		outputMultiplier, _ := strconv.ParseFloat(longContext[3], 64)
		if inputMultiplier <= 0 || outputMultiplier <= 0 {
			return empty, errors.New("invalid long-context multiplier")
		}
		scale := func(value *float64, multiplier float64) *float64 {
			if value == nil {
				return nil
			}
			v := *value * multiplier
			return &v
		}
		pricing.Tiers = nil
		for _, base := range tiers {
			retentionLabel := strings.TrimPrefix(base.Label, "Standard")
			base.Label = "Standard · ≤ " + longContext[1] + " input tokens" + retentionLabel
			pricing.Tiers = append(pricing.Tiers, base, model.OfficialPricingTier{Label: "Standard · > " + longContext[1] + " input tokens" + retentionLabel, Unit: "million_tokens", Input: scale(base.Input, inputMultiplier), Output: scale(base.Output, outputMultiplier), CacheRead: scale(base.CacheRead, inputMultiplier), CacheWrite: scale(base.CacheWrite, inputMultiplier)})
		}
	}
	if err := pricing.Normalize(model.NameRuleExact); err != nil {
		return empty, err
	}
	return pricing, nil
}
