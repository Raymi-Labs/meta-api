package channel_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/ali"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao"
	"github.com/QuantumNous/new-api/relay/channel/task/gemini"
	"github.com/QuantumNous/new-api/relay/channel/task/hailuo"
	"github.com/QuantumNous/new-api/relay/channel/task/jimeng"
	"github.com/QuantumNous/new-api/relay/channel/task/kling"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	"github.com/QuantumNous/new-api/relay/channel/task/suno"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/relay/channel/task/vertex"
	"github.com/QuantumNous/new-api/relay/channel/task/vidu"
	"github.com/QuantumNous/new-api/relay/channel/task/xai_video"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type taskHeaderTransport func(*http.Request) (*http.Response, error)

func (f taskHeaderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTaskNativePollingHeaderOverrides(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	var expectedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/v4/token" {
			io.WriteString(w, `{"access_token":"local-test-token"}`)
			return
		}
		calls.Add(1)
		require.Equal(t, "Override "+expectedKey, r.Header.Get("Authorization"))
		require.Equal(t, "42", r.Header.Get("X-Owner"))
		require.Equal(t, "static-value", r.Header.Get("X-Static"))
		require.Empty(t, r.Header.Get("X-Client"))
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	// Vertex's official token/API URLs are sent through a local HTTP server,
	// including the real HTTP request construction. No provider is contacted.
	client := service.GetHttpClient()
	oldTransport := client.Transport
	local, err := url.Parse(server.URL)
	require.NoError(t, err)
	client.Transport = taskHeaderTransport(func(r *http.Request) (*http.Response, error) {
		copyReq := r.Clone(r.Context())
		if strings.HasSuffix(copyReq.URL.Hostname(), "googleapis.com") {
			copyReq.URL.Scheme, copyReq.URL.Host, copyReq.Host = local.Scheme, local.Host, local.Host
		}
		return oldTransport.RoundTrip(copyReq)
	})
	t.Cleanup(func() { client.Transport = oldTransport })
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pkcs, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	creds, err := common.Marshal(map[string]string{"project_id": "test", "client_email": "task-test@example.test", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs}))})
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		adaptor service.TaskPollingAdaptor
		key     string
	}{
		{"ali", &ali.TaskAdaptor{}, "sk-selected"}, {"doubao", &doubao.TaskAdaptor{}, "sk-selected"},
		{"gemini", &gemini.TaskAdaptor{}, "sk-selected"}, {"hailuo", &hailuo.TaskAdaptor{}, "sk-selected"},
		{"jimeng", &jimeng.TaskAdaptor{}, "sk-selected"}, {"kling", &kling.TaskAdaptor{}, "sk-selected"},
		{"sora", &sora.TaskAdaptor{}, "sk-selected"}, {"suno", &suno.TaskAdaptor{}, "sk-selected"},
		{"vertex", &vertex.TaskAdaptor{}, string(creds)}, {"vidu", &vidu.TaskAdaptor{}, "sk-selected"},
		{"xai", &xai_video.TaskAdaptor{}, "sk-selected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectedKey = tc.key
			info := &relaycommon.RelayInfo{UserId: 42, ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "wrong-current-channel-key", ChannelBaseUrl: server.URL,
				HeadersOverride: map[string]any{"*": "", "re:.*": "", "Authorization": "Override {api_key}", "X-Owner": "{authenticated_user_id}", "X-Static": "static-value", "X-Client": "{client_header:X-Client}"}}}
			tc.adaptor.Init(info)
			before := calls.Load()
			resp, err := tc.adaptor.FetchTask(server.URL, tc.key, map[string]any{
				"task_id": taskcommon.EncodeLocalTaskID("projects/test/locations/us-central1/publishers/google/models/veo/operations/test"),
				"action":  "GENERATE", "ids": []string{"a"},
			}, "")
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, before+1, calls.Load())
		})
	}
}

func TestTaskSubmissionHeadersUseAuthenticatedIdentity(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "31", r.Header.Get("X-Owner"))
		require.Equal(t, "Override selected", r.Header.Get("Authorization"))
		require.Equal(t, "client-session", r.Header.Get("X-Session"))
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "selected", ChannelBaseUrl: server.URL,
		HeadersOverride: map[string]any{"*": "", "X-Owner": "{authenticated_user_id}", "Authorization": "Override {api_key}"}}}
	a := &sora.TaskAdaptor{}
	a.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("id", 31)
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{}`))
	c.Request.Header.Set("X-Owner", "forged")
	c.Request.Header.Set("X-Session", "client-session")
	resp, err := a.DoRequest(c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	resp.Body.Close()
	require.EqualValues(t, 1, calls.Load())
	info.HeadersOverride["X-Owner"] = "prefix-{authenticated_user_id}"
	_, err = a.DoRequest(c, info, strings.NewReader(`{}`))
	require.Error(t, err)
	require.EqualValues(t, 1, calls.Load())
}

func TestTaskPollingHeaderIdentityErrorsContentAndRedirects(t *testing.T) {
	var calls, foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1) }))
	defer foreign.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/ok", 302)
		case "/foreign":
			http.Redirect(w, r, foreign.URL, 302)
		default:
			require.Equal(t, "42", r.Header.Get("X-Owner"))
			require.Equal(t, "selected", r.Header.Get("X-Key"))
		}
	}))
	defer server.Close()
	info := &relaycommon.RelayInfo{UserId: 42, ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: map[string]any{
		"X-Owner": "{authenticated_user_id}", "X-Key": "{api_key}", "Host": "must-not-replace-content.example"}}}
	for _, path := range []string{"/same", "/foreign"} {
		req, _ := http.NewRequest("GET", server.URL+path, nil)
		resp, err := channel.DoTaskContentRequest(info, "selected", req, server.Client())
		require.NoError(t, err)
		resp.Body.Close()
		if path == "/same" {
			require.Equal(t, 200, resp.StatusCode)
		} else {
			require.Equal(t, 302, resp.StatusCode)
		}
		require.NotEqual(t, "must-not-replace-content.example", req.Host)
	}
	require.Zero(t, foreignCalls.Load())
	before := calls.Load()
	info.UserId = 0
	req, _ := http.NewRequest("GET", server.URL+"/ok", nil)
	_, err := channel.DoTaskPollingRequest(info, "selected", req, server.Client())
	require.Error(t, err)
	require.Equal(t, before, calls.Load())
}

func TestTaskContentNativeCredentialsBlockRedirectWithoutOverrides(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1) }))
	defer foreign.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/signed?signature=retain", http.StatusFound)
	}))
	defer origin.Close()
	for _, header := range []string{"Authorization", "X-Goog-Api-Key"} {
		client := origin.Client()
		checks := 0
		client.CheckRedirect = func(*http.Request, []*http.Request) error { checks++; return nil }
		info := &relaycommon.RelayInfo{UserId: 42, ChannelMeta: &relaycommon.ChannelMeta{}}
		req, err := http.NewRequest(http.MethodGet, origin.URL+"/content?key=saved-key", nil)
		require.NoError(t, err)
		req.Header.Set(header, "saved-key")
		resp, err := channel.DoTaskContentRequest(info, "saved-key", req, client)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		require.Equal(t, 1, checks, "existing redirect policy must still run")
	}
	require.Zero(t, foreignCalls.Load())
}
