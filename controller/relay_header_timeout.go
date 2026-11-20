package controller

import (
	"bytes"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func canFailoverResponseHeaderTimeout(c *gin.Context, info *relaycommon.RelayInfo, err *types.NewAPIError) bool {
	if c == nil || c.Request == nil || info == nil || err == nil || !err.ResponseHeaderTimeout || err.StatusCode != http.StatusGatewayTimeout || common.RetryTimes < 1 || len(c.GetStringSlice("use_channel")) < 1 || len(c.GetStringSlice("use_channel")) > common.RetryTimes {
		return false
	}
	return canReplayConversationBeforeOutput(c, info)
}

func canFailoverClaudeCapability(c *gin.Context, info *relaycommon.RelayInfo, err *types.NewAPIError) bool {
	if c == nil || c.Request == nil || info == nil || common.RetryTimes < 1 || len(c.GetStringSlice("use_channel")) < 1 || len(c.GetStringSlice("use_channel")) > common.RetryTimes || !service.IsClaudeUpstreamCapabilityError(err) {
		return false
	}
	return canReplayConversationBeforeOutput(c, info)
}

func canReplayConversationBeforeOutput(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if _, fixed := c.Get("specific_channel_id"); fixed {
		return false
	}
	// Opaque pass-through bodies and channel parameter overrides may add provider
	// state or server tools absent from the parsed client request.
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || (info.ChannelMeta != nil && (info.ChannelSetting.PassThroughBodyEnabled || len(info.ParamOverride) > 0)) {
		return false
	}
	// Only self-contained conversational requests. Task creation, Responses state,
	// provider containers and server-side tools cannot be safely replayed here.
	switch req := info.Request.(type) {
	case *dto.GeneralOpenAIRequest:
		if c.Request.URL.Path != "/v1/chat/completions" || req == nil || len(req.ExtraBody) > 0 || len(req.Audio) > 0 || len(req.Modalities) > 0 || req.WebSearchOptions != nil || hasTimeoutReplayState(req.SearchParameters) || hasTimeoutReplayState(req.EnableSearch) || hasTimeoutReplayState(req.WebSearch) || hasTimeoutReplayState(req.ImageConfig) {
			return false
		}
		for _, tool := range req.Tools {
			if tool.Type != "function" {
				return false
			}
		}
	case *dto.ClaudeRequest:
		if c.Request.URL.Path != "/v1/messages" || req == nil || hasTimeoutReplayState(req.Container) || hasTimeoutReplayState(req.McpServers) {
			return false
		}
		if req.Tools != nil {
			data, marshalErr := common.Marshal(req.Tools)
			var tools []struct {
				Type string `json:"type"`
			}
			if marshalErr != nil || common.Unmarshal(data, &tools) != nil {
				return false
			}
			for _, tool := range tools {
				if tool.Type != "" && tool.Type != "custom" {
					return false
				}
			}
		}
	default:
		return false
	}
	return helper.CanRetryHeaderTimeoutBeforeOutput(c, info.RelayFormat)
}

func hasTimeoutReplayState(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) > 0 && !bytes.Equal(data, []byte("null"))
}
