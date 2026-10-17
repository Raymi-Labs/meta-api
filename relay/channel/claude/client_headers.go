package claude

import (
	"net/http"
	"strings"
)

// Client identity is protocol context, never upstream authentication. Keep this
// list explicit: forwarding all headers would leak cookies, API keys and proxy
// credentials. Channel-level overrides still run after the adaptor.
func forwardClaudeClientHeaders(incoming, outgoing http.Header) {
	for _, name := range []string{
		"User-Agent", "X-App", "X-Claude-Code-Session-Id",
		"X-Claude-Code-Request-Class", "X-Client-Request-Id",
	} {
		for _, value := range claudeEndToEndHeaderValues(incoming, name) {
			outgoing.Add(name, value)
		}
	}
}

// A client can nominate additional hop-by-hop fields using Connection.
// Such values must not cross the gateway even if otherwise permitted.
func claudeEndToEndHeaderValues(headers http.Header, name string) []string {
	for _, connection := range headers.Values("Connection") {
		for _, token := range strings.Split(connection, ",") {
			if strings.EqualFold(strings.TrimSpace(token), name) {
				return nil
			}
		}
	}
	return headers.Values(name)
}
