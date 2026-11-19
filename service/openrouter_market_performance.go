package service

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	gh "github.com/QuantumNous/new-api/pkg/group_health"
)

// Verified public, exact catalog identities (unique native ID, text output).
// Never transmit an arbitrary local alias supplied in a query to OpenRouter.
//
//go:embed or_text_performance_ids.json
var marketIDsJSON []byte
var marketIDs = func() map[string]string {
	var ids map[string]string
	if err := common.Unmarshal(marketIDsJSON, &ids); err != nil {
		panic(err)
	}
	return ids
}()

// Select one named endpoint, preferring coverage, then stable tag order (not speed).
// This is an endpoint benchmark for the model, never the site's provider or SLA.
func marketSpeed(snapshot performanceSnapshot, id string) *gh.PerformanceReference {
	var best *gh.PerformanceReference
	bestFields := 0
	for _, e := range snapshot.Endpoints {
		r := selectPerformance(performanceSnapshot{[]performanceEndpoint{e}, snapshot.FetchedAt}, performancePolicy{}, id)
		if r == nil {
			continue
		}
		n := 0
		if r.TTFTMs != nil {
			n++
		}
		if r.OutputTPS != nil {
			n++
		}
		if n == 0 {
			continue
		}
		if n > bestFields || (n == bestFields && r.Provider < best.Provider) {
			best = r
			bestFields = n
		}
	}
	if best != nil {
		best.UptimeRate = nil
		best.UptimeWindowSeconds = 0
	}
	return best
}

func fetchMarketSnapshot(ctx context.Context, client *http.Client, key, id string) (performanceSnapshot, error) {
	var api performanceSnapshot
	if key != "" {
		api, _ = fetchPerformance(ctx, client, key, id)
		if ref := marketSpeed(api, id); ref != nil && ref.TTFTMs != nil && ref.OutputTPS != nil {
			return api, nil
		}
	}
	page, err := fetchMarketPage(ctx, client, id)
	if p := marketSpeed(page, id); p != nil && (p.TTFTMs != nil && p.OutputTPS != nil || marketSpeed(api, id) == nil) {
		return page, nil
	}
	if marketSpeed(api, id) != nil {
		return api, nil
	}
	if err != nil {
		return performanceSnapshot{}, err
	}
	return performanceSnapshot{}, fmt.Errorf("model page has no usable speed measurement")
}

func OpenRouterMarketSpeed(ctx context.Context, groups, names []string) map[string]map[string]*gh.PerformanceReference {
	out := map[string]map[string]*gh.PerformanceReference{}
	if len(groups) == 0 || len(names) > 100 {
		return out
	}
	allowed := []string{}
	for _, n := range names {
		if _, ok := marketIDs[n]; ok {
			allowed = append(allowed, n)
		}
	}
	if len(allowed) == 0 {
		return out
	}
	routes, err := model.GetEnabledMarketReferenceScopes(ctx, groups, allowed)
	if err != nil {
		return out
	}
	byName := map[string][]string{}
	for _, r := range routes {
		byName[r.ModelName] = append(byName[r.ModelName], r.GroupName)
	}
	key := os.Getenv("OPENROUTER_PERFORMANCE_METADATA_KEY")
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	ordered := make([]string, 0, len(byName))
	for n := range byName {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)
	jobs := make(chan string)
	for i := 0; i < min(len(ordered), 6); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range jobs {
				id := marketIDs[n]
				snap := openRouterPerformanceCache.get(ctx, "market:"+digest+":"+id, func(fc context.Context) (performanceSnapshot, error) {
					return fetchMarketSnapshot(fc, GetHttpClient(), key, id)
				})
				if ref := marketSpeed(snap, id); ref != nil {
					scope := map[string]*gh.PerformanceReference{}
					for _, g := range byName[n] {
						scope[g] = ref
					}
					mu.Lock()
					out[n] = scope
					mu.Unlock()
				}
			}
		}()
	}
loop:
	for _, n := range ordered {
		select {
		case jobs <- n:
		case <-ctx.Done():
			break loop
		}
	}
	close(jobs)
	wg.Wait()
	return out
}
