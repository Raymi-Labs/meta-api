package controller

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

var claudePriceCell = regexp.MustCompile(`^\$([0-9]+(?:\.[0-9]+)?) / MTok(?: for prompts (up to|over) ([0-9][0-9,]*) tokens)?$`)

// A price cell is either unconditional or a complete complementary pair. No
// "From" value, missing range, or unrecognized condition becomes a flat price.
func claudePriceRanges(cell string) (map[string]*float64, error) {
	parts := strings.Split(cell, ";")
	values := map[string]*float64{}
	threshold := ""
	for _, part := range parts {
		m := claudePriceCell.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil, fmt.Errorf("unsupported Claude price cell: %s", cell)
		}
		key := "Standard"
		if m[2] != "" {
			n := documentTokens(m[3])
			if n == 0 {
				return nil, fmt.Errorf("invalid Claude context boundary")
			}
			limit := strconv.FormatInt(n, 10)
			if threshold != "" && threshold != limit {
				return nil, fmt.Errorf("inconsistent Claude context boundaries")
			}
			threshold = limit
			op := "≤"
			if m[2] == "over" {
				op = ">"
			}
			key += " · " + op + " " + limit + " input tokens"
		}
		if values[key] != nil {
			return nil, fmt.Errorf("duplicate Claude price condition")
		}
		values[key] = documentUSD("$" + m[1])
	}
	if threshold != "" && (len(values) != 2 || values["Standard · ≤ "+threshold+" input tokens"] == nil || values["Standard · > "+threshold+" input tokens"] == nil) {
		return nil, fmt.Errorf("incomplete Claude context price ranges")
	}
	if threshold == "" && len(values) != 1 {
		return nil, fmt.Errorf("invalid Claude price ranges")
	}
	return values, nil
}

func parseClaudePricing(tables [][][]string, address string) (model.OfficialPricing, error) {
	prices := map[string]map[string]*float64{}
	keys := []string{"Input", "Output", "Cache read", "5m cache write", "1h cache write"}
	for _, table := range tables {
		if len(table) < 2 || len(table[0]) != 2 || table[0][0] != "Feature" {
			continue
		}
		for _, row := range table[1:] {
			if len(row) != 2 || !slices.Contains(keys, row[0]) {
				continue
			}
			if prices[row[0]] != nil {
				return model.OfficialPricing{}, fmt.Errorf("duplicate Claude price field")
			}
			v, err := claudePriceRanges(row[1])
			if err != nil {
				return model.OfficialPricing{}, err
			}
			prices[row[0]] = v
		}
	}
	p := model.OfficialPricing{Currency: "USD", SourceURL: address}
	if len(prices["Input"]) == 0 || len(prices["Output"]) == 0 {
		return model.OfficialPricing{}, fmt.Errorf("Claude input/output pricing missing")
	}
	// All explicitly published price fields must describe the same conditions.
	var conditions []string
	for condition := range prices["Input"] {
		conditions = append(conditions, condition)
	}
	slices.Sort(conditions)
	// Put the ordinary short-context tier first.
	if len(conditions) == 2 && strings.Contains(conditions[0], ">") {
		conditions[0], conditions[1] = conditions[1], conditions[0]
	}
	for _, field := range prices {
		if len(field) != len(conditions) {
			return model.OfficialPricing{}, fmt.Errorf("Claude price conditions do not align")
		}
		for _, condition := range conditions {
			if field[condition] == nil {
				return model.OfficialPricing{}, fmt.Errorf("Claude price conditions do not align")
			}
		}
	}
	for _, condition := range conditions {
		tier := model.OfficialPricingTier{Label: condition, Unit: "million_tokens", Input: prices["Input"][condition], Output: prices["Output"][condition], CacheRead: prices["Cache read"][condition]}
		hasTTL := false
		for _, ttl := range []string{"5m", "1h"} {
			if write := prices[ttl+" cache write"][condition]; write != nil {
				t := tier
				t.Label += " · cache " + ttl
				t.CacheWrite = write
				p.Tiers = append(p.Tiers, t)
				hasTTL = true
			}
		}
		if !hasTTL {
			p.Tiers = append(p.Tiers, tier)
		}
	}
	return p, p.Normalize(model.NameRuleExact)
}
