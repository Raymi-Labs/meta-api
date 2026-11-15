package controller

import (
	"html"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

const geminiPricingURL = "https://aistudio.google.com/docs/pricing"

// Adapt the official Markdoc subset to the existing strict Standard parser.
// Only explicit model links before the tabs establish identity; headings and
// links to adjacent models in descriptions must never assign a price.
func geminiMarkdocPricingHTML(data []byte) []byte {
	var out strings.Builder
	sections := regexp.MustCompile(`(?m)^## `).Split(string(data), -1)
	identity := regexp.MustCompile("\\*\\[`([a-zA-Z0-9._-]+)`\\]\\(/docs/models/([a-zA-Z0-9._-]+)\\)\\*")
	tabs := regexp.MustCompile(`(?s)\{% tab title="Standard" %\}(.*?)\{% /tab %\}`)
	tables := regexp.MustCompile(`(?s)\{% table %\}(.*?)\{% /table %\}`)
	for _, section := range sections[1:] {
		before, _, ok := strings.Cut(section, "{% tabs %}")
		if !ok {
			continue
		}
		ids := identity.FindAllStringSubmatch(before, -1)
		if len(ids) != 1 {
			continue
		}
		id := ids[0][1]
		// A series landing page is allowed only when the section anchor and
		// official playground independently name this exact model.
		if ids[0][2] != id {
			validSeries := ids[0][2] == "gemini-omni-flash" && regexp.MustCompile(`^gemini-omni-[0-9]+\.[0-9]+-flash$`).MatchString(id)
			validAnchor := strings.Contains(strings.SplitN(before, "\n", 2)[0], `{% id="`+id+`" %}`)
			validPlayground := strings.Contains(before, `href="https://aistudio.google.com/prompts/new_chat?model=`+id+`"`)
			if !validSeries || !validAnchor || !validPlayground {
				continue
			}
		}
		standard := tabs.FindAllStringSubmatch(section, -1)
		if len(standard) != 1 {
			continue
		}
		out.WriteString(`<div class="models-section"><code>` + html.EscapeString(ids[0][1]) + `</code></div><div><section><h3>Standard</h3>`)
		for _, table := range tables.FindAllStringSubmatch(standard[0][1], -1) {
			out.WriteString("<table>")
			for _, row := range regexp.MustCompile(`(?m)^\s*---\s*$`).Split(table[1], -1) {
				var cells []string
				valid := true
				for _, line := range strings.Split(row, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if strings.HasPrefix(line, "* ") {
						cells = append(cells, strings.TrimPrefix(line, "* "))
					} else if len(cells) > 0 {
						cells[len(cells)-1] += " " + line
					} else {
						valid = false
					}
				}
				if !valid || len(cells) != 3 {
					continue
				}
				out.WriteString("<tr>")
				for _, cell := range cells {
					out.WriteString("<td>" + html.EscapeString(strings.ReplaceAll(cell, "{% sup %}*{% /sup %}", "")) + "</td>")
				}
				out.WriteString("</tr>")
			}
			out.WriteString("</table>")
		}
		// Retain the official conversion explanation without converting rates.
		if _, tail, ok := strings.Cut(section, "{% /tabs %}"); ok && strings.Contains(tail, "Billing is based on total output token consumption") {
			tail = strings.ReplaceAll(strings.ReplaceAll(tail, "{% sup %}*{% /sup %}", ""), "\n", " ")
			out.WriteString("<aside>" + html.EscapeString(strings.TrimSpace(tail)) + "</aside>")
		}
		out.WriteString("</section></div>")
	}
	return []byte(out.String())
}

// Split explicit modality rates into existing units. A shared input price is
// repeated only for the modalities explicitly listed by the manufacturer.
func geminiModalityTiers(input, output, cache string) []model.OfficialPricingTier {
	parse := func(value string) map[string]*float64 {
		parts := pricePart.FindAllStringSubmatch(strings.TrimSpace(value), -1)
		if len(parts) == 0 || !strings.HasPrefix(strings.TrimSpace(value), "$") {
			return nil
		}
		out := map[string]*float64{}
		for _, part := range parts {
			condition := strings.TrimSpace(part[2])
			if !regexp.MustCompile(`^\((text|image|video|audio)( / (text|image|video|audio))*\)$`).MatchString(condition) {
				return nil
			}
			for _, mode := range strings.Split(strings.Trim(condition, "()"), " / ") {
				if out[mode] != nil {
					return nil
				}
				out[mode] = documentUSD("$" + part[1])
			}
		}
		return out
	}
	in, out := parse(input), parse(output)
	if len(in) == 0 || len(out) == 0 {
		return nil
	}
	cached := map[string]*float64{}
	if cache != "" {
		cached = parse(cache)
		if len(cached) == 0 {
			return nil
		}
	}
	for mode := range cached {
		if in[mode] == nil {
			return nil
		}
	}
	var tiers []model.OfficialPricingTier
	for _, mode := range []string{"text", "image", "audio", "video"} {
		if in[mode] == nil && out[mode] == nil {
			continue
		}
		unit := "million_" + mode + "_tokens"
		if mode == "text" {
			unit = "million_tokens"
		}
		tiers = append(tiers, model.OfficialPricingTier{Label: "Standard · " + mode, Unit: unit, Input: in[mode], Output: out[mode], CacheRead: cached[mode]})
	}
	return tiers
}
