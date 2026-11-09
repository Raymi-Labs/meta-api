package relay

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type imageAttemptBilling struct {
	quota  int
	reject bool
}

func (b *imageAttemptBilling) Reserve(q int) error {
	if b.reject {
		return errors.New("insufficient test quota")
	}
	b.quota = q
	return nil
}
func (b *imageAttemptBilling) GetPreConsumedQuota() int { return b.quota }
func (*imageAttemptBilling) NeedsRefund() bool          { return false }
func (*imageAttemptBilling) Refund(*gin.Context)        {}
func (*imageAttemptBilling) Settle(int) error           { return nil }

func TestImageHelperReservesBeforeSendingOverriddenQuantity(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	billing := &imageAttemptBilling{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.EqualValues(t, 4, gjson.GetBytes(body, "n").Int())
		require.Equal(t, 200, billing.quota)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"retry later"}}`))
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelId, 42)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-image-2")
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]interface{}{"n": 4})
	info := &relaycommon.RelayInfo{Request: &dto.ImageRequest{Model: "gpt-image-2", N: common.GetPointer(uint(1))},
		OriginModelName: "gpt-image-2", RequestURLPath: "/v1/images/generations", RelayMode: relayconstant.RelayModeImagesGenerations,
		Billing: billing, PriceData: types.PriceData{UsePrice: true, ModelPrice: 50 / common.QuotaPerUnit, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}}
	apiErr := ImageHelper(c, info)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	require.EqualValues(t, 1, calls.Load())
	billing.reject = true
	apiErr = ImageHelper(c, info)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.EqualValues(t, 1, calls.Load()) // insufficient quota never reaches upstream
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]interface{}{"n": -1})
	apiErr = ImageHelper(c, info)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.EqualValues(t, 1, calls.Load())
}

func TestOutboundImageQuantityAfterOverrides(t *testing.T) {
	for _, tc := range []struct {
		body         string
		ali, invalid bool
		count        int
	}{
		{`{"n":2,"parameters":{"n":4},"input":{"prompt":"keep"}}`, true, false, 4},
		{`{"n":2,"parameters":{},"input":{"prompt":"keep"}}`, true, false, 2},
		{`{"n":2,"parameters":{"n":0}}`, true, true, 0},
		{`{"n":129}`, false, true, 0},
		{`{"n":0}`, false, false, 1},
		{`{"n":2,"parameters":{"n":4}}`, false, false, 2},
		{`{"n":"3"}`, false, false, 3},
		{`{"n":"3","parameters":{"n":"4"}}`, true, false, 4},
		{`{"n":"x"}`, false, true, 0},
	} {
		t.Run(tc.body, func(t *testing.T) {
			body, count, _, err := normalizeOutboundImageQuantity([]byte(tc.body), tc.ali)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.count, count)
			if tc.ali {
				require.EqualValues(t, tc.count, gjson.GetBytes(body, "parameters.n").Int())
			}
			if !tc.ali {
				require.Equal(t, gjson.Number, gjson.GetBytes(body, "n").Type)
			}
			if gjson.Get(tc.body, "input").Exists() {
				require.Equal(t, "keep", gjson.GetBytes(body, "input.prompt").String())
			}
		})
	}
}
