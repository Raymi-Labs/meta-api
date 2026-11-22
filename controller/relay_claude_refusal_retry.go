package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// claudeRefusalRetryAllowed determines whether a Claude refusal can be retried by switching channels.
// This function is called before each relay attempt to pre-authorize the retry mechanism.
func claudeRefusalRetryAllowed(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam, channel *model.Channel, refusalRetries int) bool {
	// Only Claude and OpenAI formats are supported
	if info.RelayFormat != types.RelayFormatClaude && info.RelayFormat != types.RelayFormatOpenAI {
		return false
	}

	// Check retry budget and channel usage
	used := len(c.GetStringSlice("use_channel"))
	if common.RetryTimes < 1 || refusalRetries >= common.RetryTimes || used < 1 || used > common.RetryTimes {
		return false
	}

	// Do not retry if a specific channel was requested
	if _, fixed := c.Get("specific_channel_id"); fixed {
		return false
	}

	// Check if channel affinity settings allow retry after failure
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}

	// Check if the request is replayable (no pass-through, no param override, self-contained)
	setting, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if !canReplayConversationRequest(c, info, setting.PassThroughBodyEnabled, len(common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)) > 0) {
		return false
	}

	// Check if there are alternate channels available
	return service.HasAlternateChannelCandidate(retryParam, channel.Id)
}
