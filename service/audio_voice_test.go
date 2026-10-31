package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestAudioVoiceProviderNeverFollowsRedirects(t *testing.T) {
	InitHttpClient()
	var leaked atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer destination.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	_, err := callVoiceProvider(context.Background(), &model.Channel{Key: "private", BaseURL: common.GetPointer(upstream.URL)}, map[string]any{"action": "query_voice", "voice_id": "fixture"})
	require.Error(t, err)
	require.False(t, leaked.Load())
	require.NotContains(t, err.Error(), "private")
}

func TestAudioVoiceProviderBoundsResponseAndURL(t *testing.T) {
	InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", (1<<20)+1))) }))
	defer upstream.Close()
	_, err := callVoiceProvider(context.Background(), &model.Channel{Key: "private", BaseURL: common.GetPointer(upstream.URL)}, map[string]any{"action": "query_voice"})
	require.Error(t, err)
	for _, base := range []string{"file:///tmp/recording", "https://user:password@example.com", "https://example.com/compatible-mode/v1", "https://example.com?key=private"} {
		_, err = AudioVoiceURL(base)
		require.Error(t, err)
	}
	require.Equal(t, AudioVoiceCredentialHash("https://example.com", "one"), AudioVoiceCredentialHash("wss://example.com/api-ws/v1/inference", "one"))
	require.NotEqual(t, AudioVoiceCredentialHash("https://example.com", "one"), AudioVoiceCredentialHash("https://example.com", "two"))
}

func TestAudioVoiceProviderRejectsEmptySuccess(t *testing.T) {
	InitHttpClient()
	for _, body := range []string{`{}`, `{"output":null}`, `{"code":"InvalidParameter","output":{}}`} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := callVoiceProvider(context.Background(), &model.Channel{Key: "private", BaseURL: common.GetPointer(upstream.URL)}, map[string]any{"action": "delete_voice"})
		upstream.Close()
		require.Error(t, err)
	}
}
