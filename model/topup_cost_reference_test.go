package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRecentTopUpCostSamplesFilterCurrencyProviderStatusAndAge(t *testing.T) {
	setupPaymentRecordTestDB(t)
	now := int64(1793880000)
	base := TopUp{Amount: 100, PaidMoney: 3, PaidCurrency: "USD", PresentmentMoney: 20.81, PresentmentCurrency: "CNY", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, CompleteTime: now - 1, Status: common.TopUpStatusSuccess}
	for index, change := range []func(*TopUp){
		func(*TopUp) {},
		func(v *TopUp) { v.PaymentProvider = "" },
		func(v *TopUp) { v.Status = common.TopUpStatusPending },
		func(v *TopUp) { v.PaidCurrency = "EUR" },
		func(v *TopUp) { v.PresentmentCurrency = "USD" },
		func(v *TopUp) { v.PaymentProvider = PaymentProviderEpay },
		func(v *TopUp) { v.PaymentMethod = "alipay" },
		func(v *TopUp) { v.CompleteTime = now - 31*86400 },
		func(v *TopUp) { v.CompleteTime = now + 1 },
	} {
		row := base
		row.TradeNo = string(rune('A' + index))
		change(&row)
		require.NoError(t, DB.Create(&row).Error)
	}
	samples, err := GetRecentTopUpCostSamples(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, samples, 2)
	for i := range 30 {
		row := base
		row.TradeNo = "bounded-" + string(rune('A'+i))
		require.NoError(t, DB.Create(&row).Error)
	}
	samples, err = GetRecentTopUpCostSamples(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, samples, 20)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetRecentTopUpCostSamples(ctx, now)
	require.Error(t, err)
}
