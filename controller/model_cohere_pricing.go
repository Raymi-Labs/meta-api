package controller

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

// Alias evidence: the Model ID section on each linked Cohere model page.
// Never strip arbitrary dates, :free or provider-specific variant suffixes.
var coherePriceModels = map[string]struct{ slug, native string }{
	"command-a":              {"command-a", "command-a-03-2025"},
	"command-a-03-2025":      {"command-a", "command-a-03-2025"},
	"command-r-08-2024":      {"command-r", "command-r-08-2024"},
	"command-r-plus-08-2024": {"command-r-plus", "command-r-plus-08-2024"},
	"command-r7b-12-2024":    {"command-r7b", "command-r7b-12-2024"},
}

var cohereTokenPrices = regexp.MustCompile(`^Input \$\s*([0-9]+(?:\.[0-9]+)?) / 1M tokens Output \$\s*([0-9]+(?:\.[0-9]+)?) / 1M tokens$`)

func parseCoherePricing(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	empty := model.ModelCapabilityMetadata{}
	ref, ok := coherePriceModels[id]
	if !ok {
		return empty, errors.New("unknown Cohere model alias")
	}
	n, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return empty, err
	}
	lines := documentLines(n)
	identity := slices.Index(lines, "Model ID")
	if identity < 0 || identity+1 >= len(lines) || lines[identity+1] != ref.native {
		return empty, errors.New("Cohere model identity mismatch")
	}
	start, end := slices.Index(lines, "Pricing"), slices.Index(lines, "Specifications")
	if start < 0 || end <= start {
		return empty, errors.New("Cohere price section missing")
	}
	match := cohereTokenPrices.FindStringSubmatch(strings.Join(lines[start+1:end], " "))
	if match == nil {
		return empty, errors.New("unsupported Cohere price layout or unit")
	}
	p := model.OfficialPricing{Currency: "USD", SourceURL: address, Tiers: []model.OfficialPricingTier{{
		Label: "Cohere Standard", Unit: "million_tokens", Input: documentUSD("$" + match[1]), Output: documentUSD("$" + match[2]),
	}}}
	if err := p.Normalize(model.NameRuleExact); err != nil {
		return empty, err
	}
	return officialPriceEntry("cohere", address, p), nil
}
