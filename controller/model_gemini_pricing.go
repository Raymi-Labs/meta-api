package controller

import (
	"html"
	"regexp"
	"strings"
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
		if len(ids) != 1 || ids[0][1] != ids[0][2] {
			continue
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
					out.WriteString("<td>" + html.EscapeString(cell) + "</td>")
				}
				out.WriteString("</tr>")
			}
			out.WriteString("</table>")
		}
		out.WriteString("</section></div>")
	}
	return []byte(out.String())
}
