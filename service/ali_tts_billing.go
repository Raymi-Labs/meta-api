package service

import (
	"errors"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// A duplex connection settles once. Reserve the caller's cumulative estimate
// plus the normal request window; settlement uses confirmed upstream usage.
func ReserveAliTTSUsage(info *relaycommon.RelayInfo, usage *dto.Usage) error {
	if info.PriceData.UsePrice || info.PriceData.UseAudioDurationPrice || info.TieredBillingSnapshot != nil {
		return errors.New("Ali TTS requires per-unit model/audio ratios; per-call, duration and tiered prices are not supported")
	}
	if info.PriceData.FreeModel {
		return nil
	}
	if info.Billing == nil {
		return errors.New("Ali TTS billing session is missing")
	}
	quota := 0
	if usage.TotalTokens > 0 {
		quota = calculateAudioQuota(QuotaInfo{
			InputDetails:  TokenDetails{TextTokens: usage.PromptTokensDetails.TextTokens},
			OutputDetails: TokenDetails{AudioTokens: usage.CompletionTokenDetails.AudioTokens},
			ModelName:     info.OriginModelName,
			ModelRatio:    info.PriceData.ModelRatio,
			GroupRatio:    info.PriceData.GroupRatioInfo.GroupRatio,
			TimeRatio:     info.PriceData.TimeRatioInfo.EffectiveRatio(),
		})
	}
	return info.Billing.Reserve(quota + max(1, info.PriceData.QuotaToPreConsume))
}
