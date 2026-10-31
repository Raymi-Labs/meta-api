package service

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func imagePricingInfo(t *testing.T) *relaycommon.RelayInfo {
	t.Helper()
	b, err := os.ReadFile("testdata/image_price_expression.txt")
	require.NoError(t, err)
	expr := strings.TrimSpace(string(b))
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAli, UpstreamModelName: "qwen-image-3.0-pro"},
		Billing: &imageReserveRecorder{}, PriceData: types.PriceData{TimeRatioInfo: types.TimeRatioInfo{Ratio: 2}},
		BillingRequestInput:   &billingexpr.RequestInput{Body: []byte(`{"_image_billing":{"input_count":0,"output_count":0}}`)},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), QuotaPerUnit: 1000000, GroupRatio: 0.5, TimeRatio: 2}}
}
func TestImageInputOutputPricingReservationSettlementRetry(t *testing.T) {
	info := imagePricingInfo(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"qwen-image-3.0-pro","input":{"messages":[{"content":[{"image":"https://example.com/a.png"},{"image":"https://example.com/b.png"},{"text":"edit"}]}]},"parameters":{"size":"auto","n":3},"_image_billing":{"input_count":0}}`)
	require.NoError(t, NormalizeImagePricing(info, body, 3))
	require.Nil(t, PrepareImageBilling(c, info, 3, false))
	require.Equal(t, 640000, info.PriceData.QuotaToPreConsume) // (2*.02+3*.2)*1e6, group .5*time 2
	require.Nil(t, PrepareImageBilling(c, info, 3, false))
	require.Equal(t, 640000, info.PriceData.QuotaToPreConsume)
	inputs, outputs := 2, 1
	info.ImageResponseCount = &outputs
	require.NoError(t, SetImagePricingResult(info, &inputs, &outputs, 1024, 1024, "qima_output_1k", 1))
	n, err := ApplyImageResponseQuantity(info)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	used, quota, _ := TryTieredSettle(info, billingexpr.TokenParams{})
	require.True(t, used)
	require.Equal(t, 120000, quota)
	// Retry rebuilds a new canonical request and never reuses actual response tier.
	require.NoError(t, NormalizeImagePricing(info, body, 3))
	require.Nil(t, info.ImagePricingResult)
	require.Nil(t, PrepareImageBilling(c, info, 3, false))
	require.Equal(t, 640000, info.PriceData.QuotaToPreConsume)
	require.Contains(t, string(body), `"input_count":0`, "source payload is not modified")
}
func TestImagePriceResolutionAndInvalidInput(t *testing.T) {
	for _, tc := range []struct{ size, tier string }{{"1024*1024", "1K"}, {"1500*1500", "1K"}, {"1501*1500", "2K"}, {"2048x2048", "2K"}, {"auto", "auto"}} {
		info := imagePricingInfo(t)
		body := []byte(`{"model":"qwen-image-3.0-pro","input":{"messages":[{"content":[{"text":"draw"}]}]},"parameters":{"size":"` + tc.size + `"}}`)
		require.NoError(t, NormalizeImagePricing(info, body, 1))
		require.Equal(t, tc.tier, info.ImagePricingInput.Resolution)
		require.Zero(t, info.ImagePricingInput.InputCount)
	}
	for _, body := range []string{
		`{}`, `{"model":"wan2.7-image-pro"}`, `{"model":"qwen-image-3.0-pro","input":{"messages":[]}}`,
		`{"model":"qwen-image-3.0-pro","input":{"messages":[{"content":[{"image":null}]}]}}`,
		`{"model":"qwen-image-3.0-pro","input":{"messages":[{"content":[{"text":"x"}]}]},"parameters":{"size":"4K"}}`,
	} {
		require.Error(t, NormalizeImagePricing(imagePricingInfo(t), []byte(body), 1))
	}
	info := imagePricingInfo(t)
	info.ChannelType = constant.ChannelTypeOpenAI
	require.Error(t, NormalizeImagePricing(info, []byte(`{"model":"qwen-image-3.0-pro"}`), 1))
}
func TestImageUsageMustBeVerifiable(t *testing.T) {
	info := imagePricingInfo(t)
	info.ImagePricingInput = &relaycommon.ImagePricingDimensions{InputCount: 2, OutputCount: 2, Resolution: "auto"}
	one, two, three := 1, 2, 3
	require.Error(t, SetImagePricingResult(info, nil, &one, 1024, 1024, "", 1))
	require.Error(t, SetImagePricingResult(info, &two, &three, 1024, 1024, "", 3))
	require.Error(t, SetImagePricingResult(info, &two, &two, 1024, 1024, "", 1))
	require.Error(t, SetImagePricingResult(info, &two, &one, 0, 0, "", 1))
	require.Error(t, SetImagePricingResult(info, &two, &one, 2048, 2048, "qima_output_1k", 1))
	require.NoError(t, SetImagePricingResult(info, &two, &one, 0, 0, "qima_output_2k", 1))
}
