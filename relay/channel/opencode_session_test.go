package channel

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeAPIURLScope(t *testing.T) {
	require.False(t, isOpenCodeAPIURL(nil))
	for _, tc := range []struct {
		address string
		want    bool
	}{
		{"https://opencode.ai/zen/go/v1/chat/completions", true},
		{"https://OPENCODE.AI:443/zen/v1/messages", true},
		{"https://opencode.ai/zen/go/v1/responses?foo=bar", true},
		{"http://opencode.ai/zen/go/v1/chat/completions", false},
		{"https://opencode.ai:8443/zen/go/v1/chat/completions", false},
		{"https://opencode.ai.example.com/zen/go/v1/chat/completions", false},
		{"https://opencode.ai@example.com/zen/go/v1/chat/completions", false},
		{"https://example.com/opencode.ai/zen/go/v1/chat/completions", false},
		{"https://opencode.ai/api/other", false},
		{"https://opencode.ai/zen/go/v10/chat/completions", false},
		{"https://opencode.ai/zen/go/v1-extra/chat/completions", false},
	} {
		t.Run(tc.address, func(t *testing.T) {
			target, err := url.Parse(tc.address)
			require.NoError(t, err)
			require.Equal(t, tc.want, isOpenCodeAPIURL(target))
		})
	}
}

type openCodeTestTransport func(*http.Request) (*http.Response, error)

func (f openCodeTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type openCodeTestAdaptor struct {
	Adaptor
	address string
}

func (a openCodeTestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.address, nil
}

func (a openCodeTestAdaptor) SetupRequestHeader(_ *gin.Context, header *http.Header, _ *relaycommon.RelayInfo) error {
	header.Set("Content-Type", "application/json")
	header.Set("Authorization", "Bearer upstream-test-key")
	return nil
}

func TestOpenCodeSessionActualRequests(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	originalTransport := client.Transport
	t.Cleanup(func() { client.Transport = originalTransport })
	var received http.Header
	const body = `{"model":"mimo-v2.6-pro","messages":[{"role":"user","content":"test"}]}`
	client.Transport = openCodeTestTransport(func(req *http.Request) (*http.Response, error) {
		received = req.Header.Clone()
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, body, string(data))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
	})
	send := func(address, session string, info *relaycommon.RelayInfo) http.Header {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		c.Request.Header.Set(openCodeSessionHeader, session)
		c.Request.Header.Set("Authorization", "Bearer client-test-key")
		c.Request.Header.Set("Cookie", "private=test")
		c.Request.Header.Set("X-Private-Header", "private")
		resp, err := DoApiRequest(openCodeTestAdaptor{address: address}, c, info, strings.NewReader(body))
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, "Bearer upstream-test-key", received.Get("Authorization"))
		require.Empty(t, received.Get("Cookie"))
		require.Empty(t, received.Get("X-Private-Header"))
		return received
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	for _, path := range []string{"chat/completions", "messages", "responses"} {
		for _, stream := range []bool{false, true} {
			info.IsStream = stream
			for _, id := range []int{1360, 98765} {
				info.ChannelId = id
				header := send("https://opencode.ai/zen/go/v1/"+path, "conversation-{api_key}", info)
				require.Equal(t, "conversation-{api_key}", header.Get(openCodeSessionHeader))
			}
		}
	}
	info.IsStream = false
	address := "https://opencode.ai/zen/go/v1/chat/completions"
	for _, session := range []string{"", "   "} {
		require.Empty(t, send(address, session, info).Get(openCodeSessionHeader))
	}
	require.Empty(t, send("https://example.com/v1/chat/completions", "conversation-a", info).Get(openCodeSessionHeader))

	info.HeadersOverride = map[string]any{"x-OPENCODE-session": "administrator-value"}
	require.Equal(t, "administrator-value", send(address, "conversation-a", info).Get(openCodeSessionHeader))
	info.UseRuntimeHeadersOverride = true
	info.RuntimeHeadersOverride = map[string]any{"X-Opencode-Session": "runtime-value"}
	require.Equal(t, "runtime-value", send(address, "conversation-a", info).Get(openCodeSessionHeader))
	info.UseRuntimeHeadersOverride = false
	info.HeadersOverride = map[string]any{"x-opencode-session": "{client_header:x-opencode-session}"}
	require.Equal(t, "conversation-a", send(address, "conversation-a", info).Get(openCodeSessionHeader))
	// Custom proxy URLs retain the existing explicit configuration mechanism.
	require.Equal(t, "conversation-a", send("https://example.com/v1/chat/completions", "conversation-a", info).Get(openCodeSessionHeader))

	info.IsChannelTest = true
	var previous string
	for _, headers := range []map[string]any{nil, info.HeadersOverride} {
		info.HeadersOverride = headers
		for range 2 {
			session := send(address, "dashboard-browser-session", info).Get(openCodeSessionHeader)
			_, err := uuid.Parse(session)
			require.NoError(t, err)
			require.NotEqual(t, previous, session)
			require.NotEqual(t, "dashboard-browser-session", session)
			previous = session
		}
	}
	info.HeadersOverride = nil
	require.Empty(t, send("https://example.com/v1/chat/completions", "dashboard-browser-session", info).Get(openCodeSessionHeader))
}

func TestOpenCodeSessionRedirectScope(t *testing.T) {
	for _, destination := range []string{
		"https://opencode.ai/zen/v1/responses",
		"https://example.com/v1/chat/completions",
		"http://opencode.ai/zen/go/v1/chat/completions",
		"https://opencode.ai/another-service",
	} {
		t.Run(destination, func(t *testing.T) {
			initial, err := http.NewRequest(http.MethodGet, "https://opencode.ai/zen/go/v1/models", nil)
			require.NoError(t, err)
			initial.Header.Set(openCodeSessionHeader, "conversation-a")
			checked := false
			base := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
				checked = true
				return nil
			}, Transport: openCodeTestTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == initial.URL.String() {
					return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": {destination}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
				}
				if isOpenCodeAPIURL(req.URL) {
					require.Equal(t, "conversation-a", req.Header.Get(openCodeSessionHeader))
				} else {
					require.Empty(t, req.Header.Get(openCodeSessionHeader))
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
			})}
			client := openCodeSessionHTTPClient(base, initial)
			require.NotSame(t, base, client)
			resp, err := client.Do(initial)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.True(t, checked)
			require.Equal(t, "conversation-a", initial.Header.Get(openCodeSessionHeader))
		})
	}
	initial, _ := http.NewRequest(http.MethodGet, "https://opencode.ai/zen/go/v1/models", nil)
	base := &http.Client{}
	require.Same(t, base, openCodeSessionHTTPClient(base, initial))
	initial.Header.Set(openCodeSessionHeader, "conversation-a")
	client := openCodeSessionHTTPClient(base, initial)
	require.Nil(t, base.CheckRedirect)
	require.Error(t, client.CheckRedirect(initial, make([]*http.Request, 10)))
	sentinel := errors.New("existing redirect policy")
	base.CheckRedirect = func(*http.Request, []*http.Request) error { return sentinel }
	require.ErrorIs(t, openCodeSessionHTTPClient(base, initial).CheckRedirect(initial, nil), sentinel)
}
