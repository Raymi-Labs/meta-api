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

func safeLoginNext(raw string) string {
	if len(raw) > 4096 || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	decoded := raw
	for i := 0; i < 3; i++ {
		if strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "\\\r\n\t") {
			return ""
		}
		for _, r := range decoded {
			if r < 32 || r == 127 {
				return ""
			}
		}
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return ""
		}
		if next == decoded {
			break
		}
		decoded = next
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host != "" || u.Scheme != "" {
		return ""
	}
	// Destinations needed by login and desktop consent; no arbitrary external URLs.
	if u.Path == "/desktop-login" {
		ids := u.Query()["request_id"]
		if len(ids) != 1 || !validLoginRequestID(ids[0]) {
			return ""
		}
		return "/desktop-login?request_id=" + ids[0]
	}
	if u.Path == "/api/forum/sso/start" {
		return u.Path
	}
	if u.Path == "/console" || strings.HasPrefix(u.Path, "/console/") {
		return u.EscapedPath()
	}
	return ""
}
func validLoginRequestID(s string) bool {
	if len(s) != 43 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// Only browser entry routes are redirected. Never move an OAuth callback code,
// API request, password POST, or arbitrary query parameter between domains.
func CanonicalLoginPage(host, path, rawQuery string) string {
	if host == "meta-api.com" || !IsMetaLoginHost(host) {
		return ""
	}
	path = strings.TrimSuffix(path, "/")
	if path != "/login" && path != "/register" && path != "/desktop-login" {
		return ""
	}
	input, err := url.ParseQuery(rawQuery)
	if err != nil {
		input = url.Values{}
	}
	output := url.Values{}
	if path == "/desktop-login" {
		if ids := input["request_id"]; len(ids) == 1 && validLoginRequestID(ids[0]) {
			output.Set("request_id", ids[0])
		}
	} else {
		if next := input["next"]; len(next) == 1 {
			if safe := safeLoginNext(next[0]); safe != "" {
				output.Set("next", safe)
			}
		}
		if aff := input["aff"]; len(aff) == 1 && len(aff[0]) > 0 && len(aff[0]) <= 64 {
			valid := true
			for _, c := range aff[0] {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
					valid = false
				}
			}
			if valid {
				output.Set("aff", aff[0])
			}
		}
	}
	target := MetaLoginOrigin + path
	if q := output.Encode(); q != "" {
		target += "?" + q
	}
	return target
}
