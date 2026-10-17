package channel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

// Claude client context is scoped to the configured origin. Refuse redirects
// that could disclose forwarded identity or session headers elsewhere, without
// mutating the shared transport or replacing an existing redirect policy.
func claudeClientContextHTTPClient(client *http.Client, info *relaycommon.RelayInfo) *http.Client {
	if client == nil || info == nil || info.RelayFormat != types.RelayFormatClaude || info.ChannelMeta == nil || info.ApiType != constant.APITypeAnthropic {
		return client
	}
	cloned := *client
	previous := client.CheckRedirect
	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && (!strings.EqualFold(req.URL.Scheme, via[0].URL.Scheme) || !strings.EqualFold(req.URL.Host, via[0].URL.Host)) {
			return errors.New("Claude client context redirect to another origin is not allowed")
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &cloned
}
