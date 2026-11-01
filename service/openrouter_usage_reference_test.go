package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	"github.com/stretchr/testify/require"
)

func usageFixtureClient(t *testing.T, catalog, activity string) *http.Client {
	return &http.Client{Transport: performanceTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "openrouter.ai", r.URL.Host)
		require.Empty(t, r.Header.Get("Authorization"))
		require.Equal(t, "GET", r.Method)
		body := activity
		if r.URL.Path == "/api/frontend/v1/catalog/models" {
			require.Equal(t, "vendor/model", r.URL.Query().Get("slug"))
			body = catalog
		} else {
			require.Equal(t, "/api/frontend/v1/stats/model-activity", r.URL.Path)
			require.Equal(t, "vendor/model-20261101", r.URL.Query().Get("permaslug"))
			require.Equal(t, "standard", r.URL.Query().Get("variant"))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
}

const usageCatalog = `{"data":[{"slug":"vendor/model","permaslug":"vendor/model-20261101"}]}`
const usageActivity = `{"data":{"cachedAt":1793534400000,"analytics":[{"date":"2026-11-01 00:00:00","model_permaslug":"vendor/model-20261101","variant":"standard","total_prompt_tokens":20,"total_completion_tokens":5,"count":0},{"date":"2026-10-31 00:00:00","model_permaslug":"vendor/model-20261101","variant":"standard","count":2}]}}`

func TestPublicUsageExactDailyData(t *testing.T) {
	now := time.UnixMilli(1793534400000)
	ref, err := fetchModelUsageReference(context.Background(), usageFixtureClient(t, usageCatalog, usageActivity), "vendor/model", now)
	require.NoError(t, err)
	require.Equal(t, "model_global", ref.Scope)
	require.Len(t, ref.Series, 2)
	require.Nil(t, ref.Series[0].TotalTokens)
	require.EqualValues(t, 2, *ref.Series[0].RequestCount)
	require.EqualValues(t, 25, *ref.Series[1].TotalTokens)
	require.EqualValues(t, 0, *ref.Series[1].RequestCount)
	require.EqualValues(t, 86400, ref.Series[1].Ts-ref.Series[0].Ts)
	require.Empty(t, ref.Groups)
}
func TestPublicUsageRejectsInvalidIdentityDatesAndStaleness(t *testing.T) {
	now := time.UnixMilli(1793534400000)
	for _, tc := range []struct{ name, catalog, activity string }{
		{"wrong slug", strings.ReplaceAll(usageCatalog, "vendor/model\"", "vendor/other\""), usageActivity},
		{"private", strings.ReplaceAll(usageCatalog, "\"slug\":", "\"is_private\":true,\"slug\":"), usageActivity},
		{"other model", usageCatalog, strings.ReplaceAll(usageActivity, "vendor/model-20261101", "vendor/other")},
		{"other variant", usageCatalog, strings.ReplaceAll(usageActivity, "standard", "free")},
		{"hourly date", usageCatalog, strings.ReplaceAll(usageActivity, "00:00:00", "01:00:00")},
		{"duplicate date", usageCatalog, strings.ReplaceAll(usageActivity, "2026-10-31", "2026-11-01")},
		{"stale", usageCatalog, strings.ReplaceAll(usageActivity, "1793534400000", "1")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := fetchModelUsageReference(context.Background(), usageFixtureClient(t, tc.catalog, tc.activity), "vendor/model", now)
			require.Error(t, err)
			require.Nil(t, ref)
		})
	}
	bad := strings.ReplaceAll(usageActivity, `"total_prompt_tokens":20`, `"total_prompt_tokens":-1`)
	ref, err := fetchModelUsageReference(context.Background(), usageFixtureClient(t, usageCatalog, bad), "vendor/model", now)
	require.NoError(t, err)
	require.Nil(t, ref.Series[1].TotalTokens)
	_, err = fetchModelUsageReference(context.Background(), fixtureClient(strings.Repeat("x", (2<<20)+1)), "vendor/model", now)
	require.Error(t, err)
}
func TestPublicUsageCacheCoalescesAndExpires(t *testing.T) {
	cache := &usageReferenceCache{entries: map[string]usageReferenceEntry{}, slots: make(chan struct{}, 2)}
	var calls atomic.Int32
	var wg sync.WaitGroup
	fetch := func(context.Context) (*grouphealth.ModelUsageReference, error) {
		calls.Add(1)
		time.Sleep(5 * time.Millisecond)
		return &grouphealth.ModelUsageReference{Model: "vendor/model"}, nil
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); require.NotNil(t, cache.get(context.Background(), "model", fetch)) }()
	}
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
	cache.mu.Lock()
	entry := cache.entries["model"]
	entry.Expires = time.Now().Add(-time.Second)
	cache.entries["model"] = entry
	cache.mu.Unlock()
	require.NotNil(t, cache.get(context.Background(), "model", fetch))
	require.EqualValues(t, 2, calls.Load())
}
