package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type discoveryTransport func(*http.Request) (*http.Response, error)

func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func previewModels(t *testing.T, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, err := common.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/channel/fetch_models", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	FetchModels(c)
	var result map[string]any
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
	return w.Code, result
}

func TestDiscoveryDraftUsesStoredKeyWithoutSaving(t *testing.T) {
	setupChannelSearchControllerTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/draft/v1/models", r.URL.Path)
		require.Equal(t, "Bearer saved-key", r.Header.Get("Authorization"))
		require.Equal(t, "draft-tenant", r.Header.Get("X-Tenant"))
		w.Write([]byte(`{"data":[{"id":"gemini-4-carbon"}]}`))
	}))
	defer server.Close()
	savedURL := server.URL + "/saved"
	ch := model.Channel{Id: 901, Type: 24, Key: "saved-key", BaseURL: &savedURL, Models: "original", Status: 1}
	require.NoError(t, model.DB.Create(&ch).Error)
	status, result := previewModels(t, map[string]any{"channel_id": 901, "type": 24, "base_url": server.URL + "/draft/v1/", "header_override": `{"X-Tenant":"draft-tenant"}`})
	require.Equal(t, 200, status)
	require.Equal(t, true, result["success"])
	require.Equal(t, "upstream", result["source"])
	require.NotContains(t, result, "key")
	var after model.Channel
	require.NoError(t, model.DB.First(&after, 901).Error)
	require.Equal(t, savedURL, *after.BaseURL)
	require.Equal(t, "original", after.Models)
	require.Nil(t, after.HeaderOverride)
	status, _ = previewModels(t, map[string]any{"channel_id": 901, "type": 24, "base_url": "https://different.example"})
	require.Equal(t, 400, status)
	defaultChannel := model.Channel{Id: 902, Type: 1, Key: "saved-default-key", Status: 1}
	require.NoError(t, model.DB.Create(&defaultChannel).Error)
	status, _ = previewModels(t, map[string]any{"channel_id": 902, "type": 24})
	require.Equal(t, 400, status)
}

func TestDiscoveryReferenceIsManualAndAnonymous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private-key", 401) }))
	defer server.Close()
	original := http.DefaultTransport
	http.DefaultTransport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "aistudio.google.com" {
			require.Empty(t, r.Header.Get("Authorization"))
			require.Empty(t, r.Header.Get("X-Goog-Api-Key"))
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("*[`gemini-4-carbon`](/docs/models/gemini-4-carbon)*\n*[`gemini-4-hydrogen`](/docs/models/gemini-4-hydrogen)*"))}, nil
		}
		return original.RoundTrip(r)
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	_, result := previewModels(t, map[string]any{"type": 24, "base_url": server.URL, "key": "private-key", "allow_reference_fallback": true})
	require.Equal(t, true, result["success"])
	require.Equal(t, "official_reference", result["source"])
	require.Len(t, result["data"], 2)
	_, strict := previewModels(t, map[string]any{"type": 24, "base_url": server.URL, "key": "private-key"})
	require.Equal(t, false, strict["success"])
	ch := &model.Channel{Type: 24, Key: "private-key", BaseURL: common.GetPointer(server.URL)}
	_, err := fetchChannelUpstreamModelIDs(ch)
	require.Error(t, err)
}

func TestDiscoveryBuiltinFallbackAndBadHeader(t *testing.T) {
	old := channelId2Models
	channelId2Models = map[int][]string{1: {"reference-model"}}
	t.Cleanup(func() { channelId2Models = old })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":[]}`)) }))
	defer server.Close()
	_, result := previewModels(t, map[string]any{"type": 1, "base_url": server.URL + "/v1/", "allow_reference_fallback": true})
	require.Equal(t, "builtin_reference", result["source"])
	status, _ := previewModels(t, map[string]any{"type": 1, "base_url": server.URL, "header_override": "broken"})
	require.Equal(t, 400, status)
}
