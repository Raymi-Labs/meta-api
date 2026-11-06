package common

import (
	"net/url"
	"strings"
)

const MetaLoginOrigin = "https://meta-api.com"

// Exact allowlist; never trust suffix matches or proxy-supplied redirect hosts.
func IsMetaLoginHost(host string) bool {
	switch host {
	case "meta-api.com", "meta-api.vip", "cn.meta-api.vip", "nan.meta-api.vip":
		return true
	}
	return false
}

// Normalize only company-owned HTTPS origins. Other installations retain their
// configured callback origin; this is not a global ServerAddress mutation.
func CanonicalLoginOrigin(configured string) string {
	raw := strings.TrimRight(strings.TrimSpace(configured), "/")
	u, err := url.Parse(raw)
	if err == nil && u.Scheme == "https" && u.User == nil && IsMetaLoginHost(u.Host) && u.Path == "" && u.RawQuery == "" && u.Fragment == "" {
		return MetaLoginOrigin
	}
	return raw
}
