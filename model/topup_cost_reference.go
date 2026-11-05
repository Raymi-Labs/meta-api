package model

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// TopUpCostSample deliberately excludes account and order identifiers.
type TopUpCostSample struct {
	Amount           int64
	PaidMoney        float64
	PresentmentMoney float64
	CompleteTime     int64
}

func GetRecentTopUpCostSamples(ctx context.Context, now int64) ([]TopUpCostSample, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var samples []TopUpCostSample
	err := DB.WithContext(ctx).Model(&TopUp{}).
		Select("amount, paid_money, presentment_money, complete_time").
		Where("status = ? AND payment_method = ?", common.TopUpStatusSuccess, PaymentMethodStripe).
		Where("payment_provider = ? OR payment_provider = ?", PaymentProviderStripe, "").
		Where("paid_currency = ? AND presentment_currency = ?", "USD", "CNY").
		Where("complete_time >= ? AND complete_time <= ?", now-30*86400, now).
		Order("id DESC").Limit(20).Find(&samples).Error
	return samples, err
}
