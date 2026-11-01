package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const performanceFixture = `{"data":{"id":"vendor/model","endpoints":[
 {"tag":"cheap","status":0,"pricing":{"prompt":"0.0000002","completion":"0.0000006","discount":0},"latency_last_30m":{"p50":1915},"throughput_last_30m":{"p50":20}},
 {"tag":"fast","status":0,"pricing":{"prompt":"0.000002","completion":"0.000006"},"latency_last_30m":{"p50":100},"throughput_last_30m":{"p50":200}},
 {"tag":"down","status":-2,"pricing":{"prompt":"0","completion":"0"},"latency_last_30m":{"p50":1},"throughput_last_30m":{"p50":999}}
]}}`

type performanceTransport func(*http.Request) (*http.Response, error)

func (f performanceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureClient(body string) *http.Client {
	return &http.Client{Transport: performanceTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
}

func TestOpenRouterPerformanceUnitsAndPolicy(t *testing.T) {
	s, err := fetchPerformance(context.Background(), fixtureClient(performanceFixture), "test-only", "vendor/model")
	require.NoError(t, err)
	p := performancePolicy{Only: []string{"cheap", "fast", "down"}, MaxPrice: map[string]float64{"prompt": .2, "completion": .6}}
	r := selectPerformance(s, p, "vendor/model")
	require.NotNil(t, r)
	require.Equal(t, "cheap", r.Provider)
	require.Equal(t, 1915.0, *r.TTFTMs)
	require.Equal(t, 20.0, *r.OutputTPS)
	require.Equal(t, 50, r.Percentile)
	require.WithinDuration(t, time.Now(), time.Unix(r.FetchedAt, 0), time.Second)
	p.Ignore = []string{"cheap"}
	require.Nil(t, selectPerformance(s, p, "vendor/model"), "over-cap and degraded endpoints cannot supply values")
	p = performancePolicy{Only: []string{"fast"}}
	require.Equal(t, "fast", selectPerformance(s, p, "vendor/model").Provider)
	s.Endpoints[0].Latency = nil
	s.Endpoints[0].Throughput = nil
	require.Nil(t, selectPerformance(s, performancePolicy{}, "vendor/model"), "do not silently choose a pricier fast endpoint when cheapest lacks data")
	s.FetchedAt = time.Now().Add(-performanceTTL)
	require.Nil(t, selectPerformance(s, p, "vendor/model"))
}
func TestOpenRouterPerformanceFetchBoundary(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: performanceTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "https://openrouter.ai/api/v1/models/vendor/model/endpoints", r.URL.String())
		require.Equal(t, "Bearer test-only", r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.invalid/leak"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	_, err := fetchPerformance(context.Background(), client, "test-only", "vendor/model")
	require.Error(t, err)
	require.Equal(t, 1, calls)
	_, err = fetchPerformance(context.Background(), client, "test-only", "vendor/../escape")
	require.Error(t, err)
	require.Equal(t, 1, calls)
	_, err = fetchPerformance(context.Background(), fixtureClient(strings.ReplaceAll(performanceFixture, "vendor/model", "wrong/model")), "test-only", "vendor/model")
	require.Error(t, err)
	_, err = fetchPerformance(context.Background(), fixtureClient(strings.Repeat("x", (2<<20)+1)), "test-only", "vendor/model")
	require.Error(t, err)
}
func TestOpenRouterPerformanceCacheCoalescesExpiresAndBacksOff(t *testing.T) {
	cache := newPerformanceCache()
	var calls atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	fetch := func(context.Context) (performanceSnapshot, error) {
		calls.Add(1)
		<-start
		return performanceSnapshot{FetchedAt: time.Now()}, nil
	}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); cache.get(context.Background(), "same", fetch) }()
	}
	time.Sleep(20 * time.Millisecond)
	close(start)
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
	cache.mu.Lock()
	cache.entries["same"] = performanceEntry{Expires: time.Now().Add(-time.Second)}
	cache.mu.Unlock()
	failure := func(context.Context) (performanceSnapshot, error) {
		calls.Add(1)
		return performanceSnapshot{}, errors.New("upstream unavailable")
	}
	require.True(t, cache.get(context.Background(), "same", failure).FetchedAt.IsZero())
	cache.get(context.Background(), "same", failure)
	require.EqualValues(t, 2, calls.Load())
}
func TestOpenRouterPerformanceMappedRoutesDoNotMutateChannels(t *testing.T) {
	mapping := `{"local-model":"vendor/model"}`
	override := `{"provider":{"sort":"price","only":["cheap"],"max_price":{"prompt":0.2,"completion":0.6}}}`
	ch := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenRouter, Key: "test-only", ModelMapping: &mapping, ParamOverride: &override}
	upstream, key, _, p, ok := performanceRoute(ch, "local-model")
	require.True(t, ok)
	require.Equal(t, "vendor/model", upstream)
	require.Equal(t, "test-only", key)
	require.Equal(t, []string{"cheap"}, p.Only)
	bad := `{"proxy":`
	ch.Setting = &bad
	_, _, _, _, ok = performanceRoute(ch, "local-model")
	require.False(t, ok)
	require.Equal(t, bad, *ch.Setting)
	ch.Setting = nil
	ch.BaseURL = common.GetPointer("https://other.invalid/api")
	_, _, _, _, ok = performanceRoute(ch, "local-model")
	require.False(t, ok)
	ch.BaseURL = nil
	ch.ModelMapping = common.GetPointer(`{"local-model":"vendor/model?key=bad"}`)
	_, _, _, _, ok = performanceRoute(ch, "local-model")
	require.False(t, ok)
}
func TestOpenRouterPerformanceOnlyEnabledExactAuthorizedRoutes(t *testing.T) {
	oldDB, oldClient, oldCache := model.DB, httpClient, openRouterPerformanceCache
	t.Cleanup(func() { model.DB = oldDB; httpClient = oldClient; openRouterPerformanceCache = oldCache })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	mapping := `{"local":"vendor/model"}`
	channels := []model.Channel{
		{Id: 1, Type: constant.ChannelTypeOpenRouter, Status: 1, Key: "test-only", ModelMapping: &mapping},
		{Id: 2, Type: constant.ChannelTypeOpenRouter, Status: 2, Key: "never-send", ModelMapping: &mapping},
		{Id: 3, Type: 1, Status: 1, Key: "never-send", ModelMapping: &mapping},
	}
	require.NoError(t, db.Create(&channels).Error)
	abilities := []model.Ability{{Group: "public", Model: "local", ChannelId: 1, Enabled: true}, {Group: "private", Model: "secret", ChannelId: 1, Enabled: true}, {Group: "public", Model: "disabled", ChannelId: 2, Enabled: true}, {Group: "public", Model: "native", ChannelId: 3, Enabled: true}, {Group: "public", Model: "off", ChannelId: 1, Enabled: false}}
	require.NoError(t, db.Create(&abilities).Error)
	httpClient = fixtureClient(performanceFixture)
	openRouterPerformanceCache = newPerformanceCache()
	refs := OpenRouterPerformance(context.Background(), []string{"public"}, []string{"local", "secret", "disabled", "native", "off"})
	require.Len(t, refs, 1)
	require.Equal(t, 1915.0, *refs["local"]["public"].TTFTMs)
	require.Len(t, refs["local"], 1)
	require.Empty(t, OpenRouterPerformance(context.Background(), nil, []string{"local"}))
	require.Empty(t, OpenRouterPerformance(context.Background(), []string{"public"}, []string{"vendor/model"}), "mapping targets do not grant access to undeclared model aliases")
}
