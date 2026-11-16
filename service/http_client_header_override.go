package service

import (
	"container/list"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const headerOverrideCacheLimit = 32

type headerOverrideKey struct {
	base *http.Client
	wait time.Duration
}
type headerOverrideEntry struct {
	key    headerOverrideKey
	client *http.Client
}

var headerOverrides = struct {
	sync.Mutex
	entries map[headerOverrideKey]*list.Element
	order   *list.List
}{entries: make(map[headerOverrideKey]*list.Element), order: list.New()}

// WithResponseHeaderTimeout preserves all client policies and proxy/TLS options.
// Never mutate a shared Transport: a concurrent streaming request uses the base.
func WithResponseHeaderTimeout(base *http.Client, wait time.Duration) (*http.Client, error) {
	if base == nil {
		return nil, fmt.Errorf("relay HTTP client is not initialized")
	}
	if wait <= 0 || wait > 600*time.Second {
		return nil, fmt.Errorf("invalid response header timeout")
	}
	transport, ok := base.Transport.(*http.Transport)
	if !ok || transport == nil {
		return nil, fmt.Errorf("relay transport does not support header timeout override")
	}
	key := headerOverrideKey{base, wait}
	headerOverrides.Lock()
	defer headerOverrides.Unlock()
	if entry := headerOverrides.entries[key]; entry != nil {
		headerOverrides.order.MoveToFront(entry)
		return entry.Value.(headerOverrideEntry).client, nil
	}
	cloned := *base
	clonedTransport := transport.Clone()
	clonedTransport.ResponseHeaderTimeout = wait
	cloned.Transport = clonedTransport
	entry := headerOverrides.order.PushFront(headerOverrideEntry{key, &cloned})
	headerOverrides.entries[key] = entry
	if headerOverrides.order.Len() > headerOverrideCacheLimit {
		old := headerOverrides.order.Back()
		value := old.Value.(headerOverrideEntry)
		delete(headerOverrides.entries, value.key)
		headerOverrides.order.Remove(old)
		value.client.CloseIdleConnections()
	}
	return &cloned, nil
}

func resetHeaderOverrideClients() {
	headerOverrides.Lock()
	defer headerOverrides.Unlock()
	for _, entry := range headerOverrides.entries {
		entry.Value.(headerOverrideEntry).client.CloseIdleConnections()
	}
	headerOverrides.entries = make(map[headerOverrideKey]*list.Element)
	headerOverrides.order.Init()
}

const relayHTTPTimeoutContextKey = "relay_http_timeout"

func RecordRelayHTTPTimeout(c *gin.Context, client *http.Client, nonstreamOverridden, streamOverridden bool) {
	c.Set(relayHTTPTimeoutContextKey, map[string]interface{}{
		"response_header_timeout_seconds": responseHeaderTimeout(client).Seconds(),
		"client_total_timeout_seconds":    client.Timeout.Seconds(),
		"channel_nonstream_override":      nonstreamOverridden,
		"channel_stream_override":         streamOverridden,
	})
}

func AppendRelayHTTPTimeout(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil {
		return
	}
	if value, ok := c.Get(relayHTTPTimeoutContextKey); ok {
		adminInfo[relayHTTPTimeoutContextKey] = value
	}
}
