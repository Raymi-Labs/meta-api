package dto

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const (
	ResponsesOutputTypeImageGenerationCall = "image_generation_call"
)

type SimpleResponse struct {
	Usage `json:"usage"`
	Error any `json:"error"`
}

// GetOpenAIError 从动态错误类型中提取OpenAIError结构
func (s *SimpleResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(s.Error)
}

type TextResponse struct {
	Id      string                     `json:"id"`
	Object  string                     `json:"object"`
	Created int64                      `json:"created"`
	Model   string                     `json:"model"`
	Choices []OpenAITextResponseChoice `json:"choices"`
	Usage   `json:"usage"`
}

type OpenAITextResponseChoice struct {
	Index        int `json:"index"`
	Message      `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type OpenAITextResponse struct {
	Id      string                     `json:"id"`
	Model   string                     `json:"model"`
	Object  string                     `json:"object"`
	Created any                        `json:"created"`
	Choices []OpenAITextResponseChoice `json:"choices"`
	Error   any                        `json:"error,omitempty"`
	Usage   `json:"usage"`
}

// GetOpenAIError 从动态错误类型中提取OpenAIError结构
func (o *OpenAITextResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}

type OpenAIEmbeddingResponseItem struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type OpenAIEmbeddingResponse struct {
	Object string                        `json:"object"`
	Data   []OpenAIEmbeddingResponseItem `json:"data"`
	Model  string                        `json:"model"`
	Usage  `json:"usage"`
}

type FlexibleEmbeddingResponseItem struct {
	Object    string `json:"object"`
	Index     int    `json:"index"`
	Embedding any    `json:"embedding"`
}

type FlexibleEmbeddingResponse struct {
	Object string                          `json:"object"`
	Data   []FlexibleEmbeddingResponseItem `json:"data"`
	Model  string                          `json:"model"`
	Usage  `json:"usage"`
}

type ChatCompletionsStreamResponseChoice struct {
	Delta        ChatCompletionsStreamResponseChoiceDelta `json:"delta,omitempty"`
	Logprobs     *any                                     `json:"logprobs"`
	FinishReason *string                                  `json:"finish_reason"`
	Index        int                                      `json:"index"`
}

type ChatCompletionsStreamResponseChoiceDelta struct {
	Content          *string            `json:"content,omitempty"`
	ReasoningContent *string            `json:"reasoning_content,omitempty"`
	Reasoning        *string            `json:"reasoning,omitempty"`
	Role             string             `json:"role,omitempty"`
	FunctionCall     *FunctionResponse  `json:"function_call,omitempty"`
	ToolCalls        []ToolCallResponse `json:"tool_calls,omitempty"`
}

func (c *ChatCompletionsStreamResponseChoiceDelta) SetContentString(s string) {
	c.Content = &s
}

func (c *ChatCompletionsStreamResponseChoiceDelta) GetContentString() string {
	if c.Content == nil {
		return ""
	}
	return *c.Content
}

func (c *ChatCompletionsStreamResponseChoiceDelta) GetReasoningContent() string {
	if c.ReasoningContent == nil && c.Reasoning == nil {
		return ""
	}
	if c.ReasoningContent != nil {
		return *c.ReasoningContent
	}
	return *c.Reasoning
}

func (c *ChatCompletionsStreamResponseChoiceDelta) SetReasoningContent(s string) {
	c.ReasoningContent = &s
	//c.Reasoning = &s
}

type ToolCallResponse struct {
	// Index is not nil only in chat completion chunk object
	Index    *int             `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     any              `json:"type"`
	Function FunctionResponse `json:"function"`
}

func (c *ToolCallResponse) SetIndex(i int) {
	c.Index = &i
}

type FunctionResponse struct {
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`
	// call function with arguments in JSON format
	Parameters any    `json:"parameters,omitempty"` // request
	Arguments  string `json:"arguments"`            // response
}

type ChatCompletionsStreamResponse struct {
	Id                string                                `json:"id"`
	Object            string                                `json:"object"`
	Created           int64                                 `json:"created"`
	Model             string                                `json:"model"`
	SystemFingerprint *string                               `json:"system_fingerprint"`
	Choices           []ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage             *Usage                                `json:"usage"`
}

func (c *ChatCompletionsStreamResponse) IsFinished() bool {
	if len(c.Choices) == 0 {
		return false
	}
	return c.Choices[0].FinishReason != nil && *c.Choices[0].FinishReason != ""
}

func (c *ChatCompletionsStreamResponse) IsToolCall() bool {
	if len(c.Choices) == 0 {
		return false
	}
	return len(c.Choices[0].Delta.ToolCalls) > 0
}

func (c *ChatCompletionsStreamResponse) GetFirstToolCall() *ToolCallResponse {
	if c.IsToolCall() {
		return &c.Choices[0].Delta.ToolCalls[0]
	}
	return nil
}

func (c *ChatCompletionsStreamResponse) ClearToolCalls() {
	if !c.IsToolCall() {
		return
	}
	for choiceIdx := range c.Choices {
		for callIdx := range c.Choices[choiceIdx].Delta.ToolCalls {
			c.Choices[choiceIdx].Delta.ToolCalls[callIdx].ID = ""
			c.Choices[choiceIdx].Delta.ToolCalls[callIdx].Type = nil
			c.Choices[choiceIdx].Delta.ToolCalls[callIdx].Function.Name = ""
		}
	}
}

func (c *ChatCompletionsStreamResponse) Copy() *ChatCompletionsStreamResponse {
	choices := make([]ChatCompletionsStreamResponseChoice, len(c.Choices))
	copy(choices, c.Choices)
	return &ChatCompletionsStreamResponse{
		Id:                c.Id,
		Object:            c.Object,
		Created:           c.Created,
		Model:             c.Model,
		SystemFingerprint: c.SystemFingerprint,
		Choices:           choices,
		Usage:             c.Usage,
	}
}

func (c *ChatCompletionsStreamResponse) GetSystemFingerprint() string {
	if c.SystemFingerprint == nil {
		return ""
	}
	return *c.SystemFingerprint
}

func (c *ChatCompletionsStreamResponse) SetSystemFingerprint(s string) {
	c.SystemFingerprint = &s
}

type ChatCompletionsStreamResponseSimple struct {
	Choices []ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage   *Usage                                `json:"usage"`
}

type CompletionsStreamResponse struct {
	Choices []struct {
		Text         string `json:"text"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type Usage struct {
	CacheCreationTotalReported bool `json:"-"`
	// InputTokensEstimated distinguishes locally estimated usage (input or output)
	// from upstream measurements. The name is retained for existing adapters.
	InputTokensEstimated bool            `json:"-"`
	InterruptedOutput    bool            `json:"-"` // diagnostic partial output; failed responses must not be billed
	PromptTokens         int             `json:"prompt_tokens"`
	CompletionTokens     int             `json:"completion_tokens"`
	TotalTokens          int             `json:"total_tokens"`
	PromptCacheHitTokens int             `json:"prompt_cache_hit_tokens,omitempty"`
	UsageSemantic        string          `json:"usage_semantic,omitempty"`
	UsageSource          string          `json:"usage_source,omitempty"`
	WebSearchRequests    int             `json:"-"`
	WebSearchUsage       *WebSearchUsage `json:"web_search_usage,omitempty"`

	PromptTokensDetails    InputTokenDetails   `json:"prompt_tokens_details"`
	CompletionTokenDetails OutputTokenDetails  `json:"completion_tokens_details"`
	InputTokens            int                 `json:"input_tokens"`
	OutputTokens           int                 `json:"output_tokens"`
	InputTokensDetails     *InputTokenDetails  `json:"input_tokens_details"`
	OutputTokensDetails    *OutputTokenDetails `json:"output_tokens_details,omitempty"`

	// claude cache 1h
	ClaudeCacheCreation5mTokens int `json:"claude_cache_creation_5_m_tokens"`
	ClaudeCacheCreation1hTokens int `json:"claude_cache_creation_1_h_tokens"`

	// OpenRouter Params
	Cost any `json:"cost,omitempty"`
}

type WebSearchUsage struct {
	ToolUsage int `json:"tool_usage"`
	PageUsage int `json:"page_usage"`
}

func (u *Usage) NormalizeWebSearchUsage() {
	if u == nil || u.WebSearchUsage == nil {
		return
	}
	if u.WebSearchUsage.ToolUsage > u.WebSearchRequests {
		u.WebSearchRequests = u.WebSearchUsage.ToolUsage
	}
	u.WebSearchUsage = nil
}

// OpenAIChatCompletionsUsage is the public Chat Completions usage shape.
//
// Usage is also used internally for Responses, Claude and billing semantics,
// so it intentionally contains provider-specific fields. Those fields must
// not cross an OpenAI Chat Completions response boundary.
type OpenAIChatCompletionsUsage struct {
	PromptTokens            int                               `json:"prompt_tokens"`
	CompletionTokens        int                               `json:"completion_tokens"`
	TotalTokens             int                               `json:"total_tokens"`
	PromptTokensDetails     OpenAIChatPromptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokensDetails OpenAIChatCompletionTokensDetails `json:"completion_tokens_details"`
}

type OpenAIChatPromptTokensDetails struct {
	CachedTokens        int                 `json:"cached_tokens"`
	CacheWriteTokens    *int                `json:"cache_write_tokens,omitempty"`
	AudioTokens         int                 `json:"audio_tokens"`
	CachedTokensDetails *CachedTokenDetails `json:"cached_tokens_details,omitempty"`
}

type OpenAIChatCompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
	AudioTokens     int `json:"audio_tokens"`
}

func NewOpenAIChatCompletionsUsage(usage *Usage) *OpenAIChatCompletionsUsage {
	if usage == nil {
		return nil
	}
	return &OpenAIChatCompletionsUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		PromptTokensDetails: OpenAIChatPromptTokensDetails{
			CachedTokens:        usage.PromptTokensDetails.CachedTokens,
			CacheWriteTokens:    cloneTokenCount(usage.PromptTokensDetails.CacheWriteTokens),
			AudioTokens:         usage.PromptTokensDetails.AudioTokens,
			CachedTokensDetails: usage.PromptTokensDetails.CachedTokensDetails.Clone(),
		},
		CompletionTokensDetails: OpenAIChatCompletionTokensDetails{
			ReasoningTokens: usage.CompletionTokenDetails.ReasoningTokens,
			AudioTokens:     usage.CompletionTokenDetails.AudioTokens,
		},
	}
}

// OpenAITextResponseWire is the protocol boundary representation used when a
// response is sent to an OpenAI Chat Completions client.
type OpenAITextResponseWire struct {
	Id      string                     `json:"id"`
	Model   string                     `json:"model"`
	Object  string                     `json:"object"`
	Created any                        `json:"created"`
	Choices []OpenAITextResponseChoice `json:"choices"`
	Error   any                        `json:"error,omitempty"`
	Usage   OpenAIChatCompletionsUsage `json:"usage"`
}

func NewOpenAITextResponseWire(response *OpenAITextResponse) *OpenAITextResponseWire {
	if response == nil {
		return nil
	}
	usage := NewOpenAIChatCompletionsUsage(&response.Usage)
	return &OpenAITextResponseWire{
		Id:      response.Id,
		Model:   response.Model,
		Object:  response.Object,
		Created: response.Created,
		Choices: response.Choices,
		Error:   response.Error,
		Usage:   *usage,
	}
}

// ChatCompletionsStreamResponseWire applies the same usage whitelist to SSE
// chunks without mutating the internal usage used for billing and conversion.
type ChatCompletionsStreamResponseWire struct {
	Id                string                                `json:"id"`
	Object            string                                `json:"object"`
	Created           int64                                 `json:"created"`
	Model             string                                `json:"model"`
	SystemFingerprint *string                               `json:"system_fingerprint"`
	Choices           []ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage             *OpenAIChatCompletionsUsage           `json:"usage"`
}

func NewChatCompletionsStreamResponseWire(response *ChatCompletionsStreamResponse) *ChatCompletionsStreamResponseWire {
	if response == nil {
		return nil
	}
	return &ChatCompletionsStreamResponseWire{
		Id:                response.Id,
		Object:            response.Object,
		Created:           response.Created,
		Model:             response.Model,
		SystemFingerprint: response.SystemFingerprint,
		Choices:           response.Choices,
		Usage:             NewOpenAIChatCompletionsUsage(response.Usage),
	}
}

type OpenAIVideoResponse struct {
	Id        string `json:"id" example:"file-abc123"`
	Object    string `json:"object" example:"file"`
	Bytes     int64  `json:"bytes" example:"120000"`
	CreatedAt int64  `json:"created_at" example:"1677610602"`
	ExpiresAt int64  `json:"expires_at" example:"1677614202"`
	Filename  string `json:"filename" example:"mydata.jsonl"`
	Purpose   string `json:"purpose" example:"fine-tune"`
}

type InputTokenDetails struct {
	CachedTokens         int                 `json:"cached_tokens"`
	CacheWriteTokens     *int                `json:"cache_write_tokens,omitempty"`
	CachedTokensDetails  *CachedTokenDetails `json:"cached_tokens_details,omitempty"`
	CachedCreationTokens int                 `json:"-"`
	TextTokens           int                 `json:"text_tokens"`
	AudioTokens          int                 `json:"audio_tokens"`
	ImageTokens          int                 `json:"image_tokens"`
}

type OutputTokenDetails struct {
	TextTokens      int `json:"text_tokens"`
	AudioTokens     int `json:"audio_tokens"`
	ImageTokens     int `json:"image_tokens"`
	ReasoningTokens int `json:"reasoning_tokens"`
}

type OpenAIResponsesResponse struct {
	ID                 string             `json:"id"`
	Object             string             `json:"object"`
	CreatedAt          int                `json:"created_at"`
	Status             string             `json:"status"`
	Error              any                `json:"error,omitempty"`
	IncompleteDetails  *IncompleteDetails `json:"incomplete_details,omitempty"`
	Instructions       json.RawMessage    `json:"instructions"`
	MaxOutputTokens    int                `json:"max_output_tokens"`
	Model              string             `json:"model"`
	Output             []ResponsesOutput  `json:"output"`
	ParallelToolCalls  bool               `json:"parallel_tool_calls"`
	PreviousResponseID string             `json:"previous_response_id"`
	Reasoning          *Reasoning         `json:"reasoning"`
	Store              bool               `json:"store"`
	Temperature        float64            `json:"temperature"`
	ToolChoice         json.RawMessage    `json:"tool_choice"`
	Tools              []map[string]any   `json:"tools"`
	TopP               float64            `json:"top_p"`
	Truncation         string             `json:"truncation"`
	Usage              *Usage             `json:"usage"`
	User               json.RawMessage    `json:"user"`
	Metadata           json.RawMessage    `json:"metadata"`
}

// GetOpenAIError 从动态错误类型中提取OpenAIError结构
func (o *OpenAIResponsesResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}

func (o *OpenAIResponsesResponse) HasImageGenerationCall() bool {
	return o.CountImageGenerationCalls() > 0
}

func (o *OpenAIResponsesResponse) CountImageGenerationCalls() int {
	count := 0
	for _, output := range o.Output {
		if output.Type == ResponsesOutputTypeImageGenerationCall {
			count++
		}
	}
	return count
}

func (o *OpenAIResponsesResponse) GetQuality() string {
	if len(o.Output) == 0 {
		return ""
	}
	for _, output := range o.Output {
		if output.Type == ResponsesOutputTypeImageGenerationCall {
			return output.Quality
		}
	}
	return ""
}

func (o *OpenAIResponsesResponse) GetSize() string {
	if len(o.Output) == 0 {
		return ""
	}
	for _, output := range o.Output {
		if output.Type == ResponsesOutputTypeImageGenerationCall {
			return output.Size
		}
	}
	return ""
}

type IncompleteDetails struct {
	Reason    string `json:"reason,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

type ResponsesOutput struct {
	Type      string                          `json:"type"`
	ID        string                          `json:"id"`
	Status    string                          `json:"status"`
	Role      string                          `json:"role"`
	Content   []ResponsesOutputContent        `json:"content"`
	Summary   []ResponsesReasoningSummaryPart `json:"summary,omitempty"`
	Quality   string                          `json:"quality"`
	Size      string                          `json:"size"`
	CallId    string                          `json:"call_id,omitempty"`
	Name      string                          `json:"name,omitempty"`
	Arguments ResponsesArguments              `json:"arguments,omitempty"`
	Action    json.RawMessage                 `json:"action,omitempty"`
}

// ResponsesArguments accepts provider-native JSON but always marshals as the
// string required by the OpenAI Responses API wire format.
type ResponsesArguments json.RawMessage

func (r ResponsesArguments) MarshalJSON() ([]byte, error) {
	return common.Marshal(common.JsonRawMessageToString(json.RawMessage(r)))
}

func (r *ResponsesArguments) UnmarshalJSON(data []byte) error {
	if r == nil {
		return nil
	}
	var value string
	if err := common.Unmarshal(data, &value); err == nil {
		*r = ResponsesArguments([]byte(value))
		return nil
	}
	cp := append([]byte(nil), data...)
	*r = ResponsesArguments(cp)
	return nil
}

// ArgumentsString returns function call arguments in the string form expected by Chat Completions.
func (r *ResponsesOutput) ArgumentsString() string {
	if r == nil {
		return ""
	}
	return ResponsesArgumentsString(json.RawMessage(r.Arguments))
}

// ResponsesArgumentsString returns function call arguments in the string form expected by Chat Completions.
func ResponsesArgumentsString(arguments json.RawMessage) string {
	return common.JsonRawMessageToString(arguments)
}

type ResponsesOutputContent struct {
	Type        string                      `json:"type"`
	Text        string                      `json:"text"`
	Annotations []ResponsesOutputAnnotation `json:"annotations"`
}

type ResponsesReasoningSummaryPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ResponsesOutputAnnotation struct {
	Type             string                          `json:"type"`
	URL              string                          `json:"url,omitempty"`
	Title            string                          `json:"title,omitempty"`
	StartIndex       int                             `json:"start_index,omitempty"`
	EndIndex         int                             `json:"end_index,omitempty"`
	Text             string                          `json:"text,omitempty"`
	URLCitation      *ResponsesURLCitationAnnotation `json:"url_citation,omitempty"`
	FileCitation     json.RawMessage                 `json:"file_citation,omitempty"`
	FilePathCitation json.RawMessage                 `json:"file_path_citation,omitempty"`
}

type ResponsesURLCitationAnnotation struct {
	URL              string `json:"url,omitempty"`
	Title            string `json:"title,omitempty"`
	StartIndex       int    `json:"start_index,omitempty"`
	EndIndex         int    `json:"end_index,omitempty"`
	Text             string `json:"text,omitempty"`
	EncryptedIndex   string `json:"encrypted_index,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

type ResponsesWebSearchAction struct {
	Query   string                     `json:"query,omitempty"`
	Queries []string                   `json:"queries,omitempty"`
	Sources []ResponsesWebSearchSource `json:"sources,omitempty"`
}

type ResponsesWebSearchSource struct {
	Type             string `json:"type,omitempty"`
	URL              string `json:"url,omitempty"`
	Title            string `json:"title,omitempty"`
	Text             string `json:"text,omitempty"`
	Snippet          string `json:"snippet,omitempty"`
	PageAge          string `json:"page_age,omitempty"`
	EncryptedIndex   string `json:"encrypted_index,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

const (
	BuildInToolWebSearch        = "web_search"
	BuildInToolWebSearchPreview = "web_search_preview"
	BuildInToolFileSearch       = "file_search"
	BuildInToolImageGeneration  = "image_generation"
)

const (
	BuildInCallWebSearchCall = "web_search_call"
)

func NormalizeBuiltInToolType(toolType string) string {
	switch strings.TrimSpace(toolType) {
	case BuildInToolWebSearch, BuildInToolWebSearchPreview:
		return BuildInToolWebSearchPreview
	default:
		return strings.TrimSpace(toolType)
	}
}

func IsBuiltInWebSearchToolType(toolType string) bool {
	return NormalizeBuiltInToolType(toolType) == BuildInToolWebSearchPreview
}

const (
	ResponsesOutputTypeItemAdded = "response.output_item.added"
	ResponsesOutputTypeItemDone  = "response.output_item.done"
)

// ResponsesStreamResponse 用于处理 /v1/responses 流式响应
type ResponsesStreamResponse struct {
	Type     string                   `json:"type"`
	Response *OpenAIResponsesResponse `json:"response,omitempty"`
	Error    any                      `json:"error,omitempty"`
	Delta    string                   `json:"delta,omitempty"`
	Item     *ResponsesOutput         `json:"item,omitempty"`
	// - response.function_call_arguments.delta
	// - response.function_call_arguments.done
	OutputIndex  *int                           `json:"output_index,omitempty"`
	ContentIndex *int                           `json:"content_index,omitempty"`
	SummaryIndex *int                           `json:"summary_index,omitempty"`
	ItemID       string                         `json:"item_id,omitempty"`
	Part         *ResponsesReasoningSummaryPart `json:"part,omitempty"`
}

// GetOpenAIError 从动态错误类型中提取OpenAIError结构
func GetOpenAIError(errorField any) *types.OpenAIError {
	if errorField == nil {
		return nil
	}

	switch err := errorField.(type) {
	case types.OpenAIError:
		return &err
	case *types.OpenAIError:
		return err
	case map[string]interface{}:
		// 处理从JSON解析来的map结构
		openaiErr := &types.OpenAIError{}
		if errType, ok := err["type"].(string); ok {
			openaiErr.Type = errType
		}
		if errMsg, ok := err["message"].(string); ok {
			openaiErr.Message = errMsg
		}
		if errParam, ok := err["param"].(string); ok {
			openaiErr.Param = errParam
		}
		if errCode, ok := err["code"]; ok {
			openaiErr.Code = errCode
		}
		return openaiErr
	case string:
		// 处理简单字符串错误
		return &types.OpenAIError{
			Type:    "error",
			Message: err,
		}
	default:
		// 未知类型，尝试转换为字符串
		return &types.OpenAIError{
			Type:    "unknown_error",
			Message: fmt.Sprintf("%v", err),
		}
	}
}
