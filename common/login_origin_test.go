package common

import (
	"github.com/stretchr/testify/require"
	"net/url"
	"strings"
	"testing"
)

func TestCanonicalLoginOrigin(t *testing.T) {
	for _, h := range []string{"meta-api.com", "meta-api.vip", "cn.meta-api.vip", "nan.meta-api.vip"} {
		require.Equal(t, MetaLoginOrigin, CanonicalLoginOrigin("https://"+h))
		require.Equal(t, MetaLoginOrigin, CanonicalLoginOrigin("https://"+h+"/"))
	}
	for _, origin := range []string{"https://meta-api.vip.evil.test", "https://evil-meta-api.com", "https://meta-api.com:444", "https://user@meta-api.com", "http://localhost:3000", "https://example.com", "https://meta-api.vip/prefix"} {
		require.Equal(t, origin, CanonicalLoginOrigin(origin))
	}
}
func TestCanonicalLoginPageFourOrigins(t *testing.T) {
	id := strings.Repeat("a", 43)
	for _, h := range []string{"meta-api.vip", "cn.meta-api.vip", "nan.meta-api.vip"} {
		got := CanonicalLoginPage(h, "/login/", "next="+url.QueryEscape("/desktop-login?request_id="+id)+"&aff=123&code=secret&state=secret&token=secret")
		u, err := url.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "meta-api.com", u.Host)
		require.Equal(t, "/login", u.Path)
		require.Equal(t, "/desktop-login?request_id="+id, u.Query().Get("next"))
		require.Equal(t, "123", u.Query().Get("aff"))
		require.Len(t, u.Query(), 2)
		require.Equal(t, MetaLoginOrigin+"/desktop-login?request_id="+id, CanonicalLoginPage(h, "/desktop-login", "request_id="+id+"&token=secret"))
	}
	require.Empty(t, CanonicalLoginPage("meta-api.com", "/login", ""))
	for _, h := range []string{"meta-api.vip.evil.test", "meta-api.com:444", "localhost:3000", "evil.test"} {
		require.Empty(t, CanonicalLoginPage(h, "/login", ""))
	}
	for _, p := range []string{"/api/user/login", "/api/desktop-auth/exchange", "/v1/responses", "/oauth/google", "/console/personal", "/login-evil"} {
		require.Empty(t, CanonicalLoginPage("meta-api.vip", p, "code=secret"))
	}
	for _, q := range []string{"next=https://evil.test", "next=%2F%2Fevil.test", "next=%2F%5Cevil.test", "next=%2Fconsole&next=%2Fapi%2Fforum%2Fsso%2Fstart", "next=%2Fconsole%0Aevil", "aff=ok&aff=bad", "aff=%3Cscript%3E", "request_id=bad", "next=%2Fdesktop-login%3Frequest_id%3Dbad"} {
		require.Equal(t, MetaLoginOrigin+"/login", CanonicalLoginPage("meta-api.vip", "/login", q))
	}
}
