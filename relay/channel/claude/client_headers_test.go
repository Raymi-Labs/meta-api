package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeReviewNativeHeaderBoundary(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			incoming := ctx.Request.Header
			incoming.Set("User-Agent", "claude-cli/2.1.301 (external, cli)")
			incoming.Set("X-App", "cli")
			incoming.Set("X-Claude-Code-Session-Id", "synthetic-session")
			incoming.Set("X-Claude-Code-Request-Class", "main")
			incoming.Set("X-Client-Request-Id", "synthetic-request")
			for _, name := range []string{"Authorization", "X-Api-Key", "Cookie", "Proxy-Authorization", "X-Unrelated-Private"} {
				incoming.Set(name, "caller-secret")
			}
			incoming.Add("Anthropic-Beta", "dangerous-tool-use-2026-09-03")
			incoming.Add("Anthropic-Beta", "future-protocol-2099-01-01")
			out := make(http.Header)
			info := &relaycommon.RelayInfo{RelayFormat: format, ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "upstream-secret"}}
			require.NoError(t, (&Adaptor{}).SetupRequestHeader(ctx, &out, info))
			require.Equal(t, "upstream-secret", out.Get("X-Api-Key"))
			require.Equal(t, "dangerous-tool-use-2026-09-03,future-protocol-2099-01-01", out.Get("Anthropic-Beta"))
			for _, name := range []string{"User-Agent", "X-App", "X-Claude-Code-Session-Id", "X-Claude-Code-Request-Class", "X-Client-Request-Id"} {
				if format == types.RelayFormatClaude {
					require.Equal(t, incoming.Get(name), out.Get(name))
				} else {
					require.Empty(t, out.Get(name))
				}
			}
			for name, values := range out {
				require.NotContains(t, strings.Join(values, ","), "caller-secret", name)
			}
		})
	}
}

func TestClaudeReviewConnectionNominatedHeadersAreNotForwarded(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	for _, name := range []string{"User-Agent", "X-App", "Anthropic-Beta", "Anthropic-Version", "X-Claude-Code-Session-Id"} {
		ctx.Request.Header.Set(name, "do-not-forward")
	}
	ctx.Request.Header.Add("Connection", "keep-alive, uSeR-aGeNt, x-app")
	ctx.Request.Header.Add("Connection", "anthropic-beta, anthropic-version, x-claude-code-session-id")
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "upstream"}}
	out := make(http.Header)
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(ctx, &out, info))
	for _, name := range []string{"User-Agent", "X-App", "Anthropic-Beta", "X-Claude-Code-Session-Id"} {
		require.Empty(t, out.Get(name))
	}
	require.Equal(t, "2023-06-01", out.Get("Anthropic-Version"))
}

func TestClaudeReviewRealRequestKeepsAdminOverrideAuthority(t *testing.T) {
	service.InitHttpClient()
	t.Cleanup(func() { service.GetHttpClient().CloseIdleConnections() })
	seen := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstream.Close)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.301 (external, cli)")
	ctx.Request.Header.Set("X-App", "cli")
	ctx.Request.Header.Set("Authorization", "Bearer caller-secret")
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: upstream.URL, ApiKey: "upstream", HeadersOverride: map[string]interface{}{"User-Agent": "operator-selected"}, ChannelSetting: dto.ChannelSettings{}}}
	resp, err := channel.DoApiRequest(&Adaptor{}, ctx, info, strings.NewReader(`{"model":"claude-test"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	h := <-seen
	require.Equal(t, "operator-selected", h.Get("User-Agent"))
	require.Equal(t, "cli", h.Get("X-App"))
	require.Equal(t, "upstream", h.Get("X-Api-Key"))
	require.Empty(t, h.Get("Authorization"))
}
