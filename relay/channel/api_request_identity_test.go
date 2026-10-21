package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedUserHeaderRequiresTrustedContext(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   any
	}{
		{"missing", nil}, {"zero", 0}, {"negative", -1},
		{"string", "42"}, {"other_integer_type", int64(42)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("X-VM2API-Tenant", "42")
			c.Request.Header.Set("New-Api-User", "42")
			if tc.id != nil {
				c.Set("id", tc.id)
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				HeadersOverride: map[string]any{"*": "", "x-vm2api-tenant": authenticatedUserIDPlaceholder},
			}}
			headers, err := processHeaderOverride(info, c)
			require.Error(t, err)
			require.Nil(t, headers)
		})
	}
	_, _, err := applyHeaderOverridePlaceholders(authenticatedUserIDPlaceholder, nil, "unused")
	require.Error(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("id", 42)
	value, include, err := applyHeaderOverridePlaceholders(" \t"+authenticatedUserIDPlaceholder+"\n", c, "unused")
	require.NoError(t, err)
	require.True(t, include)
	require.Equal(t, "42", value)
	for _, malformed := range []string{"tenant-" + authenticatedUserIDPlaceholder, authenticatedUserIDPlaceholder + "/suffix"} {
		_, _, err = applyHeaderOverridePlaceholders(malformed, c, "unused")
		require.Error(t, err)
	}
}

func TestAuthenticatedUserHeaderOverridesEveryClientSpelling(t *testing.T) {
	t.Parallel()
	for _, rule := range []string{"", "*", "re:(?i)^x-", "regex:(?i)tenant$"} {
		for _, name := range []string{"x-vm2api-tenant", "X-VM2API-Tenant", "X-vM2Api-tENanT"} {
			t.Run(rule+"/"+name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				c.Request.Header["X-Vm2api-Tenant"] = []string{"spoofed-a", "spoofed-b"}
				c.Request.Header["x-vm2api-tenant"] = []string{"spoofed-c"}
				c.Set("id", 42)
				overrides := map[string]any{name: authenticatedUserIDPlaceholder}
				if rule != "" {
					overrides[rule] = ""
				}
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: overrides}}
				headers, err := processHeaderOverride(info, c)
				require.NoError(t, err)
				// Check the entire map, not just one spelling of the identity header.
				require.Equal(t, map[string]string{"x-vm2api-tenant": "42"}, headers)
				outbound := httptest.NewRequest(http.MethodPost, "/", nil)
				outbound.Header["X-Vm2api-Tenant"] = []string{"adapter-a"}
				outbound.Header["x-vm2api-tenant"] = []string{"adapter-b"}
				outbound.Header["X-VM2API-TENANT"] = []string{"adapter-c"}
				applyHeaderOverrideToRequest(outbound, headers)
				require.Equal(t, http.Header{"X-Vm2api-Tenant": {"42"}}, outbound.Header)
			})
		}
	}
}

func TestAuthenticatedUserHeaderRuntimeAndChannelTest(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("X-Session-Id", "browser-session")
	c.Request.Header.Set("X-VM2API-Tenant", "spoofed")
	c.Set("id", 7)
	info := &relaycommon.RelayInfo{
		IsChannelTest: true, UseRuntimeHeadersOverride: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
		RuntimeHeadersOverride: map[string]any{
			"*": "", "x-vm2api-tenant": authenticatedUserIDPlaceholder,
			"x-session-id": "{client_header:x-session-id}",
		},
	}
	headers, err := processHeaderOverride(info, c)
	require.NoError(t, err)
	// Internal channel tests use their configured test user, never browser session headers.
	require.Equal(t, map[string]string{"x-vm2api-tenant": "7"}, headers)
	c.Set("id", 0)
	_, err = processHeaderOverride(info, c)
	require.Error(t, err)
}

func TestAuthenticatedUserHeaderActualOutboundRequests(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	originalTransport := client.Transport
	t.Cleanup(func() { client.Transport = originalTransport })
	var received http.Header
	calls := 0
	client.Transport = openCodeTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		received = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
	})
	sessions := map[string]string{
		"x-session-id": "shared-session-{api_key}", "session-id": "shared-session",
		"x-thread-id": "shared-thread", "x-codex-parent-thread-id": "parent-thread",
	}
	for _, path := range []string{"responses", "chat/completions", "messages"} {
		for _, stream := range []bool{false, true} {
			for _, userID := range []int{7, 8} {
				for _, channelID := range []int{100, 101, 102} {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+path, strings.NewReader("{}"))
					c.Set("id", userID)
					c.Request.Header.Set("X-VM2API-Tenant", "same-spoofed-user")
					c.Request.Header.Set("Authorization", "Bearer client-secret")
					c.Request.Header.Set("Cookie", "private=session")
					overrides := map[string]any{"*": "", "x-vm2api-tenant": authenticatedUserIDPlaceholder}
					for name, value := range sessions {
						c.Request.Header.Set(name, value)
						overrides[name] = "{client_header:" + name + "}"
					}
					info := &relaycommon.RelayInfo{IsStream: stream, RequestId: "local-request", ChannelMeta: &relaycommon.ChannelMeta{
						ChannelId: channelID, HeadersOverride: overrides,
					}}
					resp, err := DoApiRequest(openCodeTestAdaptor{address: "https://example.invalid/v1/" + path}, c, info, strings.NewReader("{}"))
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					require.Equal(t, []string{strconv.Itoa(userID)}, received.Values("x-vm2api-tenant"))
					for name, value := range sessions {
						require.Equal(t, []string{value}, received.Values(name))
					}
					require.Equal(t, "Bearer upstream-test-key", received.Get("Authorization"))
					require.Empty(t, received.Get("Cookie"))
					require.Equal(t, "local-request", received.Get(upstreamNewAPIRequestIDHeader))
				}
			}
		}
	}
	require.Equal(t, 36, calls)
	// A missing authenticated identity must fail before any upstream request.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}"))
	c.Request.Header.Set("X-VM2API-Tenant", "7")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: map[string]any{
		"*": "", "x-vm2api-tenant": authenticatedUserIDPlaceholder,
	}}}
	_, err := DoApiRequest(openCodeTestAdaptor{address: "https://example.invalid/v1/responses"}, c, info, strings.NewReader("{}"))
	require.Error(t, err)
	require.Equal(t, 36, calls)
}

func TestHeaderOverrideClientValuesAreNotIdentityTemplates(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("X-Session-Id", "{authenticated_user_id}-{api_key}")
	value, include, err := applyHeaderOverridePlaceholders("{client_header:x-session-id}", c, "secret")
	require.NoError(t, err)
	require.True(t, include)
	require.Equal(t, "{authenticated_user_id}-{api_key}", value)
}

func TestHeaderOverrideExistingConfigurationCompatibility(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("X-Trace-Id", "client-value")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ApiKey: "channel-key",
		HeadersOverride: map[string]any{
			"*": "", "x-TRACE-id": "administrator-value", "Authorization": "Bearer {api_key}",
			"x-session-id": "{client_header:x-session-id}",
		},
	}}
	headers, err := processHeaderOverride(info, c)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"x-trace-id": "administrator-value", "authorization": "Bearer channel-key",
	}, headers)
	// Existing configurations do not need an authenticated-user placeholder.
	outbound := &http.Request{}
	applyHeaderOverrideToRequest(outbound, headers)
	require.Equal(t, "administrator-value", outbound.Header.Get("X-Trace-Id"))
	require.Equal(t, "Bearer channel-key", outbound.Header.Get("Authorization"))
	require.Empty(t, outbound.Header.Get("X-Session-Id"))
	require.Empty(t, outbound.Header.Get("X-VM2API-Tenant"))
}
