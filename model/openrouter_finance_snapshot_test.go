package model

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func waitFinanceSnapshot(t *testing.T, cache *openRouterFinanceCache, id int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		done := cache.entries[id] != nil && !cache.entries[id].refreshing
		cache.mu.Unlock()
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("snapshot collection did not finish")
}

func TestOpenRouterFinanceSnapshotNonblockingAndCoalescing(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	cache := newOpenRouterFinanceCache(func(ctx context.Context, account UpstreamAccount) *OpenRouterAccountFinance {
		calls.Add(1)
		<-release
		return &OpenRouterAccountFinance{FetchedAt: 123, Activity: []OpenRouterAccountActivity{}}
	})
	account := UpstreamAccount{Id: 1, AccountType: UpstreamAccountTypeOpenRouter}
	for i := 0; i < 20; i++ {
		result := cache.read(account)
		if !result.Refreshing || result.FetchedAt != 0 {
			t.Fatal("cold read must return pending immediately")
		}
	}
	close(release)
	waitFinanceSnapshot(t, cache, 1)
	for i := 0; i < 20; i++ {
		result := cache.read(account)
		if result.Refreshing || result.FetchedAt != 123 {
			t.Fatal("expected warm snapshot")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("collections = %d", calls.Load())
	}
}

func TestOpenRouterFinanceSnapshotFailureRetentionAndRecovery(t *testing.T) {
	amount := 10.0
	var calls atomic.Int32
	cache := newOpenRouterFinanceCache(func(ctx context.Context, account UpstreamAccount) *OpenRouterAccountFinance {
		n := calls.Add(1)
		result := &OpenRouterAccountFinance{FetchedAt: int64(n), StartDate: "2026-11-01", EndDate: "2026-11-12", Credits: &OpenRouterAccountCredits{TotalCredits: &amount}, Activity: []OpenRouterAccountActivity{}, SiteError: "no matching channel"}
		if n == 2 {
			result.Credits = nil
			result.CreditsError = "timeout"
		}
		return result
	})
	account := UpstreamAccount{Id: 1}
	cache.read(account)
	waitFinanceSnapshot(t, cache, 1)
	expire := func() { cache.mu.Lock(); cache.entries[1].attempted = time.Time{}; cache.mu.Unlock() }
	expire()
	cache.read(account)
	waitFinanceSnapshot(t, cache, 1)
	failed := cache.read(account)
	if failed.Credits == nil || failed.RefreshError == "" || failed.FetchedAt != 1 {
		t.Fatal("failure lost last good credits or timestamp")
	}
	if calls.Load() != 2 {
		t.Fatal("failed requests must be throttled")
	}
	expire()
	cache.read(account)
	waitFinanceSnapshot(t, cache, 1)
	recovered := cache.read(account)
	if recovered.CreditsError != "" || recovered.RefreshError != "" || recovered.FetchedAt != 3 {
		t.Fatal("one permanently unavailable section must not freeze healthy sections")
	}
}

func TestOpenRouterFinanceSnapshotCredentialIsolationAndBound(t *testing.T) {
	release := make(chan struct{})
	cache := newOpenRouterFinanceCache(func(ctx context.Context, account UpstreamAccount) *OpenRouterAccountFinance {
		<-release
		return &OpenRouterAccountFinance{FetchedAt: 123}
	})
	account := UpstreamAccount{Id: 1, AccountType: UpstreamAccountTypeOpenRouter, AccessTokenEncrypted: "old"}
	cache.read(account)
	account.AccessTokenEncrypted = "new"
	if result := cache.read(account); result.FetchedAt != 0 {
		t.Fatal("rotation reused old snapshot")
	}
	cache.read(UpstreamAccount{Id: 2})
	if len(cache.slots) != 2 {
		t.Fatal("global concurrency must be bounded")
	}
	close(release)
	waitFinanceSnapshot(t, cache, 1)
	cache.mu.Lock()
	empty := cache.entries[2].data == nil
	cache.mu.Unlock()
	if !empty {
		t.Fatal("third collection should not start until a later dispatch")
	}
	cache.read(UpstreamAccount{Id: 2})
	waitFinanceSnapshot(t, cache, 2)
}
