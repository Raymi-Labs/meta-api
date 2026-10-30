package controller

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const officialAttributeFixture = `<nav>Streaming Supported</nav><main><astro-island component-export="ModelView" props="{&quot;modelName&quot;:[0,&quot;gpt-example&quot;]}"><div>1,050,000 context window</div><div>128,000 max output tokens</div><div>Reasoning token support</div><div>Modalities</div><div>Text</div><div>Input and output</div><div>Image</div><div>Input only</div><div>Features</div><div>Streaming</div><div>Supported</div><div>Function calling</div><div>Not supported</div><div>Tools</div><div>Web search</div><div>Supported</div><div>Snapshots</div><div>Computer use</div><div>Supported</div></astro-island></main>`

func TestOfficialAttributeParsingAndPriority(t *testing.T) {
	entry, err := parseOpenAIModelAttributes([]byte(officialAttributeFixture), "gpt-example")
	require.NoError(t, err)
	require.EqualValues(t, 1050000, entry.ContextLength)
	require.EqualValues(t, 128000, entry.MaxOutputTokens)
	require.Equal(t, model.ModelModalities{"text", "image"}, entry.InputModalities)
	require.Equal(t, model.ModelModalities{"text"}, entry.OutputModalities)
	require.True(t, entry.Capabilities["streaming"])
	require.False(t, entry.Capabilities["function_calling"])
	require.NotContains(t, entry.Capabilities, "computer_use")
	require.NotContains(t, entry.Capabilities, "prompt_caching")
	_, err = parseOpenAIModelAttributes([]byte(officialAttributeFixture), "other-model")
	require.Error(t, err)
	_, err = parseOpenAIModelAttributes([]byte(`<main>Something went wrong</main>`), "gpt-example")
	require.Error(t, err)
	community := model.ModelCapabilityMetadata{ContextLength: 10000, Capabilities: model.ModelCapabilities{"function_calling": true, "prompt_caching": true}, ReleaseDate: "2026-01-01"}
	merged := mergeOfficialAttributes(community, markOfficialAttributes(entry, "https://developers.openai.com/api/docs/models/gpt-example"))
	require.EqualValues(t, 1050000, merged.ContextLength)
	require.False(t, merged.Capabilities["function_calling"])
	require.True(t, merged.Capabilities["prompt_caching"])
	require.Equal(t, "2026-01-01", merged.ReleaseDate)
	require.NotContains(t, merged.FieldSources, "release_date")
	require.Equal(t, "official", merged.FieldSources["capabilities.function_calling"].Kind)
	if path := os.Getenv("TEST_OPENAI_MODEL_HTML"); path != "" {
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		live, err := parseOpenAIModelAttributes(body, "gpt-6-astra")
		require.NoError(t, err)
		require.EqualValues(t, 1050000, live.ContextLength)
		require.EqualValues(t, 128000, live.MaxOutputTokens)
		require.True(t, live.Capabilities["computer_use"])
	}
}

type attributeTransport func(*http.Request) (*http.Response, error)

func (f attributeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOfficialAPIAdaptersAndNoCredentialRedirect(t *testing.T) {
	original := getHTTPClient()
	copyClient := *original
	defer func() { httpClient = original }()
	httpClient = &copyClient
	calls := 0
	copyClient.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "https", r.URL.Scheme)
		body := ""
		status := 200
		switch r.URL.Host {
		case "api.anthropic.com":
			require.Equal(t, "Bearer test-only", r.Header.Get("Authorization"))
			require.Equal(t, "2026-10-21", r.Header.Get("anthropic-version"))
			if r.URL.Query().Get("after_id") == "" {
				body = `{"data":[{"id":"claude-example","max_input_tokens":200000,"max_tokens":64000,"capabilities":{"thinking":{"supported":true},"citations":{"supported":false},"server_tools":{"web_search":{"supported":true}},"image_input":{"supported":true}}}],"has_more":true,"last_id":"claude-example"}`
			} else {
				body = `{"data":[],"has_more":false}`
			}
		case "generativelanguage.googleapis.com":
			require.Equal(t, "test-only", r.Header.Get("x-goog-api-key"))
			require.Empty(t, r.Header.Get("Authorization"))
			body = `{"models":[{"name":"models/gemini-example","inputTokenLimit":1000000,"outputTokenLimit":65000,"thinking":false}]}`
		default:
			status = 302
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://untrusted.example/"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	claude, err := fetchClaudeAttributes(context.Background(), "test-only")
	require.NoError(t, err)
	require.Len(t, claude, 1)
	require.True(t, claude["claude-example"].Capabilities["reasoning"])
	require.False(t, claude["claude-example"].Capabilities["citations"])
	require.Empty(t, claude["claude-example"].InputModalities)
	gemini, err := fetchGeminiAttributes(context.Background(), "test-only")
	require.NoError(t, err)
	require.EqualValues(t, 65000, gemini["gemini-example"].MaxOutputTokens)
	require.False(t, gemini["gemini-example"].Capabilities["reasoning"])
	require.NotContains(t, gemini["gemini-example"].Capabilities, "streaming")
	_, err = fetchOfficialAttributeData(context.Background(), "https://redirect.example/", map[string]string{"Authorization": "test-only"})
	require.ErrorContains(t, err, "302")
	require.Equal(t, 4, calls)
}

// Existing community-only integration tests explicitly isolate official endpoints.
func isolateOfficialAttributeRequests(t *testing.T) {
	t.Helper()
	original := getHTTPClient()
	client := *original
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "developers.openai.com" || r.URL.Host == "api.anthropic.com" || r.URL.Host == "generativelanguage.googleapis.com" {
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return transport.RoundTrip(r)
	})
	httpClient = &client
	t.Cleanup(func() { httpClient = original })
}

func TestOfficialAttributesSyncIndependentSourcesAndPricing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Vendor{}, &model.Channel{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1=1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = originalDB
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	m := model.Model{ModelName: "gpt-example", SyncOfficial: 1, Status: 1}
	require.NoError(t, m.Insert())
	require.NoError(t, db.Create(&model.Ability{Model: m.ModelName, Group: "default", ChannelId: 1, Enabled: true}).Error)
	original := getHTTPClient()
	client := *original
	httpClient = &client
	t.Cleanup(func() { httpClient = original })
	t.Setenv("SYNC_UPSTREAM_BASE", "https://community.example")
	t.Setenv("MODEL_METADATA_ANTHROPIC_API_KEY", "")
	t.Setenv("MODEL_METADATA_GEMINI_API_KEY", "")
	officialDown, communityDown := false, false
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		body := officialAttributeFixture
		status := 200
		if r.URL.Host == "community.example" {
			body = `{"openai":{"id":"openai","models":{"gpt-example":{"id":"gpt-example","release_date":"2026-01-01","limit":{"context":200000,"output":64000},"reasoning":false,"tool_call":true}}}}`
			if communityDown {
				status = 429
			}
		} else {
			require.Equal(t, "developers.openai.com", r.URL.Host)
			if officialDown {
				status = 503
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	run := func() model.ModelCapabilitySyncResult {
		t.Helper()
		result, err := runModelCapabilitySync(context.Background())
		require.NoError(t, err)
		return result
	}
	result := run()
	require.Equal(t, 1, result.Updated)
	require.Zero(t, result.SourceFailures)
	pricing := model.GetPricing()
	require.Len(t, pricing, 1)
	require.EqualValues(t, 1050000, pricing[0].ContextLength)
	require.EqualValues(t, 128000, pricing[0].MaxOutputTokens)
	require.False(t, pricing[0].Capabilities["function_calling"])
	require.Equal(t, "community", pricing[0].AttributeSources["release_date"].Kind)
	require.Equal(t, "official", pricing[0].AttributeSources["context_length"].Kind)
	officialDown = true
	result = run()
	require.Equal(t, 1, result.SourceFailures)
	require.EqualValues(t, 128000, model.GetPricing()[0].MaxOutputTokens)
	officialDown = false
	communityDown = true
	result = run()
	require.Equal(t, 1, result.SourceFailures)
	require.EqualValues(t, 128000, model.GetPricing()[0].MaxOutputTokens)
	require.NoError(t, db.Model(&m).Update("max_output_tokens", 8000).Error)
	result = run()
	require.EqualValues(t, 8000, model.GetPricing()[0].MaxOutputTokens)
	require.NotContains(t, model.GetPricing()[0].AttributeSources, "max_output_tokens")
	officialDown = true
	_, err = runModelCapabilitySync(context.Background())
	require.Error(t, err)
	require.EqualValues(t, 8000, model.GetPricing()[0].MaxOutputTokens)
}

// Opt-in acceptance check against the public vendor document, never a relay call.
func TestLiveOfficialModelAttributes(t *testing.T) {
	if os.Getenv("TEST_LIVE_OFFICIAL_METADATA") != "1" {
		t.Skip("explicit live metadata acceptance only")
	}
	data, err := fetchOfficialAttributeData(context.Background(), "https://developers.openai.com/api/docs/models/gpt-6-astra", nil)
	require.NoError(t, err)
	entry, err := parseOpenAIModelAttributes(data, "gpt-6-astra")
	require.NoError(t, err)
	require.EqualValues(t, 1050000, entry.ContextLength)
	require.EqualValues(t, 128000, entry.MaxOutputTokens)
	require.True(t, entry.Capabilities["prompt_caching"])
	require.True(t, entry.Capabilities["streaming"])
	require.Equal(t, model.ModelModalities{"text", "image"}, entry.InputModalities)
}
