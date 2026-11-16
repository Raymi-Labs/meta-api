package gemini

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelDiscoveryPrefersCompatibleDraftBase(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/api/v1/models", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.Equal(t, "tenant", r.Header.Get("X-Tenant"))
		w.Write([]byte(`{"data":[{"id":"gemini-4-carbon"},{"id":"gemini-4-hydrogen"},{"id":"gemini-4-carbon"}]}`))
	}))
	defer server.Close()
	got, err := FetchGeminiModelsWithHeaders(server.URL+"/api/v1/", "test-key", "", http.Header{"X-Tenant": []string{"tenant"}})
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-4-carbon", "gemini-4-hydrogen"}, got)
	require.Equal(t, 1, calls)
}

func TestModelDiscoveryNativePaginationAndFailure(t *testing.T) {
	for _, mode := range []string{"pages", "repeat", "partial-error", "empty", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					http.NotFound(w, r)
					return
				}
				require.Equal(t, "/v1beta/models", r.URL.Path)
				require.Equal(t, "test-key", r.Header.Get("X-Goog-Api-Key"))
				if mode == "empty" {
					w.Write([]byte(`{"models":[]}`))
					return
				}
				if mode == "malformed" {
					w.Write([]byte(`{"error":"secret-key must never surface"}`))
					return
				}
				if r.URL.Query().Get("pageToken") == "" {
					w.Write([]byte(`{"models":[{"name":"models/gemini-4-carbon"}],"nextPageToken":"a+b&c"}`))
					return
				}
				require.Equal(t, "a+b&c", r.URL.Query().Get("pageToken"))
				if mode == "partial-error" {
					http.Error(w, "secret-key", 401)
					return
				}
				if mode == "repeat" {
					w.Write([]byte(`{"models":[],"nextPageToken":"a+b&c"}`))
					return
				}
				w.Write([]byte(`{"models":[{"name":"models/gemini-4-hydrogen"},{"name":"models/gemini-4-carbon"}]}`))
			}))
			defer server.Close()
			got, err := FetchGeminiModels(server.URL+"/v1beta/", "test-key", "")
			if mode == "pages" {
				require.NoError(t, err)
				require.Len(t, got, 2)
			} else if mode == "empty" {
				require.NoError(t, err)
				require.Empty(t, got)
			} else {
				require.Error(t, err)
				require.Nil(t, got)
				require.NotContains(t, err.Error(), "secret-key")
			}
		})
	}
}

func TestModelDiscoveryNeverFollowsCredentialRedirect(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 302) }))
	defer server.Close()
	_, err := FetchGeminiModels(server.URL, "private-key", "")
	require.Error(t, err)
	require.False(t, leaked)
}

func TestModelDiscoveryEmptyCompatibleIsAuthoritative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	got, err := FetchGeminiModels(server.URL, "key", "")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestModelDiscoveryHonorsProxyAndAuthOverride(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "unreachable.example", r.URL.Host)
		require.Equal(t, "/prefix/v1/models", r.URL.Path)
		require.Equal(t, "Bearer overridden", r.Header.Get("Authorization"))
		w.Write([]byte(`{"data":[{"id":"gemini-4-hydrogen"}]}`))
	}))
	defer proxy.Close()
	got, err := FetchGeminiModelsWithHeaders("http://unreachable.example/prefix", "original", proxy.URL, http.Header{"Authorization": []string{"Bearer overridden"}})
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-4-hydrogen"}, got)
	for _, base := range []string{"file:///tmp/models", "https://user:pass@example.com", "https://example.com?key=secret"} {
		_, err := FetchGeminiModels(base, "key", "")
		require.Error(t, err)
	}
}
