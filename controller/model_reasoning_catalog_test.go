package controller

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type reasoningTestTransport func(*http.Request) (*http.Response, error)

func (f reasoningTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialReasoningFetchIsBoundedAndHasNoUserCredentials(t *testing.T) {
	for _, test := range []struct {
		code int
		body string
		ok   bool
	}{{304, "", true}, {301, "", false}, {503, "", false}, {200, "{}", false}, {200, strings.Repeat(" ", 8<<20+1), false}} {
		client := &http.Client{Transport: reasoningTestTransport(func(r *http.Request) (*http.Response, error) {
			require.Equal(t, officialReasoningURLs["codex"], r.URL.String())
			require.Equal(t, http.Header{"If-None-Match": {`"seed"`}}, r.Header)
			return &http.Response{StatusCode: test.code, Body: io.NopCloser(strings.NewReader(test.body)), Header: http.Header{}}, nil
		})}
		etag, err := fetchOfficialReasoningCatalog(context.Background(), client, "codex", `"seed"`)
		require.Equal(t, `"seed"`, etag)
		require.Equal(t, test.ok, err == nil)
	}
	client := reasoningCatalogClient()
	require.False(t, client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	require.ErrorIs(t, client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	client.Transport = reasoningTestTransport(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	go func() { defer close(done); runOfficialReasoningCatalogTask(ctx, client, "codex", time.Hour) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("catalog task did not stop")
	}
}
