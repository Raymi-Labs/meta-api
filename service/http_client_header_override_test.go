package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHeaderOverrideWaitIsolationAndBody(t *testing.T) {
	t.Cleanup(resetHeaderOverrideClients)
	base := newRelayHttpClient(10 * time.Millisecond)
	base.Timeout = 2 * time.Second
	t.Cleanup(base.CloseIdleConnections)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(70 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		// Body may exceed the header deadline; it is not a total deadline.
		select {
		case <-time.After(180 * time.Millisecond):
			_, _ = w.Write([]byte("done"))
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	_, err := base.Get(server.URL)
	require.ErrorContains(t, err, "timeout awaiting response headers")
	override, err := WithResponseHeaderTimeout(base, 150*time.Millisecond)
	require.NoError(t, err)
	resp, err := override.Get(server.URL)
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, err)
	require.Equal(t, "done", string(data))
	require.Equal(t, 10*time.Millisecond, responseHeaderTimeout(base))
	require.Equal(t, base.Timeout, override.Timeout)
	again, err := WithResponseHeaderTimeout(base, 150*time.Millisecond)
	require.NoError(t, err)
	require.Same(t, override, again)
}

func TestHeaderOverrideCancellationAndTotalLimit(t *testing.T) {
	t.Cleanup(resetHeaderOverrideClients)
	started := make(chan struct{}, 2)
	cancelled := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer server.Close()
	base := newRelayHttpClient(time.Second)
	base.Timeout = 60 * time.Millisecond
	t.Cleanup(base.CloseIdleConnections)
	client, err := WithResponseHeaderTimeout(base, 120*time.Second)
	require.NoError(t, err)
	_, err = client.Get(server.URL)
	require.Error(t, err)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("total deadline did not cancel upstream")
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	done := make(chan error, 1)
	go func() { _, err := client.Do(req); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("caller cancellation did not reach upstream")
	}
}

func TestHeaderOverridePreservesProxyAndRedirect(t *testing.T) {
	t.Cleanup(resetHeaderOverrideClients)
	proxyCalled := false
	p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		http.Redirect(w, r, "http://other.invalid/", http.StatusFound)
	}))
	defer p.Close()
	base := newRelayHttpClient(time.Second)
	t.Cleanup(base.CloseIdleConnections)
	proxyURL, _ := url.Parse(p.URL)
	base.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
	reject := errors.New("redirect rejected")
	base.CheckRedirect = func(*http.Request, []*http.Request) error { return reject }
	client, err := WithResponseHeaderTimeout(base, 2*time.Second)
	require.NoError(t, err)
	_, err = client.Get("http://unresolvable.invalid/")
	require.ErrorIs(t, err, reject)
	require.True(t, proxyCalled)
}

func TestHeaderOverrideConcurrentBoundedCache(t *testing.T) {
	resetHeaderOverrideClients()
	t.Cleanup(resetHeaderOverrideClients)
	base := newRelayHttpClient(time.Second)
	t.Cleanup(base.CloseIdleConnections)
	var wg sync.WaitGroup
	for i := 1; i <= 80; i++ {
		wg.Add(1)
		go func(seconds int) {
			defer wg.Done()
			client, err := WithResponseHeaderTimeout(base, time.Duration(seconds)*time.Second)
			require.NoError(t, err)
			require.Equal(t, time.Duration(seconds)*time.Second, responseHeaderTimeout(client))
		}(i)
	}
	wg.Wait()
	require.LessOrEqual(t, len(headerOverrides.entries), headerOverrideCacheLimit)
	require.Equal(t, time.Second, responseHeaderTimeout(base))
	resetHeaderOverrideClients()
	require.Empty(t, headerOverrides.entries)
	for _, wait := range []time.Duration{0, -1, 601 * time.Second} {
		_, err := WithResponseHeaderTimeout(base, wait)
		require.Error(t, err)
	}
}

func TestHeaderOverrideProxyProtocols(t *testing.T) {
	t.Cleanup(ResetProxyClientCache)
	for _, proxyURL := range []string{"http://127.0.0.1:1", "https://127.0.0.1:1", "socks5://127.0.0.1:1", "socks5h://127.0.0.1:1"} {
		base, err := NewProxyHttpClient(proxyURL)
		require.NoError(t, err)
		client, err := WithResponseHeaderTimeout(base, 120*time.Second)
		require.NoError(t, err)
		require.Equal(t, 120*time.Second, responseHeaderTimeout(client))
		require.Equal(t, base.Timeout, client.Timeout)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
		_, err = client.Do(req)
		require.ErrorIs(t, err, context.Canceled)
	}
}

func TestHeaderOverrideAdminDiagnostics(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	client := newRelayHttpClient(120 * time.Second)
	client.Timeout = 30 * time.Second
	t.Cleanup(client.CloseIdleConnections)
	RecordRelayHTTPTimeout(ctx, client, true, false)
	admin := map[string]interface{}{}
	AppendRelayHTTPTimeout(ctx, admin)
	raw, err := common.Marshal(admin)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"response_header_timeout_seconds":120`)
	require.Contains(t, string(raw), `"client_total_timeout_seconds":30`)
	require.Contains(t, string(raw), `"channel_nonstream_override":true`)
	require.Contains(t, string(raw), `"channel_stream_override":false`)
	RecordRelayHTTPTimeout(ctx, client, false, true)
	AppendRelayHTTPTimeout(ctx, admin)
	raw, err = common.Marshal(admin)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"channel_nonstream_override":false`)
	require.Contains(t, string(raw), `"channel_stream_override":true`)
}

func TestHeaderOverrideResetDoesNotCancelInflight(t *testing.T) {
	t.Cleanup(resetHeaderOverrideClients)
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	base := newRelayHttpClient(time.Second)
	t.Cleanup(base.CloseIdleConnections)
	client, err := WithResponseHeaderTimeout(base, 2*time.Second)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		resp, err := client.Get(server.URL)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		done <- err
	}()
	<-started
	resetHeaderOverrideClients()
	close(release)
	require.NoError(t, <-done)
}

func TestHeaderOverrideSOCKSCancellationDuringHandshake(t *testing.T) {
	t.Cleanup(ResetProxyClientCache)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	base, err := NewProxyHttpClient("socks5://" + listener.Addr().String())
	require.NoError(t, err)
	client, err := WithResponseHeaderTimeout(base, 120*time.Second)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
	done := make(chan error, 1)
	go func() { _, err := client.Do(req); done <- err }()
	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("SOCKS connection was not attempted")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("cancellation ignored during SOCKS handshake")
	}
}
