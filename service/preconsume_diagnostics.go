package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const preconsumeDiagnosticsKey = "preconsume_diagnostics"

func capturePreconsumeDiagnostics(c *gin.Context, info *relaycommon.RelayInfo, requested int, session *BillingSession, apiErr *types.NewAPIError) {
	if c == nil || info == nil {
		return
	}
	price := info.PriceData
	diagnostic := map[string]interface{}{
		"estimated_prompt_tokens":   price.EstimatedPromptTokens,
		"requested_max_tokens":      price.RequestedMaxTokens,
		"initial_completion_tokens": price.InitialCompletionTokens,
		"estimated_quota":           requested,
		"initial_chat_quota":        price.InitialChatQuota,
		"model_ratio":               price.ModelRatio, "completion_ratio": price.CompletionRatio,
		"group_ratio": price.GroupRatioInfo.GroupRatio, "time_ratio": price.TimeRatioInfo.EffectiveRatio(),
		"quota_per_unit":     common.QuotaPerUnit,
		"token_unlimited":    info.TokenUnlimited,
		"funding_preference": common.NormalizeBillingPreference(info.UserSetting.BillingPreference),
		"policy":             "full_estimate", "accepted": apiErr == nil,
	}
	if !info.TokenUnlimited {
		diagnostic["token_available_quota"] = c.GetInt("token_quota")
	}
	if info.TieredBillingSnapshot != nil {
		diagnostic["policy"] = "tiered_expression"
	}
	if usesChatWalletReservation(c, info) {
		diagnostic["policy"] = "bounded_chat_wallet"
	}
	if session != nil {
		diagnostic["source"] = session.funding.Source()
		if session.chatReservation && info.UserQuota <= common.GetTrustQuota() {
			diagnostic["policy"] = "full_estimate_low_balance"
		}
		diagnostic["reserved_quota"] = session.preConsumedQuota
		if session.funding.Source() == BillingSourceWallet {
			diagnostic["wallet_available_before"] = info.UserQuota
		}
		if session.trusted {
			diagnostic["policy"] = "trusted_no_hold"
		}
		if !session.chatReservation && session.funding.Source() != BillingSourceWallet {
			diagnostic["policy"] = "full_estimate"
		}
	}
	var rejection *types.QuotaRejection
	if apiErr != nil && errors.As(apiErr, &rejection) && rejection != nil {
		diagnostic["source"] = rejection.Source
		diagnostic["rejection_reason"] = rejection.RejectionReason()
		if diagnostic["policy"] == "bounded_chat_wallet" {
			if rejection.Source != BillingSourceWallet {
				diagnostic["policy"] = "full_estimate"
			} else if rejection.Known && rejection.Available <= int64(common.GetTrustQuota()) {
				diagnostic["policy"] = "full_estimate_low_balance"
			}
		}
		if rejection.Known {
			diagnostic["available_quota"] = rejection.Available
			diagnostic["required_quota"] = rejection.Required
		}
	}
	c.Set(preconsumeDiagnosticsKey, diagnostic)
}

func AppendPreconsumeAdminInfo(c *gin.Context, admin map[string]interface{}) {
	if c == nil || admin == nil {
		return
	}
	if value, exists := c.Get(preconsumeDiagnosticsKey); exists {
		admin["preconsume"] = value
	}
}
