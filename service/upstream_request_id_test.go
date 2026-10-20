package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamRequestIDsBoundedAndAttemptScoped(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	var firstSnapshot []upstreamRequestID
	for i := 1; i <= 7; i++ {
		c.Set("channel_id", i)
		RecordUpstreamRequestID(c, http.Header{common.RequestIdKey: {fmt.Sprintf("gateway-%d", i)}, "X-Request-Id": {"provider-id"}})
		if i == 1 {
			value, _ := c.Get(upstreamRequestIDsKey)
			firstSnapshot = value.([]upstreamRequestID)
		}
	}
	// A retry without an HTTP response/ID must not relabel an earlier ID.
	c.Set("channel_id", 8)
	RecordUpstreamRequestID(c, nil)
	admin := make(map[string]interface{})
	AppendUpstreamRequestIDs(c, admin)
	entries := admin["upstream_request_ids"].([]upstreamRequestID)
	require.Len(t, entries, 5)
	require.Equal(t, 3, entries[0].ChannelID)
	require.Equal(t, "gateway-7", entries[4].ID)
	require.Equal(t, 7, entries[4].ChannelID)
	require.Equal(t, common.RequestIdKey, entries[4].Header)
	require.Equal(t, []upstreamRequestID{{ChannelID: 1, ID: "gateway-1", Header: common.RequestIdKey}}, firstSnapshot)
}

func TestUpstreamRequestIDValidationAndErrorDiagnostics(t *testing.T) {
	for _, value := range []string{strings.Repeat("a", 257), "id\r\nextra", "id\x00extra", "  "} {
		id, _ := getUpstreamRequestID(http.Header{common.RequestIdKey: {value}})
		require.Empty(t, id)
	}
	id, source := getUpstreamRequestID(http.Header{"X-Request-Id": {" provider "}})
	require.Equal(t, "provider", id)
	require.Equal(t, "X-Request-ID", source)
	resp := &http.Response{StatusCode: 400, Header: http.Header{common.RequestIdKey: {"gateway-error"}}, Body: http.NoBody}
	diagnostics := buildUpstreamDiagnostics(resp, 0)
	require.Equal(t, "gateway-error", diagnostics.RequestID)
	require.Equal(t, common.RequestIdKey, diagnostics.RequestIDHeader)
}
