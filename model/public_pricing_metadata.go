package model

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// Public presentation policy, not routing/model identity policy. Do not use this
// on API model IDs, group keys, billing expressions, or administrator records.
var publicResellerMention = regexp.MustCompile(`(?i)open[\s_-]*router|venice|aihubmix|muzeai|gettoken|vivgrid|tokenhub`)
var publicMetadataURL = regexp.MustCompile(`(?i)(?:https?://|//|www\.)[^\s<>"'\x60)\]，；。]+`)
var publicMetadataSentence = regexp.MustCompile(`(?:[。；;\r\n]+|\.[ \t]+)`)

func decodedPublicText(text string) string {
	text = html.UnescapeString(text)
	if decoded, err := url.PathUnescape(text); err == nil {
		text = decoded
	}
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
}

func publicManufacturerURL(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"meta-api.vip", "meta-api.com", "raymi.ai"} {
		if hostWithin(host, domain) {
			return true
		}
	}
	for _, domains := range manufacturerPriceDomains {
		for _, domain := range domains {
			if hostWithin(host, domain) {
				return true
			}
		}
	}
	return false
}

func publicSourceURL(address string) string {
	u, err := url.Parse(address)
	if err != nil || u.User != nil || u.Port() != "" || publicResellerMention.MatchString(decodedPublicText(address)) {
		return ""
	}
	u.RawQuery, u.Fragment, u.ForceQuery = "", "", false
	if !publicManufacturerURL(u.String()) {
		return ""
	}
	return u.String()
}

func privatePublicMetadata(text string) bool {
	decoded := decodedPublicText(text)
	if publicResellerMention.MatchString(decoded) {
		return true
	}
	for _, link := range publicMetadataURL.FindAllString(decoded, -1) {
		if !publicManufacturerURL(strings.TrimRight(link, ".,")) {
			return true
		}
	}
	return false
}

func publicMetadataText(text string) string {
	if !privatePublicMetadata(text) {
		return text
	}
	// Drop an entire contaminated clause, never substitute a different provider
	// or delete just a brand in a way that changes a pricing condition's meaning.
	parts := publicMetadataSentence.Split(decodedPublicText(text), -1)
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !privatePublicMetadata(part) {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "\n")
}

// PublicPricingMetadata always copies nested mutable presentation fields.
// The shared pricing cache and administrator metadata remain intact.
func PublicPricingMetadata(items []Pricing, vendors []PricingVendor) ([]Pricing, []PricingVendor) {
	out := make([]Pricing, 0, len(items))
	vendorOut := make([]PricingVendor, 0, len(vendors))
	hiddenVendors := map[int]bool{}
	for _, v := range vendors {
		if privatePublicMetadata(v.Name) {
			hiddenVendors[v.ID] = true
			continue
		}
		v.Description = publicMetadataText(v.Description)
		if privatePublicMetadata(v.Icon) {
			v.Icon = ""
		}
		vendorOut = append(vendorOut, v)
	}
	for _, item := range items {
		item.Description = publicMetadataText(item.Description)
		item.DisplayName = publicMetadataText(item.DisplayName)
		item.Tags = publicMetadataText(item.Tags)
		item.OwnerBy = publicMetadataText(item.OwnerBy)
		if privatePublicMetadata(item.Icon) {
			item.Icon = ""
		}
		if hiddenVendors[item.VendorID] {
			item.VendorID = 0
		}
		item.DescriptionTranslations = item.DescriptionTranslations.Clone()
		for lang, text := range item.DescriptionTranslations {
			item.DescriptionTranslations[lang] = publicMetadataText(text)
		}
		item.FAQ = item.FAQ.Clone()
		faq := make(ModelFAQ, 0, len(item.FAQ))
		for _, entry := range item.FAQ {
			entry.Question, entry.Answer = publicMetadataText(entry.Question), publicMetadataText(entry.Answer)
			if entry.Question != "" && entry.Answer != "" {
				faq = append(faq, entry)
			}
		}
		// Preserve nil versus empty for unchanged metadata consumers.
		if item.FAQ != nil {
			item.FAQ = faq
		}
		price := item.OfficialPricing
		if len(price.Tiers) > 0 {
			if !IsManufacturerPriceSource(item.ModelName, price.SourceURL) || publicSourceURL(price.SourceURL) == "" {
				price = OfficialPricing{}
			} else {
				price.SourceURL = publicSourceURL(price.SourceURL)
				price.Tiers = append([]OfficialPricingTier(nil), price.Tiers...)
				price.Notes = publicMetadataText(price.Notes)
				for _, tier := range price.Tiers {
					if privatePublicMetadata(tier.Label) {
						price = OfficialPricing{}
						break
					}
				}
			}
		} else {
			price = OfficialPricing{}
		}
		item.OfficialPricing = price
		if item.AttributeSources != nil {
			sources := map[string]ModelAttributeSource{}
			for key, source := range item.AttributeSources {
				if source.Kind != "official" || !IsManufacturerPriceSource(item.ModelName, source.URL) || publicSourceURL(source.URL) == "" {
					continue
				}
				source.URL = publicSourceURL(source.URL)
				if key == "official_pricing" && len(price.Tiers) == 0 {
					continue
				}
				sources[key] = source
			}
			item.AttributeSources = sources
		}
		out = append(out, item)
	}
	return out, vendorOut
}
