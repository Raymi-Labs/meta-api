package common

import (
	"github.com/stretchr/testify/require"
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
