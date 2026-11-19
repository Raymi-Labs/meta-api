package service

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

// Opt-in diagnostic, not part of CI. Reads public benchmark pages only; never
// sends a model request or reads a channel credential. Output one row per ID.
func TestLiveOpenRouterMarketCoverage(t *testing.T) {
	if os.Getenv("OR_MARKET_LIVE_AUDIT") != "1" {
		t.Skip("set OR_MARKET_LIVE_AUDIT=1 to audit public endpoints")
	}
	ids := make([]string, 0, len(marketIDs))
	for n := range marketIDs {
		ids = append(ids, n)
	}
	sort.Strings(ids)
	type sample struct {
		name  string
		value string
	}
	jobs := make(chan string)
	done := make(chan sample, len(ids))
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				s, err := fetchMarketPage(ctx, http.DefaultClient, marketIDs[n])
				cancel()
				v := "fetch-or-parse-error"
				if err == nil {
					v = "no-usable-measurement"
					if r := marketSpeed(s, marketIDs[n]); r != nil {
						fmtNum := func(p *float64) string {
							if p == nil {
								return "missing"
							}
							return fmt.Sprintf("%g", *p)
						}
						v = fmtNum(r.TTFTMs) + "\t" + fmtNum(r.OutputTPS)
					}
				}
				done <- sample{n, v}
			}
		}()
	}
	for _, n := range ids {
		jobs <- n
	}
	close(jobs)
	wg.Wait()
	close(done)
	samples := map[string]string{}
	for s := range done {
		samples[s.name] = s.value
	}
	w := bufio.NewWriter(os.Stdout)
	for _, id := range ids {
		fmt.Fprintf(w, "OR_AUDIT\t%s\t%s\n", id, samples[id])
	}
	_ = w.Flush()
}
