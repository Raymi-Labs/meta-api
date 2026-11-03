package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func usesChatWalletReservation(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil || c == nil || info.RequestId == "" || !info.TokenUnlimited || info.ForcePreConsume || info.TaskRelayInfo != nil || info.IsChannelTest || info.ImageRequestCount > 0 || info.TieredBillingSnapshot != nil || info.PriceData.InitialChatQuota <= 0 || info.PriceData.UsePrice || info.PriceData.UseAudioDurationPrice || common.GetTrustQuota() <= 0 || common.GetContextKeyBool(c, constant.ContextKeyTokenPackageEnabled) {
		return false
	}
	// Text chat endpoints can also request generated images/audio. Those keep
	// their existing full reservation rules rather than borrowing this policy.
	switch request := info.Request.(type) {
	case *dto.GeneralOpenAIRequest:
		for index := range request.Messages {
			for _, content := range request.Messages[index].ParseContent() {
				if content.Type == dto.ContentTypeInputAudio || content.Type == dto.ContentTypeVideoUrl {
					return false
				}
			}
		}
		if len(request.Audio) > 0 && string(request.Audio) != "null" {
			return false
		}
		if len(request.Modalities) > 0 {
			var modalities []string
			if common.Unmarshal(request.Modalities, &modalities) != nil {
				return false
			}
			for _, modality := range modalities {
				if !strings.EqualFold(modality, "text") {
					return false
				}
			}
		}
	case *dto.GeminiChatRequest:
		for _, content := range request.Contents {
			for _, part := range content.Parts {
				if part.InlineData != nil && isAudioVideoMIME(part.InlineData.MimeType) {
					return false
				}
				if part.FileData != nil && isAudioVideoMIME(part.FileData.MimeType) {
					return false
				}
			}
		}
		for _, modality := range request.GenerationConfig.ResponseModalities {
			if !strings.EqualFold(modality, "text") {
				return false
			}
		}
	case *dto.OpenAIResponsesRequest:
		for _, tool := range gjson.ParseBytes(request.Tools).Array() {
			if tool.Get("type").String() == "image_generation" {
				return false
			}
		}
	}
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return true
	case types.RelayFormatOpenAI:
		return info.RelayMode == relayconstant.RelayModeChatCompletions || info.RelayMode == relayconstant.RelayModeCompletions
	case types.RelayFormatOpenAIResponses:
		return info.RelayMode == relayconstant.RelayModeResponses
	case types.RelayFormatGemini:
		return c.Request != nil && (strings.HasSuffix(c.Request.URL.Path, ":generateContent") || strings.HasSuffix(c.Request.URL.Path, ":streamGenerateContent"))
	}
	return false
}

func isAudioVideoMIME(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	return strings.HasPrefix(mime, "audio/") || strings.HasPrefix(mime, "video/")
}

func newChatWalletSession(c *gin.Context, info *relaycommon.RelayInfo, full int) (*BillingSession, *types.NewAPIError) {
	receipt, err := model.ReserveChatWalletQuota(info.UserId, info.RequestId, full, info.PriceData.InitialChatQuota, common.GetTrustQuota())
	if err != nil {
		var rejection *types.QuotaRejection
		if errors.As(err, &rejection) && rejection != nil {
			switch rejection.RejectionReason() {
			case types.QuotaReasonInFlight:
				rejection.Message = fmt.Sprintf("当前可用余额不足以预留本次请求，其他请求仍占用额度；请等待其结束后重试（可用 %s，需要 %s）", logger.FormatQuota(int(rejection.Available)), logger.FormatQuota(int(rejection.Required)))
			case types.QuotaReasonRequestTooLarge:
				rejection.Message = fmt.Sprintf("本次请求预估费用超过可用余额，余额尚未用尽；请缩短上下文、降低输出上限或补充额度（可用 %s，需要 %s）", logger.FormatQuota(int(rejection.Available)), logger.FormatQuota(int(rejection.Required)))
			default:
				rejection.Message = fmt.Sprintf("用户额度不足, 剩余额度: %s", logger.FormatQuota(int(rejection.Available)))
			}
			return nil, types.NewErrorWithStatusCode(rejection, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return nil, types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	info.UserQuota = receipt.AvailableBefore
	wallet := &WalletFunding{userId: info.UserId, requestID: info.RequestId, consumed: int(receipt.Quota), reservation: receipt, requireAvailableQuota: true, allowSettlementOverdraft: true}
	session := &BillingSession{relayInfo: info, funding: wallet, preConsumedQuota: int(receipt.Quota), chatReservation: true}
	// The unrestricted token is usage-only. Count it once at actual settlement,
	// avoiding a fictitious remaining-token credit when the wallet is refunded.
	session.syncRelayInfo()
	logger.LogInfo(c, fmt.Sprintf("用户 %d 同步聊天首笔预留 %s，原始预估 %s", info.UserId, logger.FormatQuota(session.preConsumedQuota), logger.FormatQuota(full)))
	return session, nil
}

// A failed fallback must not turn a positive balance or temporary hold in the
// preferred source into an exhausted-account verdict. Infrastructure failures
// and successful fallbacks always retain their own outcome.
func preferAvailableFundingRejection(primary, fallback *types.NewAPIError) *types.NewAPIError {
	if fallback == nil || fallback.GetErrorCode() != types.ErrorCodeInsufficientUserQuota {
		return fallback
	}
	var first, second *types.QuotaRejection
	if !errors.As(primary, &first) || first == nil || !errors.As(fallback, &second) || second == nil {
		return fallback
	}
	if second.RejectionReason() == types.QuotaReasonExhausted && (first.RejectionReason() == types.QuotaReasonRequestTooLarge || first.RejectionReason() == types.QuotaReasonInFlight) {
		return primary
	}
	return fallback
}
