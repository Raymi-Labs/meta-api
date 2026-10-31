package controller

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

const kimiPricingURL = "https://platform.kimi.ai/docs/pricing/chat"

// Kimi's Markdown export drops DocTable cells. Decode the HTML's flight strings
// as data, then accept only the known static table grammar. Never evaluate MDX.
func kimiPricingDocument(data []byte) (string, error) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}
	var chunks strings.Builder
	canonical, heading := false, false
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "link" {
				attrs := map[string]string{}
				for _, a := range n.Attr {
					attrs[a.Key] = a.Val
				}
				if attrs["rel"] == "canonical" && attrs["href"] == kimiPricingURL {
					canonical = true
				}
			}
			if n.Data == "h1" && strings.Join(documentLines(n), " ") == "Model Inference Pricing" {
				heading = true
			}
			if n.Data == "script" && n.FirstChild != nil {
				raw := strings.TrimSuffix(strings.TrimSpace(n.FirstChild.Data), ";")
				if strings.HasPrefix(raw, "self.__next_f.push(") && strings.HasSuffix(raw, ")") {
					var parts []json.RawMessage
					if common.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(raw, "self.__next_f.push("), ")")), &parts) == nil && len(parts) == 2 && string(parts[0]) == "1" {
						var text string
						if common.Unmarshal(parts[1], &text) == nil {
							chunks.WriteString(text)
						}
					}
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if !canonical || !heading || !strings.Contains(strings.Join(documentLines(doc), " "), "1 MTok = 1,000,000 tokens. All prices above are per MTok consumed.") {
		return "", errors.New("Kimi pricing identity or unit missing")
	}
	return chunks.String(), nil
}

// Split a literal array without interpreting expressions. Quotes and nested
// delimiters are tracked so an adjacent table or tooltip cannot shift columns.
func kimiLiteralArray(source string) ([]string, int, bool) {
	if !strings.HasPrefix(source, "[") {
		return nil, 0, false
	}
	stack := []byte{'['}
	var quote byte
	start := 1
	var items []string
	for i := 1; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if quote == '`' && c == '$' && i+1 < len(source) && source[i+1] == '{' {
				return nil, 0, false
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '`', '\'', '"':
			quote = c
		case '[', '{', '(':
			stack = append(stack, c)
		case ']', '}', ')':
			if len(stack) == 0 {
				return nil, 0, false
			}
			want := map[byte]byte{']': '[', '}': '{', ')': '('}[c]
			if stack[len(stack)-1] != want {
				return nil, 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				if i > start {
					items = append(items, strings.TrimSpace(source[start:i]))
				}
				return items, i + 1, true
			}
		case ',':
			if len(stack) == 1 {
				items = append(items, strings.TrimSpace(source[start:i]))
				start = i + 1
			}
		}
	}
	return nil, 0, false
}

var kimiPriceColumn = regexp.MustCompile("^\\{title:(?:`([^`]+)`|_jsx\\(Tooltip,\\{tip:`[^`]+`,children:`([^`]+)`\\}\\)),width:`[0-9]+%`\\}$")
var kimiPriceRow = regexp.MustCompile("^\\[_jsx\\(_components\\.a,\\{href:`/guide/[a-z0-9-]+`,children:`(kimi-[a-z0-9.-]+)`\\}\\)((?:,`\\$[0-9]+(?:\\.[0-9]+)? / MTok`){3,5})\\]$")
var kimiPriceCell = regexp.MustCompile("`(\\$[0-9]+(?:\\.[0-9]+)?) / MTok`")
var kimiEmptyPriceTable = regexp.MustCompile("^className:`doc-table-bordered`,corner:\\{top:`Model`,bottom:`Name`\\},groups:\\[\\{title:`Base tokens`,span:2\\},\\{title:`Prompt caching`,span:[13]\\}\\]\\}\\)")

func parseKimiPricing(data []byte, names map[string]bool) (map[string]model.ModelCapabilityMetadata, error) {
	source, err := kimiPricingDocument(data)
	if err != nil {
		return nil, err
	}
	out := map[string]model.ModelCapabilityMetadata{}
	seen := map[string]bool{}
	tables := 0
	for _, block := range strings.Split(source, "children:_jsx(DocTable,{")[1:] {
		// The flight payload also contains the Markdown export's empty copies.
		// Only skip that exact placeholder; changed populated tables still fail.
		if kimiEmptyPriceTable.MatchString(block) {
			continue
		}
		_, columnSource, found := strings.Cut(block, "columns:")
		if !found {
			return nil, errors.New("Kimi price columns missing")
		}
		columns, end, ok := kimiLiteralArray(columnSource)
		if !ok || (len(columns) != 4 && len(columns) != 6) || !strings.HasPrefix(columnSource[end:], ",rows:") {
			return nil, errors.New("unknown Kimi price layout")
		}
		expected := []string{"Model", "Input", "Output", "Cached Input", "5m writes", "1h writes"}
		for i, col := range columns {
			match := kimiPriceColumn.FindStringSubmatch(col)
			if match == nil || match[1]+match[2] != expected[i] {
				return nil, errors.New("unknown Kimi price column")
			}
		}
		rows, _, ok := kimiLiteralArray(strings.TrimPrefix(columnSource[end:], ",rows:"))
		if !ok || len(rows) == 0 {
			return nil, errors.New("Kimi price rows missing")
		}
		tables++
		for _, row := range rows {
			match := kimiPriceRow.FindStringSubmatch(row)
			if match == nil {
				return nil, errors.New("unknown Kimi price row or currency")
			}
			id := match[1]
			if seen[id] {
				return nil, errors.New("duplicate Kimi price model")
			}
			seen[id] = true
			values := kimiPriceCell.FindAllStringSubmatch(match[2], -1)
			if len(values) != len(columns)-1 {
				return nil, errors.New("incomplete Kimi price row")
			}
			if !names[id] {
				continue
			}
			tier := model.OfficialPricingTier{Label: "Standard", Unit: "million_tokens", Input: documentUSD(values[0][1]), Output: documentUSD(values[1][1]), CacheRead: documentUSD(values[2][1])}
			p := model.OfficialPricing{Currency: "USD", SourceURL: kimiPricingURL, Notes: "Prices exclude applicable taxes. Cached input is billed at the cache-hit price; cache writes use the selected TTL."}
			if len(columns) == 6 {
				tier.Label, tier.CacheWrite = "Standard · cache 5m", documentUSD(values[3][1])
				p.Tiers = append(p.Tiers, tier)
				tier.Label, tier.CacheWrite = "Standard · cache 1h", documentUSD(values[4][1])
			}
			p.Tiers = append(p.Tiers, tier)
			if err := p.Normalize(model.NameRuleExact); err != nil {
				return nil, err
			}
			out[id] = officialPriceEntry("moonshotai", kimiPricingURL, p)
		}
	}
	if tables == 0 {
		return nil, errors.New("Kimi price table missing")
	}
	return out, nil
}
