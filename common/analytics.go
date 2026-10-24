package common

import (
	"bytes"
	"net/url"
	"os"
	"strings"
)

type umamiAnalyticsConfig struct {
	WebsiteID string `json:"websiteId"`
	ScriptURL string `json:"scriptUrl"`
}

type googleAnalyticsConfig struct {
	MeasurementID string `json:"measurementId"`
}

// InjectAnalyticsConfig only publishes inert configuration. The browser decides
// whether to load either provider after reading the visitor's saved consent.
// Call this for both the SPA fallback and prerendered HTML; repeated calls are safe.
func InjectAnalyticsConfig(template []byte) []byte {
	umami := umamiAnalyticsConfig{WebsiteID: strings.TrimSpace(os.Getenv("UMAMI_WEBSITE_ID"))}
	if umami.WebsiteID != "" {
		umami.ScriptURL = strings.TrimSpace(os.Getenv("UMAMI_SCRIPT_URL"))
		if umami.ScriptURL == "" {
			umami.ScriptURL = "https://analytics.umami.is/script.js"
		}
		parsed, err := url.Parse(umami.ScriptURL)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
			umami = umamiAnalyticsConfig{}
		}
	}
	google := googleAnalyticsConfig{MeasurementID: strings.TrimSpace(os.Getenv("GOOGLE_ANALYTICS_ID"))}
	template = injectAnalyticsJSON(template, "newapi-analytics-umami", "<!--umami-->", "<!--Umami QuantumNous-->", umami)
	return injectAnalyticsJSON(template, "newapi-analytics-google", "<!--Google Analytics-->", "<!--Google Analytics QuantumNous-->", google)
}

func injectAnalyticsJSON(template []byte, id, placeholder, attribution string, config any) []byte {
	idAttribute := `id="` + id + `"`
	if bytes.Contains(template, []byte(idAttribute)) {
		return template
	}
	// Marshal escapes HTML-sensitive characters, including a literal </script>.
	payload, err := Marshal(config)
	if err != nil {
		return template
	}
	block := []byte(`<script type="application/json" ` + idAttribute + `>` + string(payload) + "</script>\n" + attribution + "\n")
	if bytes.Contains(template, []byte(placeholder)) {
		return bytes.Replace(template, []byte(placeholder), block, 1)
	}
	if bytes.Contains(template, []byte(attribution)) {
		return bytes.Replace(template, []byte(attribution), block, 1)
	}
	if end := bytes.Index(template, []byte("</head>")); end >= 0 {
		out := make([]byte, 0, len(template)+len(block))
		out = append(out, template[:end]...)
		out = append(out, block...)
		return append(out, template[end:]...)
	}
	// Missing HTML markers must not turn configuration into executable code.
	out := append([]byte(nil), template...)
	return append(out, block...)
}
