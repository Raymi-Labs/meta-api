package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestRealtimeWebSocketTransport(t *testing.T) {
	for _, protocol := range []bool{false, true} {
		t.Run(map[bool]string{false: "authorization", true: "browser-subprotocol"}[protocol], func(t *testing.T) {
			headers := make(chan http.Header, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headers <- r.Header.Clone()
				upgrader := websocket.Upgrader{Subprotocols: []string{"realtime"}}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				for {
					kind, data, err := conn.ReadMessage()
					if err != nil {
						return
					}
					if err = conn.WriteMessage(kind, data); err != nil {
						return
					}
				}
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=gpt-realtime", nil)
			if protocol {
				c.Request.Header.Set("Sec-WebSocket-Protocol", "realtime")
			}
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeRealtime, RequestURLPath: "/v1/realtime?model=gpt-realtime", OriginModelName: "gpt-realtime", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelBaseUrl: server.URL, ApiKey: "fake-test-key", UpstreamModelName: "gpt-realtime", HeadersOverride: map[string]interface{}{"X-Transport-Test": "override"}}}
			response, err := (&Adaptor{}).DoRequest(c, info, nil)
			require.NoError(t, err)
			conn := response.(*websocket.Conn)
			defer conn.Close()
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
			h := <-headers
			require.Equal(t, "override", h.Get("X-Transport-Test"))
			if protocol {
				require.Equal(t, "realtime", conn.Subprotocol())
				require.Contains(t, h.Get("Sec-WebSocket-Protocol"), "openai-insecure-api-key.fake-test-key")
			} else {
				require.Equal(t, "Bearer fake-test-key", h.Get("Authorization"))
				require.Equal(t, "realtime=v1", h.Get("OpenAI-Beta"))
			}
			for _, kind := range []int{websocket.TextMessage, websocket.BinaryMessage} {
				payload := []byte(`{"type":"session.update","session":{}}`)
				require.NoError(t, conn.WriteMessage(kind, payload))
				gotKind, got, err := conn.ReadMessage()
				require.NoError(t, err)
				require.Equal(t, kind, gotKind)
				require.Equal(t, payload, got)
			}
			require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
			_, _, err = conn.ReadMessage()
			require.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
		})
	}
}
