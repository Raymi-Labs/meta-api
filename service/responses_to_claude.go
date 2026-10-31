package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func ResponseOpenAIResponses2Claude(resp *dto.OpenAIResponsesResponse, id string) *dto.ClaudeResponse {
	if resp == nil {
		return nil
	}

	claudeID := strings.TrimSpace(id)
	if claudeID == "" {
		claudeID = strings.TrimSpace(resp.ID)
	}

	contents := buildClaudeContentsFromResponses(resp)
	stopReason := stopReasonOpenAI2Claude("stop")
	for _, content := range contents {
		if content.Type == "tool_use" {
			stopReason = "tool_use"
			break
		}
	}
	if stopReason == "null" || stopReason == "" {
		stopReason = "end_turn"
	}

	return &dto.ClaudeResponse{
		Id:         claudeID,
		Type:       "message",
		Role:       "assistant",
		Model:      resp.Model,
		Content:    contents,
		StopReason: stopReason,
		Usage:      buildClaudeUsageFromOpenAIUsage(buildOpenAIUsageFromResponses(resp)),
	}
}

func StreamClaudeResponse(resp *dto.ClaudeResponse) []*dto.ClaudeResponse {
	if resp == nil {
		return nil
	}

	msg := &dto.ClaudeMediaMessage{
		Id:    resp.Id,
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
		Usage: resp.Usage,
	}
	msg.SetContent(make([]any, 0))

	events := make([]*dto.ClaudeResponse, 0, len(resp.Content)*3+3)
	events = append(events, &dto.ClaudeResponse{
		Type:    "message_start",
		Message: msg,
	})

	for idx := range resp.Content {
		block := resp.Content[idx]
		blockIndex := idx
		switch block.Type {
		case "thinking":
			events = append(events, &dto.ClaudeResponse{
				Type:  "content_block_start",
				Index: &blockIndex,
				ContentBlock: &dto.ClaudeMediaMessage{
					Type:     "thinking",
					Thinking: common.GetPointer(""),
				},
			})
			if block.Thinking != nil && *block.Thinking != "" {
				events = append(events, &dto.ClaudeResponse{
					Type:  "content_block_delta",
					Index: &blockIndex,
					Delta: &dto.ClaudeMediaMessage{
						Type:     "thinking_delta",
						Thinking: block.Thinking,
					},
				})
			}
		case "text":
			events = append(events, &dto.ClaudeResponse{
				Type:  "content_block_start",
				Index: &blockIndex,
				ContentBlock: &dto.ClaudeMediaMessage{
					Type:      "text",
					Text:      common.GetPointer(""),
					Citations: block.Citations,
				},
			})
			if block.Text != nil && *block.Text != "" {
				events = append(events, &dto.ClaudeResponse{
					Type:  "content_block_delta",
					Index: &blockIndex,
					Delta: &dto.ClaudeMediaMessage{
						Type: "text_delta",
						Text: block.Text,
					},
				})
			}
		default:
			blockCopy := block
			events = append(events, &dto.ClaudeResponse{
				Type:         "content_block_start",
				Index:        &blockIndex,
				ContentBlock: &blockCopy,
			})
		}
		events = append(events, &dto.ClaudeResponse{
			Type:  "content_block_stop",
			Index: &blockIndex,
		})
	}

	stopReason := resp.StopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}
	events = append(events, &dto.ClaudeResponse{
		Type:  "message_delta",
		Usage: resp.Usage,
		Delta: &dto.ClaudeMediaMessage{
			StopReason: &stopReason,
		},
	})
	events = append(events, &dto.ClaudeResponse{Type: "message_stop"})
	return events
}

func buildOpenAIUsageFromResponses(resp *dto.OpenAIResponsesResponse) *dto.Usage {
	if resp == nil {
		return nil
	}

	usage := &dto.Usage{}
	if resp.Usage != nil {
		if resp.Usage.InputTokens != 0 {
			usage.PromptTokens = resp.Usage.InputTokens
			usage.InputTokens = resp.Usage.InputTokens
		}
		if resp.Usage.OutputTokens != 0 {
			usage.CompletionTokens = resp.Usage.OutputTokens
			usage.OutputTokens = resp.Usage.OutputTokens
		}
		if resp.Usage.TotalTokens != 0 {
			usage.TotalTokens = resp.Usage.TotalTokens
		} else {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if resp.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.ImageTokens = resp.Usage.InputTokensDetails.ImageTokens
			usage.PromptTokensDetails.AudioTokens = resp.Usage.InputTokensDetails.AudioTokens
		}
		if resp.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
			usage.CompletionTokenDetails.ReasoningTokens = resp.Usage.CompletionTokenDetails.ReasoningTokens
		}
	}
	for _, out := range resp.Output {
		if out.Type == dto.BuildInCallWebSearchCall {
			usage.WebSearchRequests++
		}
	}
	return usage
}

func buildClaudeContentsFromResponses(resp *dto.OpenAIResponsesResponse) []dto.ClaudeMediaMessage {
	if resp == nil {
		return nil
	}

	fallbackSources := buildFallbackSourcesFromAnnotations(resp)
	webSearchCalls := countWebSearchCalls(resp)
	contents := make([]dto.ClaudeMediaMessage, 0, len(resp.Output)+2)

	for outputIndex := range resp.Output {
		out := resp.Output[outputIndex]
		switch out.Type {
		case "reasoning":
			reasoning := strings.TrimSpace(joinReasoningSummary(out.Summary))
			if reasoning == "" {
				continue
			}
			contents = append(contents, dto.ClaudeMediaMessage{
				Type:     "thinking",
				Thinking: &reasoning,
			})
		case dto.BuildInCallWebSearchCall:
			toolUseID := buildClaudeWebSearchToolUseID(out, outputIndex)
			contents = append(contents, dto.ClaudeMediaMessage{
				Type:  "server_tool_use",
				Id:    toolUseID,
				Name:  "web_search",
				Input: buildClaudeWebSearchInput(out),
			})

			results := buildClaudeWebSearchResultsFromAction(out)
			if len(results) == 0 && webSearchCalls == 1 {
				results = fallbackSources
			}

			resultBlock := dto.ClaudeMediaMessage{
				Type:      "web_search_tool_result",
				ToolUseId: toolUseID,
			}
			if len(results) > 0 {
				resultBlock.Content = results
			} else {
				resultBlock.ErrorCode = "unavailable"
			}
			contents = append(contents, resultBlock)
		case "function_call":
			arguments := out.ArgumentsString()
			var input any = map[string]any{}
			if arguments != "" {
				if err := common.DecodeJsonUseNumberStrict(strings.NewReader(arguments), &input); err != nil {
					input = arguments
				}
			}
			toolUseID := strings.TrimSpace(out.CallId)
			if toolUseID == "" {
				toolUseID = strings.TrimSpace(out.ID)
			}
			contents = append(contents, dto.ClaudeMediaMessage{
				Type:  "tool_use",
				Id:    toolUseID,
				Name:  out.Name,
				Input: SanitizeClaudeToolInput(out.Name, input),
			})
		case "message":
			if out.Role != "" && out.Role != "assistant" {
				continue
			}
			for contentIndex := range out.Content {
				blocks := buildClaudeTextBlocksFromResponsesContent(out.Content[contentIndex])
				contents = append(contents, blocks...)
			}
		}
	}

	return contents
}

func countWebSearchCalls(resp *dto.OpenAIResponsesResponse) int {
	if resp == nil {
		return 0
	}
	count := 0
	for _, out := range resp.Output {
		if out.Type == dto.BuildInCallWebSearchCall {
			count++
		}
	}
	return count
}

func joinReasoningSummary(summary []dto.ResponsesReasoningSummaryPart) string {
	if len(summary) == 0 {
		return ""
	}
	parts := make([]string, 0, len(summary))
	for _, item := range summary {
		text := strings.TrimSpace(item.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildClaudeWebSearchToolUseID(out dto.ResponsesOutput, outputIndex int) string {
	callID := strings.TrimSpace(out.CallId)
	if callID != "" {
		return "servertoolu_" + callID
	}
	itemID := strings.TrimSpace(out.ID)
	if itemID != "" {
		return "servertoolu_" + itemID
	}
	return fmt.Sprintf("servertoolu_ws_%d", outputIndex+1)
}

func buildClaudeWebSearchInput(out dto.ResponsesOutput) map[string]any {
	input := map[string]any{}
	action := parseResponsesWebSearchAction(out)
	if action == nil {
		return input
	}
	if action.Query != "" {
		input["query"] = action.Query
	}
	if len(action.Queries) > 0 {
		input["queries"] = action.Queries
	}
	return input
}

func buildClaudeWebSearchResultsFromAction(out dto.ResponsesOutput) []dto.ClaudeWebSearchResult {
	action := parseResponsesWebSearchAction(out)
	if action == nil || len(action.Sources) == 0 {
		return nil
	}
	results := make([]dto.ClaudeWebSearchResult, 0, len(action.Sources))
	for _, source := range action.Sources {
		url := strings.TrimSpace(source.URL)
		title := strings.TrimSpace(source.Title)
		text := strings.TrimSpace(source.Text)
		if text == "" {
			text = strings.TrimSpace(source.Snippet)
		}
		if url == "" && title == "" && text == "" {
			continue
		}
		results = append(results, dto.ClaudeWebSearchResult{
			Type:             "web_search_result",
			URL:              url,
			Title:            title,
			Text:             text,
			PageAge:          strings.TrimSpace(source.PageAge),
			EncryptedIndex:   strings.TrimSpace(source.EncryptedIndex),
			EncryptedContent: strings.TrimSpace(source.EncryptedContent),
		})
	}
	return results
}

func buildFallbackSourcesFromAnnotations(resp *dto.OpenAIResponsesResponse) []dto.ClaudeWebSearchResult {
	if resp == nil {
		return nil
	}
	results := make([]dto.ClaudeWebSearchResult, 0)
	seen := make(map[string]struct{})
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		for _, content := range out.Content {
			for _, annotation := range normalizeResponsesOutputAnnotations(content.Annotations) {
				citation, ok := responsesAnnotationToCitation(annotation, content.Text)
				if !ok {
					continue
				}
				key := citation.URL + "\n" + citation.Title
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				results = append(results, dto.ClaudeWebSearchResult{
					Type:             "web_search_result",
					URL:              citation.URL,
					Title:            citation.Title,
					Text:             citation.CitedText,
					EncryptedIndex:   citation.EncryptedIndex,
					EncryptedContent: citation.EncryptedContent,
				})
			}
		}
	}
	return results
}

func buildClaudeTextBlocksFromResponsesContent(content dto.ResponsesOutputContent) []dto.ClaudeMediaMessage {
	if content.Type != "output_text" {
		text := strings.TrimSpace(content.Text)
		if text == "" {
			return nil
		}
		return []dto.ClaudeMediaMessage{{
			Type: "text",
			Text: &text,
		}}
	}

	text := content.Text
	if text == "" {
		return nil
	}

	annotations := normalizeResponsesOutputAnnotations(content.Annotations)
	if len(annotations) == 0 {
		return []dto.ClaudeMediaMessage{{
			Type: "text",
			Text: &text,
		}}
	}

	sort.Slice(annotations, func(i, j int) bool {
		if annotations[i].StartIndex == annotations[j].StartIndex {
			return annotations[i].EndIndex < annotations[j].EndIndex
		}
		return annotations[i].StartIndex < annotations[j].StartIndex
	})

	blocks := make([]dto.ClaudeMediaMessage, 0, len(annotations)*2+1)
	cursor := 0
	for _, annotation := range annotations {
		citation, ok := responsesAnnotationToCitation(annotation, text)
		if !ok {
			return []dto.ClaudeMediaMessage{{
				Type: "text",
				Text: &text,
			}}
		}
		if citation.StartIndex < cursor || citation.EndIndex > textRuneLen(text) {
			return []dto.ClaudeMediaMessage{{
				Type: "text",
				Text: &text,
			}}
		}
		if citation.StartIndex > cursor {
			plainText, ok := sliceTextByCharacterRange(text, cursor, citation.StartIndex)
			if !ok {
				return []dto.ClaudeMediaMessage{{
					Type: "text",
					Text: &text,
				}}
			}
			if plainText != "" {
				plain := plainText
				blocks = append(blocks, dto.ClaudeMediaMessage{
					Type: "text",
					Text: &plain,
				})
			}
		}

		citedText, ok := sliceTextByCharacterRange(text, citation.StartIndex, citation.EndIndex)
		if !ok {
			return []dto.ClaudeMediaMessage{{
				Type: "text",
				Text: &text,
			}}
		}
		if citedText != "" {
			citation.CitedText = citedText
			cited := citedText
			blocks = append(blocks, dto.ClaudeMediaMessage{
				Type:      "text",
				Text:      &cited,
				Citations: []dto.ClaudeTextCitation{citation.toClaude()},
			})
		}
		cursor = citation.EndIndex
	}

	if cursor < textRuneLen(text) {
		tail, ok := sliceTextByCharacterRange(text, cursor, textRuneLen(text))
		if !ok {
			return []dto.ClaudeMediaMessage{{
				Type: "text",
				Text: &text,
			}}
		}
		if tail != "" {
			blocks = append(blocks, dto.ClaudeMediaMessage{
				Type: "text",
				Text: &tail,
			})
		}
	}

	if len(blocks) == 0 {
		return []dto.ClaudeMediaMessage{{
			Type: "text",
			Text: &text,
		}}
	}
	return blocks
}

func parseResponsesWebSearchAction(out dto.ResponsesOutput) *dto.ResponsesWebSearchAction {
	if len(out.Action) == 0 {
		return nil
	}

	var action dto.ResponsesWebSearchAction
	if err := common.Unmarshal(out.Action, &action); err == nil {
		action.Query = strings.TrimSpace(action.Query)
		action.Queries = normalizeQueries(action.Queries)
		return &action
	}

	var raw map[string]any
	if err := common.Unmarshal(out.Action, &raw); err != nil {
		return nil
	}

	action.Query = strings.TrimSpace(common.Interface2String(raw["query"]))
	action.Queries = normalizeQueries(interfaceSliceToStrings(raw["queries"]))
	if len(action.Queries) == 0 && action.Query != "" {
		action.Queries = []string{action.Query}
	}
	if action.Query == "" && len(action.Queries) == 1 {
		action.Query = action.Queries[0]
	}
	if sources, ok := raw["sources"].([]any); ok {
		action.Sources = normalizeWebSearchSources(sources)
	}
	return &action
}

func normalizeQueries(queries []string) []string {
	if len(queries) == 0 {
		return nil
	}
	out := make([]string, 0, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query != "" {
			out = append(out, query)
		}
	}
	return out
}

func interfaceSliceToStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(common.Interface2String(item))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func normalizeWebSearchSources(items []any) []dto.ResponsesWebSearchSource {
	out := make([]dto.ResponsesWebSearchSource, 0, len(items))
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		source := dto.ResponsesWebSearchSource{
			Type:             strings.TrimSpace(common.Interface2String(itemMap["type"])),
			URL:              strings.TrimSpace(common.Interface2String(itemMap["url"])),
			Title:            strings.TrimSpace(common.Interface2String(itemMap["title"])),
			Text:             strings.TrimSpace(common.Interface2String(itemMap["text"])),
			Snippet:          strings.TrimSpace(common.Interface2String(itemMap["snippet"])),
			PageAge:          strings.TrimSpace(common.Interface2String(itemMap["page_age"])),
			EncryptedIndex:   strings.TrimSpace(common.Interface2String(itemMap["encrypted_index"])),
			EncryptedContent: strings.TrimSpace(common.Interface2String(itemMap["encrypted_content"])),
		}
		if source.URL == "" && source.Title == "" && source.Text == "" && source.Snippet == "" {
			continue
		}
		out = append(out, source)
	}
	return out
}

type normalizedResponsesCitation struct {
	URL              string
	Title            string
	StartIndex       int
	EndIndex         int
	CitedText        string
	EncryptedIndex   string
	EncryptedContent string
}

func (c normalizedResponsesCitation) toClaude() dto.ClaudeTextCitation {
	return dto.ClaudeTextCitation{
		Type:             "web_search_result_location",
		URL:              c.URL,
		Title:            c.Title,
		CitedText:        c.CitedText,
		EncryptedIndex:   c.EncryptedIndex,
		EncryptedContent: c.EncryptedContent,
	}
}

func normalizeResponsesOutputAnnotations(annotations []dto.ResponsesOutputAnnotation) []dto.ResponsesOutputAnnotation {
	out := make([]dto.ResponsesOutputAnnotation, 0, len(annotations))
	for _, annotation := range annotations {
		annotation.Type = strings.TrimSpace(annotation.Type)
		if annotation.URLCitation != nil {
			if annotation.URL == "" {
				annotation.URL = annotation.URLCitation.URL
			}
			if annotation.Title == "" {
				annotation.Title = annotation.URLCitation.Title
			}
			if annotation.StartIndex == 0 && annotation.URLCitation.StartIndex > 0 {
				annotation.StartIndex = annotation.URLCitation.StartIndex
			}
			if annotation.EndIndex == 0 && annotation.URLCitation.EndIndex > 0 {
				annotation.EndIndex = annotation.URLCitation.EndIndex
			}
			if annotation.Text == "" {
				annotation.Text = annotation.URLCitation.Text
			}
		}
		out = append(out, annotation)
	}
	return out
}

func responsesAnnotationToCitation(annotation dto.ResponsesOutputAnnotation, text string) (normalizedResponsesCitation, bool) {
	if annotation.Type != "url_citation" && annotation.URLCitation == nil {
		return normalizedResponsesCitation{}, false
	}
	startIndex := annotation.StartIndex
	endIndex := annotation.EndIndex
	if annotation.URLCitation != nil {
		if startIndex == 0 && annotation.URLCitation.StartIndex > 0 {
			startIndex = annotation.URLCitation.StartIndex
		}
		if endIndex == 0 && annotation.URLCitation.EndIndex > 0 {
			endIndex = annotation.URLCitation.EndIndex
		}
	}
	if startIndex < 0 || endIndex <= startIndex || endIndex > textRuneLen(text) {
		return normalizedResponsesCitation{}, false
	}

	citation := normalizedResponsesCitation{
		URL:        strings.TrimSpace(annotation.URL),
		Title:      strings.TrimSpace(annotation.Title),
		StartIndex: startIndex,
		EndIndex:   endIndex,
		CitedText:  strings.TrimSpace(annotation.Text),
	}
	if annotation.URLCitation != nil {
		if citation.URL == "" {
			citation.URL = strings.TrimSpace(annotation.URLCitation.URL)
		}
		if citation.Title == "" {
			citation.Title = strings.TrimSpace(annotation.URLCitation.Title)
		}
		if citation.CitedText == "" {
			citation.CitedText = strings.TrimSpace(annotation.URLCitation.Text)
		}
		citation.EncryptedIndex = strings.TrimSpace(annotation.URLCitation.EncryptedIndex)
		citation.EncryptedContent = strings.TrimSpace(annotation.URLCitation.EncryptedContent)
	}
	if citation.URL == "" && citation.Title == "" {
		return normalizedResponsesCitation{}, false
	}
	return citation, true
}

func textRuneLen(text string) int {
	return len([]rune(text))
}

func sliceTextByCharacterRange(text string, startIndex int, endIndex int) (string, bool) {
	runes := []rune(text)
	if startIndex < 0 || endIndex < startIndex || endIndex > len(runes) {
		return "", false
	}
	return string(runes[startIndex:endIndex]), true
}
