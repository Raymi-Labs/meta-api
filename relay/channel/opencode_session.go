package channel

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const openCodeSessionHeader = "X-Opencode-Session"

func isOpenCodeAPIURL(target *url.URL) bool {
	if target == nil || !strings.EqualFold(target.Scheme, "https") ||
		!strings.EqualFold(target.Hostname(), "opencode.ai") ||
		(target.Port() != "" && target.Port() != "443") {
		return false
	}
	return strings.HasPrefix(target.Path, "/zen/v1/") || strings.HasPrefix(target.Path, "/zen/go/v1/")
}

// Apply defaults before administrator overrides. A client owns its conversation
// identity; only an isolated dashboard channel test may create a new one here.
func setupOpenCodeSessionHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) {
	if req == nil || !isOpenCodeAPIURL(req.URL) {
		return
	}
	if info != nil && info.IsChannelTest {
		req.Header.Set(openCodeSessionHeader, uuid.NewString())
		return
	}
	if c == nil || c.Request == nil {
		return
	}
	if value := c.Request.Header.Get(openCodeSessionHeader); strings.TrimSpace(value) != "" {
		req.Header.Set(openCodeSessionHeader, value)
	}
}

// Go copies custom headers on redirects. Keep this default scoped to OpenCode
// API destinations without changing shared clients or existing redirect checks.
func openCodeSessionHTTPClient(client *http.Client, initial *http.Request) *http.Client {
	if initial == nil || !isOpenCodeAPIURL(initial.URL) || initial.Header.Get(openCodeSessionHeader) == "" {
		return client
	}
	cloned := *client
	previous := client.CheckRedirect
	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !isOpenCodeAPIURL(req.URL) {
			req.Header.Del(openCodeSessionHeader)
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
