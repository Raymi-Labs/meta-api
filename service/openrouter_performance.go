package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"golang.org/x/sync/singleflight"
)

const performanceTTL = 5 * time.Minute
const performanceFailureTTL = time.Minute

var performanceModelID = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._:-]+$`)

type performancePercentile struct {
	P50 *float64 `json:"p50"`
}
type performanceEndpoint struct {
	Tag          string                 `json:"tag"`
	Status       *int                   `json:"status"`
	ServiceTier  *string                `json:"service_tier"`
	Quantization string                 `json:"quantization"`
	Pricing      map[string]interface{} `json:"pricing"`
	Latency      *performancePercentile `json:"latency_last_30m"`
	Throughput   *performancePercentile `json:"throughput_last_30m"`
}
type performanceSnapshot struct {
	Endpoints []performanceEndpoint
	FetchedAt time.Time
}
type performanceEntry struct {
	Snapshot performanceSnapshot
	Expires  time.Time
}
type performanceCache struct {
	mu      sync.Mutex
	entries map[string]performanceEntry
	flight  singleflight.Group
	slots   chan struct{}
}

func newPerformanceCache() *performanceCache {
	return &performanceCache{entries: map[string]performanceEntry{}, slots: make(chan struct{}, 6)}
}

var openRouterPerformanceCache = newPerformanceCache()

func (cache *performanceCache) get(ctx context.Context, key string, fetch func(context.Context) (performanceSnapshot, error)) performanceSnapshot {
	read := func() (performanceSnapshot, bool) {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		e, ok := cache.entries[key]
		return e.Snapshot, ok && time.Now().Before(e.Expires)
	}
	if s, ok := read(); ok {
		return s
	}
	result := cache.flight.DoChan(key, func() (interface{}, error) {
		if s, ok := read(); ok {
			return s, nil
		}
		// Detached, bounded metadata reads survive a single browser cancellation.
		fetchCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		select {
		case cache.slots <- struct{}{}:
			defer func() { <-cache.slots }()
		case <-fetchCtx.Done():
			return performanceSnapshot{}, nil
		}
		s, err := fetch(fetchCtx)
		ttl := performanceTTL
		if err != nil {
			s = performanceSnapshot{}
			ttl = performanceFailureTTL
		}
		cache.mu.Lock()
		if len(cache.entries) >= 2048 {
			for k, e := range cache.entries {
				if time.Now().After(e.Expires) {
					delete(cache.entries, k)
				}
			}
		}
		if len(cache.entries) < 2048 {
			cache.entries[key] = performanceEntry{s, time.Now().Add(ttl)}
		}
		cache.mu.Unlock()
		return s, nil
	})
	select {
	case r := <-result:
		if s, ok := r.Val.(performanceSnapshot); ok {
			return s
		}
	case <-ctx.Done():
	}
	return performanceSnapshot{}
}

func fetchPerformance(ctx context.Context, client *http.Client, key, upstream string) (performanceSnapshot, error) {
	if !performanceModelID.MatchString(upstream) {
		return performanceSnapshot{}, errors.New("invalid metadata model")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models/"+upstream+"/endpoints", nil)
	if err != nil {
		return performanceSnapshot{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("metadata redirect rejected") }
	res, err := copyClient.Do(req)
	if err != nil {
		return performanceSnapshot{}, errors.New("performance metadata unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return performanceSnapshot{}, errors.New("performance metadata unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return performanceSnapshot{}, errors.New("invalid metadata size")
	}
	var body struct {
		Data struct {
			ID        string                `json:"id"`
			Endpoints []performanceEndpoint `json:"endpoints"`
		} `json:"data"`
	}
	if common.Unmarshal(raw, &body) != nil || body.Data.ID != upstream || len(body.Data.Endpoints) > 256 {
		return performanceSnapshot{}, errors.New("invalid metadata response")
	}
	return performanceSnapshot{body.Data.Endpoints, time.Now()}, nil
}

type performancePolicy struct {
	Only           []string           `json:"only"`
	Ignore         []string           `json:"ignore"`
	MaxPrice       map[string]float64 `json:"max_price"`
	Sort           string             `json:"sort"`
	Order          []string           `json:"order"`
	Quantizations  []string           `json:"quantizations"`
	DataCollection string             `json:"data_collection"`
	ZDR            bool               `json:"zdr"`
}

func performanceRoute(ch *model.Channel, name string) (string, string, dto.ChannelSettings, performancePolicy, bool) {
	var settings dto.ChannelSettings
	var policy performancePolicy
	if ch.Setting != nil && *ch.Setting != "" && common.UnmarshalJsonStr(*ch.Setting, &settings) != nil {
		return "", "", settings, policy, false
	}
	base := strings.TrimRight(ch.GetBaseURL(), "/")
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || u.Scheme != "https" || u.Host != "openrouter.ai" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "/api" && u.Path != "") {
			return "", "", settings, policy, false
		}
	}
	keys := ch.GetKeys()
	key := ""
	for i, k := range keys {
		if status, ok := ch.ChannelInfo.MultiKeyStatusList[i]; ok && status != 1 {
			continue
		}
		if ch.ChannelInfo.MultiKeyPendingDisableUntil[i] > time.Now().Unix() {
			continue
		}
		if strings.TrimSpace(k) != "" {
			key = strings.TrimSpace(k)
			break
		}
	}
	if key == "" || strings.ContainsAny(key, "\r\n\"") {
		return "", "", settings, policy, false
	}
	upstream := name
	mapping := map[string]string{}
	if raw := ch.GetModelMapping(); raw != "" {
		if common.UnmarshalJsonStr(raw, &mapping) != nil {
			return "", "", settings, policy, false
		}
		if mapped, ok := mapping[name]; ok {
			upstream = mapped
		}
	}
	if !performanceModelID.MatchString(upstream) {
		return "", "", settings, policy, false
	}
	override := map[string]interface{}{}
	if ch.ParamOverride != nil && *ch.ParamOverride != "" && common.UnmarshalJsonStr(*ch.ParamOverride, &override) != nil {
		return "", "", settings, policy, false
	}
	raw, _ := common.Marshal(map[string]interface{}{"model": upstream})
	raw, err := relaycommon.ApplyParamOverride(raw, override, map[string]interface{}{"original_model": name, "model": upstream})
	if err != nil {
		return "", "", settings, policy, false
	}
	var applied struct {
		Model    string            `json:"model"`
		Provider performancePolicy `json:"provider"`
		Tier     string            `json:"service_tier"`
	}
	if common.Unmarshal(raw, &applied) != nil || applied.Model != upstream || (applied.Tier != "" && applied.Tier != "default") {
		return "", "", settings, policy, false
	}
	policy = applied.Provider
	// Endpoint metadata cannot verify privacy filters or contextual request policies.
	if policy.ZDR || policy.DataCollection != "" || (policy.Sort != "" && policy.Sort != "price") {
		return "", "", settings, policy, false
	}
	return upstream, key, settings, policy, true
}
func providerMatches(list []string, tag string) bool {
	for _, v := range list {
		if tag == v || (!strings.Contains(v, "/") && strings.HasPrefix(tag, v+"/")) {
			return true
		}
	}
	return false
}
func finiteNonnegative(n float64) bool { return n >= 0 && !math.IsNaN(n) && !math.IsInf(n, 0) }
func endpointPrice(e performanceEndpoint, key string) (float64, bool) {
	raw, ok := e.Pricing[key]
	if !ok {
		return 0, false
	}
	var value float64
	switch v := raw.(type) {
	case string:
		var err error
		value, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
	case float64:
		value = v
	default:
		return 0, false
	}
	return value, finiteNonnegative(value)
}

func selectPerformance(s performanceSnapshot, p performancePolicy, upstream string) *grouphealth.PerformanceReference {
	if s.FetchedAt.IsZero() || time.Since(s.FetchedAt) >= performanceTTL {
		return nil
	}
	candidates := []performanceEndpoint{}
	for _, e := range s.Endpoints {
		if e.Tag == "" || e.Status == nil || *e.Status != 0 || (e.ServiceTier != nil && *e.ServiceTier != "" && *e.ServiceTier != "default") {
			continue
		}
		if len(p.Only) > 0 && !providerMatches(p.Only, e.Tag) || providerMatches(p.Ignore, e.Tag) {
			continue
		}
		if len(p.Quantizations) > 0 {
			found := false
			for _, q := range p.Quantizations {
				found = found || q == e.Quantization
			}
			if !found {
				continue
			}
		}
		_, a := endpointPrice(e, "prompt")
		_, b := endpointPrice(e, "completion")
		if !a || !b {
			continue
		}
		valid := true
		for k, cap := range p.MaxPrice {
			value, exists := endpointPrice(e, k)
			if !exists && (k == "request" || k == "image") {
				value = 0
				exists = true
			}
			multiplier := 1.0
			switch k {
			case "prompt", "completion":
				multiplier = 1e6
			case "request", "image":
			default:
				valid = false
			}
			if !exists || !finiteNonnegative(cap) || value*multiplier > cap+1e-12 {
				valid = false
			}
		}
		if valid {
			candidates = append(candidates, e)
		}
	}
	rank := func(e performanceEndpoint) int {
		for i, tag := range p.Order {
			if providerMatches([]string{tag}, e.Tag) {
				return i
			}
		}
		return len(p.Order)
	}
	cost := func(e performanceEndpoint) float64 {
		a, _ := endpointPrice(e, "prompt")
		b, _ := endpointPrice(e, "completion")
		return a + b
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if cost(a) != cost(b) {
			return cost(a) < cost(b)
		}
		return a.Tag < b.Tag
	})
	if len(candidates) == 0 {
		return nil
	}
	e := candidates[0]
	r := &grouphealth.PerformanceReference{Source: "openrouter", Model: upstream, Provider: e.Tag, Percentile: 50, FetchedAt: s.FetchedAt.Unix(), ExpiresAt: s.FetchedAt.Add(performanceTTL).Unix()}
	// Authenticated endpoint responses report latency in milliseconds (e.g. 1915 ms).
	if e.Latency != nil && e.Latency.P50 != nil && finiteNonnegative(*e.Latency.P50) {
		v := *e.Latency.P50
		r.TTFTMs = &v
	}
	if e.Throughput != nil && e.Throughput.P50 != nil && finiteNonnegative(*e.Throughput.P50) {
		v := *e.Throughput.P50
		r.OutputTPS = &v
	}
	if r.TTFTMs == nil && r.OutputTPS == nil {
		return nil
	}
	return r
}

// OpenRouterPerformance returns read-only references for the caller's exact scope.
// Nothing from this function is written to health snapshots or billing data.
func OpenRouterPerformance(ctx context.Context, groups, names []string) map[string]map[string]*grouphealth.PerformanceReference {
	result := map[string]map[string]*grouphealth.PerformanceReference{}
	if len(names) == 0 || len(names) > 100 || len(groups) == 0 {
		return result
	}
	routes, err := model.GetOpenRouterPerformanceRoutes(groups, names)
	if err != nil {
		return result
	}
	ids := []int{}
	seen := map[int]bool{}
	for _, r := range routes {
		if !seen[r.ChannelID] {
			seen[r.ChannelID] = true
			ids = append(ids, r.ChannelID)
		}
	}
	if len(ids) == 0 {
		return result
	}
	channels, err := model.GetChannelsByIds(ids)
	if err != nil {
		return result
	}
	byID := map[int]*model.Channel{}
	for _, ch := range channels {
		byID[ch.Id] = ch
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	workers := make(chan struct{}, 6)
	// One selected channel per model/group, preserving configured priority order.
	chosen := map[string]bool{}
	for _, route := range routes {
		route := route
		routeKey := route.GroupName + "\x00" + route.ModelName
		if chosen[routeKey] {
			continue
		}
		ch := byID[route.ChannelID]
		if ch == nil {
			continue
		}
		upstream, key, settings, policy, ok := performanceRoute(ch, route.ModelName)
		if !ok {
			continue
		}
		chosen[routeKey] = true
		select {
		case workers <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return result
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-workers }()
			client, err := GetHttpClientWithProxy(settings.Proxy)
			if err != nil {
				return
			}
			cacheKey := fmt.Sprintf("%x", sha256.Sum256([]byte(upstream+"\x00"+key+"\x00"+settings.Proxy)))
			snapshot := openRouterPerformanceCache.get(ctx, cacheKey, func(ctx context.Context) (performanceSnapshot, error) {
				return fetchPerformance(ctx, client, key, upstream)
			})
			ref := selectPerformance(snapshot, policy, upstream)
			if ref == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if result[route.ModelName] == nil {
				result[route.ModelName] = map[string]*grouphealth.PerformanceReference{}
			}
			result[route.ModelName][route.GroupName] = ref
		}()
	}
	wg.Wait()
	return result
}
