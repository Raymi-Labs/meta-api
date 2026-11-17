package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPricingDiscountPolicyIsLiveWithoutMutatingCatalog(t *testing.T) {
	old, times := ratio_setting.NoDiscountModelsJSON(), ratio_setting.TimeRatioRules2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateNoDiscountModels(old))
		require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(times))
	})
	require.NoError(t, ratio_setting.UpdateNoDiscountModels(`["luna"]`))
	require.NoError(t, ratio_setting.UpdateTimeRatioRulesByJSONString(`[{"id":"sale","enabled":true,"timezone":"UTC","start":"00:00","end":"23:59","ratio":0.5}]`))
	cached := []model.Pricing{{ModelName: "luna"}, {ModelName: "other"}}
	out := pricingWithDiscountPolicy(cached)
	require.True(t, out[0].NoDiscount)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/option/time_ratio_preview?model=luna&group=default", nil)
	GetTimeRatioPreview(c)
	var previewResponse struct {
		Data struct {
			Ratio      float64
			Matched    bool
			NoDiscount bool `json:"no_discount"`
		}
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &previewResponse))
	require.Equal(t, 1.0, previewResponse.Data.Ratio)
	require.True(t, previewResponse.Data.NoDiscount)
	require.False(t, previewResponse.Data.Matched)
	require.False(t, out[1].NoDiscount)
	require.False(t, cached[0].NoDiscount)
	preview := buildPricingTimeRatioMap(out, map[string]string{"default": "default"}, "default", time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	require.NotContains(t, preview, "luna")
	require.Equal(t, 0.5, preview["other"]["default"].Ratio)
	require.NoError(t, ratio_setting.UpdateNoDiscountModels(`[]`))
	require.False(t, pricingWithDiscountPolicy(cached)[0].NoDiscount)
}
