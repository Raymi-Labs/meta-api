package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestInitHttpClientUsesImageHeaderTimeout(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	originalIdleConnTimeout := common.RelayIdleConnTimeout
	originalHeaderTimeout := common.RelayResponseHeaderTimeout
	originalImageHeaderTimeout := common.RelayImageResponseHeaderTimeout
	defer func() {
		common.RelayTimeout = originalRelayTimeout
		common.RelayIdleConnTimeout = originalIdleConnTimeout
		common.RelayResponseHeaderTimeout = originalHeaderTimeout
		common.RelayImageResponseHeaderTimeout = originalImageHeaderTimeout
		InitHttpClient()
	}()

	common.RelayTimeout = 0
	common.RelayIdleConnTimeout = 45
	common.RelayResponseHeaderTimeout = 60
	common.RelayImageResponseHeaderTimeout = 300
	InitHttpClient()

	if got := GetResponseHeaderTimeout(); got != 60*time.Second {
		t.Fatalf("default response header timeout = %s, want 60s", got)
	}
	if got := GetImageResponseHeaderTimeout(); got != 300*time.Second {
		t.Fatalf("image response header timeout = %s, want 300s", got)
	}
	if got := GetHttpClient().Transport.(*http.Transport).IdleConnTimeout; got != 45*time.Second {
		t.Fatalf("idle conn timeout = %s, want 45s", got)
	}
}

func TestHTTPClientTLSConfigsAreIndependent(t *testing.T) {
	old := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = true
	t.Cleanup(func() {
		common.TLSInsecureSkipVerify = old
		ResetProxyClientCache()
	})
	ResetProxyClientCache()
	transports := []*http.Transport{newRelayTransport(time.Second), newRelayTransport(2 * time.Second), newCRSHTTPClient().Transport.(*http.Transport)}
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		client, err := NewProxyHttpClient(scheme + "://127.0.0.1:19080")
		require.NoError(t, err)
		transports = append(transports, client.Transport.(*http.Transport))
	}
	for i, transport := range transports {
		require.NotSame(t, common.InsecureTLSConfig, transport.TLSClientConfig)
		require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
		for _, other := range transports[:i] {
			require.NotSame(t, transport.TLSClientConfig, other.TLSClientConfig)
		}
	}
	transports[0].TLSClientConfig.InsecureSkipVerify = false
	for _, transport := range transports[1:] {
		require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	}
	require.True(t, common.InsecureTLSConfig.InsecureSkipVerify)
	common.TLSInsecureSkipVerify = false
	require.Nil(t, newRelayTransport(time.Second).TLSClientConfig)
}
