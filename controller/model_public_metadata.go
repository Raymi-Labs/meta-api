package controller

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

func metadataVendorName(provider string) string {
	return map[string]string{"openai": "OpenAI", "anthropic": "Anthropic", "google": "Google", "deepseek": "DeepSeek", "xai": "xAI", "alibaba": "阿里巴巴", "alibaba-cn": "阿里巴巴", "moonshotai": "Moonshot", "moonshotai-cn": "Moonshot", "xiaomi": "小米 MiMo", "minimax": "MiniMax", "minimax-cn": "MiniMax", "zai": "智谱", "zhipuai": "智谱", "mistral": "Mistral", "inception": "Inception"}[provider]
}

// A reseller catalog can have a higher metadata priority than the original
// provider. Do not turn its billing provider into the model's manufacturer.
func communityMetadataVendor(provider, id string) string {
	prefixes := map[string][]string{"openai": {"gpt-", "chatgpt-", "o1", "o3", "o4", "whisper", "dall-e", "text-"}, "anthropic": {"claude-"}, "google": {"gemini-", "gemma-", "imagen-", "veo-"}, "deepseek": {"deepseek-"}, "xai": {"grok-"}, "alibaba": {"qwen", "qwq", "qvq"}, "alibaba-cn": {"qwen", "qwq", "qvq"}, "moonshotai": {"kimi-", "moonshot-"}, "moonshotai-cn": {"kimi-", "moonshot-"}, "xiaomi": {"mimo-"}, "minimax": {"minimax-"}, "minimax-cn": {"minimax-"}, "zai": {"glm-"}, "zhipuai": {"glm-"}, "mistral": {"mistral", "codestral", "devstral", "ministral", "pixtral"}, "inception": {"mercury"}}
	for _, prefix := range prefixes[provider] {
		if strings.HasPrefix(strings.ToLower(id), prefix) {
			return metadataVendorName(provider)
		}
	}
	return ""
}

func metadataTags(values ...string) string {
	var out []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !strings.Contains(value, ",") {
			out = append(out, value)
		}
	}
	return strings.Join(out, ",")
}

// Public document redirects may remain on the same official host. Credentialed
// API requests still reject every redirect. Never follow source-supplied URLs.
func fetchPublicModelDocument(ctx context.Context, address string) ([]byte, error) {
	accept := "text/html"
	if strings.HasPrefix(address, "https://platform.claude.com/") || strings.HasSuffix(address, ".md") {
		accept = "text/markdown, text/html;q=0.9"
	}
	return fetchOfficialAttributeData(ctx, address, map[string]string{"Accept": accept, "User-Agent": "MetaAPI-ModelMetadata/1.0"})
}

func documentNodes(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "svg" || n.Data == "nav") {
			return
		}
		if n.Type == html.ElementNode && n.Data == tag {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return out
}
func documentLines(n *html.Node) []string {
	var out []string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "svg" || n.Data == "nav") {
			return
		}
		if n.Type == html.TextNode {
			if v := strings.TrimSpace(n.Data); v != "" {
				out = append(out, v)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return out
}

var markdownLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)

func cleanDocumentText(s string) string {
	s = markdownLink.ReplaceAllString(s, "$1")
	n, _ := html.Parse(strings.NewReader(s))
	if n != nil {
		s = strings.Join(documentLines(n), " ")
	}
	return strings.Join(strings.Fields(strings.NewReplacer("`", "", "**", "", "\u200b", "").Replace(s)), " ")
}

// Expand row/column spans before interpreting columns; shifted cells must never
// attach a neighboring model's limits or prices to the requested model.
func documentTables(data []byte) [][][]string {
	var tables [][][]string
	n, _ := html.Parse(strings.NewReader(string(data)))
	if n != nil {
		for _, t := range documentNodes(n, "table") {
			type span struct {
				text string
				left int
			}
			spans := map[int]span{}
			var rows [][]string
			for _, tr := range documentNodes(t, "tr") {
				cells := []*html.Node{}
				for c := tr.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
						cells = append(cells, c)
					}
				}
				if len(cells) == 0 {
					continue
				}
				row := []string{}
				col := 0
				carry := func() {
					for {
						v, ok := spans[col]
						if !ok {
							break
						}
						row = append(row, v.text)
						v.left--
						if v.left == 0 {
							delete(spans, col)
						} else {
							spans[col] = v
						}
						col++
					}
				}
				for _, cell := range cells {
					carry()
					cols, rowsCount := 1, 1
					for _, a := range cell.Attr {
						if a.Key == "colspan" {
							cols, _ = strconv.Atoi(a.Val)
						}
						if a.Key == "rowspan" {
							rowsCount, _ = strconv.Atoi(a.Val)
						}
					}
					if cols < 1 || cols > 100 || rowsCount < 1 || rowsCount > 100 {
						cols, rowsCount = 1, 1
					}
					text := cleanDocumentText(strings.Join(documentLines(cell), " "))
					for i := 0; i < cols; i++ {
						row = append(row, text)
						if rowsCount > 1 {
							spans[col] = span{text, rowsCount - 1}
						}
						col++
					}
				}
				carry()
				rows = append(rows, row)
			}
			if len(rows) > 1 {
				tables = append(tables, rows)
			}
		}
	}
	// Markdown sources can contain HTML tables as well as pipe tables.
	var rows [][]string
	flush := func() {
		if len(rows) > 1 {
			tables = append(tables, rows)
		}
		rows = nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			flush()
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		row := make([]string, len(parts))
		separator := true
		for i, p := range parts {
			row[i] = cleanDocumentText(p)
			if strings.Trim(row[i], " :-") != "" {
				separator = false
			}
		}
		if !separator {
			rows = append(rows, row)
		}
	}
	flush()
	return tables
}

var numericTokens = regexp.MustCompile(`(?i)^([0-9][0-9,]*(?:\.[0-9]+)?)\s*([km]?)\s*(?:tokens?)?$`)

func documentTokens(s string) int64 {
	m := numericTokens.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(m[2]) {
	case "k":
		v *= 1000
	case "m":
		v *= 1000000
	}
	if v <= 0 || v > 9007199254740991 || v != float64(int64(v)) {
		return 0
	}
	return int64(v)
}
func documentBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "supported", "yes", "true", "✓", "✅", "支持", "adaptive", "adaptive (always on)", "extended":
		return true, true
	case "not supported", "no", "false", "✗", "❌", "不支持":
		return false, true
	}
	return false, false
}
func documentModalities(s string) model.ModelModalities {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " and ", ",")
	var out model.ModelModalities
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		switch p {
		case "text", "image", "audio", "video":
			out = append(out, p)
		case "images":
			out = append(out, "image")
		case "pdf", "file", "files":
			out = append(out, "file")
		default:
			if p != "" {
				return nil
			}
		}
	}
	return out
}

var documentCapabilityKeys = map[string]string{
	"thinking": "reasoning", "thinking mode": "reasoning", "reasoning": "reasoning", "思考模式": "reasoning",
	"streaming": "streaming", "streaming messages": "streaming", "function calling": "function_calling", "tool calls": "function_calling",
	"structured outputs": "structured_output", "structured output": "structured_output", "json output": "structured_output", "结构化输出": "structured_output",
	"search grounding": "web_search", "web search": "web_search", "url context": "url_context", "code execution": "code_interpreter", "code interpreter": "code_interpreter",
	"computer use": "computer_use", "file search": "file_search", "memory": "memory", "citations": "citations", "caching": "prompt_caching", "context caching": "prompt_caching", "prompt caching": "prompt_caching",
	"background": "background", "server-side context": "server_side_context", "structured decision": "structured_decision",
}

func applyDocumentField(entry *model.ModelCapabilityMetadata, label, value string) {
	label = strings.ToLower(strings.TrimSpace(label))
	switch label {
	case "description", "描述", "模型介绍":
		if len(value) > 5 && value != "Deprecated" {
			entry.Description = value
		}
	case "context", "context window", "context length", "上下文", "上下文长度":
		if n := documentTokens(value); n > 0 {
			entry.ContextLength = n
		}
	case "length limit (token)":
		if m := regexp.MustCompile(`Context Window: ([0-9,.]+[KkMm]?) Maximum Output: ([0-9,.]+[KkMm]?)`).FindStringSubmatch(value); m != nil {
			entry.ContextLength = documentTokens(m[1])
			entry.MaxOutputTokens = documentTokens(m[2])
		}
	case "capability support":
		for text, key := range map[string]string{"Deep Thinking": "reasoning", "Streaming Output": "streaming", "Function Call": "function_calling", "Structured Output": "structured_output", "Web Search": "web_search"} {
			if strings.Contains(value, text) {
				if entry.Capabilities == nil {
					entry.Capabilities = model.ModelCapabilities{}
				}
				entry.Capabilities[key] = true
			}
		}
	case "max output", "max output tokens", "maximum output", "最大输出":
		if n := documentTokens(strings.TrimPrefix(value, "MAXIMUM: ")); n > 0 {
			entry.MaxOutputTokens = n
		}
	case "release date", "released", "发布日期":
		for _, layout := range []string{"2006-01-02", "January 2, 2006", "Jan 2, 2006"} {
			if d, e := time.Parse(layout, value); e == nil {
				entry.ReleaseDate = d.Format("2006-01-02")
				break
			}
		}
	case "modalities", "input → output":
		p := strings.Split(value, "→")
		if len(p) == 2 {
			entry.InputModalities = documentModalities(p[0])
			entry.OutputModalities = documentModalities(p[1])
		}
	case "input modalities", "input types":
		entry.InputModalities = documentModalities(value)
	case "output modalities", "output types":
		entry.OutputModalities = documentModalities(value)
	}
	if key := documentCapabilityKeys[label]; key != "" {
		if v, ok := documentBool(value); ok {
			if entry.Capabilities == nil {
				entry.Capabilities = model.ModelCapabilities{}
			}
			entry.Capabilities[key] = v
		}
	}
}
func exactDocumentID(cell, id string) bool {
	for _, v := range strings.FieldsFunc(cell, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-/", r))
	}) {
		if v == id {
			return true
		}
	}
	return false
}
func parseCatalogTables(data []byte, names map[string]bool, provider, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	for _, table := range documentTables(data) {
		header := table[0]
		if len(header) < 2 {
			continue
		}
		label := strings.ToLower(header[0])
		if label != "model" && label != "model name" && label != "model id" && label != "model id (model id)" && label != "模型 id" && label != "模型名称" {
			continue
		}
		idColumn := 0
		for i, name := range header {
			if name == "Endpoint" {
				idColumn = i
			}
		}
		for _, row := range table[1:] {
			if len(row) != len(header) {
				continue
			}
			for id := range names {
				if !exactDocumentID(row[idColumn], id) {
					continue
				}
				entry := out[id]
				entry.Provider = provider
				for col := 1; col < len(header); col++ {
					applyDocumentField(&entry, header[col], row[col])
				}
				out[id] = markOfficialAttributes(entry, address)
			}
		}
	}
	return out
}

func parseClaudeDocument(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	tables := documentTables(data)
	identity := false
	for _, table := range tables {
		for _, row := range table {
			if len(row) == 2 && (row[0] == "Claude API" || row[0] == "Claude API alias") && row[1] == id {
				identity = true
			}
		}
	}
	if !identity {
		return model.ModelCapabilityMetadata{}, errors.New("Claude API model identity missing")
	}
	entry := model.ModelCapabilityMetadata{Provider: "anthropic"}
	for _, table := range tables {
		for _, row := range table {
			if len(row) == 2 {
				applyDocumentField(&entry, row[0], row[1])
			}
		}
	}
	// The short summary precedes the explicit model ID, separate from page SEO
	// descriptions and comparison sections about other models.
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "Model ID:") && strings.Contains(line, "`"+id+"`") {
			for j := i - 1; j >= 0; j-- {
				v := strings.TrimSpace(lines[j])
				if v != "" {
					entry.Description = cleanDocumentText(v)
					break
				}
			}
			break
		}
	}
	// Each cache TTL gets a separate tier; ignore non-numeric/dynamic price cells.
	prices := map[string]*float64{}
	for _, table := range tables {
		if len(table) < 2 || len(table[0]) != 2 || table[0][0] != "Feature" {
			continue
		}
		for _, row := range table[1:] {
			if len(row) == 2 {
				if m := regexp.MustCompile(`^\$([0-9.]+) / MTok$`).FindStringSubmatch(row[1]); m != nil {
					v, e := strconv.ParseFloat(m[1], 64)
					if e == nil {
						prices[row[0]] = &v
					}
				}
			}
		}
	}
	if prices["Input"] != nil && prices["Output"] != nil {
		p := model.OfficialPricing{Currency: "USD", SourceURL: address}
		for _, ttl := range []string{"5m", "1h"} {
			if write := prices[ttl+" cache write"]; write != nil {
				p.Tiers = append(p.Tiers, model.OfficialPricingTier{Label: "Standard · cache " + ttl, Unit: "million_tokens", Input: prices["Input"], Output: prices["Output"], CacheRead: prices["Cache read"], CacheWrite: write})
			}
		}
		if len(p.Tiers) == 0 {
			p.Tiers = []model.OfficialPricingTier{{Label: "Standard", Unit: "million_tokens", Input: prices["Input"], Output: prices["Output"]}}
		}
		// Only complete numeric price cells reach this path; starting-price and
		// conditional cells cannot silently become a flat rate.
		entry.OfficialPricing = p
		if prices["Cache read"] != nil {
			if entry.Capabilities == nil {
				entry.Capabilities = model.ModelCapabilities{}
			}
			entry.Capabilities["prompt_caching"] = true
		}
	}
	return markOfficialAttributes(entry, address), nil
}

func parseGeminiDocument(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	entry := model.ModelCapabilityMetadata{Provider: "google", Capabilities: model.ModelCapabilities{}}
	found := false
	for _, table := range documentTables(data) {
		identity := false
		for _, row := range table {
			if len(row) == 2 && strings.Contains(row[0], "Model code") && row[1] == id {
				identity = true
			}
		}
		if !identity {
			continue
		}
		found = true
		for _, row := range table {
			if len(row) != 2 {
				continue
			}
			v := row[1]
			switch {
			case strings.Contains(row[0], "Token limits"):
				if m := regexp.MustCompile(`Input token limit ([0-9,]+) Output token limit ([0-9,]+)`).FindStringSubmatch(v); m != nil {
					entry.ContextLength = documentTokens(m[1])
					entry.MaxOutputTokens = documentTokens(m[2])
				}
			case strings.Contains(row[0], "Supported data types"):
				if m := regexp.MustCompile(`^Inputs? (.+) Outputs? (.+)$`).FindStringSubmatch(v); m != nil {
					entry.InputModalities = documentModalities(m[1])
					entry.OutputModalities = documentModalities(m[2])
				}
			case strings.Contains(row[0], "Capabilities"):
				for label, key := range documentCapabilityKeys {
					pattern := regexp.MustCompile(`(?i)(?:^| )` + regexp.QuoteMeta(label) + ` (Not supported|Supported)(?: |$)`)
					if m := pattern.FindStringSubmatch(v); m != nil {
						b, _ := documentBool(m[1])
						entry.Capabilities[key] = b
					}
				}
			}
		}
	}
	if !found {
		return entry, errors.New("Gemini model code not found")
	}
	return markOfficialAttributes(entry, address), nil
}

func parseXAIModelDocument(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	n, _ := html.Parse(strings.NewReader(string(data)))
	if n == nil {
		return model.ModelCapabilityMetadata{}, errors.New("invalid xAI document")
	}
	lines := documentLines(n)
	entry := model.ModelCapabilityMetadata{Provider: "xai", Capabilities: model.ModelCapabilities{}}
	identity := false
	section := ""
	for i, line := range lines {
		if i+1 >= len(lines) {
			continue
		}
		next := lines[i+1]
		if line == "Model name" && next == id {
			identity = true
		}
		if line == "Context window" {
			entry.ContextLength = documentTokens(next)
		}
		if line == id && i+1 < len(lines) && len(next) > 25 && !strings.Contains(next, "{") {
			entry.Description = next
		}
		if line == "Modalities" && i+2 < len(lines) {
			entry.InputModalities = documentModalities(next)
			entry.OutputModalities = documentModalities(lines[i+2])
		}
		if line == "Capabilities" || line == "Pricing" || line == "Details" {
			section = line
		}
		if section == "Capabilities" {
			if key := documentCapabilityKeys[strings.ToLower(line)]; key != "" {
				entry.Capabilities[key] = true
			}
		}
	}
	if !identity {
		return entry, errors.New("xAI model identity missing")
	}
	return markOfficialAttributes(entry, address), nil
}

func parseZAIModelDocument(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	text := string(data)
	entry := model.ModelCapabilityMetadata{Provider: "zai", Capabilities: model.ModelCapabilities{}}
	// The request example must use this exact API ID; navigation headings alone
	// are not identity evidence for a model-specific document.
	if !regexp.MustCompile(`"model"\s*:\s*"` + regexp.QuoteMeta(id) + `"`).MatchString(text) {
		return entry, errors.New("GLM model identity missing")
	}
	if m := regexp.MustCompile(`(?i)([0-9,.]+[KM]?)[- ]token context window`).FindStringSubmatch(text); m != nil {
		entry.ContextLength = documentTokens(m[1])
	}
	if m := regexp.MustCompile(`(?i)maximum output length of ([0-9,.]+[KM]?) tokens`).FindStringSubmatch(text); m != nil {
		entry.MaxOutputTokens = documentTokens(m[1])
	}
	if strings.Contains(text, "text-only inputs") {
		entry.InputModalities = model.ModelModalities{"text"}
	}
	if m := regexp.MustCompile(`(?s)## Overview\s+(.+?)\n\n`).FindStringSubmatch(text); m != nil {
		entry.Description = cleanDocumentText(m[1])
	}
	if m := regexp.MustCompile(`(?s)## Capability Support\s+(.+?)(?:\n## |$)`).FindStringSubmatch(text); m != nil {
		for label, key := range documentCapabilityKeys {
			if strings.Contains(strings.ToLower(m[1]), "["+label+"]") {
				entry.Capabilities[key] = true
			}
		}
	}
	return markOfficialAttributes(entry, address), nil
}

// Providers declare fixed official origins and parsers, not model versions.
// New versions automatically use the same adapters; unknown/private aliases
// fail identity checks and remain eligible for a later refresh.
func fetchPublicModelDocuments(ctx context.Context, names map[string]bool) (map[string]model.ModelCapabilityMetadata, []string) {
	type job struct {
		url, id, provider string
		parse             func([]byte, string, string) (model.ModelCapabilityMetadata, error)
	}
	var jobs []job
	groups := map[string]map[string]bool{}
	for id := range names {
		if !officialModelID.MatchString(id) {
			continue
		}
		provider := ""
		switch {
		case strings.HasPrefix(id, "claude-"):
			provider = "anthropic"
			slug := strings.TrimPrefix(id, "claude-")
			slug = regexp.MustCompile(`-\d{8}$`).ReplaceAllString(slug, "")
			jobs = append(jobs, job{"https://platform.claude.com/docs/en/models/" + slug + "/overview", id, provider, parseClaudeDocument})
		case strings.HasPrefix(id, "gemini-"):
			provider = "google"
			jobs = append(jobs, job{"https://ai.google.dev/gemini-api/docs/models/" + id, id, provider, parseGeminiDocument})
		case strings.HasPrefix(id, "grok-"):
			provider = "xai"
			jobs = append(jobs, job{"https://docs.x.ai/developers/models/" + id, id, provider, parseXAIModelDocument})
		case strings.HasPrefix(id, "glm-"):
			provider = "zai"
			jobs = append(jobs, job{"https://docs.z.ai/guides/llm/" + id + ".md", id, provider, parseZAIModelDocument})
		case strings.HasPrefix(id, "deepseek-"):
			provider = "deepseek"
		case strings.HasPrefix(id, "mimo-"):
			provider = "xiaomi"
		case strings.HasPrefix(id, "kimi-"):
			provider = "moonshotai"
		case strings.HasPrefix(strings.ToLower(id), "minimax-"):
			provider = "minimax"
		case strings.HasPrefix(id, "qwen"):
			provider = "alibaba"
		}
		if provider != "" {
			if groups[provider] == nil {
				groups[provider] = map[string]bool{}
			}
			groups[provider][id] = true
		}
	}
	for _, source := range []struct{ provider, url string }{
		{"google", "https://ai.google.dev/gemini-api/docs/models"},
		{"google", "https://ai.google.dev/gemini-api/docs/pricing"},
		{"xiaomi", "https://mimo.mi.com/static/docs/price/pay-as-you-go.md"},
		{"zai", "https://docs.z.ai/guides/overview/pricing.md"},
		{"deepseek", "https://api-docs.deepseek.com/quick_start/pricing/"},
		{"xiaomi", "https://mimo.mi.com/static/docs/quick-start/summary/model.md"},
		{"moonshotai", "https://platform.kimi.ai/docs/models.md"},
		{"moonshotai", kimiPricingURL},
		{"minimax", "https://platform.minimax.io/docs/guides/text-generation.md"},
		{"alibaba", "https://help.aliyun.com/zh/model-studio/text-generation-model"},
		{"alibaba", "https://help.aliyun.com/zh/model-studio/vision-model"},
		{"alibaba", "https://help.aliyun.com/zh/model-studio/image-model"},
	} {
		if len(groups[source.provider]) > 0 {
			jobs = append(jobs, job{url: source.url, provider: source.provider})
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].url < jobs[j].url })
	type result struct {
		entries map[string]model.ModelCapabilityMetadata
		warning string
	}
	results := make([]result, len(jobs))
	queue := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range queue {
				j := jobs[index]
				data, err := fetchPublicModelDocument(ctx, j.url)
				if err != nil {
					results[index].warning = j.provider + ": " + j.id
					continue
				}
				if j.parse != nil {
					entry, err := j.parse(data, j.id, j.url)
					if err != nil {
						results[index].warning = j.provider + ": " + j.id
						continue
					}
					results[index].entries = map[string]model.ModelCapabilityMetadata{j.id: entry}
				} else {
					entries := parseCatalogTables(data, groups[j.provider], j.provider, j.url)
					if j.provider == "deepseek" {
						entries = parseDeepSeekDocument(data, groups[j.provider], j.url)
						addDeepSeekPrices(data, entries, j.url)
					}
					switch j.url {
					case kimiPricingURL:
						entries, err = parseKimiPricing(data, groups[j.provider])
						if err != nil {
							results[index].warning = "moonshotai pricing"
							continue
						}
					case "https://ai.google.dev/gemini-api/docs/pricing":
						entries = parseGeminiPricing(data, groups[j.provider], j.url)
					case "https://mimo.mi.com/static/docs/price/pay-as-you-go.md":
						entries = parseMiMoPricing(data, groups[j.provider], j.url)
					case "https://docs.z.ai/guides/overview/pricing.md":
						entries = parseZAIPricing(data, groups[j.provider], j.url)
					}
					results[index].entries = entries
				}
			}
		}()
	}
	for i := range jobs {
		queue <- i
	}
	close(queue)
	wg.Wait()
	out := map[string]model.ModelCapabilityMetadata{}
	var warnings []string
	for _, r := range results {
		if r.warning != "" {
			warnings = append(warnings, r.warning)
		}
		for id, entry := range r.entries {
			out[id] = mergeOfficialAttributes(out[id], entry)
		}
	}
	return out, warnings
}

func parseDeepSeekDocument(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	out := map[string]model.ModelCapabilityMetadata{}
	for _, table := range documentTables(data) {
		if len(table) < 2 || len(table[0]) < 2 || table[0][0] != "MODEL" {
			continue
		}
		header := table[0]
		firstModel := 1
		for firstModel < len(header) && header[firstModel] == "MODEL" {
			firstModel++
		}
		for col := firstModel; col < len(header); col++ {
			for id := range names {
				if !exactDocumentID(header[col], id) {
					continue
				}
				entry := model.ModelCapabilityMetadata{Provider: "deepseek", Capabilities: model.ModelCapabilities{}}
				for _, row := range table[1:] {
					if len(row) != len(header) {
						continue
					}
					label := row[0]
					for k := 1; k < firstModel; k++ {
						if row[k] != label {
							label = row[k]
						}
					}
					applyDocumentField(&entry, label, row[col])
				}
				out[id] = markOfficialAttributes(entry, address)
			}
		}
	}
	return out
}
