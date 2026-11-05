package controller

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type topUpCostReference struct {
	Method            string  `json:"method"`
	BaselineMethod    string  `json:"baseline_method"`
	SampleCount       int     `json:"sample_count"`
	SampleAmount      int64   `json:"sample_amount"`
	ObservedAt        int64   `json:"observed_at"`
	ExpiresAt         int64   `json:"expires_at"`
	BaselineAmount    float64 `json:"baseline_amount"`
	ReferencePaid     float64 `json:"reference_paid"`
	ReferenceUSD      float64 `json:"reference_usd"`
	DifferencePercent float64 `json:"difference_percent"`
}

func buildTopUpCostReference(samples []model.TopUpCostSample, now int64, epayQuote, stripeQuote func(int64) float64) *topUpCostReference {
	finitePositive := func(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
	ref := &topUpCostReference{Method: model.PaymentMethodStripe, BaselineMethod: "alipay"}
	total := decimal.Zero
	for _, sample := range samples {
		if sample.Amount <= 0 || sample.CompleteTime < now-30*86400 || sample.CompleteTime > now ||
			!finitePositive(sample.PaidMoney) || !finitePositive(sample.PresentmentMoney) {
			continue
		}
		base, usd := epayQuote(sample.Amount), stripeQuote(sample.Amount)
		if !finitePositive(base) || !finitePositive(usd) || math.Abs(sample.PaidMoney-usd) > 0.005 {
			continue
		}
		base = decimal.NewFromFloat(base).Round(2).InexactFloat64()
		if base <= 0 {
			continue
		}
		if ref.SampleCount > 0 && sample.Amount != ref.SampleAmount {
			continue
		}
		ref.SampleAmount, ref.BaselineAmount, ref.ReferenceUSD = sample.Amount, base, usd
		if sample.CompleteTime > ref.ObservedAt {
			ref.ObservedAt = sample.CompleteTime
		}
		// Expire when the oldest included sample leaves the eligible window.
		if ref.ExpiresAt == 0 || sample.CompleteTime+30*86400 < ref.ExpiresAt {
			ref.ExpiresAt = sample.CompleteTime + 30*86400
		}
		ref.SampleCount++
		total = total.Add(decimal.NewFromFloat(sample.PresentmentMoney))
		if ref.SampleCount == 3 {
			break
		}
	}
	if ref.SampleCount < 3 {
		return nil
	}
	mean := total.Div(decimal.NewFromInt(int64(ref.SampleCount)))
	ref.ReferencePaid = mean.Round(2).InexactFloat64()
	ref.DifferencePercent = mean.Div(decimal.NewFromFloat(ref.BaselineAmount)).Sub(decimal.NewFromInt(1)).Mul(decimal.NewFromInt(100)).Round(2).InexactFloat64()
	if math.IsNaN(ref.DifferencePercent) || math.IsInf(ref.DifferencePercent, 0) {
		return nil
	}
	ref.BaselineAmount = decimal.NewFromFloat(ref.BaselineAmount).Round(2).InexactFloat64()
	return ref
}

func getTopUpCostReference(c *gin.Context) *topUpCostReference {
	if c.GetInt("id") <= 0 || !isStripeTopUpEnabled() || !isEpayTopUpEnabled() ||
		!operation_setting.ContainsPayMethod("alipay") ||
		operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return nil
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		return nil
	}
	now := time.Now().Unix()
	samples, err := model.GetRecentTopUpCostSamples(c.Request.Context(), now)
	if err != nil {
		return nil
	}
	return buildTopUpCostReference(samples, now,
		func(amount int64) float64 { return service.CalculateEpayTopUpPayMoney(amount, group) },
		func(amount int64) float64 { return service.CalculateStripeTopUpPayMoney(float64(amount), group) })
}
