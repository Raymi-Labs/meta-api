package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

// Public docs and dedicated metadata credentials only. Never send relay keys.
func fetchOfficialAttributeData(ctx context.Context, address string, headers map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := *getHTTPClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("official metadata request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("official metadata returned HTTP %d", resp.StatusCode)
	}
	const limit = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, errors.New("official metadata exceeds size limit")
	}
	return body, nil
}

var openAIReasoningModelID = regexp.MustCompile(`^o[1-9]($|-)`)
var officialCachePrice = regexp.MustCompile(`^\$[0-9]+(?:\.[0-9]+)?$`)

var officialModelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
var officialTokenLimit = regexp.MustCompile(`^([0-9][0-9,]*) (context window|max output tokens)$`)

// Parse visible model body, not navigation or script text. Require the page's
// model identity; a redirect, error document or changed schema cannot supply data.
func parseOpenAIModelAttributes(data []byte, id string) (model.ModelCapabilityMetadata, error) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return model.ModelCapabilityMetadata{}, err
	}
	var lines []string
	identity := false
	var visit func(*html.Node, bool)
	visit = func(n *html.Node, main bool) {
		if n.Type == html.ElementNode {
			if n.Data == "script" || n.Data == "style" || n.Data == "svg" {
				return
			}
			if n.Data == "main" {
				main = true
			}
			if main && n.Data == "astro-island" {
				attrs := map[string]string{}
				for _, a := range n.Attr {
					attrs[a.Key] = a.Val
				}
				if attrs["component-export"] == "ModelView" {
					var props struct {
						ModelName []interface{} `json:"modelName"`
					}
					if common.UnmarshalJsonStr(attrs["props"], &props) == nil && len(props.ModelName) == 2 && props.ModelName[1] == id {
						identity = true
					}
				}
			}
		}
		if main && n.Type == html.TextNode {
			if s := strings.TrimSpace(n.Data); s != "" {
				lines = append(lines, s)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, main)
		}
	}
	visit(doc, false)
	if !identity {
		return model.ModelCapabilityMetadata{}, errors.New("official model identity not found")
	}
	entry := model.ModelCapabilityMetadata{Provider: "openai", Capabilities: model.ModelCapabilities{}}
	section := ""
	features := map[string]string{"Streaming": "streaming", "Function calling": "function_calling", "Structured outputs": "structured_output"}
	tools := map[string]string{"Web search": "web_search", "File search": "file_search", "Code interpreter": "code_interpreter", "Computer use": "computer_use"}
	modalities := map[string]string{"Text": "text", "Image": "image", "Audio": "audio", "Video": "video"}
	for i, line := range lines {
		if match := officialTokenLimit.FindStringSubmatch(line); match != nil {
			v, _ := strconv.ParseInt(strings.ReplaceAll(match[1], ",", ""), 10, 64)
			if v > 0 && v <= 9007199254740991 {
				if match[2] == "context window" {
					entry.ContextLength = v
				} else {
					entry.MaxOutputTokens = v
				}
			}
		}
		if line == "Reasoning token support" {
			entry.Capabilities["reasoning"] = true
		}
		switch line {
		case "Pricing", "Modalities", "Endpoints", "Features", "Tools", "Snapshots", "Rate limits":
			section = line
		}
		if i+1 >= len(lines) {
			continue
		}
		next := lines[i+1]
		if section == "Pricing" && line == "Cached input" && officialCachePrice.MatchString(next) {
			entry.Capabilities["prompt_caching"] = true
		}
		if section == "Modalities" {
			if modality, ok := modalities[line]; ok {
				switch next {
				case "Input and output":
					entry.InputModalities = append(entry.InputModalities, modality)
					entry.OutputModalities = append(entry.OutputModalities, modality)
				case "Input only":
					entry.InputModalities = append(entry.InputModalities, modality)
				case "Output only":
					entry.OutputModalities = append(entry.OutputModalities, modality)
				}
			}
		}
		key := ""
		if section == "Features" {
			key = features[line]
		}
		if section == "Tools" {
			key = tools[line]
		}
		if key != "" && (next == "Supported" || next == "Not supported") {
			entry.Capabilities[key] = next == "Supported"
		}
	}
	if entry.ContextLength == 0 && entry.MaxOutputTokens == 0 && len(entry.InputModalities) == 0 && len(entry.Capabilities) == 0 {
		return entry, errors.New("official model attributes not found")
	}
	return entry, nil
}

func markOfficialAttributes(entry model.ModelCapabilityMetadata, address string) model.ModelCapabilityMetadata {
	if entry.ContextLength < 0 || entry.ContextLength > 9007199254740991 {
		entry.ContextLength = 0
	}
	if entry.MaxOutputTokens < 0 || entry.MaxOutputTokens > 9007199254740991 {
		entry.MaxOutputTokens = 0
	}
	entry.FieldSources = map[string]model.ModelAttributeSource{}
	source := model.ModelAttributeSource{Kind: "official", URL: address}
	if entry.ContextLength > 0 {
		entry.FieldSources["context_length"] = source
	}
	if entry.MaxOutputTokens > 0 {
		entry.FieldSources["max_output_tokens"] = source
	}
	if len(entry.InputModalities) > 0 {
		entry.FieldSources["input_modalities"] = source
	}
	if len(entry.OutputModalities) > 0 {
		entry.FieldSources["output_modalities"] = source
	}
	if entry.ReleaseDate != "" {
		entry.FieldSources["release_date"] = source
	}
	for key := range entry.Capabilities {
		entry.FieldSources["capabilities."+key] = source
	}
	return entry
}

func mergeOfficialAttributes(base, official model.ModelCapabilityMetadata) model.ModelCapabilityMetadata {
	if official.ContextLength > 0 {
		base.ContextLength = official.ContextLength
	}
	if official.MaxOutputTokens > 0 {
		base.MaxOutputTokens = official.MaxOutputTokens
	}
	if len(official.InputModalities) > 0 {
		base.InputModalities = official.InputModalities
	}
	if len(official.OutputModalities) > 0 {
		base.OutputModalities = official.OutputModalities
	}
	if official.ReleaseDate != "" {
		base.ReleaseDate = official.ReleaseDate
	}
	if base.Capabilities == nil {
		base.Capabilities = model.ModelCapabilities{}
	}
	for key, value := range official.Capabilities {
		base.Capabilities[key] = value
	}
	if base.FieldSources == nil {
		base.FieldSources = map[string]model.ModelAttributeSource{}
	}
	for key, value := range official.FieldSources {
		base.FieldSources[key] = value
	}
	if base.Provider == "" {
		base.Provider = official.Provider
	}
	return base
}

func fetchClaudeAttributes(ctx context.Context, key string) (map[string]model.ModelCapabilityMetadata, error) {
	entries := map[string]model.ModelCapabilityMetadata{}
	cursor := ""
	const endpoint = "https://api.anthropic.com/v1/models"
	for page := 0; page < 20; page++ {
		address := endpoint + "?limit=1000"
		if cursor != "" {
			address += "&after_id=" + url.QueryEscape(cursor)
		}
		data, err := fetchOfficialAttributeData(ctx, address, map[string]string{"Authorization": "Bearer " + key, "anthropic-version": "2026-10-21"})
		if err != nil {
			return nil, err
		}
		var response struct {
			Data []struct {
				ID           string `json:"id"`
				MaxInput     int64  `json:"max_input_tokens"`
				MaxOutput    int64  `json:"max_tokens"`
				Capabilities map[string]struct {
					Supported *bool `json:"supported"`
					WebSearch *struct {
						Supported *bool `json:"supported"`
					} `json:"web_search"`
				} `json:"capabilities"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := common.Unmarshal(data, &response); err != nil {
			return nil, err
		}
		if response.Data == nil {
			return nil, errors.New("official model list is missing data")
		}
		for _, m := range response.Data {
			if !officialModelID.MatchString(m.ID) {
				continue
			}
			entry := model.ModelCapabilityMetadata{Provider: "anthropic", ContextLength: m.MaxInput, MaxOutputTokens: m.MaxOutput, Capabilities: model.ModelCapabilities{}}
			for remote, local := range map[string]string{"thinking": "reasoning", "citations": "citations", "code_execution": "code_interpreter", "structured_outputs": "structured_output"} {
				if v := m.Capabilities[remote].Supported; v != nil {
					entry.Capabilities[local] = *v
				}
			}
			if v := m.Capabilities["server_tools"].WebSearch; v != nil && v.Supported != nil {
				entry.Capabilities["web_search"] = *v.Supported
			}
			// This API does not enumerate all modalities. Do not label a partial list as complete.
			entries[m.ID] = markOfficialAttributes(entry, "https://platform.claude.com/docs/en/api/models/list")
		}
		if !response.HasMore {
			return entries, nil
		}
		if response.LastID == "" || response.LastID == cursor {
			return nil, errors.New("invalid official pagination")
		}
		cursor = response.LastID
	}
	return nil, errors.New("official pagination limit exceeded")
}

func fetchGeminiAttributes(ctx context.Context, key string) (map[string]model.ModelCapabilityMetadata, error) {
	entries := map[string]model.ModelCapabilityMetadata{}
	cursor := ""
	for page := 0; page < 20; page++ {
		address := "https://generativelanguage.googleapis.com/v1beta/models?pageSize=1000"
		if cursor != "" {
			address += "&pageToken=" + url.QueryEscape(cursor)
		}
		data, err := fetchOfficialAttributeData(ctx, address, map[string]string{"x-goog-api-key": key})
		if err != nil {
			return nil, err
		}
		var response struct {
			Models []struct {
				Name     string `json:"name"`
				Input    int64  `json:"inputTokenLimit"`
				Output   int64  `json:"outputTokenLimit"`
				Thinking *bool  `json:"thinking"`
			} `json:"models"`
			Next string `json:"nextPageToken"`
		}
		if err := common.Unmarshal(data, &response); err != nil {
			return nil, err
		}
		if response.Models == nil {
			return nil, errors.New("official model list is missing models")
		}
		for _, m := range response.Models {
			if !strings.HasPrefix(m.Name, "models/") {
				continue
			}
			id := strings.TrimPrefix(m.Name, "models/")
			if !officialModelID.MatchString(id) {
				continue
			}
			entry := model.ModelCapabilityMetadata{Provider: "google", ContextLength: m.Input, MaxOutputTokens: m.Output, Capabilities: model.ModelCapabilities{}}
			if m.Thinking != nil {
				entry.Capabilities["reasoning"] = *m.Thinking
			}
			entries[id] = markOfficialAttributes(entry, "https://ai.google.dev/api/models")
		}
		if response.Next == "" {
			return entries, nil
		}
		if response.Next == cursor {
			return nil, errors.New("invalid official pagination")
		}
		cursor = response.Next
	}
	return nil, errors.New("official pagination limit exceeded")
}

// Fetch only locally configured/enabled IDs. Four workers bound public docs load.
// Each source failure is independent of the community source and other providers.
func fetchOfficialModelAttributes(ctx context.Context) (map[string]model.ModelCapabilityMetadata, []string, error) {
	names, err := model.ModelAttributeSyncCandidates(ctx)
	if err != nil {
		return nil, nil, err
	}
	entries := map[string]model.ModelCapabilityMetadata{}
	var warnings []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	merge := func(values map[string]model.ModelCapabilityMetadata) {
		mu.Lock()
		defer mu.Unlock()
		for id, value := range values {
			if names[id] {
				entries[id] = value
			}
		}
	}
	warn := func(value string) { mu.Lock(); defer mu.Unlock(); warnings = append(warnings, value) }
	for _, provider := range []struct {
		env, name string
		fetch     func(context.Context, string) (map[string]model.ModelCapabilityMetadata, error)
	}{
		{"MODEL_METADATA_ANTHROPIC_API_KEY", "Claude", fetchClaudeAttributes}, {"MODEL_METADATA_GEMINI_API_KEY", "Gemini", fetchGeminiAttributes},
	} {
		if key := strings.TrimSpace(os.Getenv(provider.env)); key != "" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				values, err := provider.fetch(ctx, key)
				if err != nil {
					warn(provider.name + " official metadata unavailable")
					return
				}
				merge(values)
			}()
		}
	}
	ids := make([]string, 0)
	for id := range names {
		if officialModelID.MatchString(id) && (strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "chatgpt-") || openAIReasoningModelID.MatchString(id)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	jobs := make(chan string)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				address := "https://developers.openai.com/api/docs/models/" + id
				data, err := fetchOfficialAttributeData(ctx, address, nil)
				if err != nil {
					warn("OpenAI: " + id)
					continue
				}
				entry, err := parseOpenAIModelAttributes(data, id)
				if err != nil {
					warn("OpenAI: " + id)
					continue
				}
				merge(map[string]model.ModelCapabilityMetadata{id: markOfficialAttributes(entry, address)})
			}
		}()
	}
	for _, id := range ids {
		select {
		case jobs <- id:
		case <-ctx.Done():
			warn("OpenAI official metadata timed out")
			close(jobs)
			wg.Wait()
			return entries, warnings, nil
		}
	}
	close(jobs)
	wg.Wait()
	sort.Strings(warnings)
	return entries, warnings, nil
}
