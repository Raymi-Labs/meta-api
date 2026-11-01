package controller

import (
	"bytes"
	"errors"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

const (
	inceptionPricingURL = "https://docs.inceptionlabs.ai/get-started/models.md"
	writerPricingURL    = "https://dev.writer.com/home/pricing.md"
	morphPricingURL     = "https://www.morphllm.com/api/models/json?prices=list"
	aionPricingURL      = "https://api.aionlabs.ai/v1/models"
	minimaxPricingURL   = "https://platform.minimax.io/docs/pricing/overview.md"
	mistralPricingURL   = "https://docs.mistral.ai/inference/pricing"
)

// Only an explicit struck-out old price followed by a new price is a replacement.
// A slash, range or "from" price must not be mistaken for a flat unit rate.
var replacedUSD = regexp.MustCompile(`^~~\\?\$[0-9]+(?:\.[0-9]+)?~~ (?:→ )?(\\?\$[0-9]+(?:\.[0-9]+)?)$`)

func currentDocumentUSD(s string) *float64 {
	if m := replacedUSD.FindStringSubmatch(s); m != nil {
		return documentUSD(m[1])
	}
	return documentUSD(s)
}

func parseInceptionPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	for _, t := range documentTables(data) {
		if len(t[0]) < 4 || !slices.Equal(t[0][:4], []string{"Model", "Input Price (1M Tokens)", "Cached Input Price (1M Tokens)", "Output Price (1M Tokens)"}) {
			continue
		}
		for _, r := range t[1:] {
			if len(r) != len(t[0]) {
				continue
			}
			id := strings.ToLower(strings.ReplaceAll(r[0], " ", "-"))
			// Require an exact API ID in the same official model reference, not only a display name.
			if !names[id] || !strings.Contains(string(data), `"`+id+`"`) {
				continue
			}
			input, cache, output := currentDocumentUSD(r[1]), currentDocumentUSD(r[2]), currentDocumentUSD(r[3])
			if input == nil || output == nil || cache == nil {
				continue
			}
			label := "Standard"
			if strings.Contains(r[1], "~~") {
				label = "Launch discount"
			}
			out[id] = officialPriceEntry("inception", address, model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: "Current published token rates; launch discounts apply only while offered by the manufacturer.", Tiers: []model.OfficialPricingTier{{Label: label, Unit: "million_tokens", Input: input, Output: output, CacheRead: cache}}})
		}
	}
	return out
}

func parseWriterPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	for _, t := range documentTables(data) {
		if !slices.Equal(t[0], []string{"Model name", "Model ID", "Input / 1M", "Output / 1M"}) {
			continue
		}
		for _, r := range t[1:] {
			if len(r) != 4 || !names[r[1]] || !strings.HasPrefix(r[1], "palmyra-") {
				continue
			}
			input, output := documentUSD(r[2]), documentUSD(r[3])
			if input == nil || output == nil {
				continue
			}
			out[r[1]] = officialPriceEntry("writer", address, model.OfficialPricing{Currency: "USD", SourceURL: address, Tiers: []model.OfficialPricingTier{{Label: "Standard", Unit: "million_tokens", Input: input, Output: output}}})
		}
	}
	return out
}

var solarPrice = regexp.MustCompile(`(?m)^- \*\*Price\*\*: (\$[0-9]+(?:\.[0-9]+)?)/1M tokens \((Input|Output|Cached) Token\)$`)

func parseUpstagePricing(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	empty := model.ModelCapabilityMetadata{}
	identity := regexp.MustCompile(`(?m)^### ` + regexp.QuoteMeta(id) + `-[0-9]{6} \(Alias: ` + regexp.QuoteMeta(id) + `\)$`)
	if !identity.Match(data) {
		return empty, errors.New("Upstage model identity missing")
	}
	tier := model.OfficialPricingTier{Label: "Standard", Unit: "million_tokens"}
	fields := map[string]**float64{"Input": &tier.Input, "Output": &tier.Output, "Cached": &tier.CacheRead}
	for _, m := range solarPrice.FindAllStringSubmatch(string(data), -1) {
		if *fields[m[2]] != nil {
			return empty, errors.New("duplicate Upstage rate")
		}
		*fields[m[2]] = documentUSD(m[1])
	}
	if tier.Input == nil || tier.Output == nil || tier.CacheRead == nil {
		return empty, errors.New("incomplete Upstage rates")
	}
	return officialPriceEntry("upstage", address, model.OfficialPricing{Currency: "USD", SourceURL: address, Tiers: []model.OfficialPricingTier{tier}}), nil
}

func perTokenUSD(s string) *float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	v *= 1e6
	return &v
}
func parseAionPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	var catalog struct {
		Data []struct {
			ID      string            `json:"id"`
			Pricing map[string]string `json:"pricing"`
		} `json:"data"`
	}
	if common.Unmarshal(data, &catalog) != nil {
		return out
	}
	for _, m := range catalog.Data {
		id := model.ManufacturerNativeModel(m.ID)
		if !names[id] || !strings.HasPrefix(m.ID, "aion-labs/aion-") {
			continue
		}
		input, output := perTokenUSD(m.Pricing["prompt"]), perTokenUSD(m.Pricing["completion"])
		if input == nil || output == nil {
			continue
		}
		out[id] = officialPriceEntry("aion-labs", address, model.OfficialPricing{Currency: "USD", SourceURL: address, Tiers: []model.OfficialPricingTier{{Label: "Standard", Unit: "million_tokens", Input: input, Output: output, CacheRead: perTokenUSD(m.Pricing["input_cache_read"])}}})
	}
	return out
}
func parseMorphPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	type modality struct {
		Type    string `json:"type"`
		Pricing []struct {
			Type string `json:"type"`
			Unit string `json:"unit"`
			Cost string `json:"cost_usd"`
		} `json:"pricing"`
	}
	var catalog struct {
		Data []struct {
			ID     string     `json:"id"`
			Input  []modality `json:"input_modalities"`
			Output []modality `json:"output_modalities"`
		} `json:"data"`
	}
	if common.Unmarshal(data, &catalog) != nil {
		return out
	}
	for _, m := range catalog.Data {
		if !names[m.ID] || !strings.HasPrefix(m.ID, "morph-v3-") {
			continue
		}
		rate := func(ms []modality, kind string) *float64 {
			if len(ms) != 1 || ms[0].Type != "text" || len(ms[0].Pricing) != 1 {
				return nil
			}
			p := ms[0].Pricing[0]
			if p.Type != kind || p.Unit != "token" {
				return nil
			}
			return perTokenUSD(p.Cost)
		}
		input, output := rate(m.Input, "prompt"), rate(m.Output, "completion")
		if input == nil || output == nil {
			continue
		}
		out[m.ID] = officialPriceEntry("morph", address, model.OfficialPricing{Currency: "USD", SourceURL: address, Tiers: []model.OfficialPricingTier{{Label: "List price", Unit: "million_tokens", Input: input, Output: output}}})
	}
	return out
}

// Model ID aliases verified on the linked Mistral model references. Prices are
// fetched from the current manufacturer sheet, never frozen in this mapping.
var mistralPricingModels = map[string]string{
	"ministral-14b-2512": "Ministral 3 14B", "ministral-8b-2512": "Ministral 3 8B", "ministral-3b-2512": "Ministral 3 3B",
	"mistral-large-4-0": "Mistral Large 4", "mistral-small-2603": "Mistral Small 4", "codestral-2508": "Codestral",
}

var mistralPricingSlugs = map[string]string{
	"ministral-14b-2512": "ministral-3-14b-25-12", "ministral-8b-2512": "ministral-3-8b-25-12", "ministral-3b-2512": "ministral-3-3b-25-12",
	"mistral-large-4-0": "mistral-large-4-0", "mistral-small-2603": "mistral-small-4-0-26-03", "codestral-2508": "codestral-25-08",
}

func parseMistralPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	doc, _ := html.Parse(bytes.NewReader(data))
	for _, node := range documentNodes(doc, "table") {
		context := node
		for n := 0; n < 3 && context.Parent != nil; n++ {
			context = context.Parent
		}
		if len(documentNodes(context, "table")) != 1 || !slices.Contains(documentLines(context), "Prices /M Tokens") {
			continue
		}
		var markup bytes.Buffer
		_ = html.Render(&markup, node)
		tables := documentTables(markup.Bytes())
		if len(tables) != 1 {
			continue
		}
		t := tables[0]
		if !slices.Equal(t[0], []string{"Model", "Input", "Cached input", "Output"}) {
			continue
		}
		for _, r := range t[1:] {
			if len(r) != 4 {
				continue
			}
			for id, display := range mistralPricingModels {
				linked := false
				for _, a := range documentNodes(node, "a") {
					if strings.Join(documentLines(a), " ") != display+" ↗" {
						continue
					}
					for _, attr := range a.Attr {
						if attr.Key == "href" && (attr.Val == "/models/"+mistralPricingSlugs[id] || attr.Val == "https://docs.mistral.ai/models/"+mistralPricingSlugs[id]) {
							linked = true
						}
					}
				}
				if !linked || !names[id] || r[0] != display+" ↗" {
					continue
				}
				input, cache, output := documentUSD(r[1]), documentUSD(r[2]), documentUSD(r[3])
				if input == nil || cache == nil || output == nil {
					continue
				}
				out[id] = officialPriceEntry("mistralai", address, model.OfficialPricing{Currency: "USD", SourceURL: address, Tiers: []model.OfficialPricingTier{{Label: "Standard", Unit: "million_tokens", Input: input, Output: output, CacheRead: cache}}})
			}
		}
	}
	return out
}

func parseMiniMaxPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	prices := map[string]model.OfficialPricing{}
	invalid := map[string]bool{}
	add := func(id string, t model.OfficialPricingTier) {
		p := prices[id]
		p.Tiers = append(p.Tiers, t)
		prices[id] = p
	}
	text := string(data)
	// Preserve service-tier headings in blockquoted Markdown tables.
	for _, part := range strings.Split(text, "> [tab] ") {
		band := "Standard"
		if strings.HasPrefix(part, "Priority") {
			band = "Priority"
		}
		lines := strings.Split(part, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimPrefix(l, "> ")
		}
		for _, table := range documentTables([]byte(strings.Join(lines, "\n"))) {
			h := table[0]
			if len(h) == 4 && h[0] == "Model" && strings.Contains(h[1], "M tokens") && strings.Contains(h[2], "M tokens") {
				for _, r := range table[1:] {
					if len(r) != 4 {
						continue
					}
					for id := range names {
						if !strings.EqualFold(r[0], id) && !strings.HasPrefix(strings.ToLower(r[0]), strings.ToLower(id)+" ") {
							continue
						}
						input, output, cache := currentDocumentUSD(r[1]), currentDocumentUSD(r[2]), currentDocumentUSD(r[3])
						if input == nil || output == nil || cache == nil {
							invalid[id] = true
							continue
						}
						suffix := strings.TrimSpace(r[0][len(id):])
						add(id, model.OfficialPricingTier{Label: strings.TrimSpace(band + " " + suffix), Unit: "million_tokens", Input: input, Output: output, CacheRead: cache})
					}
				}
			}
			if len(h) == 5 && h[0] == "Model" && h[1] == "Output Resolution" && strings.Contains(h[2], "per second") && strings.Contains(h[4], "beyond free quota") {
				for _, r := range table[1:] {
					if len(r) != 5 {
						continue
					}
					id := strings.Split(r[0], "\\")[0]
					if id != "MiniMax-H3" || !names[id] {
						continue
					}
					input, output, images := documentUSD(r[3]), documentUSD(r[2]), documentUSD(r[4])
					if input == nil || output == nil || images == nil {
						invalid[id] = true
						continue
					}
					add(id, model.OfficialPricingTier{Label: r[1] + " · video", Unit: "second", Input: input, Output: output})
					p := prices[id]
					label := "Image input beyond first 5 free images"
					found := false
					for _, t := range p.Tiers {
						if t.Label == label {
							found = true
						}
					}
					if !found {
						add(id, model.OfficialPricingTier{Label: label, Unit: "image", Input: images})
					}
				}
			}
		}
	}
	out := map[string]model.ModelCapabilityMetadata{}
	for id, p := range prices {
		if invalid[id] {
			continue
		}
		p.Currency = "USD"
		p.SourceURL = address
		p.Notes = "Published pay-as-you-go rates. Service tier, context range and video resolution are retained in each tier; video inputs are billed at output resolution."
		if id == "MiniMax-H3" {
			p.Notes += " First 5 image inputs are free for MiniMax-H3; audio input is free."
		}
		e := officialPriceEntry("minimax", address, p)
		if len(e.OfficialPricing.Tiers) > 0 {
			out[id] = e
		}
	}
	return out
}

const (
	metaPricingURL      = "https://dev.meta.ai/docs/pricing-rate-limits"
	liquidPricingURL    = "https://www.liquid.ai/pricing"
	moondreamPricingURL = "https://moondream.ai/pricing"
	rekaPricingURL      = "https://developer.reka.ai/pricing"
	longcatPricingURL   = "https://longcat.chat/platform/docs/Pricing/LongCat-2.0.html"
	fireworksPricingURL = "https://fireworks.ai/models/fireworks/ember-1"
	particlePricingURL  = "https://particle.ai/blog/introducing-particle-1"
	inferencePricingURL = "https://inference.net/blog/schematron-v2/"
	unbiasedPricingURL  = "https://unbiased.ai/changelog/"
)

func priceLines(data []byte) []string {
	n, _ := html.Parse(strings.NewReader(string(data)))
	if n == nil {
		return nil
	}
	return documentLines(n)
}
func simplePrice(provider, id, address string, t model.OfficialPricingTier) map[string]model.ModelCapabilityMetadata {
	if t.Input == nil || t.Output == nil {
		return nil
	}
	return map[string]model.ModelCapabilityMetadata{id: officialPriceEntry(provider, address, model.OfficialPricing{Currency: "USD", SourceURL: address, Tiers: []model.OfficialPricingTier{t}})}
}

func parseMetaPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	doc, _ := html.Parse(bytes.NewReader(data))
	if doc == nil {
		return out
	}
	sections := map[string][]string{"Muse Wonder": {"muse-wonder-1.0"}, "Muse Spark": {"muse-spark-1.3", "muse-spark-1.2", "muse-spark-1.1"}}
	section := ""
	identities := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
			section = strings.Join(documentLines(n), " ")
			identities = map[string]bool{}
			return
		}
		if n.Type == html.TextNode {
			id := strings.TrimSpace(n.Data)
			if slices.Contains(sections[section], id) {
				identities[id] = true
			}
		}
		if n.Type == html.ElementNode && n.Data == "table" {
			ids := sections[section]
			if len(ids) == 0 {
				return
			}
			var markup bytes.Buffer
			_ = html.Render(&markup, n)
			tables := documentTables(markup.Bytes())
			if len(tables) != 1 || !slices.Equal(tables[0][0], []string{"Usage", "Standard mode (per 1M tokens)", "Contributor mode (per 1M tokens)"}) {
				return
			}
			tier := model.OfficialPricingTier{Label: "Standard mode", Unit: "million_tokens"}
			seen := map[string]bool{}
			for _, r := range tables[0][1:] {
				if len(r) != 3 {
					return
				}
				var field **float64
				switch r[0] {
				case "Input":
					field = &tier.Input
				case "Output":
					field = &tier.Output
				case "Cached input":
					field = &tier.CacheRead
				default:
					continue
				}
				if seen[r[0]] {
					return
				}
				seen[r[0]] = true
				*field = documentUSD(r[1])
				if *field == nil {
					return
				}
			}
			for _, id := range ids {
				if !names[id] || !identities[id] {
					continue
				}
				for k, e := range simplePrice("meta", id, address, tier) {
					out[k] = e
				}
			}
			section = ""
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}
func parseLiquidPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	id := "lfm-2.5-24b-a2b"
	if !names[id] {
		return nil
	}
	lines := priceLines(data)
	start := slices.Index(lines, "LFM2.5-24B-A2B")
	if start < 0 || start+10 >= len(lines) {
		return nil
	}
	s := lines[start+1:]
	if s[0] != "Input" || s[2] != "/ MTok" || s[3] != "Cached input" || s[5] != "/ MTok" || s[6] != "Output" || s[8] != "/ MTok" {
		return nil
	}
	return simplePrice("liquid", id, address, model.OfficialPricingTier{Label: "Standard", Unit: "million_tokens", Input: documentUSD(s[1]), CacheRead: documentUSD(s[4]), Output: documentUSD(s[7])})
}
func parseMoondreamPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	id := "moondream3-preview"
	if !names[id] {
		return nil
	}
	lines := priceLines(data)
	p := model.OfficialPricing{Currency: "USD", SourceURL: address}
	for _, band := range []string{"Real-time", "Batch"} {
		start := slices.Index(lines, "Moondream 3 Preview — "+band)
		if start < 0 || start+4 >= len(lines) {
			return nil
		}
		s := lines[start+1 : start+5]
		if s[1] != "/ 1M input tokens" || s[3] != "/ 1M output tokens" {
			return nil
		}
		input, output := documentUSD(s[0]), documentUSD(s[2])
		if input == nil || output == nil {
			return nil
		}
		p.Tiers = append(p.Tiers, model.OfficialPricingTier{Label: band, Unit: "million_tokens", Input: input, Output: output})
	}
	return map[string]model.ModelCapabilityMetadata{id: officialPriceEntry("moondream", address, p)}
}
func parseRekaPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	for _, table := range documentTables(data) {
		if len(table[0]) < 4 || !slices.Equal(table[0][:4], []string{"Model ID", "Input $/M", "Output $/M", "Cache read $/M"}) {
			continue
		}
		for _, r := range table[1:] {
			if len(r) != len(table[0]) || len(strings.Fields(r[0])) == 0 {
				continue
			}
			id := strings.Fields(r[0])[0]
			if !names[id] || !strings.HasPrefix(id, "reka-") {
				continue
			}
			for k, v := range simplePrice("rekaai", id, address, model.OfficialPricingTier{Label: "Standard", Unit: "million_tokens", Input: documentUSD(r[1]), Output: documentUSD(r[2]), CacheRead: documentUSD(r[3])}) {
				out[k] = v
			}
		}
	}
	return out
}
func parseLongcatPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	id := "longcat-2.0"
	if !names[id] || !slices.Contains(priceLines(data), "LongCat-2.0") {
		return nil
	}
	for _, table := range documentTables(data) {
		if !slices.Equal(table[0], []string{"Item", "Price $/1M Tokens", "Discounted Price $/1M Tokens (limited-time)"}) {
			continue
		}
		tier := model.OfficialPricingTier{Label: "Limited-time discount", Unit: "million_tokens"}
		for _, r := range table[1:] {
			if len(r) != 3 {
				continue
			}
			switch r[0] {
			case "Uncached Input":
				tier.Input = documentUSD(r[2])
			case "Cached Input":
				tier.CacheRead = documentUSD(r[2])
			case "Output":
				tier.Output = documentUSD(r[2])
			}
		}
		if tier.Input != nil && tier.Output != nil {
			return simplePrice("meituan", id, address, tier)
		}
	}
	return nil
}
func parseFireworksPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	id := "ember-1"
	if !names[id] {
		return nil
	}
	lines := priceLines(data)
	if !slices.Contains(lines, "accounts/fireworks/models/ember-1") {
		return nil
	}
	i := slices.Index(lines, "Available Serverless")
	if i < 0 || i+3 >= len(lines) || lines[i+3] != "Per 1M Tokens (input/cached input/output)" {
		return nil
	}
	r := strings.Split(lines[i+1], " / ")
	if len(r) != 3 {
		return nil
	}
	return simplePrice("fireworks", id, address, model.OfficialPricingTier{Label: "Serverless", Unit: "million_tokens", Input: documentUSD(r[0]), CacheRead: documentUSD(r[1]), Output: documentUSD(r[2])})
}

var perceptronRates = regexp.MustCompile(`^\$([0-9]+(?:\.[0-9]+)?)/M input tokens \| \$([0-9]+(?:\.[0-9]+)?)/M output tokens$`)

func parsePerceptronPricing(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	lines := priceLines(data)
	for i, s := range lines {
		if s != id || i+2 >= len(lines) || lines[i+1] != "Pricing:" {
			continue
		}
		m := perceptronRates.FindStringSubmatch(lines[i+2])
		if m != nil {
			o := simplePrice("perceptron", id, address, model.OfficialPricingTier{Label: "Published API price", Unit: "million_tokens", Input: documentUSD("$" + m[1]), Output: documentUSD("$" + m[2])})
			return o[id], nil
		}
	}
	return model.ModelCapabilityMetadata{}, errors.New("Perceptron identity/pricing missing")
}

var particleRates = regexp.MustCompile(`^Prices per million tokens for uncached input, cached input, and output \(including reasoning\) are (\$[0-9.]+), (\$[0-9.]+), and (\$[0-9.]+) for both Particle-1 and DeepSeek V4.1 Flash,`)

func parseParticlePricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	id := "particle-1"
	if !names[id] {
		return nil
	}
	for _, s := range priceLines(data) {
		if m := particleRates.FindStringSubmatch(s); m != nil {
			return simplePrice("particle-ai", id, address, model.OfficialPricingTier{Label: "Published API price", Unit: "million_tokens", Input: documentUSD(m[1]), CacheRead: documentUSD(m[2]), Output: documentUSD(m[3])})
		}
	}
	return nil
}

var inferenceRates = regexp.MustCompile(`^(\$[0-9.]+)/1M input tokens – (\$[0-9.]+)/1M output tokens$`)

func parseInferencePricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	lines := priceLines(data)
	start := slices.Index(lines, "Schematron V2 is priced to make internet-scale extraction accessible:")
	if start < 0 {
		return out
	}
	lines = lines[start+1:]
	for id, label := range map[string]string{"schematron-v2-small": "Schematron V2:", "schematron-v2-turbo": "Schematron V2 Turbo:"} {
		if !names[id] || !strings.Contains(string(data), "inference-net/schematron-v2-small") || !strings.Contains(string(data), "two new models: Schematron V2 Small and Schematron V2 Turbo") {
			continue
		}
		i := slices.Index(lines, label)
		if i < 0 || i+1 >= len(lines) {
			continue
		}
		if m := inferenceRates.FindStringSubmatch(lines[i+1]); m != nil {
			for k, v := range simplePrice("inference-net", id, address, model.OfficialPricingTier{Label: "Published API price", Unit: "million_tokens", Input: documentUSD(m[1]), Output: documentUSD(m[2])}) {
				out[k] = v
			}
		}
	}
	return out
}

var unbiasedRates = regexp.MustCompile(`^Pricing is unchanged from the preview: (\$[0-9.]+) input, (\$[0-9.]+) cached input, (\$[0-9.]+) output per 1M tokens,`)

func parseUnbiasedPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	id := "pareto-26.10"
	if !names[id] {
		return nil
	}
	lines := priceLines(data)
	i := slices.Index(lines, "Pareto 26.10 is generally available")
	if i < 0 {
		return nil
	}
	for j := i + 1; j < len(lines) && j < i+25; j++ {
		if lines[j] == "October 1, 2026" {
			break
		}
		if m := unbiasedRates.FindStringSubmatch(lines[j]); m != nil {
			return simplePrice("unbiased", id, address, model.OfficialPricingTier{Label: "26.10 GA", Unit: "million_tokens", Input: documentUSD(m[1]), CacheRead: documentUSD(m[2]), Output: documentUSD(m[3])})
		}
	}
	return nil
}

const thinkingMachinesPricingURL = "https://tinker-docs.thinkingmachines.ai/tinker/models/models_and_pricing/"

var thinkingInputPrice = regexp.MustCompile(`^(\$[0-9.]+) (\$[0-9.]+) \(cached\)$`)

func parseThinkingMachinesPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	if !names["inkling"] || !slices.Contains(priceLines(data), "All prices are per million tokens. Checkpoint storage is charged at $0.10 per GB per month.") {
		return nil
	}
	for _, t := range documentTables(data) {
		if !slices.Equal(t[0], []string{"Model", "Tinker ID", "Context", "Prefill (Input)", "Sample (Output)"}) {
			continue
		}
		for _, r := range t[1:] {
			if len(r) != 5 || r[0] != "Inkling" || r[1] != "thinkingmachines/Inkling:peft:262144:sampling-nvfp4" || r[2] != "256K" {
				continue
			}
			m := thinkingInputPrice.FindStringSubmatch(r[3])
			if m == nil {
				continue
			}
			return simplePrice("thinkingmachines", "inkling", address, model.OfficialPricingTier{Label: "Tinker inference · 256K · NVFP4", Unit: "million_tokens", Input: documentUSD(m[1]), CacheRead: documentUSD(m[2]), Output: documentUSD(r[4])})
		}
	}
	return nil
}
