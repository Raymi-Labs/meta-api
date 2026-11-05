package controller

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestTopUpCostReferenceIsHistoricalComparisonNotFee(t *testing.T) {
	now := int64(1793880000)
	samples := []model.TopUpCostSample{
		{Amount: 100, PaidMoney: 3, PresentmentMoney: 20.81, CompleteTime: now - 10},
		{Amount: 100, PaidMoney: 3, PresentmentMoney: 20.81, CompleteTime: now - 20},
		{Amount: 100, PaidMoney: 3, PresentmentMoney: 20.81, CompleteTime: now - 30},
	}
	ep := func(a int64) float64 { return float64(a) * .2 }
	sp := func(a int64) float64 { return float64(a) * .03 }
	ref := buildTopUpCostReference(samples, now, ep, sp)
	require.NotNil(t, ref)
	require.Equal(t, 4.05, ref.DifferencePercent)
	require.Equal(t, 20.0, ref.BaselineAmount)
	require.Equal(t, 20.81, ref.ReferencePaid)
	require.Equal(t, now-30+30*86400, ref.ExpiresAt)
	require.Nil(t, buildTopUpCostReference(samples[:2], now, ep, sp))
	require.Nil(t, buildTopUpCostReference(samples, now+31*86400, ep, sp))
	require.Nil(t, buildTopUpCostReference(samples, now, ep, func(int64) float64 { return 4 }))
	// Different current CNY pricing changes the comparison, never the order.
	ref = buildTopUpCostReference(samples, now, func(int64) float64 { return 25 }, sp)
	require.Equal(t, -16.76, ref.DifferencePercent)
	encoded, err := common.Marshal(ref)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "user_id")
	require.NotContains(t, string(encoded), "trade_no")
	for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		require.Nil(t, buildTopUpCostReference(samples, now, func(int64) float64 { return bad }, sp))
		changed := append([]model.TopUpCostSample(nil), samples...)
		changed[1].PresentmentMoney = bad
		require.Nil(t, buildTopUpCostReference(changed, now, ep, sp))
	}
	changed := append([]model.TopUpCostSample(nil), samples...)
	changed[1].Amount, changed[1].PaidMoney = 200, 6
	require.Nil(t, buildTopUpCostReference(changed, now, ep, sp))
	changed[1] = samples[1]
	changed[1].CompleteTime = now + 1
	require.Nil(t, buildTopUpCostReference(changed, now, ep, sp))
}
