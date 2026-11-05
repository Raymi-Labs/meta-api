package oauth

import (
	"context"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGoogleTokenExchangeCanonicalLoginOrigin(t *testing.T) {
	old := system_setting.ServerAddress
	t.Cleanup(func() { system_setting.ServerAddress = old })
	for _, base := range []string{"https://meta-api.com", "https://meta-api.vip", "https://cn.meta-api.vip/", "https://nan.meta-api.vip"} {
		t.Run(base, func(t *testing.T) {
			system_setting.ServerAddress = base
			var got url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				got = r.PostForm
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"synthetic","token_type":"Bearer"}`))
			}))
			defer server.Close()
			p := NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: "google", Name: "Google", TokenEndpoint: server.URL, ClientId: "synthetic-client", ClientSecret: "synthetic-secret"})
			_, err := p.ExchangeToken(context.Background(), "synthetic-code", nil)
			require.NoError(t, err)
			require.Equal(t, "https://meta-api.com/oauth/google", got.Get("redirect_uri"))
			require.Equal(t, "synthetic-code", got.Get("code"))
		})
	}
}
