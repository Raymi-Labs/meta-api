package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseRealtimeClosesReadersBeforeReturningUsage(t *testing.T) {
	// Two real loopback socket pairs exercise shutdown; no upstream credentials.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	makePair := func() (*websocket.Conn, *websocket.Conn) {
		accepted := make(chan *websocket.Conn, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err == nil {
				accepted <- conn
			}
		}))
		t.Cleanup(server.Close)
		client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		require.NoError(t, err)
		remote := <-accepted
		t.Cleanup(func() { client.Close(); remote.Close() })
		return client, remote
	}
	user, gatewayClient := makePair()
	gatewayTarget, upstream := makePair()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{ClientWs: gatewayClient, TargetWs: gatewayTarget, EnterpriseId: 1, EnterpriseMemberId: 1}
	// Per-call mode isolates socket lifecycle from balance writes; enterprise
	// incremental accounting is exercised by service tests with the real ledger.
	info.UsePrice = true
	result := make(chan *dto.RealtimeUsage, 1)
	go func() { _, usage := OpenaiRealtimeHandler(c, info); result <- usage }()
	require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(`{"event_id":"event-fixture","type":"response.done","response":{"usage":{"total_tokens":30,"input_tokens":10,"output_tokens":20,"input_token_details":{"text_tokens":10},"output_token_details":{"text_tokens":20}}}}`)))
	require.NoError(t, user.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, _, err := user.ReadMessage()
	require.NoError(t, err)
	// Replayed response.done is forwarded but must not accrue a second charge.
	require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(`{"event_id":"event-fixture","type":"response.done","response":{"usage":{"total_tokens":30,"input_tokens":10,"output_tokens":20}}}`)))
	_, _, err = user.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, user.Close())
	select {
	case usage := <-result:
		require.Equal(t, 30, usage.TotalTokens)
		require.Equal(t, 10, usage.InputTokens)
		require.Equal(t, 20, usage.OutputTokens)
	case <-time.After(3 * time.Second):
		t.Fatal("socket readers survived client close")
	}
	// The peer is closed before the function returns, so no further usage can accrue.
	require.NoError(t, upstream.SetReadDeadline(time.Now().Add(time.Second)))
	_, _, err = upstream.ReadMessage()
	require.Error(t, err)
}
