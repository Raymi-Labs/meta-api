package controller

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAliImageChannelTestsUseImageRouteAndPricing(t *testing.T) {
	if runRelayMatrixInSubprocess(t) {
		return
	}
	db := useRelayMatrixProcessDB(t)
	common.RedisEnabled = false
	require.NoError(t, db.Create(&model.User{Id: 8051, Username: "image-tester", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"wan2.7-image-pro":0.1,"z-image-turbo":0.02}`))
	expr, err := os.ReadFile("../service/testdata/image_price_expression.txt")
	require.NoError(t, err)
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{"qwen-image-3.0-pro": "tiered_expr"}, BillingExpr: map[string]string{"qwen-image-3.0-pro": strings.TrimSpace(string(expr))}})
	service.InitHttpClient()
	calls := 0
	service.GetImageHttpClient().Transport = healthTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/api/v1/services/aigc/multimodal-generation/generation", req.URL.Path)
		require.Empty(t, req.Header.Get("X-DashScope-Async"))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, int64(1), gjson.GetBytes(body, "parameters.n").Int())
		require.Equal(t, "1024*1024", gjson.GetBytes(body, "parameters.size").String())
		require.False(t, gjson.GetBytes(body, "_image_billing").Exists())
		response := `{"output":{"choices":[{"message":{"content":[{"image":"https://example.com/image.png"}]}}]},"usage":{"input_image_count":0,"output_image_count":1,"output_image_type":"qima_output_1k","output_width":1024,"output_height":1024}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
	})
	for i, name := range []string{"qwen-image-3.0-pro", "wan2.7-image-pro", "z-image-turbo"} {
		ch := &model.Channel{Id: 8060 + i, Type: constant.ChannelTypeAli, Name: name, Models: name, Group: "default", Key: "fixture-only", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer("https://image.invalid")}
		require.NoError(t, db.Create(ch).Error)
		result := testChannel(ch, 8051, name, "", common.GetPointer(false), nil)
		require.Nil(t, result.preflight)
		require.NoError(t, result.localErr)
		require.Nil(t, result.newAPIError)
		require.NotNil(t, result.info.ImageResponseCount)
		require.Equal(t, 1, *result.info.ImageResponseCount)
		if name == "qwen-image-3.0-pro" {
			require.NotNil(t, result.info.ImagePricingResult)
			require.Greater(t, result.priceData.QuotaToPreConsume, 0)
		}
	}
	require.Equal(t, 3, calls)
	// Removing a local price fails before any upstream request, without a fake
	// video-preflight state or a successful generation claim.
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{})
	result := testChannel(&model.Channel{Type: constant.ChannelTypeAli, Models: "qwen-image-3.0-pro", Key: "fixture-only", BaseURL: common.GetPointer("https://image.invalid")}, 8051, "qwen-image-3.0-pro", "", common.GetPointer(false), nil)
	require.Nil(t, result.preflight)
	require.Error(t, result.localErr)
	require.Equal(t, 3, calls)
}
