package controller

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

const awsNovaPricingURL = "https://aws.amazon.com/nova/pricing/"
const awsNovaRatesURL = "https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/bedrock/USD/current/bedrock.json"

var awsPriceToken = regexp.MustCompile(`^\{priceOf!bedrock/bedrock!([a-zA-Z0-9_-]+)!\*!1000(?:!opt)?\}$`)

func fetchAWSNovaPricing(ctx context.Context, page []byte, names map[string]bool) (map[string]model.ModelCapabilityMetadata, error) {
	rates, err := fetchPublicModelDocument(ctx, awsNovaRatesURL)
	if err != nil {
		return nil, err
	}
	return parseAWSNovaPricing(page, rates, names)
}

// Join the manufacturer's visible on-demand table with its published rate map.
// The expression fixes the unit conversion; no arbitrary expressions or URLs
// from the page are evaluated. Keep one explicit region and service tier.
func parseAWSNovaPricing(page, rates []byte, names map[string]bool) (map[string]model.ModelCapabilityMetadata, error) {
	var catalog struct {
		Manifest struct {
			Currency string `json:"currencyCode"`
			Service  string `json:"serviceId"`
		} `json:"manifest"`
		Regions map[string]map[string]struct {
			Price string `json:"price"`
		} `json:"regions"`
	}
	if err := common.Unmarshal(rates, &catalog); err != nil {
		return nil, err
	}
	const region = "US East (N. Virginia)"
	if catalog.Manifest.Currency != "USD" || catalog.Manifest.Service != "bedrock" || len(catalog.Regions[region]) == 0 {
		return nil, errors.New("AWS currency/region/rates missing")
	}
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, err
	}
	out := map[string]model.ModelCapabilityMetadata{}
	ids := map[string]string{"Amazon Nova 2.0 Lite": "nova-2-lite-v1", "Amazon Nova Micro": "nova-micro-v1", "Amazon Nova Lite": "nova-lite-v1", "Amazon Nova Pro": "nova-pro-v1"}
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for _, a := range n.Attr {
			if a.Key == "data-pricing-markup" {
				nodes = append(nodes, n)
				break
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for _, node := range nodes {
		markup := ""
		for _, a := range node.Attr {
			if a.Key == "data-pricing-markup" {
				markup = a.Val
			}
		}
		if !strings.Contains(markup, "On Demand Inference") {
			continue
		}
		for _, table := range documentTables([]byte(markup)) {
			h := table[0]
			if len(h) != 4 || h[0] != "Amazon Nova Models" || h[1] != "Price per 1M input tokens" || h[2] != "Price per 1M input tokens (cache read)" || h[3] != "Price per 1M output tokens" {
				continue
			}
			rate := func(cell string) *float64 {
				m := awsPriceToken.FindStringSubmatch(cell)
				if m == nil {
					return nil
				}
				v := documentUSD("$" + catalog.Regions[region][m[1]].Price)
				if v == nil {
					return nil
				}
				*v *= 1000
				return v
			}
			for _, r := range table[1:] {
				if len(r) != 4 {
					continue
				}
				id := ids[r[0]]
				if id == "" || !names[id] {
					continue
				}
				t := model.OfficialPricingTier{Label: region + " · On-demand", Unit: "million_tokens", Input: rate(r[1]), CacheRead: rate(r[2]), Output: rate(r[3])}
				if t.Input == nil || t.Output == nil || t.CacheRead == nil {
					continue
				}
				out[id] = officialPriceEntry("amazon", awsNovaPricingURL, model.OfficialPricing{Currency: "USD", SourceURL: awsNovaPricingURL, Notes: "On-demand inference in US East (N. Virginia); other regions, global routing, latency optimization and Batch rates may differ.", Tiers: []model.OfficialPricingTier{t}})
			}
		}
	}
	return out, nil
}
