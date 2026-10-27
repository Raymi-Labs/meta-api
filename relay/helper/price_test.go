package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestTaskExplicitFixedPriceLocksSettlementUnit(t *testing.T) {
	saved, _ := common.Marshal(ratio_setting.GetModelPriceCopy())
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(saved))) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"video-fixed-unit-test":0.2}`))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{OriginModelName: "video-fixed-unit-test", UsingGroup: "default", UserGroup: "default"}
	price, err := ModelPriceHelperTask(c, info)
	require.NoError(t, err)
	require.True(t, price.UsePrice)
	require.Equal(t, 0.2, price.ModelPrice)
	info.OriginModelName = "video-unset-unit-test"
	price, err = ModelPriceHelperTask(c, info)
	require.NoError(t, err)
	require.False(t, price.UsePrice)
}

func TestModelPriceHelperTieredReservesDefaultCompletionWhenUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := billing_setting.TieredBundle{
		BillingMode: billing_setting.GetBillingModeCopy(),
		BillingExpr: billing_setting.GetBillingExprCopy(),
	}
	t.Cleanup(func() { billing_setting.ReplaceBundle(saved) })
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{
		BillingMode: map[string]string{"tiered-reserve-model": billing_setting.BillingModeTieredExpr},
		BillingExpr: map[string]string{"tiered-reserve-model": `tier("base", c * 10)`},
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	priceData, err := ModelPriceHelper(ctx, &relaycommon.RelayInfo{
		OriginModelName: "tiered-reserve-model",
		UsingGroup:      "default",
		UserGroup:       "default",
	}, 0, &types.TokenCountMeta{})
	if err != nil {
		t.Fatal(err)
	}
	want := defaultConservativeCompletionTokens * 10 * int(common.QuotaPerUnit) / 1_000_000
	if priceData.QuotaToPreConsume != want {
		t.Fatalf("QuotaToPreConsume = %d, want %d", priceData.QuotaToPreConsume, want)
	}
	if priceData.ConservativeQuotaToPreConsume != want {
		t.Fatalf("ConservativeQuotaToPreConsume = %d, want %d", priceData.ConservativeQuotaToPreConsume, want)
	}
}

func TestModelPriceHelperUsesMiMoAudioDurationPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalAudioDurationPrices := ratio_setting.AudioDurationPrice2JSONString()
	if err := ratio_setting.UpdateAudioDurationPriceByJSONString(`{"mimo-v2.5-asr":0.074}`); err != nil {
		t.Fatalf("set audio duration price: %v", err)
	}
	t.Cleanup(func() {
		if err := ratio_setting.UpdateAudioDurationPriceByJSONString(originalAudioDurationPrices); err != nil {
			t.Fatalf("restore audio duration price: %v", err)
		}
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		OriginModelName:           "mimo-v2.5-asr",
		UsingGroup:                "default",
		UserGroup:                 "default",
		InputAudioDurationSeconds: 3600,
	}
	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
	if err != nil {
		t.Fatalf("ModelPriceHelper returned error: %v", err)
	}
	if !priceData.UseAudioDurationPrice {
		t.Fatal("expected audio duration billing")
	}
	if priceData.AudioDurationPrice != 0.074 {
		t.Fatalf("AudioDurationPrice = %v, want 0.074", priceData.AudioDurationPrice)
	}
	if priceData.QuotaToPreConsume != 37000 {
		t.Fatalf("QuotaToPreConsume = %d, want 37000", priceData.QuotaToPreConsume)
	}
	if priceData.ConservativeQuotaToPreConsume != 37000 {
		t.Fatalf("ConservativeQuotaToPreConsume = %d, want 37000", priceData.ConservativeQuotaToPreConsume)
	}
}

func TestModelPriceHelperRejectsDurationPricedModelWithoutAudio(t *testing.T) {
	originalAudioDurationPrices := ratio_setting.AudioDurationPrice2JSONString()
	if err := ratio_setting.UpdateAudioDurationPriceByJSONString(`{"mimo-v2.5-asr":0.074}`); err != nil {
		t.Fatalf("set audio duration price: %v", err)
	}
	t.Cleanup(func() {
		if err := ratio_setting.UpdateAudioDurationPriceByJSONString(originalAudioDurationPrices); err != nil {
			t.Fatalf("restore audio duration price: %v", err)
		}
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, err := ModelPriceHelper(ctx, &relaycommon.RelayInfo{
		OriginModelName: "mimo-v2.5-asr",
		UsingGroup:      "default",
		UserGroup:       "default",
	}, 0, &types.TokenCountMeta{})
	if err == nil {
		t.Fatal("duration-priced model without audio duration should be rejected")
	}
}
