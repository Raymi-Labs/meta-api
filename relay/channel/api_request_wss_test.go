package channel

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type tlsWSSAdaptor struct {
	Adaptor
	url string
}

func (a tlsWSSAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) { return a.url, nil }
func (a tlsWSSAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}

func TestAliWSSAfterHTTP2DoesNotShareALPN(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("audio"))
	}))
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	server.StartTLS()
	defer server.Close()

	service.InitHttpClient()
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	transport := &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()}
	defer transport.CloseIdleConnections()
	client.Transport = transport
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 2, response.ProtoMajor)
	require.Contains(t, transport.TLSClientConfig.NextProtos, "h2")
	protocols := append([]string(nil), transport.TLSClientConfig.NextProtos...)
	require.False(t, transport.TLSClientConfig.InsecureSkipVerify)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatAliTTS, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: appconstant.ChannelTypeAli}}
	conn, err := DoWssRequest(tlsWSSAdaptor{url: "wss" + strings.TrimPrefix(server.URL, "https")}, c, info, nil)
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, "http/1.1", conn.UnderlyingConn().(*tls.Conn).ConnectionState().NegotiatedProtocol)
	kind, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, kind)
	require.Equal(t, []byte("audio"), data)
	require.Equal(t, protocols, transport.TLSClientConfig.NextProtos)
	response, err = client.Get(server.URL)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 2, response.ProtoMajor)

	// Selecting HTTP/1.1 must not bypass the upstream certificate check.
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{NextProtos: []string{"h2", "http/1.1"}}}
	_, err = DoWssRequest(tlsWSSAdaptor{url: "wss" + strings.TrimPrefix(server.URL, "https")}, c, info, nil)
	require.ErrorContains(t, err, "certificate")
}
