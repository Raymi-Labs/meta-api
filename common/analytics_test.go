package common

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

const analyticsTestTemplate = "<html><head><!--umami-->\n<!--Google Analytics-->\n</head><body>App</body></html>"

func setAnalyticsTestEnvironment(t *testing.T, websiteID, scriptURL, measurementID string) {
	t.Helper()
	t.Setenv("UMAMI_WEBSITE_ID", websiteID)
	t.Setenv("UMAMI_SCRIPT_URL", scriptURL)
	t.Setenv("GOOGLE_ANALYTICS_ID", measurementID)
}

func readAnalyticsTestConfig(t *testing.T, html []byte, id string, target any) {
	t.Helper()
	pattern := regexp.MustCompile(`<script type="application/json" id="` + regexp.QuoteMeta(id) + `">([^<]*)</script>`)
	matches := pattern.FindAllSubmatch(html, -1)
	if len(matches) != 1 {
		t.Fatalf("expected exactly one inert %s configuration, got %d: %s", id, len(matches), html)
	}
	if err := Unmarshal(matches[0][1], target); err != nil {
		t.Fatalf("invalid %s JSON: %v", id, err)
	}
}

func TestInjectAnalyticsConfigDisabledByDefault(t *testing.T) {
	setAnalyticsTestEnvironment(t, "", "", "")
	html := InjectAnalyticsConfig([]byte(analyticsTestTemplate))
	var umami umamiAnalyticsConfig
	var google googleAnalyticsConfig
	readAnalyticsTestConfig(t, html, "newapi-analytics-umami", &umami)
	readAnalyticsTestConfig(t, html, "newapi-analytics-google", &google)
	if umami != (umamiAnalyticsConfig{}) || google != (googleAnalyticsConfig{}) {
		t.Fatalf("expected empty provider configuration, got %#v and %#v", umami, google)
	}
	if bytes.Contains(html, []byte("src=")) || bytes.Contains(html, []byte("gtag(")) {
		t.Fatalf("configuration must not execute or fetch analytics: %s", html)
	}
	for _, attribution := range []string{"<!--Umami QuantumNous-->", "<!--Google Analytics QuantumNous-->"} {
		if !bytes.Contains(html, []byte(attribution)) {
			t.Fatalf("missing attribution %s", attribution)
		}
	}
}

func TestInjectAnalyticsConfigEscapesHTMLAndPreservesValues(t *testing.T) {
	websiteID := `site</script><script>alert("umami")</script>`
	scriptURL := `https://analytics.example/script.js?label=</script>&quote="`
	measurementID := `G-TEST</script><script>alert("ga")</script>`
	setAnalyticsTestEnvironment(t, websiteID, scriptURL, measurementID)
	html := InjectAnalyticsConfig([]byte(analyticsTestTemplate))
	var umami umamiAnalyticsConfig
	var google googleAnalyticsConfig
	readAnalyticsTestConfig(t, html, "newapi-analytics-umami", &umami)
	readAnalyticsTestConfig(t, html, "newapi-analytics-google", &google)
	if umami.WebsiteID != websiteID || umami.ScriptURL != scriptURL || google.MeasurementID != measurementID {
		t.Fatalf("configuration did not survive JSON encoding: %#v, %#v", umami, google)
	}
	if bytes.Count(html, []byte("<script")) != 2 || bytes.Count(html, []byte("</script>")) != 2 {
		t.Fatalf("configuration escaped its inert script block: %s", html)
	}
}

func TestInjectAnalyticsConfigValidatesUmamiURL(t *testing.T) {
	for _, scriptURL := range []string{
		"javascript:alert(1)", "data:text/javascript,alert(1)", "file:///script.js",
		"/script.js", "//analytics.example/script.js", "https:script.js",
		"https://", "https://user:password@analytics.example/script.js", "https://analytics.example/%zz",
	} {
		t.Run(scriptURL, func(t *testing.T) {
			setAnalyticsTestEnvironment(t, "test-site", scriptURL, "")
			var umami umamiAnalyticsConfig
			readAnalyticsTestConfig(t, InjectAnalyticsConfig([]byte(analyticsTestTemplate)), "newapi-analytics-umami", &umami)
			if umami != (umamiAnalyticsConfig{}) {
				t.Fatalf("invalid script URL must disable Umami, got %#v", umami)
			}
		})
	}
	for _, scriptURL := range []string{"", "https://analytics.example/script.js", "http://localhost:3000/script.js"} {
		t.Run("valid-"+scriptURL, func(t *testing.T) {
			setAnalyticsTestEnvironment(t, "test-site", scriptURL, "")
			var umami umamiAnalyticsConfig
			readAnalyticsTestConfig(t, InjectAnalyticsConfig([]byte(analyticsTestTemplate)), "newapi-analytics-umami", &umami)
			wantURL := scriptURL
			if wantURL == "" {
				wantURL = "https://analytics.umami.is/script.js"
			}
			if umami.WebsiteID != "test-site" || umami.ScriptURL != wantURL {
				t.Fatalf("expected supported URL %q, got %#v", wantURL, umami)
			}
		})
	}
}

func TestInjectAnalyticsConfigIsIdempotent(t *testing.T) {
	setAnalyticsTestEnvironment(t, "test-site", "", "G-TEST")
	for _, template := range []string{
		analyticsTestTemplate,
		"<html><head></head><body>Prerendered</body></html>",
		"<html><head><!--Umami QuantumNous--><!--Google Analytics QuantumNous--></head></html>",
	} {
		once := InjectAnalyticsConfig([]byte(template))
		twice := InjectAnalyticsConfig(once)
		if !bytes.Equal(once, twice) {
			t.Fatalf("repeated injection changed HTML: %s", twice)
		}
		for _, attribution := range []string{"<!--Umami QuantumNous-->", "<!--Google Analytics QuantumNous-->"} {
			if strings.Count(string(twice), attribution) != 1 {
				t.Fatalf("expected one attribution %s: %s", attribution, twice)
			}
		}
	}
}
