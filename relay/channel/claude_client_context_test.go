package channel

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestClaudeClientContextRedirectBoundary(t *testing.T) {
	base := &http.Client{}
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAnthropic}}
	client := claudeClientContextHTTPClient(base, info)
	require.NotSame(t, base, client)
	require.Nil(t, base.CheckRedirect)
	first, _ := http.NewRequest(http.MethodGet, "https://upstream.example/v1/messages", nil)
	for _, dest := range []string{"https://another.example/v1/messages", "http://upstream.example/v1/messages", "https://upstream.example:8443/v1/messages"} {
		req, _ := http.NewRequest(http.MethodGet, dest, nil)
		require.Error(t, client.CheckRedirect(req, []*http.Request{first}))
	}
	same, _ := http.NewRequest(http.MethodGet, "https://upstream.example/next", nil)
	require.NoError(t, client.CheckRedirect(same, []*http.Request{first}))
	chain := make([]*http.Request, 10)
	for i := range chain {
		chain[i] = first
	}
	require.Error(t, client.CheckRedirect(same, chain))
	sentinel := errors.New("existing policy")
	base.CheckRedirect = func(*http.Request, []*http.Request) error { return sentinel }
	require.ErrorIs(t, claudeClientContextHTTPClient(base, info).CheckRedirect(same, []*http.Request{first}), sentinel)
	require.Same(t, base, claudeClientContextHTTPClient(base, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI}))
	require.Same(t, base, claudeClientContextHTTPClient(base, nil))
	require.Same(t, base, claudeClientContextHTTPClient(base, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeOpenAI}}))
}
