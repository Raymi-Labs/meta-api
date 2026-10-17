package dto

import (
	"fmt"
	"strings"
)

type ChatCompletionsToResponsesMode string

const (
	ChatCompletionsToResponsesModeInherit  ChatCompletionsToResponsesMode = "inherit"
	ChatCompletionsToResponsesModeEnabled  ChatCompletionsToResponsesMode = "enabled"
	ChatCompletionsToResponsesModeDisabled ChatCompletionsToResponsesMode = "disabled"
)

type CapabilityMode string

const (
	CapabilityModeInherit  CapabilityMode = "inherit"
	CapabilityModeEnabled  CapabilityMode = "enabled"
	CapabilityModeDisabled CapabilityMode = "disabled"
)

func NormalizeCapabilityMode(mode CapabilityMode) CapabilityMode {
	switch mode {
	case "", CapabilityModeInherit:
		return CapabilityModeInherit
	case CapabilityModeEnabled:
		return CapabilityModeEnabled
	case CapabilityModeDisabled:
		return CapabilityModeDisabled
	default:
		return CapabilityModeInherit
	}
}

func NormalizeChatCompletionsToResponsesMode(mode ChatCompletionsToResponsesMode) ChatCompletionsToResponsesMode {
	switch mode {
	case "", ChatCompletionsToResponsesModeInherit:
		return ChatCompletionsToResponsesModeInherit
	case ChatCompletionsToResponsesModeEnabled:
		return ChatCompletionsToResponsesModeEnabled
	case ChatCompletionsToResponsesModeDisabled:
		return ChatCompletionsToResponsesModeDisabled
	default:
		return ChatCompletionsToResponsesModeInherit
	}
}

type ClaudeImageTransportMode string

const (
	ClaudeImageTransportModeInherit ClaudeImageTransportMode = "inherit"
	ClaudeImageTransportModeAuto    ClaudeImageTransportMode = "auto"
	ClaudeImageTransportModeData    ClaudeImageTransportMode = "data"
	ClaudeImageTransportModeBridge  ClaudeImageTransportMode = "bridge"
)

func NormalizeClaudeImageTransportMode(mode ClaudeImageTransportMode) ClaudeImageTransportMode {
	switch mode {
	case "", ClaudeImageTransportModeInherit:
		return ClaudeImageTransportModeInherit
	case ClaudeImageTransportModeAuto:
		return ClaudeImageTransportModeAuto
	case ClaudeImageTransportModeData:
		return ClaudeImageTransportModeData
	case ClaudeImageTransportModeBridge:
		return ClaudeImageTransportModeBridge
	default:
		return ClaudeImageTransportModeInherit
	}
}

type ClientRestrictionMode string

const (
	ClientRestrictionModeNone      ClientRestrictionMode = ""
	ClientRestrictionModeAllowlist ClientRestrictionMode = "allowlist"
	ClientRestrictionModeBlocklist ClientRestrictionMode = "blocklist"
)

type QuotaPolicy struct {
	Enabled    bool   `json:"enabled,omitempty"`
	Period     string `json:"period,omitempty"`
	QuotaLimit int64  `json:"quota_limit,omitempty"`
	CountLimit int64  `json:"count_limit,omitempty"`
	AnchorTime int64  `json:"anchor_time,omitempty"`
}

func (p QuotaPolicy) IsActive() bool {
	return p.Enabled && (p.QuotaLimit > 0 || p.CountLimit > 0)
}

func (p QuotaPolicy) Validate() error {
	if p.QuotaLimit < 0 {
		return fmt.Errorf("quota_limit cannot be negative")
	}
	if p.CountLimit < 0 {
		return fmt.Errorf("count_limit cannot be negative")
	}
	if p.Period == "" && !p.IsActive() {
		return nil
	}
	switch p.Period {
	case "day", "week", "month":
		return nil
	default:
		return fmt.Errorf("invalid quota_policy period: %s", p.Period)
	}
}

type ChannelSettings struct {
	// 0 inherits the default; only non-streaming text relay requests use this override.
	NonStreamResponseHeaderTimeoutSec int `json:"nonstream_response_header_timeout_sec,omitempty"`

	CPAUserIdentityEnabled         bool                           `json:"cpa_user_identity_enabled,omitempty"`
	CPAInstanceID                  string                         `json:"cpa_instance_id,omitempty"`
	ForceFormat                    bool                           `json:"force_format,omitempty"`
	ThinkingToContent              bool                           `json:"thinking_to_content,omitempty"`
	Proxy                          string                         `json:"proxy"`
	PassThroughBodyEnabled         bool                           `json:"pass_through_body_enabled,omitempty"`
	SystemPrompt                   string                         `json:"system_prompt,omitempty"`
	SystemPromptOverride           bool                           `json:"system_prompt_override,omitempty"`
	ChatCompletionsToResponsesMode ChatCompletionsToResponsesMode `json:"chat_completions_to_responses_mode,omitempty"`
	ClaudeImageTransportMode       ClaudeImageTransportMode       `json:"claude_image_transport_mode,omitempty"`
	ClaudeIncrementalCacheEnabled  bool                           `json:"claude_incremental_cache_enabled,omitempty"`
	ClientRestrictionMode          ClientRestrictionMode          `json:"client_restriction_mode,omitempty"`
	ClientRestrictionClients       []string                       `json:"client_restriction_clients,omitempty"`
	QuotaPolicy                    QuotaPolicy                    `json:"quota_policy,omitempty"`
	CRSSiteID                      int                            `json:"crs_site_id,omitempty"`
	CRSPlatform                    string                         `json:"crs_platform,omitempty"`
	CRSAutoManage                  bool                           `json:"crs_auto_manage,omitempty"`
	// MaxConcurrency 覆盖该渠道的在途并发上限（>0 时生效）。0/缺省表示沿用全局
	// channel_concurrency_setting.default_max_concurrency。仅在全局开关开启时才起作用。
	MaxConcurrency int `json:"max_concurrency,omitempty"`
	// RpmLimit 覆盖该渠道的 RPM 上限（>0 时生效）。0/缺省表示沿用全局
	// channel_concurrency_setting.default_rpm_limit；全局也为 0 时表示不限制。
	RpmLimit int `json:"rpm_limit,omitempty"`
}

func (s ChannelSettings) Validate() error {
	if s.NonStreamResponseHeaderTimeoutSec < 0 || s.NonStreamResponseHeaderTimeoutSec > 600 {
		return fmt.Errorf("nonstream_response_header_timeout_sec must be an integer from 0 to 600")
	}
	if err := s.ValidateCPAIdentity(); err != nil {
		return err
	}
	if err := s.ValidateChatCompletionsToResponsesMode(); err != nil {
		return err
	}
	if err := s.ValidateClaudeImageTransportMode(); err != nil {
		return err
	}
	if err := s.ValidateClientRestriction(); err != nil {
		return err
	}
	if err := s.QuotaPolicy.Validate(); err != nil {
		return err
	}
	if s.MaxConcurrency < 0 {
		return fmt.Errorf("max_concurrency cannot be negative")
	}
	if s.RpmLimit < 0 {
		return fmt.Errorf("rpm_limit cannot be negative")
	}
	if s.CRSSiteID < 0 {
		return fmt.Errorf("crs_site_id cannot be negative")
	}
	if s.CRSAutoManage && s.CRSSiteID <= 0 {
		return fmt.Errorf("crs_auto_manage requires crs_site_id")
	}
	if s.CRSAutoManage && s.CRSPlatform != "openai" && s.CRSPlatform != "openai-responses" {
		return fmt.Errorf("crs_auto_manage requires a valid crs_platform")
	}
	return nil
}

func (s ChannelSettings) ValidateClientRestriction() error {
	switch s.ClientRestrictionMode {
	case "", ClientRestrictionModeBlocklist:
		return nil
	case ClientRestrictionModeAllowlist:
		if len(NormalizeClientRestrictionClients(s.ClientRestrictionClients)) == 0 {
			return fmt.Errorf("allowlist client_restriction_clients cannot be empty")
		}
		return nil
	default:
		return fmt.Errorf("invalid client_restriction_mode: %s", s.ClientRestrictionMode)
	}
}

func NormalizeClientRestrictionClients(clients []string) []string {
	if len(clients) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(clients))
	seen := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		client = strings.TrimSpace(client)
		if client == "" {
			continue
		}
		if _, ok := seen[client]; ok {
			continue
		}
		seen[client] = struct{}{}
		normalized = append(normalized, client)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func (s ChannelSettings) ValidateChatCompletionsToResponsesMode() error {
	switch s.ChatCompletionsToResponsesMode {
	case "", ChatCompletionsToResponsesModeInherit, ChatCompletionsToResponsesModeEnabled, ChatCompletionsToResponsesModeDisabled:
		return nil
	default:
		return fmt.Errorf("invalid chat_completions_to_responses_mode: %s", s.ChatCompletionsToResponsesMode)
	}
}

func (s ChannelSettings) GetChatCompletionsToResponsesMode() ChatCompletionsToResponsesMode {
	return NormalizeChatCompletionsToResponsesMode(s.ChatCompletionsToResponsesMode)
}

func (s ChannelSettings) ValidateClaudeImageTransportMode() error {
	switch s.ClaudeImageTransportMode {
	case "", ClaudeImageTransportModeInherit, ClaudeImageTransportModeAuto, ClaudeImageTransportModeData, ClaudeImageTransportModeBridge:
		return nil
	default:
		return fmt.Errorf("invalid claude_image_transport_mode: %s", s.ClaudeImageTransportMode)
	}
}

func (s ChannelSettings) GetClaudeImageTransportMode() ClaudeImageTransportMode {
	return NormalizeClaudeImageTransportMode(s.ClaudeImageTransportMode)
}

type VertexKeyType string

const (
	VertexKeyTypeJSON   VertexKeyType = "json"
	VertexKeyTypeAPIKey VertexKeyType = "api_key"
)

type AwsKeyType string

const (
	AwsKeyTypeAKSK   AwsKeyType = "ak_sk" // default
	AwsKeyTypeApiKey AwsKeyType = "api_key"
)

type ChannelOtherSettings struct {
	VertexProjectID                       string         `json:"vertex_project_id,omitempty"`
	AzureResponsesVersion                 string         `json:"azure_responses_version,omitempty"`
	VertexKeyType                         VertexKeyType  `json:"vertex_key_type,omitempty"` // "json" or "api_key"
	OpenRouterEnterprise                  *bool          `json:"openrouter_enterprise,omitempty"`
	ChatStreamOptionsMode                 CapabilityMode `json:"chat_stream_options_mode,omitempty"`
	ResponsesAPIMode                      CapabilityMode `json:"responses_api_mode,omitempty"`
	ResponsesStreamOptionsMode            CapabilityMode `json:"responses_stream_options_mode,omitempty"`
	ClaudeBetaQuery                       bool           `json:"claude_beta_query,omitempty"`         // Claude channel always appends ?beta=true
	AllowServiceTier                      bool           `json:"allow_service_tier,omitempty"`        // Allow service_tier passthrough.
	AllowInferenceGeo                     bool           `json:"allow_inference_geo,omitempty"`       // Allow inference_geo passthrough for Claude.
	AllowSpeed                            bool           `json:"allow_speed,omitempty"`               // Allow speed passthrough for Claude.
	DisableStore                          bool           `json:"disable_store,omitempty"`             // Disable store passthrough when enabled.
	AllowSafetyIdentifier                 bool           `json:"allow_safety_identifier,omitempty"`   // Allow safety_identifier passthrough.
	AllowIncludeObfuscation               bool           `json:"allow_include_obfuscation,omitempty"` // Allow stream_options.include_obfuscation passthrough.
	AwsKeyType                            AwsKeyType     `json:"aws_key_type,omitempty"`
	UpstreamModelUpdateCheckEnabled       bool           `json:"upstream_model_update_check_enabled,omitempty"`        // Detect upstream model updates.
	UpstreamModelUpdateAutoSyncEnabled    bool           `json:"upstream_model_update_auto_sync_enabled,omitempty"`    // Auto sync upstream model updates.
	UpstreamModelUpdateLastCheckTime      int64          `json:"upstream_model_update_last_check_time,omitempty"`      // Last detection time.
	UpstreamModelUpdateLastDetectedModels []string       `json:"upstream_model_update_last_detected_models,omitempty"` // Models that can be added.
	UpstreamModelUpdateLastRemovedModels  []string       `json:"upstream_model_update_last_removed_models,omitempty"`  // Models that can be removed.
	UpstreamModelUpdateIgnoredModels      []string       `json:"upstream_model_update_ignored_models,omitempty"`       // Manually ignored models.
}

func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}

func (s ChannelOtherSettings) GetChatStreamOptionsMode() CapabilityMode {
	return NormalizeCapabilityMode(s.ChatStreamOptionsMode)
}

func (s ChannelOtherSettings) GetResponsesAPIMode() CapabilityMode {
	return NormalizeCapabilityMode(s.ResponsesAPIMode)
}

func (s ChannelOtherSettings) GetResponsesStreamOptionsMode() CapabilityMode {
	return NormalizeCapabilityMode(s.ResponsesStreamOptionsMode)
}

func (s ChannelOtherSettings) ValidateCapabilityModes() error {
	for _, entry := range []struct {
		name string
		mode CapabilityMode
	}{
		{name: "chat_stream_options_mode", mode: s.ChatStreamOptionsMode},
		{name: "responses_api_mode", mode: s.ResponsesAPIMode},
		{name: "responses_stream_options_mode", mode: s.ResponsesStreamOptionsMode},
	} {
		switch entry.mode {
		case "", CapabilityModeInherit, CapabilityModeEnabled, CapabilityModeDisabled:
		default:
			return fmt.Errorf("invalid %s: %s", entry.name, entry.mode)
		}
	}
	return nil
}
