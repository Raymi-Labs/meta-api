package relay

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/ali"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func AliTTSHelper(c *gin.Context, info *relaycommon.RelayInfo, request *dto.AudioRequest) *types.NewAPIError {
	info.InitChannelMeta(c)
	invalid := func(err error) *types.NewAPIError {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if info.ChannelType != constant.ChannelTypeAli {
		return invalid(errors.New("this streaming speech endpoint requires an Ali channel"))
	}
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return invalid(err)
	}
	if err := service.ReserveAliTTSUsage(info, &dto.Usage{}); err != nil {
		return invalid(err)
	}
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RequireTerminal()
	usage, apiErr := ali.RelayTTS(c, info, request, func(u *dto.Usage) error { return service.ReserveAliTTSUsage(info, u) })
	if apiErr != nil {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeFailed)
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, apiErr)
	} else {
		info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeCompleted)
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	// Settle confirmed usage even on cancellation/partial failure. BillingSession
	// is idempotent, so the controller's failure refund cannot undo this charge.
	if apiErr == nil || (usage != nil && usage.TotalTokens > 0) {
		content := "阿里云 TTS 上游累计用量；每连接结算一次"
		if info.UpstreamModelName != "qwen-audio-3.1-tts-flash" {
			content += "；输入单位为计费字符"
		}
		if apiErr != nil {
			content += "；任务未完整结束，仅结算已确认用量"
		}
		service.PostAudioConsumeQuota(c, info, usage, content)
	}
	return apiErr
}
