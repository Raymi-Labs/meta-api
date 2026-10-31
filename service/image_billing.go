package service

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// PrepareImageBilling reserves the effective outbound quantity on each attempt.
// The immutable base prevents a retry from multiplying an earlier reservation.
// Expressions use actual image quantities without multiplying token totals again.
func PrepareImageBilling(c *gin.Context, info *relaycommon.RelayInfo, count int, promptExtend bool, reserveQuota ...bool) *types.NewAPIError {
	if count < 1 || count > dto.MaxImageN {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid image count: %d", count), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if info.ImageBasePriceData == nil {
		base := info.PriceData
		info.ImageBasePriceData = &base
		input := billingexpr.RequestInput{}
		if info.BillingRequestInput != nil {
			input = *info.BillingRequestInput
			input.Body = bytes.Clone(input.Body)
		}
		info.ImageBaseBillingInput = &input
	}
	base := info.ImageBasePriceData
	info.ImageRequestCount = count
	info.ImageResponseCount = nil
	// Remove only image-specific multipliers; other local billing settings stay.
	info.PriceData.AddOtherRatio("n", 1)
	info.PriceData.AddOtherRatio("prompt_extend", 1)
	if info.TieredBillingSnapshot == nil {
		if info.PriceData.UsePrice || info.ChannelType == constant.ChannelTypeAli {
			info.PriceData.AddOtherRatio("n", float64(count))
		}
		if info.ChannelType == constant.ChannelTypeAli && strings.Contains(info.UpstreamModelName, "z-image") && promptExtend {
			info.PriceData.AddOtherRatio("prompt_extend", 2)
		}
	}
	quota := float64(base.QuotaToPreConsume)
	conservative := float64(base.ConservativeQuotaToPreConsume)
	if base.UsePrice {
		// Include local time/group and legacy DALL-E size/quality adjustments.
		quota = base.ModelPrice * common.QuotaPerUnit * base.GroupRatioInfo.GroupRatio * base.TimeRatioInfo.EffectiveRatio()
		conservative = math.Max(conservative, quota)
	}
	if snap := info.TieredBillingSnapshot; snap != nil {
		// Keep every original field/header except quantity. Rebuild from the
		// frozen input each time so provider-specific paths cannot leak on retry.
		input := *info.ImageBaseBillingInput
		var err error
		input.Body, err = sjson.SetBytes(bytes.Clone(input.Body), "n", count)
		if err == nil && (info.ChannelType == constant.ChannelTypeAli || gjson.GetBytes(input.Body, "parameters.n").Exists()) {
			input.Body, err = sjson.SetBytes(input.Body, "parameters.n", count)
		}
		if err == nil && gjson.GetBytes(input.Body, "parameters.sampleCount").Exists() {
			input.Body, err = sjson.SetBytes(input.Body, "parameters.sampleCount", count)
		}
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if common.IsImagePriceExpression(snap.ExprString) {
			if info.ImagePricingInput == nil {
				return types.NewError(fmt.Errorf("image pricing dimensions missing"), types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
			}
			input.Body, err = withImagePricing(input.Body, info.ImagePricingInput)
			if err != nil {
				return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
			}
		}
		cost, trace, err := billingexpr.RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, billingexpr.TokenParams{
			P: float64(snap.EstimatedPromptTokens), C: float64(snap.EstimatedCompletionTokens), Len: float64(snap.EstimatedPromptTokens),
		}, input)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		quota = cost / 1_000_000 * snap.QuotaPerUnit * snap.GroupRatio * base.TimeRatioInfo.EffectiveRatio()
		conservative = quota
		info.BillingRequestInput = &input
		snap.EstimatedQuotaBeforeGroup = cost / 1_000_000 * snap.QuotaPerUnit
		snap.EstimatedTier = trace.MatchedTier
	} else {
		for _, ratio := range info.PriceData.OtherRatios {
			if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 {
				return types.NewErrorWithStatusCode(fmt.Errorf("invalid image billing ratio"), types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			quota *= ratio
			conservative *= ratio
		}
	}
	reserve, err := billingexpr.QuotaRoundChecked(quota)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	conservativeReserve, err := billingexpr.QuotaRoundChecked(conservative)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	info.PriceData.QuotaToPreConsume = reserve
	if info.TieredBillingSnapshot != nil {
		info.TieredBillingSnapshot.EstimatedQuotaAfterGroup = reserve
	}
	info.PriceData.ConservativeQuotaToPreConsume = max(reserve, conservativeReserve)
	if common.GetContextKeyBool(c, constant.ContextKeyTokenPackageEnabled) &&
		common.GetContextKeyString(c, constant.ContextKeyTokenBillingMode) == model.TokenBillingModeTokenOnly {
		reserve = info.PriceData.ConservativeQuotaToPreConsume
	}
	if base.FreeModel && reserve == 0 && info.Billing == nil {
		return nil
	}
	if len(reserveQuota) > 0 && !reserveQuota[0] {
		return nil
	}
	// Quantity is known now: even a normally trusted wallet must cover it.
	info.ForcePreConsume = true
	if info.Billing == nil {
		return PreConsumeBilling(c, reserve, info)
	}
	if err := info.Billing.Reserve(reserve); err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	return nil
}

// ApplyImageResponseQuantity changes only settlement inputs, never the frozen
// request used to reserve quota. Unknown adapter counts retain request billing.
func ApplyImageResponseQuantity(info *relaycommon.RelayInfo) (int, error) {
	count := info.ImageRequestCount
	if info.ImageResponseCount == nil {
		return count, nil
	}
	count = *info.ImageResponseCount
	if count < 1 {
		return 0, errors.New("upstream returned no image payload")
	}
	if info.TieredBillingSnapshot == nil {
		if info.PriceData.UsePrice || info.ChannelType == constant.ChannelTypeAli {
			info.PriceData.AddOtherRatio("n", float64(count))
		}
	} else if info.BillingRequestInput != nil {
		input := *info.BillingRequestInput
		body, err := sjson.SetBytes(bytes.Clone(input.Body), "n", count)
		if err != nil {
			return 0, err
		}
		for _, path := range []string{"parameters.n", "parameters.sampleCount"} {
			if gjson.GetBytes(body, path).Exists() {
				body, err = sjson.SetBytes(body, path, count)
				if err != nil {
					return 0, err
				}
			}
		}
		if common.IsImagePriceExpression(info.TieredBillingSnapshot.ExprString) {
			if info.ImagePricingResult == nil {
				return 0, errors.New("image pricing result missing")
			}
			body, err = withImagePricing(body, info.ImagePricingResult)
			if err != nil {
				return 0, err
			}
		}
		input.Body = body
		info.BillingRequestInput = &input
	}
	return count, nil
}
