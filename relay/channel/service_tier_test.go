package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaidServiceTierCannotReachUpstreamAfterFilteringOrOverride(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })
	calls := 0
	client.Transport = openCodeTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(raw), `"service_tier":"priority"`)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := &relaycommon.RelayInfo{ManagedServiceTier: true, RequestedServiceTier: "priority", ChannelMeta: &relaycommon.ChannelMeta{}}
	original := []byte(`{"model":"tier-test","service_tier":"priority"}`)
	kept, err := relaycommon.RemoveDisabledFields(original, dto.ChannelOtherSettings{AllowServiceTier: true}, false)
	require.NoError(t, err)
	resp, err := DoApiRequest(openCodeTestAdaptor{address: "https://example.invalid/v1/responses"}, c, info, strings.NewReader(string(kept)))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 1, calls)
	stripped, err := relaycommon.RemoveDisabledFields(original, dto.ChannelOtherSettings{}, false)
	require.NoError(t, err)
	_, err = DoApiRequest(openCodeTestAdaptor{address: "https://example.invalid/v1/responses"}, c, info, strings.NewReader(string(stripped)))
	require.Error(t, err)
	require.Equal(t, 1, calls, "no upstream call and therefore no upstream charge")
	changed, err := relaycommon.ApplyParamOverride(original, map[string]interface{}{"service_tier": "ultrafast"}, nil)
	require.NoError(t, err)
	_, err = DoApiRequest(openCodeTestAdaptor{address: "https://example.invalid/v1/responses"}, c, info, strings.NewReader(string(changed)))
	require.Error(t, err)
	require.Equal(t, 1, calls)
	info.RequestedServiceTier = "default"
	_, err = DoApiRequest(openCodeTestAdaptor{address: "https://example.invalid/v1/responses"}, c, info, strings.NewReader(string(kept)))
	require.Error(t, err)
	require.Equal(t, 1, calls, "no forced acceleration on standard requests")
}
