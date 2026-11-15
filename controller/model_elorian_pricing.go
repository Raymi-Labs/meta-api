package controller

import (
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

const elorianPricingURL = "https://elorian.ai/blog/introducing-glimpse"

func parseElorianPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	if !names["glimpse"] || address != elorianPricingURL {
		return out
	}
	text := strings.Join(strings.Fields(strings.Join(priceLines(data), " ")), " ")
	if !strings.Contains(text, "Introducing Glimpse") {
		return out
	}
	rates := regexp.MustCompile(`Input tokens cost (\$[0-9]+(?:\.[0-9]+)?) per million and output tokens (\$[0-9]+(?:\.[0-9]+)?) per million`).FindAllStringSubmatch(text, -1)
	if len(rates) != 1 {
		return out
	}
	p := model.OfficialPricing{Currency: "USD", SourceURL: address, Notes: "Glimpse launch reference rates published on 2026-10-16; cache pricing is not stated in this source.", Tiers: []model.OfficialPricingTier{{Label: "Standard", Unit: "million_tokens", Input: documentUSD(rates[0][1]), Output: documentUSD(rates[0][2])}}}
	out["glimpse"] = officialPriceEntry("elorian", address, p)
	return out
}
