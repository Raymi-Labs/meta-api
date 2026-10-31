package controller

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

var plainUSDPrice = regexp.MustCompile(`^\\?\$([0-9]+(?:\.[0-9]+)?)$`)

func documentUSD(s string) *float64 {
	m := plainUSDPrice.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return nil
	}
	v, e := strconv.ParseFloat(m[1], 64)
	if e != nil {
		return nil
	}
	return &v
}
func officialPriceEntry(provider, address string, p model.OfficialPricing) model.ModelCapabilityMetadata {
	e := model.ModelCapabilityMetadata{Provider: provider}
	if p.Normalize(model.NameRuleExact) == nil {
		e.OfficialPricing = p
	}
	return markOfficialAttributes(e, address)
}
func parseMiMoPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	prices := map[string]model.OfficialPricing{}
	for _, table := range documentTables(data) {
		h := table[0]
		if len(h) == 5 && h[0] == "Inference Type" && h[1] == "Model Name" && h[2] == "Input (Cache Hit)" && h[3] == "Input (Cache Miss)" && h[4] == "Output" {
			for _, r := range table[1:] {
				if len(r) != 5 || !names[r[1]] {
					continue
				}
				input, output, cache := documentUSD(r[3]), documentUSD(r[4]), documentUSD(r[2])
				if input == nil || output == nil || cache == nil {
					continue
				}
				p := prices[r[1]]
				p.Tiers = append(p.Tiers, model.OfficialPricingTier{Label: "Overseas · " + r[0], Unit: "million_tokens", Input: input, Output: output, CacheRead: cache})
				prices[r[1]] = p
			}
		}
		if len(h) == 2 && h[0] == "Model Name" && h[1] == "Input audio duration" {
			for _, r := range table[1:] {
				if len(r) != 2 || !names[r[0]] || !strings.HasSuffix(r[1], " /h") {
					continue
				}
				if v := documentUSD(strings.TrimSuffix(r[1], " /h")); v != nil {
					prices[r[0]] = model.OfficialPricing{Tiers: []model.OfficialPricingTier{{Label: "Overseas · audio input", Unit: "hour", Price: v}}}
				}
			}
		}
	}
	out := map[string]model.ModelCapabilityMetadata{}
	for id, p := range prices {
		p.Currency, p.SourceURL, p.Notes = "USD", address, "Overseas API rates; domestic CNY rates and tool fees are separate. Cache writes are temporarily free."
		out[id] = officialPriceEntry("xiaomi", address, p)
	}
	return out
}
func parseZAIPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	if !strings.Contains(string(data), "Prices per 1M tokens.") || !strings.Contains(string(data), "All prices are in USD.") {
		return out
	}
	for _, table := range documentTables(data) {
		h := table[0]
		if len(h) != 5 || h[0] != "Model" || h[1] != "Input" || h[2] != "Cached Input" || h[4] != "Output" {
			continue
		}
		for _, r := range table[1:] {
			if len(r) != 5 {
				continue
			}
			for id := range names { // The price table uses uppercase display names; API IDs are lowercase.
				if !strings.EqualFold(r[0], id) {
					continue
				}
				input, output, cache := documentUSD(r[1]), documentUSD(r[4]), documentUSD(r[2])
				if input == nil || output == nil || cache == nil {
					continue
				}
				p := model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: "Cached input storage: " + r[3], Tiers: []model.OfficialPricingTier{{Label: "Standard", Unit: "million_tokens", Input: input, Output: output, CacheRead: cache}}}
				out[id] = officialPriceEntry("zai", address, p)
			}
		}
	}
	return out
}
func addDeepSeekPrices(data []byte, entries map[string]model.ModelCapabilityMetadata, address string) {
	// Keep both time bands and the official schedule, including the holiday rule.
	node, _ := html.Parse(strings.NewReader(string(data)))
	notes := ""
	if node != nil {
		for _, line := range documentLines(node) {
			if strings.Contains(line, "Peak hours are") && strings.Contains(line, "UTC") {
				notes = line
				break
			}
		}
	}
	for _, table := range documentTables(data) {
		if len(table) < 2 || table[0][0] != "MODEL" {
			continue
		}
		header := table[0]
		for col := 1; col < len(header); col++ {
			for id, entry := range entries {
				if !exactDocumentID(header[col], id) {
					continue
				}
				tiers := map[string]*model.OfficialPricingTier{}
				for _, r := range table[1:] {
					if len(r) != len(header) {
						continue
					}
					band, key := "", ""
					for _, v := range r[:col] {
						switch v {
						case "OFF-PEAK", "PEAK":
							band = v
						case "1M INPUT TOKENS (CACHE HIT)":
							key = "cache"
						case "1M INPUT TOKENS (CACHE MISS)":
							key = "input"
						case "1M OUTPUT TOKENS":
							key = "output"
						}
					}
					if band == "" || key == "" {
						continue
					}
					v := documentUSD(r[col])
					if v == nil {
						continue
					}
					t := tiers[band]
					if t == nil {
						t = &model.OfficialPricingTier{Label: band, Unit: "million_tokens"}
						tiers[band] = t
					}
					switch key {
					case "input":
						t.Input = v
					case "output":
						t.Output = v
					case "cache":
						t.CacheRead = v
					}
				}
				p := model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: notes}
				for _, band := range []string{"OFF-PEAK", "PEAK"} {
					if t := tiers[band]; t != nil && t.Input != nil && t.Output != nil && t.CacheRead != nil {
						p.Tiers = append(p.Tiers, *t)
					}
				}
				if len(p.Tiers) == 2 && notes != "" {
					entry.OfficialPricing = p
					entries[id] = markOfficialAttributes(entry, address)
				}
			}
		}
	}
}

var pricePart = regexp.MustCompile(`\$([0-9]+(?:\.[0-9]+)?)([^$]*)`)
var datedPriceCondition = regexp.MustCompile(`^(through|starting) ([A-Za-z]+ [0-9]{1,2}, [0-9]{4})\.?$`)

// Parse explicit USD token rates only. Dates select the currently applicable
// rate; other conditions stay attached to their tier, never discarded.
func geminiPriceParts(text string, now time.Time) (map[string]*float64, bool) {
	out := map[string]*float64{}
	if !strings.HasPrefix(strings.TrimSpace(text), "$") {
		return nil, false
	}
	matches := pricePart.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, false
	}
	for _, m := range matches {
		condition := strings.Trim(strings.TrimSpace(m[2]), ".")
		if strings.Contains(condition, "storage price") {
			continue
		}
		if d := datedPriceCondition.FindStringSubmatch(condition); d != nil {
			date, e := time.Parse("January 2, 2006", d[2])
			if e != nil {
				return nil, false
			}
			today := now.UTC().Truncate(24 * time.Hour)
			if d[1] == "through" && today.After(date) || d[1] == "starting" && today.Before(date) {
				continue
			}
			condition = ""
		}
		// Anything with a different unit cannot be represented as a token rate.
		lower := strings.ToLower(condition)
		for _, unit := range []string{"per image", "per second", "per minute", "per hour", "/ image", "/ 1,000", "/1,000"} {
			if strings.Contains(lower, unit) {
				return nil, false
			}
		}
		if len(condition) > 95 || strings.Contains(condition, "$") {
			return nil, false
		}
		if _, ok := out[condition]; ok {
			return nil, false
		}
		v, e := strconv.ParseFloat(m[1], 64)
		if e != nil {
			return nil, false
		}
		out[condition] = &v
	}
	return out, len(out) > 0
}
func parseGeminiPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	doc, _ := html.Parse(strings.NewReader(string(data)))
	if doc == nil {
		return out
	}
	var groups []*html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "models-section" {
					groups = append(groups, n)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	for _, group := range groups {
		var ids []string
		for _, code := range documentNodes(group, "code") {
			id := strings.Join(documentLines(code), "")
			if names[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		var fragment bytes.Buffer
		for n := group.NextSibling; n != nil; n = n.NextSibling {
			stop := false
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "models-section" {
					stop = true
				}
			}
			if stop {
				break
			}
			_ = html.Render(&fragment, n)
		}
		sectionDoc, _ := html.Parse(strings.NewReader(fragment.String()))
		if sectionDoc == nil {
			continue
		}
		for _, section := range documentNodes(sectionDoc, "section") {
			head := documentNodes(section, "h3")
			if len(head) != 1 || strings.Join(documentLines(head[0]), "") != "Standard" {
				continue
			}
			var b bytes.Buffer
			_ = html.Render(&b, section)
			for _, table := range documentTables(b.Bytes()) {
				h := table[0]
				if len(h) != 3 || h[2] != "Paid Tier, per 1M tokens in USD" {
					continue
				}
				var input, output, cache map[string]*float64
				notes := []string{}
				for _, r := range table[1:] {
					if len(r) != 3 {
						continue
					}
					switch r[0] {
					case "Input price":
						input, _ = geminiPriceParts(r[2], time.Now())
					case "Output price (including thinking tokens)", "Output price":
						output, _ = geminiPriceParts(r[2], time.Now())
					case "Context caching price":
						cache, _ = geminiPriceParts(r[2], time.Now())
						notes = append(notes, r[0]+": "+r[2])
					}
				}
				if len(input) == 0 || len(output) == 0 {
					continue
				}
				p := model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: strings.Join(notes, " ")}
				var conditions []string
				for condition := range input {
					conditions = append(conditions, condition)
				}
				sort.Strings(conditions)
				valid := true
				for condition := range output {
					if condition != "" && input[condition] == nil {
						valid = false
					}
				}
				for _, condition := range conditions {
					o := output[condition]
					if o == nil {
						o = output[""]
					}
					if o == nil {
						valid = false
						break
					}
					label := "Standard"
					if condition != "" {
						label += " · " + condition
					}
					c := cache[condition]
					if c == nil {
						c = cache[""]
					}
					p.Tiers = append(p.Tiers, model.OfficialPricingTier{Label: label, Unit: "million_tokens", Input: input[condition], Output: o, CacheRead: c})
				}
				if valid {
					for _, id := range ids {
						out[id] = officialPriceEntry("google", address, p)
					}
				}
			}
		}
	}
	return out
}
