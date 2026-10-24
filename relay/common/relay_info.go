package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type ThinkingContentInfo struct {
	IsFirstThinkingContent  bool
	SendLastThinkingContent bool
	HasSentThinkingContent  bool
}

const (
	LastMessageTypeNone     = "none"
	LastMessageTypeText     = "text"
	LastMessageTypeTools    = "tools"
	LastMessageTypeThinking = "thinking"
)

// GroupHealthCacheUsage is a normalized snapshot captured before billing mutates usage.
type GroupHealthCacheUsage struct {
	ReadTokens  int64
	InputTokens int64
}

type ClaudeConvertInfo struct {
	LastMessagesType string
	Index            int
	Usage            *dto.Usage
	FinishReason     string
	Done             bool

	Started        bool
	NextBlockIndex int
	ToolStates     map[int]*ClaudeStreamToolState
	ToolOrder      []int
}

type ClaudeStreamToolState struct {
	BlockIndex       int
	ID               string
	Name             string
	PendingArguments string
	Started          bool
	Closed           bool
}

type RerankerInfo struct {
	Documents       []any
	ReturnDocuments bool
}

type BuildInToolInfo struct {
	ToolName          string
	CallCount         int
	SearchContextSize string
}

type ResponsesUsageInfo struct {
	BuiltInTools map[string]*BuildInToolInfo
}

type ResponsesCompletedSummary struct {
	Status                   string   `json:"status,omitempty"`
	OutputCount              int      `json:"output_count"`
	OutputTypes              []string `json:"output_types,omitempty"`
	MessageCount             int      `json:"message_count,omitempty"`
	FunctionCallCount        int      `json:"function_call_count,omitempty"`
	WebSearchCallCount       int      `json:"web_search_call_count,omitempty"`
	ImageGenerationCallCount int      `json:"image_generation_call_count,omitempty"`
	ActionableToolCallCount  int      `json:"actionable_tool_call_count,omitempty"`
	MessageTextChars         int      `json:"message_text_chars,omitempty"`
	HasActionableToolCall    bool     `json:"has_actionable_tool_call"`
	IncompleteReason         string   `json:"incomplete_reason,omitempty"`
}

func (info *RelayInfo) RecordResponsesCompletedSummary(response *dto.OpenAIResponsesResponse) {
	if info == nil || response == nil {
		return
	}
	info.ResponsesCompletedSummary = NewResponsesCompletedSummary(response)
}

func NewResponsesCompletedSummary(response *dto.OpenAIResponsesResponse) *ResponsesCompletedSummary {
	if response == nil {
		return nil
	}
	// This summary is written into request logs, so keep it metadata-only.
	summary := &ResponsesCompletedSummary{
		Status:      response.Status,
		OutputCount: len(response.Output),
	}
	seenOutputTypes := make(map[string]struct{}, len(response.Output))
	for _, output := range response.Output {
		if output.Type != "" {
			if _, ok := seenOutputTypes[output.Type]; !ok {
				seenOutputTypes[output.Type] = struct{}{}
				summary.OutputTypes = append(summary.OutputTypes, output.Type)
			}
		}
		switch output.Type {
		case "message":
			summary.MessageCount++
			for _, content := range output.Content {
				summary.MessageTextChars += utf8.RuneCountInString(content.Text)
			}
		case "function_call":
			summary.FunctionCallCount++
		case dto.BuildInCallWebSearchCall:
			summary.WebSearchCallCount++
		case dto.ResponsesOutputTypeImageGenerationCall:
			summary.ImageGenerationCallCount++
		}
		if isActionableResponsesOutputType(output.Type) {
			summary.ActionableToolCallCount++
			summary.HasActionableToolCall = true
		}
	}
	if response.IncompleteDetails != nil {
		incompleteReason := strings.TrimSpace(response.IncompleteDetails.Reason)
		if incompleteReason == "" {
			incompleteReason = strings.TrimSpace(response.IncompleteDetails.Reasoning)
		}
		summary.IncompleteReason = incompleteReason
	}
	return summary
}

func isActionableResponsesOutputType(outputType string) bool {
	switch outputType {
	case "function_call", "computer_call", "custom_tool_call":
		return true
	default:
		return false
	}
}

type ChannelMeta struct {
	ChannelType                    int
	ChannelId                      int
	ChannelTag                     string
	ChannelIsMultiKey              bool
	ChannelMultiKeyIndex           int
	ChannelBaseUrl                 string
	ApiType                        int
	ApiVersion                     string
	ApiKey                         string
	Organization                   string
	ChannelCreateTime              int64
	ParamOverride                  map[string]interface{}
	HeadersOverride                map[string]interface{}
	ChannelSetting                 dto.ChannelSettings
	ChannelOtherSettings           dto.ChannelOtherSettings
	UpstreamModelName              string
	IsModelMapped                  bool
	SupportsChatStreamOptions      bool // 是否支持 chat/completions 的 stream_options
	SupportsResponsesAPI           bool // 是否支持 /v1/responses
	SupportsResponsesStreamOptions bool // 是否支持 responses 的 stream_options
	NativeTextFormats              TextProtocolSet
}

type TokenCountMeta struct {
	//promptTokens int
	estimatePromptTokens int
}

type RelayInfo struct {
	TokenId                  int
	TokenKey                 string
	TokenGroup               string
	UserId                   int
	UsingGroup               string // 使用的分组，当auto跨分组重试时，会变动
	UserGroup                string // 用户所在分组
	TokenUnlimited           bool
	StartTime                time.Time
	FirstResponseTime        time.Time
	FirstEffectiveOutputTime time.Time
	// Group health observes more providers without changing the existing slow
	// TTFT routing guard's sampling coverage or first-channel attribution.
	GroupHealthCacheUsage          *GroupHealthCacheUsage
	GroupHealthFirstOutputTime     time.Time
	FirstEffectiveOutputChannelId  int
	FirstEffectiveOutputChannelTag string
	isFirstResponse                bool
	//SendLastReasoningResponse bool
	IsStream                bool
	IsGeminiBatchEmbedding  bool
	IsPlayground            bool
	UsePrice                bool
	RelayMode               int
	OriginModelName         string
	ResponseModel           *ResponseModel
	UpstreamRequestBodySize int64
	RequestURLPath          string
	RequestHeaders          map[string]string
	ShouldIncludeUsage      bool
	DisablePing             bool // 是否禁止向下游发送自定义 Ping
	// SuppressStreamResponseHeaders 为 true 时,StreamScannerHandler 不再自动向客户端
	// 写入 SSE 响应头(text/event-stream)。用于「codex 恒定强制上游流式、但客户端要非流式」
	// 的聚合场景:内部读上游 SSE,但最终以单个 JSON body 返回,因此绝不能提前把响应头设成 SSE。
	SuppressStreamResponseHeaders bool
	ClientWs                      *websocket.Conn
	TargetWs                      *websocket.Conn
	InputAudioFormat              string
	InputAudioDurationSeconds     int64
	OutputAudioFormat             string
	RealtimeTools                 []dto.RealTimeTool
	IsFirstRequest                bool
	AudioUsage                    bool
	ReasoningEffort               string
	UserSetting                   dto.UserSetting
	UserEmail                     string
	UserQuota                     int
	RelayFormat                   types.RelayFormat
	SendResponseCount             int
	ReceivedResponseCount         int
	FinalPreConsumedQuota         int // 最终预消耗的配额
	// ForcePreConsume 为 true 时禁用 BillingSession 的信任额度旁路，
	// 强制预扣全额。用于异步任务（视频/音乐生成等），因为请求返回后任务仍在运行，
	// 必须在提交前锁定全额。
	ForcePreConsume bool
	// Billing 是计费会话，封装了预扣费/结算/退款的统一生命周期。
	// 免费模型时为 nil。
	Billing                 BillingSettler
	ImageRequestCount       int
	ImageResponseCount      *int  // nil means the adapter did not report an actual payload count
	PerformanceOutputTokens int64 // observed usage only; samples are recorded once at request completion
	ImageBasePriceData      *types.PriceData
	ImageBaseBillingInput   *billingexpr.RequestInput
	// BillingSource indicates whether this request is billed from wallet quota or subscription.
	// "" or "wallet" => wallet; "subscription" => subscription
	BillingSource string
	// SubscriptionId is the user_subscriptions.id used when BillingSource == "subscription"
	SubscriptionId int
	// SubscriptionPreConsumed is the amount pre-consumed on subscription item (quota units or 1)
	SubscriptionPreConsumed int64
	// SubscriptionPostDelta is the post-consume delta applied to amount_used (quota units; can be negative).
	SubscriptionPostDelta int64
	// SubscriptionPlanId / SubscriptionPlanTitle are used for logging/UI display.
	SubscriptionPlanId    int
	SubscriptionPlanTitle string
	// RequestId is used for idempotent pre-consume/refund
	RequestId string
	// SubscriptionAmountTotal / SubscriptionAmountUsedAfterPreConsume are used to compute remaining in logs.
	SubscriptionAmountTotal               int64
	SubscriptionAmountUsedAfterPreConsume int64
	IsClaudeBetaQuery                     bool // /v1/messages?beta=true
	IsChannelTest                         bool // channel test request
	RetryIndex                            int
	LastError                             *types.NewAPIError
	RuntimeHeadersOverride                map[string]interface{}
	UseRuntimeHeadersOverride             bool
	ParamOverrideAudit                    []string
	StreamStatus                          *StreamStatus
	ResponsesCompletedSummary             *ResponsesCompletedSummary
	KimiUsageEvidence                     *KimiUsageEvidence
	ChatToolProtocol                      dto.ChatToolProtocol
	ChatToolCount                         int

	PriceData types.PriceData

	// TieredBillingSnapshot is a frozen snapshot of tiered billing rules
	// captured at pre-consume time. Non-nil only when billing mode is "tiered_expr".
	TieredBillingSnapshot *billingexpr.BillingSnapshot
	BillingRequestInput   *billingexpr.RequestInput

	Request dto.Request

	// RequestConversionChain records request format conversions in order, e.g.
	// ["openai", "openai_responses"] or ["openai", "claude"].
	RequestConversionChain []types.RelayFormat
	// 最终请求到上游的格式。可由 adaptor 显式设置；
	// 若为空，调用 GetFinalRequestRelayFormat 会回退到 RequestConversionChain 的最后一项或 RelayFormat。
	FinalRequestRelayFormat types.RelayFormat
	TextProtocolPlan        *TextProtocolPlan

	ThinkingContentInfo
	TokenCountMeta
	*ClaudeConvertInfo
	*RerankerInfo
	*ResponsesUsageInfo
	*ChannelMeta
	*TaskRelayInfo
}

func (info *RelayInfo) InitChannelMeta(c *gin.Context) {
	// Observations belong to this attempt, not an earlier failed channel.
	info.ResponseModel = nil
	info.KimiUsageEvidence = nil
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)
	headerOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelHeaderOverride)
	apiType, _ := common.ChannelType2APIType(channelType)
	channelMeta := &ChannelMeta{
		ChannelType:          channelType,
		ChannelId:            common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ChannelTag:           common.GetContextKeyString(c, constant.ContextKeyChannelTag),
		ChannelIsMultiKey:    common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey),
		ChannelMultiKeyIndex: common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex),
		ChannelBaseUrl:       common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl),
		ApiType:              apiType,
		ApiVersion:           c.GetString("api_version"),
		ApiKey:               common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		Organization:         c.GetString("channel_organization"),
		ChannelCreateTime:    c.GetInt64("channel_create_time"),
		ParamOverride:        paramOverride,
		HeadersOverride:      headerOverride,
		UpstreamModelName:    common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		IsModelMapped:        false,
	}

	if channelType == constant.ChannelTypeAzure {
		channelMeta.ApiVersion = GetAPIVersion(c)
	}
	if channelType == constant.ChannelTypeVertexAi {
		channelMeta.ApiVersion = c.GetString("region")
	}

	channelSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if ok {
		channelMeta.ChannelSetting = channelSetting
	}

	channelOtherSettings, ok := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	if ok {
		channelMeta.ChannelOtherSettings = channelOtherSettings
	}

	caps := ResolveChannelCapabilities(channelMeta.ChannelType, channelMeta.ChannelBaseUrl, channelMeta.ChannelOtherSettings)
	channelMeta.SupportsChatStreamOptions = caps.SupportsChatStreamOptions
	channelMeta.SupportsResponsesAPI = caps.SupportsResponsesAPI
	channelMeta.SupportsResponsesStreamOptions = caps.SupportsResponsesStreamOptions
	channelMeta.NativeTextFormats = caps.NativeTextFormats

	info.ChannelMeta = channelMeta

	// reset some fields based on channel meta
	// 重置某些字段，例如模型名称等
	if info.Request != nil {
		info.Request.SetModelName(info.OriginModelName)
	}
}

func (info *RelayInfo) ToString() string {
	if info == nil {
		return "RelayInfo<nil>"
	}

	// Basic info
	b := &strings.Builder{}
	fmt.Fprintf(b, "RelayInfo{ ")
	fmt.Fprintf(b, "RelayFormat: %s, ", info.RelayFormat)
	fmt.Fprintf(b, "RelayMode: %d, ", info.RelayMode)
	fmt.Fprintf(b, "IsStream: %t, ", info.IsStream)
	fmt.Fprintf(b, "IsPlayground: %t, ", info.IsPlayground)
	fmt.Fprintf(b, "RequestURLPath: %q, ", info.RequestURLPath)
	fmt.Fprintf(b, "OriginModelName: %q, ", info.OriginModelName)
	fmt.Fprintf(b, "EstimatePromptTokens: %d, ", info.estimatePromptTokens)
	fmt.Fprintf(b, "ShouldIncludeUsage: %t, ", info.ShouldIncludeUsage)
	fmt.Fprintf(b, "DisablePing: %t, ", info.DisablePing)
	fmt.Fprintf(b, "SendResponseCount: %d, ", info.SendResponseCount)
	fmt.Fprintf(b, "FinalPreConsumedQuota: %d, ", info.FinalPreConsumedQuota)

	// User & token info (mask secrets)
	fmt.Fprintf(b, "User{ Id: %d, Email: %q, Group: %q, UsingGroup: %q, Quota: %d }, ",
		info.UserId, common.MaskEmail(info.UserEmail), info.UserGroup, info.UsingGroup, info.UserQuota)
	fmt.Fprintf(b, "Token{ Id: %d, Unlimited: %t, Key: ***masked*** }, ", info.TokenId, info.TokenUnlimited)

	// Time info
	latencyMs := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	fmt.Fprintf(b, "Timing{ Start: %s, FirstResponse: %s, LatencyMs: %d }, ",
		info.StartTime.Format(time.RFC3339Nano), info.FirstResponseTime.Format(time.RFC3339Nano), latencyMs)

	// Audio / realtime
	if info.InputAudioFormat != "" || info.OutputAudioFormat != "" || len(info.RealtimeTools) > 0 || info.AudioUsage {
		fmt.Fprintf(b, "Realtime{ AudioUsage: %t, InFmt: %q, OutFmt: %q, Tools: %d }, ",
			info.AudioUsage, info.InputAudioFormat, info.OutputAudioFormat, len(info.RealtimeTools))
	}

	// Reasoning
	if info.ReasoningEffort != "" {
		fmt.Fprintf(b, "ReasoningEffort: %q, ", info.ReasoningEffort)
	}

	// Price data (non-sensitive)
	if info.PriceData.UsePrice {
		fmt.Fprintf(b, "PriceData{ %s }, ", info.PriceData.ToSetting())
	}

	// Channel metadata (mask ApiKey)
	if info.ChannelMeta != nil {
		cm := info.ChannelMeta
		fmt.Fprintf(b, "ChannelMeta{ Type: %d, Id: %d, Tag: %q, IsMultiKey: %t, MultiKeyIndex: %d, BaseURL: %q, ApiType: %d, ApiVersion: %q, Organization: %q, CreateTime: %d, UpstreamModelName: %q, IsModelMapped: %t, SupportsChatStreamOptions: %t, SupportsResponsesAPI: %t, SupportsResponsesStreamOptions: %t, ApiKey: ***masked*** }, ",
			cm.ChannelType, cm.ChannelId, cm.ChannelTag, cm.ChannelIsMultiKey, cm.ChannelMultiKeyIndex, cm.ChannelBaseUrl, cm.ApiType, cm.ApiVersion, cm.Organization, cm.ChannelCreateTime, cm.UpstreamModelName, cm.IsModelMapped, cm.SupportsChatStreamOptions, cm.SupportsResponsesAPI, cm.SupportsResponsesStreamOptions)
	}

	// Responses usage info (non-sensitive)
	if info.ResponsesUsageInfo != nil && len(info.ResponsesUsageInfo.BuiltInTools) > 0 {
		fmt.Fprintf(b, "ResponsesTools{ ")
		first := true
		for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
			if !first {
				fmt.Fprintf(b, ", ")
			}
			first = false
			if tool != nil {
				fmt.Fprintf(b, "%s: calls=%d", name, tool.CallCount)
			} else {
				fmt.Fprintf(b, "%s: calls=0", name)
			}
		}
		fmt.Fprintf(b, " }, ")
	}

	fmt.Fprintf(b, "}")
	return b.String()
}

func GenRelayInfoWs(c *gin.Context, ws *websocket.Conn) *RelayInfo {
	info := genBaseRelayInfo(c, nil)
	info.RelayFormat = types.RelayFormatOpenAIRealtime
	info.ClientWs = ws
	info.InputAudioFormat = "pcm16"
	info.OutputAudioFormat = "pcm16"
	info.IsFirstRequest = true
	return info
}

func GenRelayInfoClaude(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatClaude
	info.ShouldIncludeUsage = false
	info.ClaudeConvertInfo = &ClaudeConvertInfo{
		LastMessagesType: LastMessageTypeNone,
	}
	info.IsClaudeBetaQuery = c.Query("beta") == "true"
	return info
}

func GenRelayInfoRerank(c *gin.Context, request *dto.RerankRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeRerank
	info.RelayFormat = types.RelayFormatRerank
	info.RerankerInfo = &RerankerInfo{
		Documents:       request.Documents,
		ReturnDocuments: request.GetReturnDocuments(),
	}
	return info
}

func GenRelayInfoOpenAIAudio(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIAudio
	return info
}

func GenRelayInfoEmbedding(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatEmbedding
	return info
}

func GenRelayInfoResponses(c *gin.Context, request *dto.OpenAIResponsesRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeResponses
	info.RelayFormat = types.RelayFormatOpenAIResponses

	info.ResponsesUsageInfo = &ResponsesUsageInfo{
		BuiltInTools: make(map[string]*BuildInToolInfo),
	}
	if len(request.Tools) > 0 {
		for _, tool := range request.GetToolsMap() {
			toolType := dto.NormalizeBuiltInToolType(common.Interface2String(tool["type"]))
			info.ResponsesUsageInfo.BuiltInTools[toolType] = &BuildInToolInfo{
				ToolName:  toolType,
				CallCount: 0,
			}
			switch toolType {
			case dto.BuildInToolWebSearchPreview:
				searchContextSize := common.Interface2String(tool["search_context_size"])
				if searchContextSize == "" {
					searchContextSize = "medium"
				}
				info.ResponsesUsageInfo.BuiltInTools[toolType].SearchContextSize = searchContextSize
			}
		}
	}
	return info
}

func GenRelayInfoGemini(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatGemini
	info.ShouldIncludeUsage = false

	return info
}

func GenRelayInfoImage(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIImage
	return info
}

func GenRelayInfoOpenAI(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAI
	return info
}

func genBaseRelayInfo(c *gin.Context, request dto.Request) *RelayInfo {

	//channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	//channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	//paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)

	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	// 当令牌分组为空时，表示使用用户分组
	if tokenGroup == "" {
		tokenGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}

	isStream := false

	if request != nil {
		isStream = request.IsStream(c)
	}

	// firstResponseTime = time.Now() - 1 second

	reqId := common.GetContextKeyString(c, common.RequestIdKey)
	if reqId == "" {
		reqId = common.GetTimeString() + common.GetRandomString(8)
	}
	info := &RelayInfo{
		Request: request,

		RequestId:  reqId,
		UserId:     common.GetContextKeyInt(c, constant.ContextKeyUserId),
		UsingGroup: common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		UserGroup:  common.GetContextKeyString(c, constant.ContextKeyUserGroup),
		UserQuota:  common.GetContextKeyInt(c, constant.ContextKeyUserQuota),
		UserEmail:  common.GetContextKeyString(c, constant.ContextKeyUserEmail),

		OriginModelName: common.GetContextKeyString(c, constant.ContextKeyOriginalModel),

		TokenId:        common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenKey:       common.GetContextKeyString(c, constant.ContextKeyTokenKey),
		TokenUnlimited: common.GetContextKeyBool(c, constant.ContextKeyTokenUnlimited),
		TokenGroup:     tokenGroup,

		isFirstResponse: true,
		RelayMode:       relayconstant.Path2RelayMode(c.Request.URL.Path),
		RequestURLPath:  c.Request.URL.String(),
		RequestHeaders:  cloneRequestHeaders(c),
		IsStream:        isStream,

		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
		ThinkingContentInfo: ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
		},
		TokenCountMeta: TokenCountMeta{
			//promptTokens: common.GetContextKeyInt(c, constant.ContextKeyPromptTokens),
			estimatePromptTokens: common.GetContextKeyInt(c, constant.ContextKeyEstimatedTokens),
		},
	}

	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = c.GetInt("relay_mode")
	}

	if strings.HasPrefix(c.Request.URL.Path, "/pg") {
		info.IsPlayground = true
		info.RequestURLPath = strings.TrimPrefix(info.RequestURLPath, "/pg")
		info.RequestURLPath = "/v1" + info.RequestURLPath
	}

	userSetting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
	if ok {
		info.UserSetting = userSetting
	}

	return info
}

func cloneRequestHeaders(c *gin.Context) map[string]string {
	if c == nil || c.Request == nil {
		return nil
	}
	if len(c.Request.Header) == 0 {
		return nil
	}

	headers := make(map[string]string, len(c.Request.Header))
	for key := range c.Request.Header {
		value := strings.TrimSpace(c.Request.Header.Get(key))
		if value == "" {
			continue
		}
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func GenRelayInfo(c *gin.Context, relayFormat types.RelayFormat, request dto.Request, ws *websocket.Conn) (*RelayInfo, error) {
	var info *RelayInfo
	var err error
	switch relayFormat {
	case types.RelayFormatOpenAI:
		info = GenRelayInfoOpenAI(c, request)
	case types.RelayFormatOpenAIAudio:
		info = GenRelayInfoOpenAIAudio(c, request)
	case types.RelayFormatOpenAIImage:
		info = GenRelayInfoImage(c, request)
	case types.RelayFormatOpenAIRealtime:
		info = GenRelayInfoWs(c, ws)
	case types.RelayFormatClaude:
		info = GenRelayInfoClaude(c, request)
	case types.RelayFormatRerank:
		if request, ok := request.(*dto.RerankRequest); ok {
			info = GenRelayInfoRerank(c, request)
			break
		}
		err = errors.New("request is not a RerankRequest")
	case types.RelayFormatGemini:
		info = GenRelayInfoGemini(c, request)
	case types.RelayFormatEmbedding:
		info = GenRelayInfoEmbedding(c, request)
	case types.RelayFormatOpenAIResponses:
		if request, ok := request.(*dto.OpenAIResponsesRequest); ok {
			info = GenRelayInfoResponses(c, request)
			break
		}
		err = errors.New("request is not a OpenAIResponsesRequest")
	case types.RelayFormatOpenAIResponsesCompaction:
		if request, ok := request.(*dto.OpenAIResponsesCompactionRequest); ok {
			return GenRelayInfoResponsesCompaction(c, request), nil
		}
		return nil, errors.New("request is not a OpenAIResponsesCompactionRequest")
	case types.RelayFormatTask:
		info = genBaseRelayInfo(c, nil)
		info.TaskRelayInfo = &TaskRelayInfo{}
	case types.RelayFormatMjProxy:
		info = genBaseRelayInfo(c, nil)
		info.TaskRelayInfo = &TaskRelayInfo{}
	default:
		err = errors.New("invalid relay format")
	}

	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("failed to build relay info")
	}

	info.InitRequestConversionChain()
	return info, nil
}

func (info *RelayInfo) InitRequestConversionChain() {
	if info == nil {
		return
	}
	if len(info.RequestConversionChain) > 0 {
		return
	}
	if info.RelayFormat == "" {
		return
	}
	info.RequestConversionChain = []types.RelayFormat{info.RelayFormat}
}

func (info *RelayInfo) AppendRequestConversion(format types.RelayFormat) {
	if info == nil {
		return
	}
	if format == "" {
		return
	}
	if len(info.RequestConversionChain) == 0 {
		info.RequestConversionChain = []types.RelayFormat{format}
		return
	}
	last := info.RequestConversionChain[len(info.RequestConversionChain)-1]
	if last == format {
		return
	}
	info.RequestConversionChain = append(info.RequestConversionChain, format)
}

func (info *RelayInfo) GetFinalRequestRelayFormat() types.RelayFormat {
	if info == nil {
		return ""
	}
	if info.FinalRequestRelayFormat != "" {
		return info.FinalRequestRelayFormat
	}
	if n := len(info.RequestConversionChain); n > 0 {
		return info.RequestConversionChain[n-1]
	}
	return info.RelayFormat
}

func GenRelayInfoResponsesCompaction(c *gin.Context, request *dto.OpenAIResponsesCompactionRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = relayconstant.RelayModeResponsesCompact
	}
	info.RelayFormat = types.RelayFormatOpenAIResponsesCompaction
	return info
}

//func (info *RelayInfo) SetPromptTokens(promptTokens int) {
//	info.promptTokens = promptTokens
//}

func (info *RelayInfo) SetEstimatePromptTokens(promptTokens int) {
	info.estimatePromptTokens = promptTokens
}

func (info *RelayInfo) GetEstimatePromptTokens() int {
	return info.estimatePromptTokens
}

func (info *RelayInfo) SetFirstResponseTime() {
	if info.isFirstResponse {
		info.FirstResponseTime = time.Now()
		info.isFirstResponse = false
	}
}

func (info *RelayInfo) SetFirstEffectiveOutputTime() {
	info.SetGroupHealthFirstOutputTime()
	if info == nil || !info.FirstEffectiveOutputTime.IsZero() {
		return
	}
	info.FirstEffectiveOutputTime = time.Now()
	if info.ChannelMeta != nil {
		info.FirstEffectiveOutputChannelId = info.ChannelId
		info.FirstEffectiveOutputChannelTag = info.ChannelTag
	}
}

func (info *RelayInfo) HasSendResponse() bool {
	return info.FirstResponseTime.After(info.StartTime)
}

type TaskRelayInfo struct {
	Action       string
	OriginTaskID string
	// PublicTaskID 是提交时预生成的 task_xxxx 格式公开 ID，
	// 供 DoResponse 在返回给客户端时使用（避免暴露上游真实 ID）。
	PublicTaskID string

	ConsumeQuota bool

	// LockedChannel holds the full channel object when the request is bound to
	// a specific channel (e.g., remix on origin task's channel). Stored as any
	// to avoid an import cycle with model; callers type-assert to *model.Channel.
	LockedChannel any
}

type TaskSubmitReq struct {
	Prompt         string                 `json:"prompt"`
	Model          string                 `json:"model,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Image          string                 `json:"image,omitempty"`
	Images         []string               `json:"images,omitempty"`
	Size           string                 `json:"size,omitempty"`
	Resolution     string                 `json:"resolution,omitempty"`
	AspectRatio    string                 `json:"aspect_ratio,omitempty"`
	Height         int                    `json:"height,omitempty"`
	Duration       int                    `json:"duration,omitempty"`
	Seconds        string                 `json:"seconds,omitempty"`
	InputReference string                 `json:"input_reference,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

func (t *TaskSubmitReq) GetPrompt() string {
	return t.Prompt
}

func (t *TaskSubmitReq) HasImage() bool {
	return len(t.Images) > 0
}

func (t *TaskSubmitReq) UnmarshalJSON(data []byte) error {
	type Alias TaskSubmitReq
	aux := &struct {
		Metadata json.RawMessage `json:"metadata,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Metadata) > 0 {
		var metadataStr string
		if err := common.Unmarshal(aux.Metadata, &metadataStr); err == nil && metadataStr != "" {
			var metadataObj map[string]interface{}
			if err := common.Unmarshal([]byte(metadataStr), &metadataObj); err == nil {
				t.Metadata = metadataObj
				return nil
			}
		}

		var metadataObj map[string]interface{}
		if err := common.Unmarshal(aux.Metadata, &metadataObj); err == nil {
			t.Metadata = metadataObj
		}
	}

	return nil
}
func (t *TaskSubmitReq) UnmarshalMetadata(v any) error {
	metadata := t.Metadata
	if metadata != nil {
		metadataBytes, err := common.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata failed: %w", err)
		}
		err = common.Unmarshal(metadataBytes, v)
		if err != nil {
			return fmt.Errorf("unmarshal metadata to target failed: %w", err)
		}
	}
	return nil
}

type TaskInfo struct {
	Code             int    `json:"code"`
	TaskID           string `json:"task_id"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Url              string `json:"url,omitempty"`
	RemoteUrl        string `json:"remote_url,omitempty"`
	Progress         string `json:"progress,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"` // 用于按倍率计费
	TotalTokens      int    `json:"total_tokens,omitempty"`      // 用于按倍率计费
}

func FailTaskInfo(reason string) *TaskInfo {
	return &TaskInfo{
		Status: "FAILURE",
		Reason: reason,
	}
}

// RemoveDisabledFields 从请求 JSON 数据中移除渠道设置中禁用的字段
// service_tier: 服务层级字段，可能导致额外计费（OpenAI、Claude、Responses API 支持）
// inference_geo: Claude 数据驻留推理区域字段（仅 Claude 支持，默认过滤）
// speed: Claude 推理速度模式字段（仅 Claude 支持，默认过滤）
// store: 数据存储授权字段，涉及用户隐私（仅 OpenAI、Responses API 支持，默认允许透传，禁用后可能导致 Codex 无法使用）
// safety_identifier: 安全标识符，用于向 OpenAI 报告违规用户（仅 OpenAI 支持，涉及用户隐私）
// stream_options.include_obfuscation: 响应流混淆控制字段（仅 OpenAI Responses API 支持）
func RemoveDisabledFields(jsonData []byte, channelOtherSettings dto.ChannelOtherSettings, channelPassThroughEnabled bool) ([]byte, error) {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || channelPassThroughEnabled {
		return jsonData, nil
	}

	// Decode only the fields we edit; nested schemas retain key order and numbers.
	var data map[string]json.RawMessage
	if err := common.Unmarshal(jsonData, &data); err != nil {
		common.SysError("RemoveDisabledFields Unmarshal error :" + err.Error())
		return jsonData, nil
	}

	// 默认移除 service_tier，除非明确允许（避免额外计费风险）
	if !channelOtherSettings.AllowServiceTier {
		if _, exists := data["service_tier"]; exists {
			delete(data, "service_tier")
		}
	}

	// 默认移除 inference_geo，除非明确允许（避免在未授权情况下透传数据驻留区域）
	if !channelOtherSettings.AllowInferenceGeo {
		if _, exists := data["inference_geo"]; exists {
			delete(data, "inference_geo")
		}
	}

	// 默认移除 speed，除非明确允许（避免意外切换 Claude 推理速度模式）
	if !channelOtherSettings.AllowSpeed {
		if _, exists := data["speed"]; exists {
			delete(data, "speed")
		}
	}

	// 默认允许 store 透传，除非明确禁用（禁用可能影响 Codex 使用）
	if channelOtherSettings.DisableStore {
		if _, exists := data["store"]; exists {
			delete(data, "store")
		}
	}

	// 默认移除 safety_identifier，除非明确允许（保护用户隐私，避免向 OpenAI 报告用户信息）
	if !channelOtherSettings.AllowSafetyIdentifier {
		if _, exists := data["safety_identifier"]; exists {
			delete(data, "safety_identifier")
		}
	}

	// 默认移除 stream_options.include_obfuscation，除非明确允许（避免关闭响应流混淆保护）
	if !channelOtherSettings.AllowIncludeObfuscation {
		if rawOptions, exists := data["stream_options"]; exists {
			var streamOptions map[string]json.RawMessage
			if err := common.Unmarshal(rawOptions, &streamOptions); err == nil && streamOptions != nil {
				if _, includeExists := streamOptions["include_obfuscation"]; includeExists {
					delete(streamOptions, "include_obfuscation")
				}
				if len(streamOptions) == 0 {
					delete(data, "stream_options")
				} else {
					encoded, err := common.Marshal(streamOptions)
					if err != nil {
						return jsonData, err
					}
					data["stream_options"] = encoded
				}
			}
		}
	}

	jsonDataAfter, err := common.Marshal(data)
	if err != nil {
		common.SysError("RemoveDisabledFields Marshal error :" + err.Error())
		return jsonData, nil
	}
	return jsonDataAfter, nil
}

// RemoveGeminiDisabledFields removes disabled fields from Gemini request JSON data
// Currently supports removing functionResponse.id field which Vertex AI does not support
func RemoveGeminiDisabledFields(jsonData []byte) ([]byte, error) {
	if !model_setting.GetGeminiSettings().RemoveFunctionResponseIdEnabled {
		return jsonData, nil
	}

	if !gjson.ValidBytes(jsonData) {
		return jsonData, nil
	}
	contents := gjson.GetBytes(jsonData, "contents")
	if !contents.IsArray() {
		return jsonData, nil
	}
	// Remove only the selected IDs. Re-encoding the entire Gemini request would
	// reorder unrelated tool/response schemas and round large numeric values.
	result := jsonData
	for i, content := range contents.Array() {
		parts := content.Get("parts")
		if !parts.IsArray() {
			continue
		}
		for j, part := range parts.Array() {
			for _, name := range []string{"functionResponse", "function_response"} {
				response := part.Get(name)
				if !response.IsObject() || !response.Get("id").Exists() {
					continue
				}
				path := "contents." + strconv.Itoa(i) + ".parts." + strconv.Itoa(j) + "." + name + ".id"
				var err error
				result, err = sjson.DeleteBytes(result, path)
				if err != nil {
					return jsonData, err
				}
			}
		}
	}
	return result, nil
}
