package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	"golang.org/x/sync/singleflight"
)

const usageReferenceTTL = 15 * time.Minute
const maxPublicUsageCount = int64(1<<53 - 1)

type usageReferenceEntry struct {
	Reference *grouphealth.ModelUsageReference
	Expires   time.Time
}
type usageReferenceCache struct {
	mu      sync.Mutex
	entries map[string]usageReferenceEntry
	flight  singleflight.Group
	slots   chan struct{}
}

var publicUsageCache = &usageReferenceCache{entries: map[string]usageReferenceEntry{}, slots: make(chan struct{}, 4)}

func (cache *usageReferenceCache) get(ctx context.Context, id string, fetch func(context.Context) (*grouphealth.ModelUsageReference, error)) *grouphealth.ModelUsageReference {
	read := func() (*grouphealth.ModelUsageReference, bool) {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		e, ok := cache.entries[id]
		return e.Reference, ok && time.Now().Before(e.Expires)
	}
	if ref, ok := read(); ok {
		return ref
	}
	result := cache.flight.DoChan(id, func() (interface{}, error) {
		if ref, ok := read(); ok {
			return ref, nil
		}
		fetchCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		select {
		case cache.slots <- struct{}{}:
			defer func() { <-cache.slots }()
		case <-fetchCtx.Done():
			return (*grouphealth.ModelUsageReference)(nil), nil
		}
		ref, err := fetch(fetchCtx)
		ttl := usageReferenceTTL
		if err != nil {
			ref = nil
			ttl = time.Minute
		}
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if len(cache.entries) >= 256 {
			for k, e := range cache.entries {
				if time.Now().After(e.Expires) {
					delete(cache.entries, k)
				}
			}
		}
		if len(cache.entries) < 256 {
			cache.entries[id] = usageReferenceEntry{ref, time.Now().Add(ttl)}
		}
		return ref, nil
	})
	select {
	case r := <-result:
		ref, _ := r.Val.(*grouphealth.ModelUsageReference)
		return ref
	case <-ctx.Done():
		return nil
	}
}

// Public metadata only: no channel key or account usage is sent to these endpoints.
func readPublicUsageJSON(ctx context.Context, client *http.Client, path string, query url.Values, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai"+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("public metadata redirect rejected") }
	res, err := copyClient.Do(req)
	if err != nil {
		return errors.New("public usage unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.New("public usage unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return errors.New("invalid public usage size")
	}
	return common.Unmarshal(raw, target)
}

func fetchModelUsageReference(ctx context.Context, client *http.Client, upstream string, now time.Time) (*grouphealth.ModelUsageReference, error) {
	if !performanceModelID.MatchString(upstream) {
		return nil, errors.New("invalid model")
	}
	var catalog struct {
		Data []struct {
			Slug      string `json:"slug"`
			Permaslug string `json:"permaslug"`
			Hidden    bool   `json:"hidden"`
			Private   bool   `json:"is_private"`
		} `json:"data"`
	}
	if err := readPublicUsageJSON(ctx, client, "/api/frontend/v1/catalog/models", url.Values{"slug": {upstream}}, &catalog); err != nil {
		return nil, err
	}
	permaslug := ""
	for _, m := range catalog.Data {
		if m.Slug == upstream && !m.Hidden && !m.Private && performanceModelID.MatchString(m.Permaslug) {
			if permaslug != "" {
				return nil, errors.New("ambiguous model")
			}
			permaslug = m.Permaslug
		}
	}
	if permaslug == "" {
		return nil, errors.New("model not found")
	}
	variant := "standard"
	if _, v, ok := strings.Cut(upstream, ":"); ok {
		variant = v
	}
	var body struct {
		Data struct {
			CachedAt  int64 `json:"cachedAt"`
			Analytics []struct {
				Date     string `json:"date"`
				Model    string `json:"model_permaslug"`
				Variant  string `json:"variant"`
				Input    *int64 `json:"total_prompt_tokens"`
				Output   *int64 `json:"total_completion_tokens"`
				Requests *int64 `json:"count"`
			} `json:"analytics"`
		} `json:"data"`
	}
	if err := readPublicUsageJSON(ctx, client, "/api/frontend/v1/stats/model-activity", url.Values{"permaslug": {permaslug}, "variant": {variant}}, &body); err != nil {
		return nil, err
	}
	cached := time.UnixMilli(body.Data.CachedAt)
	if len(body.Data.Analytics) > 62 || cached.After(now.Add(time.Minute)) || now.Sub(cached) > 24*time.Hour {
		return nil, errors.New("stale or invalid public usage")
	}
	ref := &grouphealth.ModelUsageReference{Source: "openrouter", Model: upstream, Permaslug: permaslug, Variant: variant, Scope: "model_global", BucketSeconds: 86400, FetchedAt: now.Unix(), UpdatedAt: cached.Unix(), ExpiresAt: now.Add(usageReferenceTTL).Unix()}
	day := now.UTC().Truncate(24 * time.Hour)
	seen := map[int64]bool{}
	valid := func(v *int64) bool { return v != nil && *v >= 0 && *v <= maxPublicUsageCount }
	for _, row := range body.Data.Analytics {
		ts, err := time.Parse("2006-01-02 15:04:05", row.Date)
		if err != nil || ts.Hour() != 0 || ts.Minute() != 0 || ts.Second() != 0 || row.Model != permaslug || row.Variant != variant {
			return nil, errors.New("invalid public usage identity or date")
		}
		if ts.After(day) || ts.Before(day.AddDate(0, 0, -29)) {
			continue
		}
		if seen[ts.Unix()] {
			return nil, errors.New("duplicate public usage date")
		}
		seen[ts.Unix()] = true
		point := grouphealth.ModelUsagePoint{Ts: ts.Unix()}
		if valid(row.Input) && valid(row.Output) && *row.Input <= maxPublicUsageCount-*row.Output {
			v := *row.Input + *row.Output
			point.TotalTokens = &v
		}
		if valid(row.Requests) {
			point.RequestCount = row.Requests
		}
		if point.TotalTokens != nil || point.RequestCount != nil {
			ref.Series = append(ref.Series, point)
		}
	}
	if len(ref.Series) == 0 {
		return nil, errors.New("empty public usage")
	}
	sort.Slice(ref.Series, func(i, j int) bool { return ref.Series[i].Ts < ref.Series[j].Ts })
	return ref, nil
}

// The result remains model-wide reference data, never part of group aggregates.
func OpenRouterModelUsage(ctx context.Context, groups []string, name string) *grouphealth.ModelUsageReference {
	if len(groups) == 0 || name == "" {
		return nil
	}
	routes, err := model.GetOpenRouterPerformanceRoutes(groups, []string{name})
	if err != nil || len(routes) == 0 {
		return nil
	}
	ids := []int{}
	seen := map[int]bool{}
	for _, r := range routes {
		if !seen[r.ChannelID] {
			seen[r.ChannelID] = true
			ids = append(ids, r.ChannelID)
		}
	}
	channels, err := model.GetChannelsByIds(ids)
	if err != nil {
		return nil
	}
	byID := map[int]*model.Channel{}
	for _, ch := range channels {
		byID[ch.Id] = ch
	}
	// A model whose groups map to different upstream IDs cannot share one reference.
	upstream, proxy := "", ""
	eligible := []string{}
	chosen := map[string]bool{}
	for _, route := range routes {
		if chosen[route.GroupName] || byID[route.ChannelID] == nil {
			continue
		}
		id, _, settings, _, ok := performanceRoute(byID[route.ChannelID], name)
		if !ok {
			continue
		}
		chosen[route.GroupName] = true
		if upstream == "" {
			upstream = id
			proxy = settings.Proxy
		}
		if id == upstream {
			eligible = append(eligible, route.GroupName)
		}
	}
	if upstream == "" {
		return nil
	}
	client, err := GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil
	}
	ref := publicUsageCache.get(ctx, fmt.Sprintf("%s\x00%s", upstream, proxy), func(ctx context.Context) (*grouphealth.ModelUsageReference, error) {
		return fetchModelUsageReference(ctx, client, upstream, time.Now())
	})
	if ref == nil {
		return nil
	}
	copyRef := *ref
	copyRef.Groups = eligible
	return &copyRef
}
