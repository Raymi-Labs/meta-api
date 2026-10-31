package controller

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

// xAI embeds the data used by its regional/context/quality controls in a
// standalone JSON assignment. Read the JSON only; never execute remote script.
// Integer prices are USD * 10^10 per token/image/second. Cross-check against
// the visible default price card before accepting this unit conversion.
type xaiPriceModel struct {
	Name        string `json:"name"`
	Cluster     string `json:"cluster"`
	Input       string `json:"promptTextTokenPrice"`
	Output      string `json:"completionTextTokenPrice"`
	Cache       string `json:"cachedPromptTokenPrice"`
	LongInput   string `json:"promptTextTokenPriceLongContext"`
	LongOutput  string `json:"completionTokenPriceLongContext"`
	LongCache   string `json:"cachedPromptTokenPriceLongContext"`
	Threshold   string `json:"longContextThreshold"`
	InputImage  string `json:"pricePerInputImage"`
	InputVideo  string `json:"pricePerInputVideoSecond"`
	Resolutions []struct {
		Resolution string `json:"resolution"`
		Quality    string `json:"quality"`
		Image      string `json:"pricePerImage"`
		Second     string `json:"pricePerSecond"`
	} `json:"resolutionPricing"`
}

type xaiPriceCatalog struct {
	Clusters []struct {
		Name  string          `json:"clusterName"`
		Text  []xaiPriceModel `json:"languageModels"`
		Image []xaiPriceModel `json:"imageGenerationModels"`
		Video []xaiPriceModel `json:"videoGenerationModels"`
	} `json:"clusterConfigs"`
}

var xaiPriceInteger = regexp.MustCompile(`^[0-9]+$`)
var xaiPriceRegion = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

func xaiPriceAmount(raw string, divisor float64) (*float64, error) {
	if !xaiPriceInteger.MatchString(raw) {
		return nil, errors.New("missing or invalid xAI price")
	}
	integer, err := strconv.ParseUint(raw, 10, 53)
	if err != nil {
		return nil, err
	}
	value := float64(integer) / divisor
	return &value, nil
}

func xaiModelPriceTiers(m xaiPriceModel, kind string) ([]model.OfficialPricingTier, error) {
	var tiers []model.OfficialPricingTier
	add := func(label, unit, input, output, cache string, divisor float64) error {
		tier := model.OfficialPricingTier{Label: m.Cluster + " · " + label, Unit: unit}
		for _, field := range []struct {
			raw    string
			target **float64
		}{{input, &tier.Input}, {output, &tier.Output}, {cache, &tier.CacheRead}} {
			if field.raw != "" {
				value, err := xaiPriceAmount(field.raw, divisor)
				if err != nil {
					return err
				}
				*field.target = value
			}
		}
		tiers = append(tiers, tier)
		return nil
	}
	if kind == "text" {
		if m.Input == "" || m.Output == "" {
			return nil, errors.New("incomplete xAI token prices")
		}
		label := "Standard"
		if m.Threshold != "" {
			threshold, err := strconv.ParseUint(m.Threshold, 10, 53)
			if err != nil || threshold == 0 || m.LongInput == "" || m.LongOutput == "" || (m.Cache != "" && m.LongCache == "") || (m.Cache == "" && m.LongCache != "") {
				return nil, errors.New("incomplete xAI long-context prices")
			}
			label += " · ≤ " + m.Threshold + " input tokens"
		} else if m.LongInput != "" || m.LongOutput != "" || m.LongCache != "" {
			return nil, errors.New("xAI long-context threshold missing")
		}
		if err := add(label, "million_tokens", m.Input, m.Output, m.Cache, 1e4); err != nil {
			return nil, err
		}
		if m.Threshold != "" {
			if err := add("Standard · > "+m.Threshold+" input tokens", "million_tokens", m.LongInput, m.LongOutput, m.LongCache, 1e4); err != nil {
				return nil, err
			}
		}
		return tiers, nil
	}
	resolutions := map[string]string{"IMAGE_RESOLUTION_1K": "1K", "IMAGE_RESOLUTION_1P5K": "1.5K", "IMAGE_RESOLUTION_2K": "2K", "VIDEO_RESOLUTION_480P": "480p", "VIDEO_RESOLUTION_720P": "720p", "VIDEO_RESOLUTION_1080P": "1080p"}
	if len(m.Resolutions) == 0 {
		return nil, errors.New("xAI media resolution prices missing")
	}
	seen := map[string]bool{}
	for _, r := range m.Resolutions {
		label := resolutions[r.Resolution]
		if label == "" || (r.Quality != "" && r.Quality != "low" && r.Quality != "medium" && r.Quality != "high") {
			return nil, errors.New("unsupported xAI media condition")
		}
		if r.Quality != "" {
			label += " · " + r.Quality
		}
		if seen[label] {
			return nil, errors.New("duplicate xAI resolution price")
		}
		seen[label] = true
		if kind == "image" {
			if !strings.HasPrefix(r.Resolution, "IMAGE_") || r.Image == "" || r.Second != "" {
				return nil, errors.New("invalid xAI image unit")
			}
			if err := add(label, "image", m.InputImage, r.Image, "", 1e10); err != nil {
				return nil, err
			}
		} else {
			if !strings.HasPrefix(r.Resolution, "VIDEO_") || r.Second == "" || r.Image != "" {
				return nil, errors.New("invalid xAI video unit")
			}
			if err := add(label, "second", m.InputVideo, r.Second, "", 1e10); err != nil {
				return nil, err
			}
		}
	}
	if kind == "video" && m.InputImage != "" {
		if err := add("Image input", "image", m.InputImage, "", "", 1e10); err != nil {
			return nil, err
		}
	}
	return tiers, nil
}

func parseXAIPrices(n *html.Node, id, address string) (model.OfficialPricing, error) {
	empty := model.OfficialPricing{}
	const prefix = "globalThis.__XAI_PUBLIC_MODELS__="
	var payloads []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			var b strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					b.WriteString(c.Data)
				}
			}
			value := strings.TrimSpace(b.String())
			if strings.HasPrefix(value, prefix) {
				payloads = append(payloads, strings.TrimSuffix(strings.TrimPrefix(value, prefix), ";"))
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if len(payloads) != 1 {
		return empty, errors.New("xAI official price catalog missing or ambiguous")
	}
	var catalog xaiPriceCatalog
	if err := common.UnmarshalJsonStr(payloads[0], &catalog); err != nil {
		return empty, err
	}
	pricing := model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: "Standard API prices. Region, input-context threshold, resolution and quality are shown separately; batch discounts and tool fees are excluded. Media input charges are additional to output charges."}
	seen := map[string]bool{}
	var first xaiPriceModel
	var firstKind string
	var firstTiers []model.OfficialPricingTier
	for _, cluster := range catalog.Clusters {
		for _, group := range []struct {
			kind   string
			models []xaiPriceModel
		}{{"text", cluster.Text}, {"image", cluster.Image}, {"video", cluster.Video}} {
			for _, m := range group.models {
				if m.Name != id {
					continue
				}
				if !xaiPriceRegion.MatchString(cluster.Name) || m.Cluster != cluster.Name || seen[m.Cluster] {
					return empty, errors.New("invalid or duplicate xAI price region")
				}
				if firstKind != "" && firstKind != group.kind {
					return empty, errors.New("ambiguous xAI model modality")
				}
				seen[m.Cluster] = true
				tiers, err := xaiModelPriceTiers(m, group.kind)
				if err != nil {
					return empty, err
				}
				if firstKind == "" {
					first = m
					firstKind = group.kind
					firstTiers = tiers
				}
				pricing.Tiers = append(pricing.Tiers, tiers...)
			}
		}
	}
	if len(pricing.Tiers) == 0 {
		return empty, errors.New("xAI exact model price missing")
	}
	// Validate the visible default region and price card against the embedded
	// values. A changed currency/unit/layout must preserve the last valid sheet.
	lines := documentLines(n)
	start, end := -1, -1
	for i, line := range lines {
		if line == "Pricing" {
			start = i + 1
		}
		if start >= 0 && line == "Details" {
			end = i
			break
		}
	}
	if start < 0 || end < start {
		return empty, errors.New("xAI price card missing")
	}
	regionFound := false
	for i := end + 1; i+1 < len(lines); i++ {
		if lines[i] == "Region" {
			regionFound = strings.TrimSpace(strings.Split(lines[i+1], ",")[0]) == first.Cluster
			break
		}
	}
	if !regionFound {
		return empty, errors.New("xAI default price region mismatch")
	}
	card := strings.Join(lines[start:end], " ")
	matchAmount := func(pattern string, expected *float64) bool {
		matches := regexp.MustCompile(pattern).FindAllStringSubmatch(card, -1)
		if len(matches) != 1 || expected == nil {
			return false
		}
		value, err := strconv.ParseFloat(matches[0][1], 64)
		return err == nil && value == *expected
	}
	base := firstTiers[0]
	valid := false
	if firstKind == "text" {
		valid = matchAmount(`Input Tokens \$([0-9]+(?:\.[0-9]+)?) / 1M tokens`, base.Input) && matchAmount(`Output Tokens \$([0-9]+(?:\.[0-9]+)?) / 1M tokens`, base.Output)
		if base.CacheRead != nil {
			valid = valid && matchAmount(`Cached tokens \$([0-9]+(?:\.[0-9]+)?) / 1M tokens`, base.CacheRead)
		} else if strings.Contains(card, "Cached tokens") {
			valid = false
		}
		if first.Threshold != "" {
			match := regexp.MustCompile(`exceed the ([0-9.,]+[KM]?) context window`).FindStringSubmatch(card)
			threshold, _ := strconv.ParseInt(first.Threshold, 10, 64)
			valid = valid && len(match) == 2 && documentTokens(match[1]) == threshold
		} else if strings.Contains(card, "Higher context pricing") {
			valid = false
		}
	} else {
		minimum := base.Output
		for _, tier := range firstTiers {
			if tier.Output != nil && *tier.Output < *minimum {
				minimum = tier.Output
			}
		}
		unit := map[string]string{"image": "image", "video": "second"}[firstKind]
		valid = matchAmount(`Output \$([0-9]+(?:\.[0-9]+)?) per `+unit, minimum)
		if firstKind == "image" && base.Input != nil {
			valid = valid && matchAmount(`Image input \$([0-9]+(?:\.[0-9]+)?) per image`, base.Input)
		}
	}
	if !valid {
		return empty, fmt.Errorf("xAI visible and embedded prices disagree for %s", id)
	}
	if err := pricing.Normalize(model.NameRuleExact); err != nil {
		return empty, err
	}
	return pricing, nil
}
