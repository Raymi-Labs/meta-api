package controller

import (
	"math"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

const openAIImagePricingURL = "https://developers.openai.com/api/docs/pricing"

// Decode Astro's serialized component values, not executable scripts. Unknown
// value tags are rejected instead of being interpreted as price data.
func astroPricingValue(v interface{}, depth int) (interface{}, bool) {
	if depth > 12 {
		return nil, false
	}
	if m, ok := v.(map[string]interface{}); ok {
		out := map[string]interface{}{}
		for k, x := range m {
			y, valid := astroPricingValue(x, depth+1)
			if !valid {
				return nil, false
			}
			out[k] = y
		}
		return out, true
	}
	a, ok := v.([]interface{})
	if !ok {
		return v, true
	}
	if len(a) != 2 {
		return nil, false
	}
	tag, ok := a[0].(float64)
	if !ok {
		return nil, false
	}
	if tag == 0 {
		return astroPricingValue(a[1], depth+1)
	}
	if tag == 1 {
		xs, ok := a[1].([]interface{})
		if !ok {
			return nil, false
		}
		out := []interface{}{}
		for _, x := range xs {
			y, valid := astroPricingValue(x, depth+1)
			if !valid {
				return nil, false
			}
			out = append(out, y)
		}
		return out, true
	}
	return nil, false
}
func parseOpenAIImagePricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	lines := priceLines(data)
	i := slices.Index(lines, "Image generation models")
	if i < 0 || i+1 >= len(lines) || lines[i+1] != "Prices per 1M tokens." {
		return out
	}
	doc, _ := html.Parse(strings.NewReader(string(data)))
	for _, node := range documentNodes(doc, "astro-island") {
		attrs := map[string]string{}
		for _, a := range node.Attr {
			attrs[a.Key] = a.Val
		}
		if attrs["component-export"] != "GroupedPricingTable" {
			continue
		}
		standard := false
		for p := node.Parent; p != nil; p = p.Parent {
			for _, a := range p.Attr {
				if a.Key == "data-value" {
					standard = a.Val == "standard"
					break
				}
			}
			if standard {
				break
			}
		}
		if !standard {
			continue
		}
		var props map[string]interface{}
		if common.UnmarshalJsonStr(attrs["props"], &props) != nil {
			continue
		}
		// Decode only required fields; tooltips can carry unrelated serialization tags.
		marker, ok := astroPricingValue(props["expandedQueryParam"], 0)
		if !ok || marker != "image-pricing-expanded" {
			continue
		}
		groups, ok := astroPricingValue(props["groups"], 0)
		if !ok {
			continue
		}
		b, err := common.Marshal(groups)
		if err != nil {
			continue
		}
		var parsed []struct {
			Model string          `json:"model"`
			Rows  [][]interface{} `json:"rows"`
		}
		if common.Unmarshal(b, &parsed) != nil {
			continue
		}
		rate := func(v interface{}) *float64 {
			n, ok := v.(float64)
			if !ok || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil
			}
			return &n
		}
		for _, g := range parsed {
			if !names[g.Model] || !strings.HasPrefix(g.Model, "gpt-image-") {
				continue
			}
			p := model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: "Standard processing, per million tokens of the stated modality. GPT Image 2/2.5 cached-input rates apply only to images generated with the Responses API; Batch rates are separate."}
			valid := len(g.Rows) == 2
			seen := map[string]bool{}
			for _, r := range g.Rows {
				if len(r) != 4 {
					valid = false
					break
				}
				modality, ok := r[0].(string)
				unit := map[string]string{"Text": "million_tokens", "Image": "million_image_tokens"}[modality]
				if !ok || unit == "" || seen[modality] {
					valid = false
					break
				}
				seen[modality] = true
				tier := model.OfficialPricingTier{Label: "Standard · " + modality + " tokens", Unit: unit, Input: rate(r[1]), Output: rate(r[2]), CacheRead: rate(r[3])}
				if tier.Input == nil || tier.CacheRead == nil || tier.Output == nil && !(modality == "Text" && r[2] == "-") {
					valid = false
					break
				}
				p.Tiers = append(p.Tiers, tier)
			}
			if valid {
				out[g.Model] = officialPriceEntry("openai", address, p)
			}
		}
	}
	return out
}
